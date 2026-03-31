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
```

Run module CLIs:

```bash
go run ./cmd/buyer start
go run ./cmd/seller start
go run ./cmd/control start
go run ./cmd/demo run
```

## Layout

- `cmd/`: runnable binaries only
- `internal/`: private module logic (`buyer`, `seller`, `control`, `matchmaker`, `network`, `api`, `sandbox`, `policy`, `state`)
- `pkg/`: shared public code (`types`, `protocol`, `config`)
- `configs/`, `docs/`, `examples/`, `tests/`: project assets and growth areas
