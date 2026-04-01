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

Run core node (API + p2p bootstrap):

```bash
go run ./cmd/knowledgeMesh serve
go run ./cmd/knowledgeMesh serve --api-addr :8080 --p2p-addr /ip4/0.0.0.0/udp/0/quic-v1
```

Run module CLIs:

```bash
go run ./cmd/buyer start
go run ./cmd/seller start
go run ./cmd/control start
go run ./cmd/demo run
```

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

With `go run ./cmd/knowledgeMesh serve` (API + libp2p host):

- OpenAI-style: `GET /v1/models`, `POST /v1/chat/completions`
- Anthropic-style: `GET /v1/models`, `POST /v1/messages`
- `GET /healthz`

Run tests:

```bash
go test ./...
```

## Current MVP Modules

- `internal/seller`: local seller registry, login, on-duty with **Anthropic / OpenAI / Ollama** configs (mutually exclusive), token limits, usage; model engines (`internal/seller/anthropic`, `openai`, `ollama`)
- `internal/buyer`: in-memory buyer account/session state, limits, usage accounting, preference updates, prompt submission path
- `internal/matchmaker`: simple seller selection by skill match, availability, price (ascending), and reputation tie-break (descending)
- `internal/sandbox`: request-scoped execution runner with timeout + mock executor and redacted seller-safe view
- `internal/api`: minimal OpenAI- and Anthropic-compatible HTTP handlers for buyers (mock inference path)
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
