# Multi-stage build: compile in a full Rust image, run in a minimal one.
# The final image doesn't need the Rust toolchain at all, just the
# compiled binaries, so it stays small.
#
# Pinned to rust:1.85. axum and tower-http (added for Session 7's API
# server) need a newer toolchain than earlier sessions did. If this
# stops compiling, bump this tag to a newer one, or run `rustup update`
# locally and check what you're actually building with, this repo was
# verified against rustc 1.91.

FROM rust:1.85 AS builder

WORKDIR /app
COPY Cargo.toml Cargo.lock ./
COPY src ./src

# Build every binary in src/bin/ in one pass.
RUN cargo build --release

FROM debian:bookworm-slim

# ca-certificates is required for outbound HTTPS calls (Anthropic,
# OpenAI, the MCP server, etc.), the slim base image doesn't include it.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/target/release/direct_rpc /app/target/release/chainkit_fetch /app/target/release/chainkit_mcp_agent /app/target/release/advisor /app/target/release/rag /app/target/release/payment_agent /app/target/release/api_server /app/

# api_server listens on :8004, exposed for the Session 7 "separate
# backend" pattern, docker-compose.yml maps this out for you already.
EXPOSE 8004

CMD ["sh", "-c", "echo 'Specify a binary to run, e.g.: docker compose run rust ./rag \"your question\"'"]
