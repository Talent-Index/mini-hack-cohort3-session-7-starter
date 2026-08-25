"""
The agent's HTTP API, a separate-backend equivalent to the Next.js
version in javascript/web/. Reuses every function from payment_agent.py
directly, this file adds an HTTP and SSE layer on top, it does not
reimplement the agent logic.

This is the pattern for "if you're using a separate backend" from the
Session 7 slides: a real API a frontend on a different origin can call,
which means CORS has to be configured, done below with FastAPI's
CORSMiddleware.

Run it with: uvicorn api_server:app --reload --port 8001
"""

import asyncio
import json
import uuid
from datetime import datetime, timezone

from fastapi import FastAPI, HTTPException, Request
from fastapi.middleware.cors import CORSMiddleware
from sse_starlette.sse import EventSourceResponse

from payment_agent import (
    days_overdue,
    find_overdue_invoices,
    preflight_checks,
    send_payment,
    log_decision,
    SENT_PAYMENTS_LOG,
)

app = FastAPI()

# A separate backend means a separate origin, which means the browser
# will block the frontend's requests unless the API explicitly allows
# them. allow_origins=["*"] is fine for local development, tighten this
# to your actual frontend's URL before deploying anywhere real.
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)

# Holds pending runs between the two requests that make up the
# human-in-the-loop pattern. A plain in-memory dict, which only works
# because this runs as one long-lived process. On a platform where each
# request can hit a different, freshly started instance, swap this for
# Redis or a database, this limitation is exactly why: a dict here is a
# teaching simplification, not a production pattern.
pending_runs: dict = {}


def sse_event(event: str, data: dict) -> dict:
    return {"event": event, "data": json.dumps(data)}


@app.post("/api/agent")
async def start_agent_run():
    async def event_generator():
        # Steps 1 and 2 from Session 6: define the condition, evaluate it.
        yield sse_event("step", {"label": "Checking overdue invoices"})
        await asyncio.sleep(0)

        overdue = find_overdue_invoices()
        if not overdue:
            yield sse_event("final", {"text": "No overdue invoices right now."})
            return

        invoice = overdue[0]
        days = days_overdue(invoice["due_date"])
        reasoning = f"{invoice['id']} is {days} days overdue, condition met (overdue > 3 days)"

        yield sse_event("step", {"label": "Found an overdue invoice", "detail": invoice["id"]})

        # Safety pillar 1: pre-flight checks, same as the CLI version.
        errors = preflight_checks(invoice)
        if errors:
            log_decision(invoice["id"], f"Pre-flight failed: {', '.join(errors)}", approved=False)
            yield sse_event("final", {"text": f"Blocked: {', '.join(errors)}"})
            return

        # Step 3: present reasoning, then pause and wait for a real human
        # decision, this is the actual pause, not a demo, the process
        # does not know yet whether it should send anything.
        run_id = str(uuid.uuid4())
        pending_runs[run_id] = {"invoice": invoice, "reasoning": reasoning, "created_at": datetime.now(timezone.utc)}

        yield sse_event("approval_required", {
            "runId": run_id,
            "reasoning": f"Invoice {invoice['id']} from {invoice['supplier']} is {days} days overdue.",
            "action": f"Send {invoice['amount_usdc']} USDC to {invoice['recipient']}",
        })

    return EventSourceResponse(event_generator())


@app.post("/api/agent/confirm")
async def confirm_agent_run(request: Request):
    body = await request.json()
    run_id = body.get("runId")
    approved = body.get("approved", False)

    run = pending_runs.pop(run_id, None)
    if run is None:
        raise HTTPException(status_code=404, detail="Unknown or expired run")

    invoice = run["invoice"]

    # Step 3, the rejection path: the human said no, nothing executes,
    # and that decision gets logged exactly like an approval would.
    if not approved:
        log_decision(invoice["id"], run["reasoning"], approved=False)
        return {"text": "Payment rejected, nothing was sent."}

    # Step 4: only now, after a real approval, does anything touch chain.
    try:
        tx_hash = send_payment(invoice)
        SENT_PAYMENTS_LOG.add(invoice["id"])
        # Step 5: log the outcome, same as every other agent in this repo.
        log_decision(invoice["id"], run["reasoning"], approved=True, tx_hash=tx_hash)
        return {"text": f"Payment sent, transaction hash: {tx_hash}", "txHash": tx_hash}
    except Exception as err:
        log_decision(invoice["id"], f"Execution failed: {err}", approved=True)
        raise HTTPException(status_code=500, detail=str(err))
