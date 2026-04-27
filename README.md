# knowledgeMesh

knowledgeMesh is a modular marketplace-style mesh system in Go.  
Core binaries are `buyer`, `seller`, `control`, and `relay`.

For architecture and request flows, see [ARCHITECTURE.md](./ARCHITECTURE.md).

## Guides

- [Full setup guide](./docs/SETUP.md) - end-to-end local setup (control API, relay, seller, buyer)
- [Seller guide](./docs/SELLER_GUIDE.md) - seller onboarding, duty, serve, and common ops
- [Buyer guide](./docs/BUYER_GUIDE.md) - buyer onboarding, serve, prompt, and debug basics

## Tech

- Go 1.24+
- Cobra CLI
- libp2p (QUIC/TCP, NAT traversal, relay/hole-punch support)
- HTTP JSON APIs
- PostgreSQL via control API

## Build and test

From repository root:

```bash
cd knowledgeMesh
go build -o bin/ ./cmd/...
go test ./...
```

## Quickstart (Ollama example)

Prereqs:

- PostgreSQL running
- Ollama running at `http://127.0.0.1:11434`
- Example model pulled: `ollama pull llama3`

Start control API:

```bash
export DATABASE_URL='postgres://user:pass@localhost:5432/knowledgemesh?sslmode=disable'
export CONTROL_JWT_SECRET='dev-secret-change-me'
go run ./cmd/control api
```

Optional relay:

```bash
go run ./cmd/relay serve --listen-addr /ip4/0.0.0.0/udp/4001/quic-v1
```

Seller quick onboarding:

```bash
go run ./cmd/seller register --name "Demo Seller" --email seller@example.com --password 'your-password'
go run ./cmd/seller setup --email seller@example.com --password 'your-password' --model-id my-chat --rate-per-token 0.000001 --ollama-base-url http://127.0.0.1:11434 --ollama-map my-chat=llama3:latest
go run ./cmd/seller serve --email seller@example.com --password 'your-password'
```

Buyer quick onboarding + test:

```bash
go run ./cmd/buyer register --name "Demo Buyer" --email buyer@example.com --password 'your-password'
go run ./cmd/buyer serve --email buyer@example.com --password 'your-password'
go run ./cmd/buyer prompt --email buyer@example.com --password 'your-password' --api-url http://127.0.0.1:8080 --prompt 'Write a short haiku about distributed systems.'
```

If control API is not local, add `--control-url https://your-control.example` to buyer/seller commands.

## CLI overview

| Binary | Key commands |
|--------|--------------|
| `buyer` | `register`, `serve` (`start`), `prompt`, `p2p-debug-peer` |
| `seller` | `register`, `setup`, `status`, `duty on/off`, `serve` |
| `control` | `api`, `start` |
| `relay` | `serve` |
| `knowledgeMesh` | `serve` (sandbox/mock path) |
| `demo` | `run` |

## Notes

- `--control-url` is optional for buyer/seller flows and defaults to `http://127.0.0.1:8090`.
- For relay behavior and NAT traversal details, see the dedicated guides and architecture doc.
- For full environment variables and deployment steps (including Ubuntu/systemd), use [docs/SETUP.md](./docs/SETUP.md).
