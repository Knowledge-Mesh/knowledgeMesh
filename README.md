# knowledgeMesh

knowledgeMesh is a minimal open-source scaffold for a modular marketplace-style mesh system in Go. The `buyer`, `seller`, and `control` binaries are separate; they share `pkg/` types and `internal/network` protocols.

**Architecture and request flows** (components, PostgreSQL, matchmaking, billing, libp2p) are documented in [ARCHITECTURE.md](./ARCHITECTURE.md), including **Mermaid diagrams** (system context, runtime processes, code layers, billing flow — see *Visual architecture* at the top of that file).

> **Setting up via an autonomous coding agent?** See [AGENTS.md](./AGENTS.md). It tells the agent to ask you which of three onboarding paths to take (buyer with defaults, seller with defaults, or full local mesh) and walks each one end-to-end with health checks. The Advanced section there also keeps the operational reference (HTTP routes, env vars, libp2p protocols, P2P debug, NAT/relay, manual migrations, systemd, Docker).

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

Integration tests for libp2p networking (connection typing, hole punching, message routing, **P2P debug** env parsing, `BuildPeerDebugReport`, and the debug HTTP mux) live under [`tests/network/`](./tests/network/).

That produces one binary per `cmd/*` package (for example `bin/knowledgeMesh`, `bin/buyer`, `bin/seller`, `bin/control`, `bin/relay`, `bin/demo`). On Windows, use `bin\knowledgeMesh.exe`, etc.

Examples below use `go run ./cmd/...`; after building, run the same flags on `./bin/<name>`.

## Seller onboarding (Ollama)

**Prerequisites:** PostgreSQL + **`control api`** running (default `http://127.0.0.1:8090`; see [Control pane](#control-pane-http-api--postgresql)). [Ollama](https://ollama.com) listening on `http://127.0.0.1:11434` with a model pulled (example: `ollama pull llama3`).

```bash
# 1. Register the seller account
go run ./cmd/seller register \
  --name "Demo Seller" \
  --email seller@example.com \
  --password 'your-password'

# 2. One model + Ollama mapping + on-duty (catalog skill "my-chat" → Ollama tag "llama3:latest")
go run ./cmd/seller setup \
  --email seller@example.com \
  --password 'your-password' \
  --model-id my-chat \
  --rate-per-token 0.000001 \
  --ollama-base-url http://127.0.0.1:11434 \
  --ollama-map my-chat=llama3:latest

# 3. Verify profile
go run ./cmd/seller status --email seller@example.com --password 'your-password'

# 4. Toggle duty when needed
go run ./cmd/seller duty on --email seller@example.com --password 'your-password'
go run ./cmd/seller duty off --email seller@example.com --password 'your-password'

# 5. Start the seller node (libp2p + inference)
go run ./cmd/seller serve --email seller@example.com --password 'your-password'
# optional: --p2p-addr /ip4/0.0.0.0/udp/0/quic-v1
```

Use `--control-url https://your-control.example` on each command if you have your own control API host.

## Buyer onboarding (Ollama)

Use this after completing [Seller onboarding (Ollama)](#seller-onboarding-ollama) so a seller is on-duty and serving.

```bash
# 1. Register buyer account
go run ./cmd/buyer register \
  --name "Demo Buyer" \
  --email buyer@example.com \
  --password 'your-password'

# 2. Start buyer mesh API + libp2p node
go run ./cmd/buyer serve \
  --email buyer@example.com \
  --password 'your-password'

# 3. Send one test prompt through buyer API (matched to seller's Ollama model)
go run ./cmd/buyer prompt \
  --email buyer@example.com \
  --password 'your-password' \
  --api-url http://127.0.0.1:8080 \
  --prompt 'Write a short haiku about distributed systems.'
```

Use `--control-url https://your-control.example` on each command when the control API is remote.

## CLI reference

Commands are split by binary: **`knowledgeMesh`** is the sandbox/mock buyer API only; **buyer** carries the real mesh (control login, libp2p, matchmaking). Dedicated **`seller`**, **`control`**, **`relay`**, and **`demo`** binaries cover the rest. The `knowledgeMesh serve` command is implemented in `internal/sandbox` (mock API path).

| Binary | Command | Purpose |
|--------|---------|---------|
| `knowledgeMesh` | `serve` | Buyer HTTP API + libp2p host, **mock inference** only (`Mesh` nil). Flags: `--api-addr`, `--p2p-addr`. |
| `buyer` | `serve` (alias: `start`) | Buyer mesh: control login, control matchmaking/billing, libp2p inference to matched seller. |
| `buyer` | `p2p-debug-peer <peerID>` | Query local P2P debug HTTP API and print peer connectivity details (type, paths, last hole punch). |
| `buyer` | `register` | Register a buyer on the control pane (`--name`, `--email`, `--password`; `--control-url` optional, see below). |
| `buyer` | `prompt` | Log in to control and send one `POST /v1/chat/completions` to a buyer API (`--api-url`, `--prompt`, …). |
| `seller` | `register` | Register a seller on the control pane (`--name`, `--email`, `--password`; `--control-url` optional, see below). |
| `seller` | `setup` | One-step seller setup after registration: login, set one model (+ optional Ollama mapping), and set on-duty. |
| `seller` | `status` | Show seller profile status from control pane (on-duty, model count, peer id, Ollama configured). |
| `seller` | `duty on/off` | Toggle seller duty state using control pane auth. |
| `seller` | `serve` | QUIC listener + inference; requires `--email`, `--password`; `--control-url` optional (default `http://127.0.0.1:8090`). Optional `--p2p-addr`. Model backend (e.g. **Ollama**) from control API — see [Seller](#seller). |
| `control` | `api` | HTTP control pane + PostgreSQL (`DATABASE_URL`, `--http-addr`, `--jwt-secret`). |
| `control` | `start` | libp2p control protocol node (`/knowledgemesh/control/1.0.0`), optional `--p2p-addr`. |
| `relay` | `serve` | Minimal stateless **circuit relay v2 service** (accepts reservations, relayed connections, env/flag limits). |
| `demo` | `run` | Placeholder demo workflow. |


For **buyer** and **seller** commands that call the control HTTP API, **`--control-url` is optional** and defaults to **`http://127.0.0.1:8090`**. If you omit it, the process **prints a warning** and uses that default—set `--control-url` explicitly for non-local or production control panes.

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

Quick start is documented in [Buyer onboarding (Ollama)](#buyer-onboarding-ollama). Use the detailed notes below when you need explicit networking and API behavior.

1. Start PostgreSQL and run **`control api`** with `DATABASE_URL` set.
2. Ensure seller onboarding is complete and at least one seller is on-duty and running (**`seller serve`**), with Ollama mapping configured (example: `my-chat -> llama3:latest`).
3. **Register** a buyer (CLI or HTTP). You can omit `--control-url` to use the default `http://127.0.0.1:8090` (a warning is logged).
   ```bash
   go run ./cmd/buyer register \
     --name "My Name" \
     --email you@example.com \
     --password 'secure-password'
   ```
4. **Start the buyer mesh** (HTTP API + libp2p). You must pass **email** and **password** so the process can log in to the control pane (same default for `--control-url` as above). Matchmaking and billing run on the control API (PostgreSQL).
   ```bash
   go run ./cmd/buyer serve \
     --email you@example.com \
     --password 'secure-password'
   ```
   Add **`--bootstrap '/ip4/127.0.0.1/udp/<port>/quic-v1/p2p/<SELLER_PEER_ID>'`** if the seller’s addresses in the control DB are not reachable from this host (for example NAT or a stale listen list). The seller logs a full bootstrap line when it starts.
   The process logs a **session token**; use it as `Authorization: Bearer <token>` or `X-Session-ID: <token>` on the buyer HTTP API below.

   The same flags work for **`go run ./cmd/buyer start`** (alias of `serve`).

5. Optional: **one-shot prompt** via CLI (logs in to control, then calls the buyer mesh chat API):
   ```bash
   go run ./cmd/buyer prompt \
     --email you@example.com \
     --password 'secure-password' \
     --api-url http://127.0.0.1:8080 \
     --prompt 'Hello'
   ```

### Flags: `buyer serve` / `buyer start`

| Flag | Purpose |
|------|---------|
| `--control-url` | **Optional.** Control pane base URL (default `http://127.0.0.1:8090`). If omitted, a **warning** is logged and the default is used. |
| `--email` | **Required.** Buyer email (registered on the control pane) |
| `--password` | **Required.** Buyer password |
| `--api-addr` | Buyer HTTP API listen address (default `:8080`) |
| `--p2p-addr` | libp2p QUIC listen multiaddr (host also listens on **`/ip4/0.0.0.0/tcp/0`** for TCP fallback) |
| `--relay` | Optional repeatable **circuit relay v2** multiaddr (must include `/p2p/<relayID>`). Merged with **`LIBP2P_STATIC_RELAYS`**. Built-in default public relays apply on **buyer** and **seller** only when **`--control-url` is omitted** (implicit default); if you pass **`--control-url`**, only env/CLI relays are used (seller: also skips defaults when **`--server-mode`**). |
| `--bootstrap` | Optional repeatable seller multiaddr. Use when you cannot rely on **`sellerListenAddrs`** from the control pane (same LAN usually works without it once the seller has posted presence). |
| `--p2p-debug` | Enable verbose P2P diagnostics (NAT reachability, connection type transitions, hole punch attempts/failures). |
| `--p2p-debug-http` | Optional debug HTTP listen addr (example `127.0.0.1:9091`) exposing JSON connectivity diagnostics. Implies `--p2p-debug`. |

Environment toggles for debug:

- `KM_P2P_DEBUG` — set `1` / `true` / `yes` / `on` to enable verbose P2P debug logging.
- `KM_P2P_DEBUG_HTTP` — optional debug HTTP listen addr (for example `127.0.0.1:9091`).

When constructing a host with `NewHostWithConfig`, you can set `HostConfig.EnableP2PDebug` to a non-nil pointer to force debug on or off; it overrides the environment for that process (same as `--p2p-debug` / `--p2p-debug-http` on `buyer serve`, which set this field).

Debug endpoints (when enabled):

- `GET /debug/p2p/peer/<peerID>` — peer connection details, computed connection type, tagged type, last hole punch result.
- `GET /debug/p2p/reachability` — latest local AutoNAT reachability (`unknown` / `public` / `private`).

Debug CLI example:

```bash
KM_P2P_DEBUG=1 KM_P2P_DEBUG_HTTP=127.0.0.1:9091 \
  go run ./cmd/buyer serve --email you@example.com --password '...'

go run ./cmd/buyer p2p-debug-peer <PEER_ID> --http http://127.0.0.1:9091
```

**Local two-terminal sketch:** (1) Run `control api` with PostgreSQL. (2) Register buyer and seller; configure seller models, duty, and (for Ollama) `PUT /v1/control/sellers/me/ollama`; run **`seller serve`** so presence and listen addrs are stored. (3) Run **`buyer serve`** with buyer credentials; add `--bootstrap` only if dialing via stored addresses fails. Inference uses the control pane for **match → tracking → complete** and libp2p QUIC for the model call.

## Examples and quick binary check

Mock-only buyer API (no control pane):

```bash
go run ./cmd/knowledgeMesh serve
go run ./cmd/knowledgeMesh serve --api-addr :8080 --p2p-addr /ip4/0.0.0.0/udp/0/quic-v1
```

After `go build -o bin/ ./cmd/...`, smoke-test the main binaries (see [CLI reference](#cli-reference)):

```bash
./bin/knowledgeMesh serve
./bin/buyer serve --email you@example.com --password '...'
./bin/buyer serve --email you@example.com --password '...' --bootstrap '/ip4/127.0.0.1/udp/4001/quic-v1/p2p/<PEER_ID>'
./bin/buyer register --name 'Me' --email you@example.com --password '...'
./bin/buyer start --email you@example.com --password '...'
./bin/buyer start --email you@example.com --password '...' --bootstrap '/ip4/127.0.0.1/udp/4001/quic-v1/p2p/<PEER_ID>'
./bin/seller register --name 'Seller' --email seller@example.com --password '...'
./bin/seller serve --email seller@example.com --password '...'
./bin/control api
./bin/control start
./bin/relay serve
./bin/demo run
```

## Relay node (circuit relay v2 service)

Run a dedicated relay service process. The relay **peer ID** is stable across restarts when using the same `--identity` file (default `relay-identity.key`):

```bash
go run ./cmd/relay serve
go run ./cmd/relay serve --listen-addr /ip4/0.0.0.0/udp/4001/quic-v1
```

Flags:

| Flag | Purpose |
|------|---------|
| `--listen-addr` | libp2p listen multiaddr (default `/ip4/0.0.0.0/udp/4001/quic-v1`) |
| `--identity` | Path to persisted Ed25519 private key file (default `relay-identity.key` in cwd); same file keeps the relay **peer ID** stable across restarts |
| `--max-reservations` | Max active relay reservations |
| `--max-circuits-per-peer` | Max relayed circuits per peer |
| `--max-bandwidth-per-peer-bytes` | Max relayed bytes per peer circuit window |

Environment overrides:

- `RELAY_LISTEN_ADDR`
- `RELAY_IDENTITY_FILE` (override identity path; default `relay-identity.key`)
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

For a **stable relay peer ID** across container restarts, mount a volume and point the identity file at it (the default `relay-identity.key` is created in the process working directory):

```bash
docker run --rm -p 4001:4001/tcp -p 4001:4001/udp \
  -v relay-id:/data \
  -e RELAY_IDENTITY_FILE=/data/relay-identity.key \
  knowledgemesh-relay
```

## Ubuntu systemd services (control API + relay)

If your Ubuntu host does not already have a dedicated service account, create one first:

```bash
sudo useradd --system --home /var/lib/knowledgemesh --create-home --shell /usr/sbin/nologin knowledgemesh
```

You can also run services as another existing non-root user by replacing `User=` / `Group=` below.

Build binaries:

```bash
go build -o /usr/local/bin/knowledgemesh-control ./cmd/control
go build -o /usr/local/bin/knowledgemesh-relay ./cmd/relay
```

Control API environment file:

```bash
sudo mkdir -p /etc/knowledgemesh
sudo tee /etc/knowledgemesh/control.env >/dev/null <<'EOF'
DATABASE_URL=postgres://user:pass@127.0.0.1:5432/knowledgemesh?sslmode=disable
CONTROL_JWT_SECRET=replace-with-a-long-random-secret
EOF
sudo chmod 600 /etc/knowledgemesh/control.env
```

Create relay state dir (for stable peer ID):

```bash
sudo mkdir -p /var/lib/knowledgemesh/relay
sudo chown -R knowledgemesh:knowledgemesh /var/lib/knowledgemesh
```

`/etc/systemd/system/knowledgemesh-control.service`:

```ini
[Unit]
Description=knowledgeMesh control pane HTTP API
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
Type=simple
User=knowledgemesh
Group=knowledgemesh
EnvironmentFile=/etc/knowledgemesh/control.env
ExecStart=/usr/local/bin/knowledgemesh-control api --http-addr :8090
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

`/etc/systemd/system/knowledgemesh-relay.service`:

```ini
[Unit]
Description=knowledgeMesh relay (circuit relay v2)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=knowledgemesh
Group=knowledgemesh
WorkingDirectory=/var/lib/knowledgemesh/relay
ExecStart=/usr/local/bin/knowledgemesh-relay serve --listen-addr /ip4/0.0.0.0/udp/4001/quic-v1 --identity /var/lib/knowledgemesh/relay/identity.key
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Enable + start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now knowledgemesh-control.service
sudo systemctl enable --now knowledgemesh-relay.service
```

Inspect logs:

```bash
sudo journalctl -u knowledgemesh-control.service -f
sudo journalctl -u knowledgemesh-relay.service -f
```

## Seller

### Control pane (recommended for mesh integration)

Start with the copy-paste flow in [Seller onboarding (Ollama)](#seller-onboarding-ollama) above. Add `--p2p-addr` on `seller serve` if you need a fixed listen address.

Duty toggles:

```bash
go run ./cmd/seller duty on --email seller@example.com --password 'secure-password'
go run ./cmd/seller duty off --email seller@example.com --password 'secure-password'
```

Manual/API-first path (advanced): register and declare models via the control API (or `seller register`), then run the inference node with control login so PostgreSQL drives duty, models, and presence:

```bash
go run ./cmd/seller register \
  --name "Seller Name" \
  --email seller@example.com \
  --password 'secure-password'

go run ./cmd/seller serve \
  --email seller@example.com \
  --password 'secure-password' \
  --p2p-addr /ip4/0.0.0.0/udp/0/quic-v1
```

Omitting `--control-url` uses `http://127.0.0.1:8090` and logs a warning; set it explicitly when the control API is not on localhost.

**Optional DHT + bootstrap (NAT / AutoNAT):** if the seller only listens and has no other libp2p peers, AutoNAT v2 may not emit reachability updates quickly. Enable Kademlia DHT and pass at least one reachable bootstrap peer (often your **relay** multiaddr, or any well-connected node): `--p2p-dht` and repeatable `--p2p-bootstrap '/ip4/.../udp/.../quic-v1/p2p/<PEER_ID>'`. Environment equivalents: `KM_P2P_DHT=1`, `LIBP2P_BOOTSTRAP_PEERS` (comma-separated, same format as `--relay` / `LIBP2P_STATIC_RELAYS`). After relay addresses appear in `host.Addrs()`, repost seller presence if buyers still cannot dial.

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

With **`buyer serve`** (control pane + real inference):

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
- `internal/buyer` — session state after control login; CLI commands for `buyer` (`register`, `prompt` in `commands.go`; mesh entrypoints live in `cmd/buyer` via `internal/mesh`)
- `internal/matchmaker` — seller selection by skill, duty, price cap, then price/reputation (used **inside** the control pane for `/buyers/me/inference/match`)
- `internal/sandbox` — request-scoped runner; **`PassthroughExecutor`** on **`seller serve`**, **`MockExecutor`** for tests/mocks
- `internal/api` — OpenAI/Anthropic HTTP handlers
- `internal/mesh` — control client for match/tracking/complete, libp2p inference to matched seller
- `internal/control` — PostgreSQL (buyers, sellers, models, billing, inference matches), HTTP API (`control api`), golang-migrate SQL in `migrations/`, JWT, outbound client, libp2p handler (`control start`)
- `internal/network` — libp2p host (**QUIC + TCP**, Noise, Yamux, NAT, **AutoNAT v2**, relay + **AutoRelay**, optional **Kademlia DHT** (`EnableP2PDHT`, `LIBP2P_BOOTSTRAP_PEERS` / `KM_P2P_DHT`), **DCUtR** hole punching, connmgr, ping), plus `HolePunchManager` retries, `ConnectionTypeTracker`, size-aware `MessageRouter`, `NetworkMonitor`, relay→direct upgrade pruning, optional **`km_p2p_*` Prometheus metrics**, and optional P2P connectivity debug utilities (NAT reachability logging, connection path logs, hole punch result snapshots, debug HTTP API); see `HostConfig` / `NewHostWithConfig`, env **`LIBP2P_STATIC_RELAYS`**, **`KM_P2P_PROMETHEUS_EXPORT`**, **`KM_P2P_DEBUG`**, **`KM_P2P_DEBUG_HTTP`**
- `internal/relay` — minimal circuit relay v2 service (`relay serve`) with reservation/circuit limits and basic relay usage logging

## Layout

- `cmd/` — thin `main` packages only (`knowledgeMesh`, `buyer`, `seller`, `control`, `relay`, `demo`)
- `internal/` — private logic (`buyer`, `seller`, `control`, `mesh`, `matchmaker`, `network`, `api`, `sandbox`, `policy`, `state`)
- `pkg/` — shared libraries (`types`, `protocol`, `config`)
- `ARCHITECTURE.md` — system architecture and inference/billing flow
- `configs/`, `examples/`, `tests/` — configs, demos, tests (`tests/network` exercises libp2p helpers including P2P connectivity debug)

## Other CLIs

```bash
go run ./cmd/demo run
```

Run tests:

```bash
go test ./...
```
