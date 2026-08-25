# Mini Hack · Cohort 3, Session 7 Starter

**Building Agentic Solutions on Avalanche** · Team1 Kenya

Connecting your agent to a frontend, in four languages, plus Docker
support. Every agent built across Sessions 4 through 6 lived in a
terminal. This session wraps the exact same logic in a real API and
streams it to a real webpage, with the human-in-the-loop moment
redesigned as something you can actually click.

## What's new this session

**`javascript/web/`**, a complete Next.js app, the recommended path:
frontend and API routes together, one deployment, no CORS. Two real
API routes implement a genuine two-phase human-in-the-loop pattern:

1. `POST /api/agent` streams the agent's condition-checking steps with
   Server-Sent Events, and stops at an `approval_required` event rather
   than executing anything
2. `POST /api/agent/confirm` is the only route that ever touches
   `AGENT_PRIVATE_KEY`, only reached after a real human clicked Approve
   or Reject

**A separate HTTP + SSE API server in Python, Go, and Rust**
(`python/api_server.py`, `golang/api-server/`, `rust/src/bin/api_server.rs`),
implementing the exact same two-phase pattern and the exact same event
shape, for the "if you're using a separate backend" scenario the slides
cover. Each reuses its language's existing payment agent logic directly
rather than duplicating it, see "How this was refactored" below.

## The pattern, the same across all four

```
POST /api/agent          ->  SSE stream: step, step, approval_required { runId }
                              (stops here, nothing has executed yet)

POST /api/agent/confirm  ->  { runId, approved }
                              only now does anything touch a private key
```

This mirrors Sessions 4 and 6's `confirmPayment()` exactly, just over
HTTP instead of a terminal prompt: present the reasoning, wait for a
real decision, only then act.

## How this was refactored (Go and Rust specifically)

Go and Rust can't have two `func main()` / `fn main()` in one package,
so the CLI and the new API server needed the same signing logic
available in two different binaries. Rather than copy-paste the
Fuji-signing code (exactly the kind of code you don't want two
subtly-diverging copies of), it was pulled into a shared module:

- Go: `golang/paymentagent/`, imported by both `payment-agent/` (CLI)
  and `api-server/`
- Rust: `rust/src/paymentagent.rs`, imported by both
  `src/bin/payment_agent.rs` (CLI) and `src/bin/api_server.rs`

Both CLIs were rebuilt against these shared modules and their full test
suites re-run afterward to confirm the refactor didn't change any
behavior.

## Real bugs caught while building this, fixed everywhere they appeared

Building the Next.js app and actually running it end to end (not just
compiling it) surfaced two real, previously-shipped bugs, both fixed
across every language that had them, not just the new code:

1. **Bad EIP-55 checksums on the mock invoice recipient addresses.**
   `ethers.js` v6 enforces checksum validation strictly and rejected
   them outright. The same bad addresses were in all four of Session
   6's payment agents. Fixed everywhere.
2. **Hardcoded absolute calendar due dates** in the mock invoices,
   meaning the "overdue" condition would silently drift as real time
   passed, more invoices becoming overdue than intended. Fixed in JS,
   Python, and Go to compute relative to runtime instead. Rust was
   already immune, it stored `days_overdue` as a fixed value from the
   start rather than a calendar date.
3. **Go's `rpcCall` didn't check the HTTP status before parsing the
   response as JSON**, so a blocked or non-JSON error response produced
   a confusing "invalid character" error instead of a clear one. Fixed
   to surface the real error message.

Every fix was re-verified afterward: Go's and Rust's full test suites
re-run and passing, Python's logic re-tested with real assertions, the
Next.js app's live approval and rejection flows re-run end to end.

## Running with Docker

```bash
docker compose up web              # Next.js, http://localhost:3000, the recommended path
docker compose up python-api       # FastAPI + SSE, http://localhost:8001
docker compose up golang-api       # net/http + SSE, http://localhost:8002
docker compose up rust-api         # axum + SSE, http://localhost:8004
```

Each needs its own `.env` first, copied from that folder's
`.env.example`, same safety note as Session 6: `AGENT_PRIVATE_KEY`
needs to be a Fuji testnet-only key, never a real wallet.

## Running without Docker

Next.js: `cd javascript/web && npm install && npm run dev`.
Python: `uvicorn api_server:app --reload --port 8001` from the `python/`
folder. Go: `go run ./api-server` from `golang/`. Rust: `cargo run --bin
api_server` from `rust/`.

## A note on how the streaming and approval flow was verified

Every route in every language was started for real and hit with real
requests, both the approval path and the rejection path, not just
compiled and assumed correct. The Next.js app's build was run through
`next build` with TypeScript checking enabled, then the dev server was
started and both flows tested live with real HTTP requests. Where the
approval path reaches the actual Fuji network call, every language
correctly attempted the real signed transaction and correctly surfaced
a clear error when the network call couldn't complete in this
environment (a sandboxed network policy, not a code defect, each
language's HTTP stack reports it slightly differently, Go and Python
get a plain 403, Rust's TLS stack rejects the connection earlier with a
certificate error, same underlying cause).

## Picking a language

| Language | Folder |
|---|---|
| JavaScript / Next.js | [`javascript/web/`](./javascript/web) |
| JavaScript (CLI agents from earlier sessions) | [`javascript/`](./javascript) |
| Python | [`python/`](./python) |
| Go | [`golang/`](./golang) |
| Rust | [`rust/`](./rust) |

## Submission

Test everything yourself, both the approve and reject paths, screenshot
the working test and your PR, post on X tagging **@code_mwangi** and
**@AvaxAfrica**, then submit that link on the quest page.
