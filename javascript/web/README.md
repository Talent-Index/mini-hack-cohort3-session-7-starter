# Session 7 · Next.js Frontend

The recommended path: frontend and API routes together, one Next.js
app, one deployment, no CORS setup at all since the frontend and the
API never leave the same origin.

## Setup

```bash
npm install
cp .env.example .env
# fill in AGENT_PRIVATE_KEY (Fuji testnet-only!), FUJI_USDC_ADDRESS
npm run dev
```

Open http://localhost:3000, click "Check overdue invoices."

**With Docker**, from the repo root: `docker compose up web`.

## A critical safety note

`AGENT_PRIVATE_KEY` signs real transactions. Use a wallet you generated
specifically for this cohort, funded only with Fuji testnet AVAX and
USDC. Never commit `.env` with a real value filled in.

## Files

| Path | What it does |
|---|---|
| `app/page.tsx` | The UI: check invoices, render streamed steps, render the approve/reject decision |
| `app/api/agent/route.ts` | Phase one: streams condition-checking, stops at `approval_required` |
| `app/api/agent/confirm/route.ts` | Phase two: the only route that ever touches `AGENT_PRIVATE_KEY` |
| `lib/invoices.ts` | Mock invoice database and condition logic, ported from Session 6 |
| `lib/payment.ts` | Real Fuji USDC transfer via `ethers.js`, ported from Session 6 |
| `lib/runs.ts` | In-memory store bridging the two-phase request pattern, plus the audit log |

## Why two requests instead of one long stream

SSE is one-directional, the server can push updates, but the browser
can't send a decision back over that same open connection cleanly. The
real pattern here is two separate requests: the first streams progress
and stops at the approval moment, the second, sent only after a real
human clicks a button, carries the decision and is the only place a
private key is ever touched. `lib/runs.ts` bridges the two with an
in-memory map, explicitly documented in that file as a teaching
simplification, a real deployment needs Redis or a database there
instead, since serverless platforms can route the two requests to
different, freshly-started instances.

## How this was verified

Not just compiled, actually run. `next build` was run with TypeScript
checking enabled, then the dev server was started and both the approval
and rejection paths were tested live with real HTTP requests. This
process caught a real bug during development: the mock recipient
addresses had invalid EIP-55 checksums, which `ethers.js` v6 rejects
outright. Fixed here, and in every one of Session 6's payment agents
that had the same bad addresses.

## Deployment

`vercel deploy` from inside this folder. Same app, frontend and API
route together, live at one public URL.

## Submission

Test both the approve and reject paths, screenshot the working test and
your PR, post on X tagging **@code_mwangi** and **@AvaxAfrica**, then
submit that link on the quest page.

Post in the Week 3 WhatsApp group for anything you get stuck on.
