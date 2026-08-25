# Multi-stage build: compile in a full Go image, run in a minimal one.
# The final image doesn't need the Go toolchain at all, just the
# compiled binaries, so it stays small.

FROM golang:1.23 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build every runnable example as its own binary. If you add a new
# command directory, add its build line here too.
RUN mkdir -p /out && \
    go build -o /out/direct-rpc ./direct-rpc && \
    go build -o /out/chainkit-fetch ./chainkit-fetch && \
    go build -o /out/chainkit-mcp-agent ./chainkit-mcp-agent && \
    go build -o /out/advisor ./advisor && \
    go build -o /out/rag ./rag && \
    go build -o /out/payment-agent ./payment-agent && \
    go build -o /out/api-server ./api-server

FROM debian:bookworm-slim

# ca-certificates is required for outbound HTTPS calls (Anthropic,
# OpenAI, Glacier, etc.), the slim base image doesn't include it.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/ /app/

# api-server listens on :8000, exposed for the Session 7 "separate
# backend" pattern, docker-compose.yml maps this out for you already.
EXPOSE 8000

CMD ["sh", "-c", "echo 'Specify a binary to run, e.g.: docker compose run golang ./rag \"your question\"'"]
