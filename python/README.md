# Session 7 Starter · Python

Autonomous payment agents and safety architecture, plus Docker support.
Same pattern as the JavaScript starter, Python idioms throughout.

**Session 7 adds `api_server.py`**, a FastAPI + Server-Sent Events
wrapper around `payment_agent.py`, the "separate backend" pattern for
builders not using Next.js. See the root README for the full
two-request human-in-the-loop pattern this implements.

## Running the API server

```bash
uvicorn api_server:app --reload --port 8001
```

Then, from a separate origin (a plain React app, for example):

```bash
curl -N -X POST http://localhost:8001/api/agent
# note the runId from the approval_required event, then:
curl -X POST http://localhost:8001/api/agent/confirm \
  -H "Content-Type: application/json" \
  -d '{"runId": "...", "approved": true}'
```

CORS is already configured in `api_server.py` for local development
(`allow_origins=["*"]`), tighten this to your actual frontend's URL
before deploying anywhere real.

## Setup

**With Docker** (no local Python install needed):

```bash
cp .env.example .env
# fill in ANTHROPIC_API_KEY, AGENT_PRIVATE_KEY, FUJI_USDC_ADDRESS
cd .. && docker compose run --rm python python payment_agent.py
```

**Without Docker:**

```bash
python3 -m venv venv
source venv/bin/activate   # or venv\Scripts\activate on Windows
pip install -r requirements.txt
cp .env.example .env
# fill in ANTHROPIC_API_KEY, AGENT_PRIVATE_KEY, FUJI_USDC_ADDRESS
```

## A critical safety note, read this before you run anything

`AGENT_PRIVATE_KEY` signs real transactions. Use a wallet you generated
specifically for this cohort, funded only with Fuji testnet AVAX and
USDC from a faucet, never a wallet that holds anything real. `.env` is
already in `.gitignore`, double-check before you push anyway.

## Files

| File | What it does |
|---|---|
| `model_provider.py` | Provider abstraction: `create_model_client()`, four providers, one shared async interface |
| `direct_rpc.py` | Method 1: raw JSON-RPC via `web3.py`, no external chain SDK needed |
| `chainkit_fetch.py` | Method 2: calls the Glacier REST API directly |
| `chainkit_mcp_agent.py` | ChainKit as MCP server, using the official `mcp` Python SDK |
| `advisor.py` | Session 4: the Smart Wallet Advisor, with a human-in-the-loop checkpoint and audit logging |
| `rag.py` | Session 5: retrieval-augmented generation, grounded answers with citations |
| `payment_agent.py` | Session 6: the invoice payment agent, evaluates a condition, gets human approval, sends real USDC on Fuji |
| `api_server.py` | Session 7: the same agent logic wrapped in a FastAPI + SSE HTTP API |
| `kill_switch.sol` | Session 6: the on-chain safety pillar, an `onlyOwner` Solidity kill switch |
| `normalize.py` | Shared wei-to-AVAX, hex-to-decimal, Unix-to-ISO8601 conversion |

## Running each one

```bash
python direct_rpc.py
python chainkit_fetch.py
python chainkit_mcp_agent.py
python advisor.py <wallet-address>
python rag.py "your question"           # needs Chroma running
python payment_agent.py                  # Session 6: the full payment agent
```

## How the payment agent's signing was verified

`web3.py` was used to build and sign a real transaction end to end
during development, not just syntax-checked, including confirming the
exact attribute name on the signed result (`raw_transaction`, not the
older `rawTransaction` some tutorials still reference). The derived
wallet address from a test private key matches exactly what the
JavaScript, Go, and Rust versions in this repo derive from that same
key, real cross-language proof the signing logic is correct.

## A note on the Solidity file

There was no Solidity compiler available in the environment this repo
was built in, so `kill_switch.sol` was not compiled or deployed during
development, unlike every other file here. It follows standard,
well-established Solidity patterns and was carefully hand-checked, but
compile and test it yourself, on Remix or with Hardhat or Foundry
locally, before you deploy it.

## Model provider

`MODEL_PROVIDER` in `.env` picks the provider (`anthropic`, `openai`,
`gemini`, or `ollama`), defaulting to `anthropic`. This doesn't affect
`payment_agent.py`, which doesn't call a model at all, the condition
evaluation is plain code, not an LLM call, by design, you want that
logic fully deterministic and auditable when real money is involved.

## Submission

1. Test everything yourself, confirm your payment agent evaluates the condition correctly, asks for approval, and sends a real transaction on approval.
2. Screenshot the working test, including the approval prompt and the resulting Snowtrace transaction.
3. Open your PR, screenshot that too.
4. Post on X with both screenshots, tag **@code_mwangi** and **@AvaxAfrica**.
5. Copy your post link, submit it on the quest page once it's live.

Post in the Week 3 WhatsApp group for anything you get stuck on.
