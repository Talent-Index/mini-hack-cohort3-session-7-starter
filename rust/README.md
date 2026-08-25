# Session 7 Starter · Rust

Autonomous payment agents and safety architecture, plus Docker support.
Compiled, memory-safe, zero-cost abstractions, all the usual reasons
you'd reach for Rust.

**Session 7 adds `src/bin/api_server.rs`**, an `axum` + Server-Sent
Events wrapper around the same signing logic as `payment_agent.rs`, the
"separate backend" pattern for builders not using Next.js. Both now
share `src/paymentagent.rs`, since Rust can't have two `fn main()`
share code any other way, see "How this was refactored" below.

## Running the API server

```bash
cargo run --bin api_server
```

Then, from a separate origin (a plain React app, for example):

```bash
curl -N -X POST http://localhost:8004/api/agent
# note the runId from the approval_required event, then:
curl -X POST http://localhost:8004/api/agent/confirm \
  -H "Content-Type: application/json" \
  -d '{"runId": "...", "approved": true}'
```

CORS is handled by `tower_http`'s `CorsLayer::permissive()` for local
development, tighten this to your actual frontend's URL before
deploying anywhere real.

## How this was refactored

The signing logic (invoice checking, pre-flight checks, transaction
construction, signing) used to live entirely inside
`src/bin/payment_agent.rs`. Since `api_server.rs` needed the exact same
logic and Rust binaries in `src/bin/` can't import from each other
directly, it was pulled into `src/paymentagent.rs`, a proper module
both binaries import through the shared library crate, the same
reasoning as `modelprovider.rs` and `normalize.rs`: duplicating signing
code is exactly the kind of thing that quietly diverges into two
subtly different, both-wrong versions over time. The full test suite
(now `src/paymentagent.rs`'s `#[cfg(test)]` block) was re-run after the
refactor to confirm behavior didn't change, run it with `cargo test
--lib`.

**One real correctness gap caught and fixed during this refactor:**
the first draft of the API server's idempotency check was wired to an
empty set on every request, meaning it could never actually detect a
duplicate payment across two separate `/api/agent` calls. Fixed to use
real shared state (`AppState.sent_invoices`), populated after every
successful send, matching what the CLI version and the other three
languages actually do.

## Setup

**With Docker** (no local Rust install needed):

```bash
cp .env.example .env
# fill in ANTHROPIC_API_KEY, OPENAI_API_KEY, AGENT_PRIVATE_KEY, FUJI_USDC_ADDRESS
cd .. && docker compose run --rm rust ./payment_agent
```

**Without Docker:**

```bash
cp .env.example .env
# fill in ANTHROPIC_API_KEY, OPENAI_API_KEY, AGENT_PRIVATE_KEY, FUJI_USDC_ADDRESS
cargo build
```

Requires Rust 1.85 or newer if building outside Docker (this was built
and verified against 1.91).

## A critical safety note, read this before you run anything

`AGENT_PRIVATE_KEY` signs real transactions. Use a wallet you generated
specifically for this cohort, funded only with Fuji testnet AVAX and
USDC from a faucet, never a wallet that holds anything real. `.env` is
already in `.gitignore`, double-check before you push anyway.

## Layout

| Path | What it is |
|---|---|
| `src/modelprovider.rs` | Provider abstraction module: `new_model_client()`, four providers, one shared async interface |
| `src/normalize.rs` | Shared wei-to-AVAX, hex-to-decimal, Unix-to-ISO8601 conversion |
| `src/bin/direct_rpc.rs` | Method 1: raw JSON-RPC over plain HTTP, no SDK at all |
| `src/bin/chainkit_fetch.rs` | Method 2: calls the Glacier REST API directly |
| `src/bin/chainkit_mcp_agent.rs` | ChainKit as MCP server, using the `rmcp` crate |
| `src/bin/advisor.rs` | Session 4: the Smart Wallet Advisor, with a human-in-the-loop checkpoint and audit logging |
| `src/bin/rag.rs` | Session 5: retrieval-augmented generation, grounded answers with citations |
| `src/paymentagent.rs` | Session 6/7: the shared invoice-checking and Fuji signing logic, imported by both binaries below, includes real `#[cfg(test)]` tests |
| `src/bin/payment_agent.rs` | Session 6: the CLI, terminal approval prompt and run loop |
| `src/bin/api_server.rs` | Session 7: the same logic, wrapped in an HTTP + SSE API with `axum` instead of a terminal prompt |
| `kill_switch.sol` | Session 6: the on-chain safety pillar, an `onlyOwner` Solidity kill switch |

## Running each one

```bash
cargo run --bin direct_rpc
cargo run --bin chainkit_fetch
cargo run --bin chainkit_mcp_agent
cargo run --bin advisor -- <wallet-address>
cargo run --bin rag -- "your question"           # needs Chroma running
cargo run --bin payment_agent                     # Session 6: the full payment agent
```

## How the payment agent's signing was actually built and verified

Uses the `ethers` crate, the crate the whole Rust Ethereum ecosystem is
built on, with the `rustls` feature so it doesn't need a system OpenSSL
install to build. A real `LocalWallet` was constructed and used to sign
a real transaction during development, not just syntax-checked.

Run the real, included test suite yourself to see this verified:

```bash
cargo test --lib
```

One of those tests, `test_key_derivation_matches_other_languages`,
derives a wallet address from a fixed test private key and asserts it
matches exactly what the JavaScript, Python, and Go versions in this
repo derive from that same key. That's real, runnable, cross-language
proof the signing logic here is correct, not a claim you have to take
on faith.

## A note on the Solidity file

There was no Solidity compiler available in the environment this repo
was built in, so `kill_switch.sol` was not compiled or deployed during
development, unlike every other file here. It follows standard,
well-established Solidity patterns and was carefully hand-checked, but
compile and test it yourself, on Remix or with Hardhat or Foundry
locally, before you deploy it.

## A note on SDKs in this folder

There is no official Rust SDK from Anthropic, OpenAI, or Google at the
time this was written, unlike Python, JavaScript, and Go, which all
have first-party SDKs. Rather than depend on an unofficial crate of
uncertain quality, every provider in `modelprovider.rs` talks to its
REST API directly with `reqwest`. `ethers` is the one exception, that's
the genuine, maintained standard for Ethereum work in Rust, not a REST
fallback.

## Model provider

`MODEL_PROVIDER` in `.env` picks the provider (`anthropic`, `openai`,
`gemini`, or `ollama`), defaulting to `anthropic`. This doesn't affect
`payment_agent`, which doesn't call a model at all, the condition
evaluation is plain code, not an LLM call, by design, you want that
logic fully deterministic and auditable when real money is involved.

## Submission

1. Test everything yourself, confirm your payment agent evaluates the condition correctly, asks for approval, and sends a real transaction on approval.
2. Screenshot the working test, including the approval prompt and the resulting Snowtrace transaction.
3. Open your PR, screenshot that too.
4. Post on X with both screenshots, tag **@code_mwangi** and **@AvaxAfrica**.
5. Copy your post link, submit it on the quest page once it's live.

Post in the Week 3 WhatsApp group for anything you get stuck on.
