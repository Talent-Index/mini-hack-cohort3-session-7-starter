# Session 7 Starter · JavaScript

Autonomous payment agents and safety architecture, plus Docker support.
Builds on Session 5's RAG agent and the model-provider pattern from
Sessions 1 and 2.

**For Session 7's frontend content, see [`web/`](./web), a complete
Next.js app.** This folder still has all the CLI agents from earlier
sessions, unchanged.

## Setup

**With Docker** (no local Node install needed):

```bash
cp .env.example .env
# fill in ANTHROPIC_API_KEY, AGENT_PRIVATE_KEY, FUJI_USDC_ADDRESS
cd .. && docker compose run --rm javascript npm run payment-agent
```

**Without Docker:**

```bash
npm install
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
| `model-provider.js` | Same provider abstraction from Session 2, carried forward unchanged |
| `direct-rpc.js` | Method 1: raw RPC via `ethers.js`, `getBalance`/`getBlock`/`getTransactionCount` |
| `chainkit-fetch.js` | Method 2: structured wallet history via the real `@avalanche-sdk/chainkit` SDK |
| `chainkit-mcp-agent.js` | ChainKit running as an MCP server, wired into a tool-calling agent |
| `advisor.js` | Session 4: the Smart Wallet Advisor, with a human-in-the-loop checkpoint and audit logging |
| `rag.js` | Session 5: retrieval-augmented generation, grounded answers with citations |
| `payment-agent.js` | Session 6: the invoice payment agent, evaluates a condition, gets human approval, sends real USDC on Fuji |
| `kill_switch.sol` | Session 6: the on-chain safety pillar, an `onlyOwner` Solidity kill switch |
| `normalize.js` | Shared wei-to-AVAX, hex-to-decimal, Unix-to-ISO8601 conversion, used by all data methods |

## Running each one

```bash
npm run direct-rpc
npm run fetch-transactions
npm run mcp-agent
npm run advisor -- <wallet-address>
npm run rag -- "your question"          # needs Chroma running
npm run payment-agent                    # Session 6: the full payment agent
```

## How the payment agent's signing was verified

`ethers.js` was installed fresh and a real `Wallet`, `Contract`, and
`parseUnits` call were constructed and exercised directly during
development, not just syntax-checked. The derived wallet address from a
test private key matches exactly what the Python, Go, and Rust versions
in this repo derive from that same key, real cross-language proof the
signing logic is correct.

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
`payment-agent.js`, which doesn't call a model at all, the condition
evaluation is plain code, not an LLM call, by design, you want that
logic fully deterministic and auditable when real money is involved.

## Submission

1. Test everything yourself, confirm your payment agent evaluates the condition correctly, asks for approval, and sends a real transaction on approval.
2. Screenshot the working test, including the approval prompt and the resulting Snowtrace transaction.
3. Open your PR, screenshot that too.
4. Post on X with both screenshots, tag **@code_mwangi** and **@AvaxAfrica**.
5. Copy your post link, submit it on the quest page once it's live.

Post in the Week 3 WhatsApp group for anything you get stuck on.
