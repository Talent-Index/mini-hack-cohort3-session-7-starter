"""
The Smart Wallet Advisor, end to end.

Five steps: fetch a wallet's transaction history via the Glacier API
(same approach as chainkit_fetch.py), normalize it, build a
summarization prompt, send it to the model, and return a structured
plain-English summary. Includes a human-in-the-loop checkpoint before
anything gets flagged for review, and an audit log entry for every
decision the agent makes along the way.
"""

import asyncio
import json
import sys
from datetime import datetime, timezone

import httpx
from dotenv import load_dotenv
import os

from model_provider import create_model_client
from normalize import normalize_many

load_dotenv()

GLACIER_BASE_URL = "https://glacier-api.avax.network/v1"
FUJI_CHAIN_ID = "43113"

SUMMARY_SYSTEM_PROMPT = "You are a wallet analyst. Be precise and concise."


async def get_wallet_history(wallet_address: str) -> list:
    # Step 1: fetch. Same Glacier call as chainkit_fetch.py, just pulling
    # more history (50 transactions instead of 10) since a meaningful
    # summary needs more than a handful of data points to work with.
    api_key = os.environ.get("GLACIER_API_KEY")
    if not api_key:
        raise ValueError("Set GLACIER_API_KEY in your .env first. Get a free one at avacloud.io.")

    url = f"{GLACIER_BASE_URL}/chains/{FUJI_CHAIN_ID}/addresses/{wallet_address}/transactions"

    async with httpx.AsyncClient() as client:
        response = await client.get(
            url,
            params={"pageSize": 50},
            headers={"x-glacier-api-key": api_key},
            timeout=30.0,
        )

    if response.status_code != 200:
        raise RuntimeError(f"Glacier API request failed with status {response.status_code}")

    data = response.json()
    # Step 2: normalize. Same normalize.py from Session 3, unchanged,
    # this is the whole point of having built it once, reusable everywhere.
    return normalize_many(data.get("transactions", []))


def build_summary_prompt(normalized_txs: list) -> str:
    # Step 3: the summarization prompt. Specific about exactly what we
    # want back, total AVAX in and out, most frequent counterparty,
    # primary token, one notable pattern, not just "summarize this
    # wallet." A vague prompt here gets a vague summary, same lesson
    # from Session 1.
    return (
        "Summarize this wallet's activity. Include:\n"
        "- total AVAX in and out\n"
        "- most frequent counterparty\n"
        "- primary token used\n"
        "- one notable pattern\n\n"
        "Wallet transactions (normalized JSON):\n"
        f"{json.dumps(normalized_txs, indent=2)}"
    )


def confirm_action(reasoning: str, action: str) -> bool:
    # Human-in-the-loop checkpoint. Shows the agent's reasoning and the
    # action it wants to take, then waits for an explicit human decision
    # before anything happens. This is the one pattern the whole session
    # is built around: the agent reasons, the human decides, the code
    # executes.
    print(f"\nAgent reasoning: {reasoning}")
    print(f"Proposed action: {action}")
    answer = input("Approve? (y/n): ")
    return answer.strip().lower() == "y"


def log_decision(reasoning: str, tool: str, params: dict, result: str) -> None:
    # Every decision the agent makes, the fetch, the flag, the human's
    # answer, gets logged as one structured entry: what happened, why,
    # and when. This is what lets you debug a bad decision after the
    # fact and prove what actually happened, not just what you assume
    # happened.
    entry = {
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "reasoning": reasoning,
        "tool": tool,
        "params": params,
        "result": result,
    }
    print(json.dumps(entry))
    # production: append to a durable log store, not just stdout


async def main():
    if len(sys.argv) < 2:
        raise ValueError("Usage: python advisor.py <wallet-address>")
    wallet_address = sys.argv[1]

    # Steps 1 and 2: fetch and normalize.
    normalized_txs = await get_wallet_history(wallet_address)
    log_decision(
        reasoning="Fetched wallet history",
        tool="glacier.listTransactions",
        params={"wallet_address": wallet_address},
        result=f"{len(normalized_txs)} transactions",
    )

    # Steps 3, 4, and 5: build the prompt, call the model, print the summary.
    client = create_model_client()
    response = await client.generate_text(
        SUMMARY_SYSTEM_PROMPT,
        [{"role": "user", "content": build_summary_prompt(normalized_txs)}],
    )
    print(response.text)

    # Human-in-the-loop: anything 5 AVAX or larger gets flagged and needs
    # explicit approval before we log it as reviewed. Nothing here
    # executes a transaction, this is intentionally the simplest possible
    # HITL pattern, show reasoning, wait for a yes or no.
    large_tx = next((tx for tx in normalized_txs if tx["amount"] >= 5), None)
    if large_tx:
        approved = confirm_action(
            f"This wallet sent {large_tx['amount']} AVAX to {large_tx['to']}, "
            "unusually large for this pattern.",
            "Flag transaction for manual review",
        )
        log_decision(
            reasoning="Large transaction detected",
            tool="confirm_action",
            params={"hash": large_tx["hash"]},
            result="approved" if approved else "rejected",
        )


if __name__ == "__main__":
    asyncio.run(main())
