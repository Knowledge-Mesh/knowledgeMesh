# Full Setup Guide

This guide brings up a local end-to-end flow: control API + relay + seller (Ollama) + buyer.

## 1) Build

```bash
go build -o bin/ ./cmd/...
```

## 2) Start control API (PostgreSQL required)

```bash
export DATABASE_URL='postgres://user:pass@localhost:5432/knowledgemesh?sslmode=disable'
export CONTROL_JWT_SECRET='dev-secret-change-me'
go run ./cmd/control api
```

## 3) Start relay (optional but recommended)

```bash
go run ./cmd/relay serve --listen-addr /ip4/0.0.0.0/udp/4001/quic-v1
```

## 4) Seller onboarding (Ollama example)

```bash
go run ./cmd/seller register \
  --name "Demo Seller" \
  --email seller@example.com \
  --password 'your-password'

go run ./cmd/seller setup \
  --email seller@example.com \
  --password 'your-password' \
  --model-id my-chat \
  --rate-per-token 0.000001 \
  --ollama-base-url http://127.0.0.1:11434 \
  --ollama-map my-chat=llama3:latest

go run ./cmd/seller serve \
  --email seller@example.com \
  --password 'your-password'
```

## 5) Buyer onboarding + test prompt

```bash
go run ./cmd/buyer register \
  --name "Demo Buyer" \
  --email buyer@example.com \
  --password 'your-password'

go run ./cmd/buyer serve \
  --email buyer@example.com \
  --password 'your-password'

go run ./cmd/buyer prompt \
  --email buyer@example.com \
  --password 'your-password' \
  --api-url http://127.0.0.1:8080 \
  --prompt 'Write a short haiku about distributed systems.'
```

## 6) Quick checks

- Control API health: `curl http://127.0.0.1:8090/healthz`
- Buyer API health: `curl http://127.0.0.1:8080/healthz`
- Seller profile state: `go run ./cmd/seller status --email seller@example.com --password 'your-password'`

