# knowledgeMesh

knowledgeMesh is a minimal open-source scaffold for a modular marketplace-style mesh system in Go. The `buyer`, `seller`, and `control` binaries are separate; they share `pkg/` types and `internal/network` protocols.

**Architecture and request flows** (components, PostgreSQL, matchmaking, billing, libp2p) are documented in [ARCHITECTURE.md](./ARCHITECTURE.md), including **Mermaid diagrams** (system context, runtime processes, code layers, billing flow — see *Visual architecture* at the top of that file).

## Tech

- Go **1.24+** (see `go.mod`)
- Cobra CLI
- libp2p (QUIC multiaddr)
- `net/http` JSON APIs
- PostgreSQL via the **control** HTTP API: buyer and seller accounts, seller models, billing (wallets, quotas, transaction ledger), and inference match metadata
- **Schema migrations:** [golang-migrate](https://github.com/golang-migrate/migrate) SQL files in [`internal/control/migrations/`](./internal/control/migrations/) (embedded in the binary; applied automatically when `control api` starts)

## Compile and test

From the repository root:

```bash
cd knowledgeMesh   # or your clone path
go build -o bin/ ./cmd/...
go build ./...
go test ./...
```

That produces one binary per `cmd/*` package (for example `bin/knowledgeMesh`, `bin/buyer`, `bin/seller`, `bin/control`, `bin/relay`, `bin/demo`). On Windows, use `bin\knowledgeMesh.exe`, etc.

Examples below use `go run ./cmd/...`; after building, run the same flags on `./bin/<name>`.

## CLI reference

Commands are provided by the **`knowledgeMesh`** umbrella binary (`serve`, `mesh serve`), plus dedicated **`buyer`**, **`seller`**, **`control`**, **`relay`**, and **`demo`** binaries. Seller registration and the seller node use **`seller`** only (no duplicate under `knowledgeMesh`). The `knowledgeMesh serve` command is implemented in `internal/sandbox` (mock API path).

| Binary | Command | Purpose |
|--------|---------|---------|
| `knowledgeMesh` | `serve` | Buyer HTTP API + libp2p host, **mock inference** only (`Mesh` nil). Flags: `--api-addr`, `--p2p-addr`. |
| `knowledgeMesh` | `mesh serve` | Buyer mesh: control login, control matchmaking/billing, libp2p inference to matched seller. |
| `knowledgeMesh` | `mesh p2p-debug-peer <peerID>` | Query local P2P debug HTTP API and print peer connectivity details (type, paths, last hole punch). |
| `buyer` | `register` | Register a buyer on the control pane (`--control-url`, `--name`, `--email`, `--password`). |
| `buyer` | `start` | Same as `knowledgeMesh mesh serve` (buyer API + libp2p + control). |
| `buyer` | `prompt` | Log in to control and send one `POST /v1/chat/completions` to a buyer API (`--api-url`, `--prompt`, …). |
| `seller` | `register` | Register a seller on the control pane (`--control-url`, `--name`, `--email`, `--password`). |
| `seller` | `serve` | QUIC listener + inference; requires `--control-url`, `--email`, `--password`; optional `--p2p-addr`. Model backend (e.g. **Ollama**) from control API — see [Seller](#seller). |
| `control` | `api` | HTTP control pane + PostgreSQL (`DATABASE_URL`, `--http-addr`, `--jwt-secret`). |
| `control` | `start` | libp2p control protocol node (`/knowledgemesh/control/1.0.0`), optional `--p2p-addr`. |
| `relay` | `serve` | Minimal stateless **circuit relay v2 service** (accepts reservations, relayed connections, env/flag limits). |
| `demo` | `run` | Placeholder demo workflow. |

## Control pane (HTTP API + PostgreSQL)

The **control HTTP API** backs buyer and seller identity, seller models and duty, **billing**, and **inference match / tracking / settlement** (see [ARCHITECTURE.md](./ARCHITECTURE.md)).

**Environment**

| Variable | Purpose |
|----------|---------|
| `DATABASE_URL` | **Required** for `control api`. PostgreSQL connection string. |
| `CONTROL_JWT_SECRET` | HMAC secret for JWTs (optional; default dev secret with a log warning). Use `--jwt-secret` to override. |

**Run the API**

```bash
export DATABASE_URL='postgres://user:pass@localhost:5432/knowledgemesh?sslmode=disable'
export CONTROL_JWT_SECRET='your-secret'   # recommended in production
go run ./cmd/control api
go run ./cmd/control api --http-addr :8090 --jwt-secret 'your-secret'
```

**Database migrations**

Migration files are versioned pairs under [`internal/control/migrations/`](./internal/control/migrations/) (for example `000001_initial`, `000002_seller_listen_addrs`, `000003_seller_ollama_config`). The **`control api` process applies pending migrations automatically** on startup (embedded in the binary via [golang-migrate](https://github.com/golang-migrate/migrate)); you do not need a separate migrate step for normal development.

Applied versions are stored in PostgreSQL in **`schema_migrations`**.

#### Running migrations manually (CLI)

Use this when you want to migrate **without** starting the HTTP server, to inspect version, or to roll back in ops.

1. **Install the official CLI** (pick one):

   ```bash
   go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
   ```

   Ensure `$(go env GOPATH)/bin` is on your `PATH`, or invoke the binary by full path.

2. **From the repository root**, set `DATABASE_URL` to the same PostgreSQL DSN you use for `control api` (scheme `postgres://` or `postgresql://` is fine).

3. **Commands** (path must point at the directory containing the numbered `.sql` files):

   | Action | Command |
   |--------|---------|
   | Apply all pending migrations | `migrate -path internal/control/migrations -database "$DATABASE_URL" up` |
   | Roll back the last migration | `migrate -path internal/control/migrations -database "$DATABASE_URL" down 1` |
   | Show current version | `migrate -path internal/control/migrations -database "$DATABASE_URL" version` |
   | Apply / roll back a specific number of steps | `migrate ... up N` / `migrate ... down N` |

   Example session:

   ```bash
   export DATABASE_URL='postgres://user:pass@localhost:5432/knowledgemesh?sslmode=disable'
   migrate -path internal/control/migrations -database "$DATABASE_URL" up
   migrate -path internal/control/migrations -database "$DATABASE_URL" version
   ```

   **`migrate force`** can set the version in `schema_migrations` without running SQL (use only when you know the DB and files are in sync—for example after restoring a backup). See the [migrate CLI docs](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate).

**HTTP routes (summary)**

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/healthz` | Liveness |
| **Buyers** | | |
| `POST` | `/v1/control/buyers/register` | JSON: `name`, `email`, `password` → `buyerId` |
| `POST` | `/v1/control/buyers/login` | JSON: `email`, `password` → `accessToken`, `buyerId`, `name`, `email` |
| `POST` | `/v1/control/buyers/me/inference/match` | Bearer buyer JWT; JSON [`InferenceRequest`](./pkg/types/core.go) → `sellerPeerId`, `sellerListenAddrs`, `sellerId`, `requestId`, price, … |
| `POST` | `/v1/control/buyers/me/inference/tracking` | Bearer buyer JWT; `requestId`, `phase`, optional `meta` (audit) |
| `POST` | `/v1/control/buyers/me/inference/complete` | Bearer buyer JWT; `requestId`, `totalTokens`, `success` → wallet settlement |
| **Sellers** | | |
| `POST` | `/v1/control/sellers/register` | JSON: `name`, `email`, `password` → `sellerId` |
| `POST` | `/v1/control/sellers/login` | JSON: `email`, `password` → `accessToken` + profile |
| `GET` | `/v1/control/sellers/me` | Bearer seller JWT → profile + models |
| `PUT` | `/v1/control/sellers/me/duty` | JSON: `onDuty` |
| `PUT` | `/v1/control/sellers/me/models` | JSON: `models` array (replaces models) |
| `PATCH` | `/v1/control/sellers/me/models/{id}` | Toggle `active`, etc. |
| `POST` | `/v1/control/sellers/me/presence` | JSON: `peerId`, optional `listenAddrs` (QUIC multiaddrs) |
| `PUT` | `/v1/control/sellers/me/ollama` | JSON: [`OllamaSellerConfig`](./pkg/types/ollama.go) (`baseURL`, `models` mapping); `null` clears |
| `POST` | `/v1/control/sellers/me/inference/tracking` | Bearer seller JWT; execution audit for a `requestId` |

**Libp2p control node** (separate from the HTTP API): ping/pong JSON over `/knowledgemesh/control/1.0.0`:

```bash
go run ./cmd/control start
go run ./cmd/control start --p2p-addr /ip4/0.0.0.0/udp/0/quic-v1
```

## Buyer workflow

1. Start PostgreSQL and run **`control api`** with `DATABASE_URL` set.
2. **Register** a buyer (CLI or HTTP):
   ```bash
   go run ./cmd/buyer register \
     --control-url http://127.0.0.1:8090 \
     --name "My Name" \
     --email you@example.com \
     --password 'secure-password'
   ```
3. **Start the buyer mesh** (HTTP API + libp2p). You must pass credentials so the process can log in to the control pane. **Matchmaking and billing** run on the control API (PostgreSQL); ensure at least one seller is registered, on duty, has models, and has posted **presence** (`peerId` and **listen multiaddrs**) so the control pane can return `sellerPeerId` and **`sellerListenAddrs`** on match (the buyer dials the seller using those addresses).
   ```bash
   go run ./cmd/knowledgeMesh mesh serve \
     --control-url http://127.0.0.1:8090 \
     --email you@example.com \
     --password 'secure-password'
   ```
   Add **`--bootstrap '/ip4/127.0.0.1/udp/<port>/quic-v1/p2p/<SELLER_PEER_ID>'`** if the seller’s addresses in the control DB are not reachable from this host (for example NAT or a stale listen list). The seller logs a full bootstrap line when it starts.
   The process logs a **session token**; use it as `Authorization: Bearer <token>` or `X-Session-ID: <token>` on the buyer HTTP API below.

   The same flags work for **`go run ./cmd/buyer start`** (equivalent to `knowledgeMesh mesh serve`).

4. Optional: **one-shot prompt** via CLI (logs in to control, then calls the buyer mesh chat API):
   ```bash
   go run ./cmd/buyer prompt \
     --control-url http://127.0.0.1:8090 \
     --email you@example.com \
     --password 'secure-password' \
     --api-url http://127.0.0.1:8080 \
     --prompt 'Hello'
   ```

### Flags: `mesh serve` / `buyer start`

| Flag | Purpose |
|------|---------|
| `--control-url` | **Required.** Control pane base URL, e.g. `http://127.0.0.1:8090` |
| `--email` | **Required.** Buyer email (registered on the control pane) |
| `--password` | **Required.** Buyer password |
| `--api-addr` | Buyer HTTP API listen address (default `:8080`) |
| `--p2p-addr` | libp2p QUIC listen multiaddr (host also listens on **`/ip4/0.0.0.0/tcp/0`** for TCP fallback) |
| `--relay` | Optional repeatable **circuit relay v2** multiaddr (must include `/p2p/<relayID>`). Merged with **`LIBP2P_STATIC_RELAYS`** for AutoRelay (NAT / CGNAT). |
| `--bootstrap` | Optional repeatable seller multiaddr. Use when you cannot rely on **`sellerListenAddrs`** from the control pane (same LAN usually works without it once the seller has posted presence). |
| `--p2p-debug` | Enable verbose P2P diagnostics (NAT reachability, connection type transitions, hole punch attempts/failures). |
| `--p2p-debug-http` | Optional debug HTTP listen addr (example `127.0.0.1:9091`) exposing JSON connectivity diagnostics. Implies `--p2p-debug`. |

Environment toggles for debug:

- `KM_P2P_DEBUG` — set `1` / `true` / `yes` / `on` to enable verbose P2P debug logging.
- `KM_P2P_DEBUG_HTTP` — optional debug HTTP listen addr (for example `127.0.0.1:9091`).

Debug endpoints (when enabled):

- `GET /debug/p2p/peer/<peerID>` — peer connection details, computed connection type, tagged type, last hole punch result.
- `GET /debug/p2p/reachability` — latest local AutoNAT reachability (`unknown` / `public` / `private`).

Debug CLI example:

```bash
KM_P2P_DEBUG=1 KM_P2P_DEBUG_HTTP=127.0.0.1:9091 \
  go run ./cmd/knowledgeMesh mesh serve --control-url http://127.0.0.1:8090 --email you@example.com --password '...'

go run ./cmd/knowledgeMesh mesh p2p-debug-peer <PEER_ID> --http http://127.0.0.1:9091
```

**Local two-terminal sketch:** (1) Run `control api` with PostgreSQL. (2) Register buyer and seller; configure seller models, duty, and (for Ollama) `PUT /v1/control/sellers/me/ollama`; run **`seller serve`** so presence and listen addrs are stored. (3) Run **`mesh serve`** with buyer credentials; add `--bootstrap` only if dialing via stored addresses fails. Inference uses the control pane for **match → tracking → complete** and libp2p QUIC for the model call.

## Examples and quick binary check

Mock-only buyer API (no control pane):

```bash
go run ./cmd/knowledgeMesh serve
go run ./cmd/knowledgeMesh serve --api-addr :8080 --p2p-addr /ip4/0.0.0.0/udp/0/quic-v1
```

After `go build -o bin/ ./cmd/...`, smoke-test the main binaries (see [CLI reference](#cli-reference)):

```bash
./bin/knowledgeMesh serve
./bin/knowledgeMesh mesh serve --control-url http://127.0.0.1:8090 --email you@example.com --password '...'
./bin/knowledgeMesh mesh serve --control-url http://127.0.0.1:8090 --email you@example.com --password '...' --bootstrap '/ip4/127.0.0.1/udp/4001/quic-v1/p2p/<PEER_ID>'
./bin/buyer register --control-url http://127.0.0.1:8090 --name 'Me' --email you@example.com --password '...'
./bin/buyer start --control-url http://127.0.0.1:8090 --email you@example.com --password '...'
./bin/buyer start --control-url http://127.0.0.1:8090 --email you@example.com --password '...' --bootstrap '/ip4/127.0.0.1/udp/4001/quic-v1/p2p/<PEER_ID>'
./bin/seller register --control-url http://127.0.0.1:8090 --name 'Seller' --email seller@example.com --password '...'
./bin/seller serve --control-url http://127.0.0.1:8090 --email seller@example.com --password '...'
./bin/control api
./bin/control start
./bin/relay serve
./bin/demo run
```

## Relay node (circuit relay v2 service)

Run a dedicated relay service process (stateless):

```bash
go run ./cmd/relay serve
go run ./cmd/relay serve --listen-addr /ip4/0.0.0.0/udp/4001/quic-v1
```

Flags:

| Flag | Purpose |
|------|---------|
| `--listen-addr` | libp2p listen multiaddr (default `/ip4/0.0.0.0/udp/4001/quic-v1`) |
| `--max-reservations` | Max active relay reservations |
| `--max-circuits-per-peer` | Max relayed circuits per peer |
| `--max-bandwidth-per-peer-bytes` | Max relayed bytes per peer circuit window |

Environment overrides:

- `RELAY_LISTEN_ADDR`
- `RELAY_MAX_RESERVATIONS`
- `RELAY_MAX_CIRCUITS_PER_PEER`
- `RELAY_MAX_BANDWIDTH_PER_PEER_BYTES`
- `RELAY_CONN_LOW_WATER`
- `RELAY_CONN_HIGH_WATER`
- `RELAY_CONN_GRACE_SECONDS`
- `RELAY_MAX_CIRCUIT_DURATION_SECONDS`

Docker (lightweight service):

```bash
docker build -f Dockerfile.relay -t knowledgemesh-relay .
docker run --rm -p 4001:4001/tcp -p 4001:4001/udp knowledgemesh-relay
```

## Seller

### Control pane (recommended for mesh integration)

Register and declare models via the control API (or `seller register`), then run the inference node with control login so PostgreSQL drives duty, models, and presence:

```bash
go run ./cmd/seller register \
  --control-url http://127.0.0.1:8090 \
  --name "Seller Name" \
  --email seller@example.com \
  --password 'secure-password'

go run ./cmd/seller serve \
  --control-url http://127.0.0.1:8090 \
  --email seller@example.com \
  --password 'secure-password' \
  --p2p-addr /ip4/0.0.0.0/udp/0/quic-v1
```

After **`POST /v1/control/sellers/login`**, configure Ollama (example: map catalog model name `my-chat` to local tag `llama3:latest`):

```bash
curl -sS -X PUT 'http://127.0.0.1:8090/v1/control/sellers/me/ollama' \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $SELLER_ACCESS_TOKEN" \
  -d '{"baseURL":"http://127.0.0.1:11434","models":[{"id":"my-chat","name":"llama3:latest"}]}'
```

Use the printed **dial this bootstrap** line for the buyer’s **`--bootstrap`** when you need a manual dial path. Models, token limits, and rates are set with **`PUT /v1/control/sellers/me/models`**. **Ollama** endpoint and model-tag mapping are stored in PostgreSQL via **`PUT /v1/control/sellers/me/ollama`** ([`OllamaSellerConfig`](./pkg/types/ollama.go): `baseURL`, e.g. `http://127.0.0.1:11434`, and `models` with `id` = marketplace model name from your declared models, `name` = Ollama tag such as `llama3:latest`). Empty `baseURL` defaults to `http://127.0.0.1:11434`. Send JSON **`null`** to clear Ollama config. **Restart `seller serve`** after changing Ollama settings (profile is read at startup).

**Inference backends (seller):** the mesh loads **`Ollama`** settings from the control API (`GET /v1/control/sellers/me` after you **`PUT .../ollama`**). **`ModelEngineFromSellerNode`** (`internal/seller/engine.go`) chooses **OpenAI** → **Anthropic** → **Ollama** → **mock** based on which blocks are present on the node (today the control-backed profile supplies **Ollama**; Anthropic/OpenAI are for custom wiring). Prompts still go through the **sandbox** `Runner` before the backend (`internal/sandbox`); **`seller serve`** uses a **passthrough** executor so the buyer’s text reaches the model (tests and demos may still use **`MockExecutor`**).

## Buyer HTTP API (OpenAI / Anthropic style)

With **`knowledgeMesh serve`** (mock path, no control pane):

- `GET /v1/models`, `POST /v1/chat/completions` (OpenAI-style)
- `POST /v1/messages` (Anthropic-style)
- `GET /healthz`

With **`knowledgeMesh mesh serve`** (control pane + real inference):

- `POST /api/v1/buyer/register` — JSON: `name` or `username`, `email`, `password` → `buyerId` (via control pane)
- `POST /api/v1/buyer/login` — JSON: `user`, `password` → `sessionId`, `buyerId`
- `POST /v1/chat/completions`, `POST /v1/messages` — require `X-Session-ID` or `Authorization: Bearer` (use the token printed after mesh startup, or from login). Each completion triggers control **match**, libp2p **inference**, then control **tracking** and **complete** (billing) as described in [ARCHITECTURE.md](./ARCHITECTURE.md).

## libp2p protocols

| Protocol ID | Use |
|-------------|-----|
| `/knowledgemesh/control/1.0.0` | Control streams (`control start`) |
| `/knowledgemesh/inference/1.0.0` | Inference request/response |

Helpers in `internal/network`: `RegisterRequestHandler`, `SendRequest`, `ConnectBootstrapPeers`, `ConnectToPeer`, `NewLocalRegistry().BootstrapList()`.

## Current modules

- `internal/seller` — Anthropic / OpenAI / Ollama facades; `serve` loads profile (models, duty, **Ollama** from PostgreSQL) and posts inference tracking to control
- `internal/buyer` — session state after control login; CLI commands for `buyer` (`register`, `prompt` in `commands.go`)
- `internal/matchmaker` — seller selection by skill, duty, price cap, then price/reputation (used **inside** the control pane for `/buyers/me/inference/match`)
- `internal/sandbox` — request-scoped runner; **`PassthroughExecutor`** on **`seller serve`**, **`MockExecutor`** for tests/mocks
- `internal/api` — OpenAI/Anthropic HTTP handlers
- `internal/mesh` — control client for match/tracking/complete, libp2p inference to matched seller
- `internal/control` — PostgreSQL (buyers, sellers, models, billing, inference matches), HTTP API (`control api`), golang-migrate SQL in `migrations/`, JWT, outbound client, libp2p handler (`control start`)
- `internal/network` — libp2p host (**QUIC + TCP**, Noise, Yamux, NAT, **AutoNAT v2**, relay + **AutoRelay**, **DCUtR** hole punching, connmgr, ping), plus `HolePunchManager` retries, `ConnectionTypeTracker`, size-aware `MessageRouter`, `NetworkMonitor`, relay→direct upgrade pruning, optional **`km_p2p_*` Prometheus metrics**, and optional P2P connectivity debug utilities (NAT reachability logging, connection path logs, hole punch result snapshots, debug HTTP API); see `HostConfig` / `NewHostWithConfig`, env **`LIBP2P_STATIC_RELAYS`**, **`KM_P2P_PROMETHEUS_EXPORT`**, **`KM_P2P_DEBUG`**, **`KM_P2P_DEBUG_HTTP`**
- `internal/relay` — minimal circuit relay v2 service (`relay serve`) with reservation/circuit limits and basic relay usage logging

## Layout

- `cmd/` — thin `main` packages only (`knowledgeMesh`, `buyer`, `seller`, `control`, `relay`, `demo`)
- `internal/` — private logic (`buyer`, `seller`, `control`, `mesh`, `matchmaker`, `network`, `api`, `sandbox`, `policy`, `state`)
- `pkg/` — shared libraries (`types`, `protocol`, `config`)
- `ARCHITECTURE.md` — system architecture and inference/billing flow
- `configs/`, `examples/`, `tests/` — configs, demos, tests

## Other CLIs

```bash
go run ./cmd/demo run
```

Run tests:

```bash
go test ./...
```
