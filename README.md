# knowledgeMesh

knowledgeMesh is a minimal open-source scaffold for a modular marketplace-style mesh system in Go.
It is structured so `buyer`, `seller`, and `control` can evolve independently while sharing protocol and config packages.

## Tech

- Go
- Cobra CLI
- libp2p (QUIC multiaddr)
- `net/http` API
- `encoding/json` message handling

## Main Commands

Build all binaries:

```bash
go build ./cmd/...
```

Run core node (API + p2p host, **mock inference** only—no matchmaking or seller calls):

```bash
go run ./cmd/knowledgeMesh serve
go run ./cmd/knowledgeMesh serve --api-addr :8080 --p2p-addr /ip4/0.0.0.0/udp/0/quic-v1
```

Run the **wired buyer mesh** (HTTP API + libp2p QUIC + matchmaking + remote seller inference):

```bash
go run ./cmd/knowledgeMesh mesh serve
go run ./cmd/knowledgeMesh mesh serve --api-addr :8080 --p2p-addr /ip4/0.0.0.0/udp/0/quic-v1
```

Flags for `mesh serve`:

| Flag | Purpose |
|------|---------|
| `--api-addr` | HTTP listen address (default `:8080`) |
| `--p2p-addr` | libp2p QUIC listen multiaddr |
| `--bootstrap` | Repeatable: seller dial address, e.g. `/ip4/127.0.0.1/udp/4001/quic-v1/p2p/<PeerID>` |
| `--sellers-catalog` | JSON file: array of `SellerNode` for the matchmaker (peer id must match the seller) |
| `--demo` | Register/login a demo buyer (`demo@local` / `demo` / `demo`) and log a `X-Session-ID` to stdout |

**Local two-terminal demo:** terminal 1 runs `go run ./cmd/seller serve` (note the printed peer id and bootstrap multiaddr). Terminal 2 runs `mesh serve` with `--bootstrap` set to that multiaddr and `--sellers-catalog` pointing at `examples/local-demo/sellers-catalog.json` after you replace `REPLACE_WITH_SELLER_PEER_ID` with the seller’s peer id.

Run module CLIs:

```bash
go run ./cmd/buyer start
go run ./cmd/seller start
go run ./cmd/control start
go run ./cmd/demo run
```

Run a **seller libp2p inference node** (QUIC listener, sandbox + mock engine, inference protocol registered):

```bash
go run ./cmd/seller serve
go run ./cmd/seller serve --p2p-addr /ip4/0.0.0.0/udp/0/quic-v1 --skills chat --model-name kmg-mock-1 --price 0
```

Use the logged **dial this bootstrap** line as `mesh serve --bootstrap`, and align `peerId` in the sellers catalog with the printed **seller peer id**.

Seller registration and login (local JSON registry):

```bash
# Register seller account + metadata
go run ./cmd/seller start register \
  --username alice \
  --email alice@example.com \
  --password secret123 \
  --peer-id 12D3KooWSeller \
  --skills summarize,qa \
  --model-name gpt-mini \
  --model-type llm \
  --tuning-tier base \
  --price 0.02 \
  --cpu-cores 4 \
  --memory-mb 8192 \
  --gpus 1

# Login seller and print metadata
go run ./cmd/seller start login \
  --user alice@example.com \
  --password secret123
```

Seller on-duty (use **at most one** of `--anthropic-config`, `--openai-config`, `--ollama-config`):

```bash
# On-duty only (mock model engine unless a provider config is set below)
go run ./cmd/seller start on-duty --peer-id 12D3KooWSeller

# Anthropic — API key must live in the env named by `apiKeyEnv` (not in the JSON file)
go run ./cmd/seller start on-duty --peer-id 12D3KooWSeller --anthropic-config anthropic.json

# OpenAI — e.g. `OPENAI_API_KEY` via `apiKeyEnv`
go run ./cmd/seller start on-duty --peer-id 12D3KooWSeller --openai-config openai.json

# Ollama — mock backend for now; `baseURL` is for a future real HTTP client
go run ./cmd/seller start on-duty --peer-id 12D3KooWSeller --ollama-config ollama.json
```

Example `anthropic.json`:

```json
{
  "apiKeyEnv": "ANTHROPIC_API_KEY",
  "models": [
    {
      "id": "claude-haiku",
      "name": "claude-3-5-haiku-20241022",
      "hourlyTokens": 100000,
      "dailyTokens": 1000000,
      "totalTokens": 0
    }
  ]
}
```

Example `openai.json`:

```json
{
  "apiKeyEnv": "OPENAI_API_KEY",
  "baseURL": "https://api.openai.com/v1",
  "models": [
    {
      "id": "my-gpt",
      "name": "gpt-4o-mini",
      "hourlyTokens": 100000,
      "dailyTokens": 1000000,
      "totalTokens": 0
    }
  ]
}
```

Example `ollama.json`:

```json
{
  "baseURL": "http://127.0.0.1:11434",
  "models": [
    {
      "id": "local",
      "name": "llama3:latest",
      "hourlyTokens": 0,
      "dailyTokens": 0,
      "totalTokens": 0
    }
  ]
}
```

Seller registry path: OS user config directory, e.g. `~/.config/knowledgemesh/seller_registry.json` on Linux (see `DefaultRegistryPath()` in `internal/seller`).

### Buyer HTTP API (basic compatibility)

With `go run ./cmd/knowledgeMesh serve` (mock inference, optional session not required for chat):

- OpenAI-style: `GET /v1/models`, `POST /v1/chat/completions`
- Anthropic-style: `POST /v1/messages`
- `GET /healthz`

With `go run ./cmd/knowledgeMesh mesh serve` (real inference via matchmaker + libp2p):

- `POST /api/v1/buyer/register` — JSON: `email`, `username`, `password`; returns `buyerId`
- `POST /api/v1/buyer/login` — JSON: `user`, `password`; returns `sessionId`, `buyerId`
- `POST /v1/chat/completions` and `POST /v1/messages` require authentication: header `X-Session-ID: <sessionId>` or `Authorization: Bearer <sessionId>`
- Same OpenAI/Anthropic response shapes as above; errors follow each vendor’s JSON error style where applicable

Run tests:

```bash
go test ./...
```

## Current MVP Modules

- `internal/seller`: local seller registry, login, on-duty with **Anthropic / OpenAI / Ollama** configs (mutually exclusive), token limits, usage; model engines (`internal/seller/anthropic`, `openai`, `ollama`)
- `internal/buyer`: in-memory buyer account/session state, limits, usage accounting, preference updates, prompt submission path
- `internal/matchmaker`: simple seller selection by skill match, availability, price (ascending), and reputation tie-break (descending)
- `internal/sandbox`: request-scoped execution runner with timeout + mock executor and redacted seller-safe view
- `internal/api`: OpenAI- and Anthropic-compatible HTTP handlers; mock path (`serve`) or mesh path (`mesh serve`) with buyer register/login
- `internal/mesh`: buyer runtime wiring matchmaking, seller catalog, and libp2p inference calls
- `internal/network`: libp2p native peer connections over QUIC, request/response stream helpers, protocol negotiation, static/local bootstrap helpers

## libp2p Protocols

- Control protocol: `/knowledgemesh/control/1.0.0`
- Inference protocol: `/knowledgemesh/inference/1.0.0`

These are used by `internal/network` stream handlers and request senders:

- `RegisterRequestHandler(...)`
- `SendRequest(...)`
- `ConnectBootstrapPeers(...)`
- `NewLocalRegistry().BootstrapList()`

## Layout

- `cmd/`: runnable binaries only
- `internal/`: private module logic (`buyer`, `seller`, `control`, `matchmaker`, `network`, `api`, `sandbox`, `policy`, `state`)
- `pkg/`: shared public code (`types`, `protocol`, `config`)
- `configs/`, `docs/`, `examples/`, `tests/`: project assets and growth areas
