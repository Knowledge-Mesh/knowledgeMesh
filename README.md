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

Run tests:

```bash
go test ./...
```

## Current MVP Modules

- `internal/seller`: local seller registry, login, on-duty state, token limits (hour/day/total), usage tracking
- `internal/buyer`: in-memory buyer account/session state, limits, usage accounting, preference updates, prompt submission path
- `internal/matchmaker`: simple seller selection by skill match, availability, price (ascending), and reputation tie-break (descending)
- `internal/sandbox`: request-scoped execution runner with timeout + mock executor and redacted seller-safe view
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
