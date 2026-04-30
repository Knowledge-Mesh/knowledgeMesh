# knowledgeMesh

knowledgeMesh is a modular marketplace-style mesh system in Go.  
Core binaries are `buyer`, `seller`, `control`, and `relay`.

For architecture and request flows, see [ARCHITECTURE.md](./ARCHITECTURE.md).

> **Setting up via an autonomous coding agent?** See [AGENTS.md](./AGENTS.md). It tells the agent to ask you which of three onboarding paths to take (buyer with defaults, seller with defaults, or full local mesh) and walks each one end-to-end with health checks. The Advanced section there also keeps the operational reference (HTTP routes, env vars, libp2p protocols, P2P debug, NAT/relay, manual migrations, systemd, Docker).

## Quick start — buyer connecting to the hosted public mesh

Real LLM output in three commands. The buyer registers on the hosted public control pane at `http://control.p2pinfer.cloud:8090` (the default baked into [`internal/control/defaults.go`](./internal/control/defaults.go) when `--control-url` is omitted) and runs a real prompt against whichever seller is on duty there. Only outbound internet is required — no local PostgreSQL, no Ollama, no seller process on your box.

Pick a fresh email (the public database is shared, so emails must be unique) and a password you can remember — you will need both to re-`serve` later.

```bash
# 0. Confirm the public mesh is reachable.
curl -s http://control.p2pinfer.cloud:8090/healthz
# expect: {"module":"control","status":"ok"}

# 1. Register a buyer.
go run ./cmd/buyer register \
  --name "Demo Buyer" \
  --email demo-buyer@example.com \
  --password 'change-me'

# 2. Start the buyer mesh process (leave running, or background with &).
go run ./cmd/buyer serve \
  --email demo-buyer@example.com \
  --password 'change-me'

# 3. In a second terminal, send a real prompt.
go run ./cmd/buyer prompt \
  --email demo-buyer@example.com \
  --password 'change-me' \
  --api-url http://127.0.0.1:8080 \
  --prompt 'Write a short haiku about distributed systems.'
```

Each command logs `warning: no --control-url specified; using default http://control.p2pinfer.cloud:8090` — that is expected and confirms you are on the public mesh. Pass `--control-url http://127.0.0.1:8090` (or any other URL) to point at a private control pane instead.

> **Privacy:** your prompt text and the seller's reply traverse a shared control pane and a third-party Ollama instance. Do not send sensitive data on this path.

Want a fully local stack, or to host your own seller on the public mesh? See the [Full local mesh quickstart](#full-local-mesh-quickstart-ollama-example) below, the [Seller guide](./docs/SELLER_GUIDE.md), or [AGENTS.md](./AGENTS.md) for the agent-driven flow.

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

## Full local mesh quickstart (Ollama example)

The slower path — about 15 minutes — but everything runs on your machine, no public-mesh dependency, and prompts never leave the box. Use this when you want privacy, or when the hosted public mesh is unavailable, or to develop against the seller side.

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

- `--control-url` is optional for buyer/seller flows and defaults to `http://control.p2pinfer.cloud:8090` (the hosted public mesh — see [`internal/control/defaults.go`](./internal/control/defaults.go)). Pass it explicitly to target a private control pane (e.g. `--control-url http://127.0.0.1:8090` for a local control API).
- For relay behavior and NAT traversal details, see the dedicated guides and architecture doc.
- For full environment variables and deployment steps (including Ubuntu/systemd), use [docs/SETUP.md](./docs/SETUP.md).
