# Agents — start here

This file is the entry point for autonomous coding agents (Claude Code, Cursor, Aider, Codex, etc.) bringing up `knowledgeMesh` on a user's machine. **Read this before the README.** The README is human-skim documentation; this file is the executable runbook.

If you are a human, this file is also a useful index — but you may prefer the [README](./README.md) and [ARCHITECTURE.md](./ARCHITECTURE.md).

---

## What you must do first

Before running any commands, ask the user **exactly one question** and wait for their answer. Use this prompt verbatim (you may reformat it for your UI):

> I see this is **knowledgeMesh** — a marketplace-style mesh for AI inference (buyer ↔ control pane ↔ seller, over a libp2p QUIC overlay). I can set this up three ways. Pick one:
>
> **(1) Connect as a buyer with defaults** — fastest path. Spins up a mock buyer HTTP API on `localhost:8080`. No Postgres, no Ollama, no seller needed. Good for testing the OpenAI/Anthropic-style API surface end-to-end without external dependencies.
>
> **(2) Connect as a seller with defaults** — registers a seller account against an existing control pane and exposes a local Ollama model. Requires Ollama running locally and a control API to register against (your own, or someone else's URL).
>
> **(3) Help me set up my own mesh** — full local stack: PostgreSQL → control API → relay (optional) → seller → buyer, with an end-to-end smoke test at the end. Most work, full functionality.

After the user answers `1`, `2`, or `3`, jump to the matching path below. Do not proceed without their answer. If they describe a different scenario (e.g. "I just want to read the code"), default to summarising the architecture from [ARCHITECTURE.md](./ARCHITECTURE.md) and ask again.

## Decision matrix

| | (1) Buyer w/ defaults | (2) Seller w/ defaults | (3) Own mesh |
|---|---|---|---|
| Wall-clock | ~2 min | ~5 min | ~15–20 min |
| External deps | none | Ollama + reachable control API | Postgres + Ollama (+ optional Docker) |
| Long-running processes | 1 | 1 | 3–4 |
| What runs | `cmd/knowledgeMesh` (mock) | `cmd/seller serve` | `control api` + `seller serve` + `buyer serve` (+ optional `relay serve`) |
| What this verifies | OpenAI / Anthropic API surface, libp2p host bring-up | seller registration, Ollama wiring, presence | full match → inference → settlement loop |
| Touches Postgres? | no | no | yes (migrations auto-apply) |

---

## Prerequisites for every path

Run these once, regardless of which path the user picks.

```bash
# 1. Toolchain
go version    # require 1.24.6+

# 2. Repo state
go mod download
go build ./...
go test ./...
```

If `go version` is older than 1.24.6, install via the user's preferred method (`mise install go@1.24.6`, `asdf install golang 1.24.6`, Homebrew, manual). Ask the user before installing system-wide.

**The repo is private.** Clone via `gh repo clone Knowledge-Mesh/knowledgeMesh` (requires `gh auth login`).

> **Module-path gotcha.** `go.mod` declares `module github.com/knowledgemeshgrid/knowledgemesh`, but that path 404s and there is no vanity import resolver. **Do not use `go install <module-path>/cmd/<binary>@latest`.** Always clone locally and use `go build ./...` or `go run ./cmd/<binary>`.

---

## Path 1 — Buyer with defaults (mock sandbox)

**Goal:** working buyer HTTP API on `localhost:8080` with mock inference, in a single command. Zero external dependencies.

**Questions to ask the user:** none. Defaults work.

**Run (foreground, in its own terminal or as a background process):**

```bash
go run ./cmd/knowledgeMesh serve
# or with explicit flags:
go run ./cmd/knowledgeMesh serve --api-addr :8080 --p2p-addr /ip4/0.0.0.0/udp/0/quic-v1
```

**Verify in a second terminal:**

```bash
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/v1/models | head
curl -s -X POST http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"kmg-mock-1","messages":[{"role":"user","content":"hello"}]}'
```

**Success criteria:**
- `/healthz` returns HTTP 200.
- `/v1/chat/completions` returns a JSON body with `choices[0].message.content` populated (mock content).

**What this does and doesn't exercise:**
- ✅ HTTP API surface (`internal/api`)
- ✅ Sandbox runner (`internal/sandbox`) with `MockExecutor`
- ✅ libp2p host bring-up (no peers required)
- ❌ Control pane / Postgres / billing
- ❌ Real inference (no Ollama call)
- ❌ Seller side / matchmaking

**Failure modes:**
| Symptom | Fix |
|---|---|
| `bind: address already in use` | rerun with `--api-addr :PORT` (and a free port) |
| `go: cannot find module` | rerun `go mod download` |
| `go: requires Go 1.24.6` | install Go ≥ 1.24.6 (see Prerequisites) |

When the user is done, kill the process. Nothing persists; no cleanup needed.

---

## Path 2 — Seller with defaults

**Goal:** register a seller account against a control pane, configure Ollama mapping, and serve.

**Questions to ask the user (collect all before running):**

| # | Question | Default if user says "use defaults" |
|---|---|---|
| 1 | Control API URL? | `http://127.0.0.1:8090` (only correct if a local control API is running — if not, suggest Path 3 first) |
| 2 | Seller email | `seller@example.com` |
| 3 | Seller password | generate a 24-char random password and surface it to the user |
| 4 | Catalog model id | `my-chat` |
| 5 | Ollama tag to expose | `llama3:latest` |
| 6 | Ollama base URL | `http://127.0.0.1:11434` |
| 7 | Rate per token | `0.000001` |

If the user picks Path 2 but no control API is reachable, fall back to Path 3 and surface the change.

**Confirm Ollama is running and the model is pulled:**

```bash
curl -s http://127.0.0.1:11434/api/tags
ollama pull llama3    # only if the tag isn't already present
```

**Run (substitute placeholder `<...>` with answers from the table above):**

```bash
go run ./cmd/seller register \
  --control-url <CONTROL_URL> \
  --name "Demo Seller" \
  --email <EMAIL> \
  --password '<PASSWORD>'

go run ./cmd/seller setup \
  --control-url <CONTROL_URL> \
  --email <EMAIL> \
  --password '<PASSWORD>' \
  --model-id <MODEL_ID> \
  --rate-per-token <RATE> \
  --ollama-base-url <OLLAMA_URL> \
  --ollama-map <MODEL_ID>=<OLLAMA_TAG>

go run ./cmd/seller status \
  --control-url <CONTROL_URL> \
  --email <EMAIL> \
  --password '<PASSWORD>'

go run ./cmd/seller serve \
  --control-url <CONTROL_URL> \
  --email <EMAIL> \
  --password '<PASSWORD>'
```

**Success criteria:**
- `seller status` output includes `onDuty: true` and at least one model.
- `seller serve` log emits a line beginning `dial this bootstrap: /ip4/...` — capture this; buyers may need it as `--bootstrap`.

**Failure modes:**
| Symptom | Fix |
|---|---|
| `connection refused` to control URL | confirm the control API is up; if there is no control API, suggest Path 3 |
| `ollama: connection refused` | start Ollama (`ollama serve`) |
| `model "<tag>" not found` | `ollama pull <tag>` |
| `seller already registered` | skip `register`, proceed to `setup` |
| `seller serve` exits with `failed to login` | re-check email/password; if the user changed Ollama config since last serve, restart `serve` |

**Toggle duty without restarting serve:**

```bash
go run ./cmd/seller duty off --control-url <CONTROL_URL> --email <EMAIL> --password '<PASSWORD>'
go run ./cmd/seller duty on  --control-url <CONTROL_URL> --email <EMAIL> --password '<PASSWORD>'
```

---

## Path 3 — Help me set up my own mesh

**Goal:** full stack on one machine, ending in a successful end-to-end inference round-trip.

**Questions to ask the user (collect all before running):**

| # | Question | Default |
|---|---|---|
| 1 | Postgres preference: (a) Docker container, (b) existing local Postgres, (c) other DSN | (a) Docker |
| 2 | Should I install Ollama if missing? | yes |
| 3 | JWT secret for control API | generate a 48-char random string and surface it |
| 4 | Seller / buyer credentials | `seller@example.com` / `buyer@example.com` with generated passwords |

### 3.1 Bring up Postgres

**Option (a) — Docker (easiest):**

```bash
docker run -d --name kmesh-pg \
  -e POSTGRES_USER=knowledgemesh \
  -e POSTGRES_PASSWORD=knowledgemesh \
  -e POSTGRES_DB=knowledgemesh \
  -p 5432:5432 \
  postgres:16
```

DSN: `postgres://knowledgemesh:knowledgemesh@127.0.0.1:5432/knowledgemesh?sslmode=disable`

**Option (b) — existing local Postgres:** ask the user for the DSN. Confirm the named database exists or create it: `createdb knowledgemesh`.

**Option (c) — other DSN:** record verbatim.

### 3.2 Start the control API

In its own terminal / background process:

```bash
export DATABASE_URL='<DSN_FROM_3.1>'
export CONTROL_JWT_SECRET='<GENERATED_SECRET>'
go run ./cmd/control api
```

Migrations apply automatically on startup (`internal/control/migrations/*.sql` are embedded). Look for log lines confirming applied versions.

**Verify:** `curl -s http://127.0.0.1:8090/healthz` returns 200. Do not proceed until this passes.

### 3.3 Start the relay (optional, but improves NAT traversal)

```bash
go run ./cmd/relay serve --listen-addr /ip4/0.0.0.0/udp/4001/quic-v1
```

The relay logs its peer ID on startup. If the seller and buyer are on the same machine you can skip this step; add it back if NAT traversal fails later.

### 3.4 Ensure Ollama is running and a model is pulled

```bash
ollama serve &      # if not already running
ollama pull llama3
curl -s http://127.0.0.1:11434/api/tags    # confirm model is present
```

### 3.5 Onboard the seller

In its own terminal / background process — follow [Path 2](#path-2--seller-with-defaults) with `--control-url http://127.0.0.1:8090`. Capture the bootstrap multiaddr from the `seller serve` log; you may need it for the buyer.

### 3.6 Onboard the buyer

```bash
go run ./cmd/buyer register \
  --control-url http://127.0.0.1:8090 \
  --name "Demo Buyer" \
  --email buyer@example.com \
  --password '<PASSWORD>'

go run ./cmd/buyer serve \
  --control-url http://127.0.0.1:8090 \
  --email buyer@example.com \
  --password '<PASSWORD>'
```

If buyer dialing fails because of NAT or stale presence, re-run with the bootstrap multiaddr from 3.5:

```bash
go run ./cmd/buyer serve \
  --control-url http://127.0.0.1:8090 \
  --email buyer@example.com \
  --password '<PASSWORD>' \
  --bootstrap '/ip4/127.0.0.1/udp/<PORT>/quic-v1/p2p/<SELLER_PEER_ID>'
```

### 3.7 End-to-end smoke test

```bash
go run ./cmd/buyer prompt \
  --control-url http://127.0.0.1:8090 \
  --email buyer@example.com \
  --password '<PASSWORD>' \
  --api-url http://127.0.0.1:8080 \
  --prompt 'Write a short haiku about distributed systems.'
```

**Success criteria:**
- The command prints a non-empty completion (a haiku, in this case).
- In Postgres: `inference_matches` has a new row with `settled = true`; `billing_transactions` has a debit on the buyer wallet and a credit on the seller wallet.

```bash
psql "$DATABASE_URL" -c 'SELECT request_id, settled FROM inference_matches ORDER BY created_at DESC LIMIT 1;'
psql "$DATABASE_URL" -c 'SELECT entry_type, amount, created_at FROM billing_transactions ORDER BY created_at DESC LIMIT 5;'
```

If the prompt fails, diagnostics live in [Advanced § P2P debug endpoints](#p2p-debug-endpoints) and [Advanced § NAT, relay, and bootstrap](#nat-relay-and-bootstrap).

---

## Common operations

| Task | Command |
|---|---|
| Toggle seller off-duty | `go run ./cmd/seller duty off --email ... --password ...` |
| Show seller profile | `go run ./cmd/seller status --email ... --password ...` |
| Inspect buyer's view of a peer | `go run ./cmd/buyer p2p-debug-peer <PEER_ID> --http http://127.0.0.1:9091` |
| Healthcheck — control | `curl http://127.0.0.1:8090/healthz` |
| Healthcheck — buyer | `curl http://127.0.0.1:8080/healthz` |
| Migration version | `migrate -path internal/control/migrations -database "$DATABASE_URL" version` |
| Stop one binary | `pkill -f 'go run ./cmd/<name>'` |
| Stop all binaries | `pkill -f 'go run ./cmd/'` |
| Stop Postgres container | `docker stop kmesh-pg && docker rm kmesh-pg` |

---

# Advanced

The sections below are reference material, not setup steps. Read them when debugging, customising, or productionising.

## Architecture pointer

[ARCHITECTURE.md](./ARCHITECTURE.md) has Mermaid diagrams for system context, runtime processes, code layers, billing/match flow, and the end-to-end inference sequence. Read it when reasoning about *why* the components are split the way they are.

## Control API HTTP routes

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/healthz` | none | Liveness |
| `POST` | `/v1/control/buyers/register` | none | Body: `name`, `email`, `password` → `buyerId` |
| `POST` | `/v1/control/buyers/login` | none | Body: `email`, `password` → `accessToken`, `buyerId`, `name`, `email` |
| `POST` | `/v1/control/buyers/me/inference/match` | buyer JWT | Body: [`InferenceRequest`](./pkg/types/core.go) → `sellerPeerId`, `sellerListenAddrs`, `sellerId`, `requestId`, price |
| `POST` | `/v1/control/buyers/me/inference/tracking` | buyer JWT | Body: `requestId`, `phase`, optional `meta` (audit ledger) |
| `POST` | `/v1/control/buyers/me/inference/complete` | buyer JWT | Body: `requestId`, `totalTokens`, `success` → wallet settlement (idempotent per `requestId`) |
| `POST` | `/v1/control/sellers/register` | none | Body: `name`, `email`, `password` → `sellerId` |
| `POST` | `/v1/control/sellers/login` | none | Body: `email`, `password` → `accessToken` + profile |
| `GET` | `/v1/control/sellers/me` | seller JWT | Profile + models |
| `PUT` | `/v1/control/sellers/me/duty` | seller JWT | Body: `onDuty` |
| `PUT` | `/v1/control/sellers/me/models` | seller JWT | Body: `models` (replaces) |
| `PATCH` | `/v1/control/sellers/me/models/{id}` | seller JWT | Toggle `active`, etc. |
| `POST` | `/v1/control/sellers/me/presence` | seller JWT | Body: `peerId`, optional `listenAddrs` (QUIC multiaddrs) |
| `PUT` | `/v1/control/sellers/me/ollama` | seller JWT | Body: [`OllamaSellerConfig`](./pkg/types/ollama.go) — `null` clears |
| `POST` | `/v1/control/sellers/me/inference/tracking` | seller JWT | Execution audit for a `requestId` |

JWTs distinguish buyer vs seller subjects; tokens are not interchangeable across roles.

## Environment variables

### Control API
| Variable | Required | Default | Purpose |
|---|---|---|---|
| `DATABASE_URL` | yes | — | Postgres DSN (`postgres://` or `postgresql://`) |
| `CONTROL_JWT_SECRET` | recommended | dev secret with log warning | HMAC secret for JWTs; override with `--jwt-secret` |

### Buyer / seller P2P
| Variable | Purpose |
|---|---|
| `KM_P2P_DEBUG` | `1` / `true` / `yes` / `on` enables verbose P2P debug logging |
| `KM_P2P_DEBUG_HTTP` | Optional listen addr (e.g. `127.0.0.1:9091`) for the JSON connectivity diagnostics endpoint; implies `KM_P2P_DEBUG` |
| `KM_P2P_DHT` | `1` enables Kademlia DHT (also exposed as `--p2p-dht` on `seller serve`) |
| `KM_P2P_PROMETHEUS_EXPORT` | `1` to emit `km_p2p_*` Prometheus metrics |
| `LIBP2P_STATIC_RELAYS` | Comma-separated relay multiaddrs (must include `/p2p/<id>`); merged with `--relay` flags |
| `LIBP2P_BOOTSTRAP_PEERS` | Comma-separated bootstrap peer multiaddrs (used with DHT) |

### Relay
| Variable | Purpose |
|---|---|
| `RELAY_LISTEN_ADDR` | Override `--listen-addr` |
| `RELAY_IDENTITY_FILE` | Path to Ed25519 private key (default `relay-identity.key`); same file → same peer ID across restarts |
| `RELAY_MAX_RESERVATIONS` | Max active reservations |
| `RELAY_MAX_CIRCUITS_PER_PEER` | Max relayed circuits per peer |
| `RELAY_MAX_BANDWIDTH_PER_PEER_BYTES` | Max relayed bytes per peer circuit window |
| `RELAY_CONN_LOW_WATER` / `RELAY_CONN_HIGH_WATER` | connmgr watermarks |
| `RELAY_CONN_GRACE_SECONDS` | connmgr grace |
| `RELAY_MAX_CIRCUIT_DURATION_SECONDS` | per-circuit lifetime cap |

## libp2p protocols

| Protocol ID | Use |
|---|---|
| `/knowledgemesh/control/1.0.0` | Optional libp2p control stream (`control start`); JSON ping handler — orthogonal to the HTTP control API |
| `/knowledgemesh/inference/1.0.0` | JSON `InferenceRequest` / `InferenceResponse` over a single short-lived stream |

Each inference uses a one-shot stream: connection used for one request/response pair, then released. The size-aware message router prefers direct paths, can trigger DCUtR hole punching, and falls back to relay within timeout.

## P2P debug endpoints

When `--p2p-debug-http` (or `KM_P2P_DEBUG_HTTP`) is set on `buyer serve` or `seller serve`:

| Method | Path | Returns |
|---|---|---|
| `GET` | `/debug/p2p/peer/<peerID>` | Peer connection details, computed connection type, tagged type, last hole punch result |
| `GET` | `/debug/p2p/reachability` | Latest local AutoNAT reachability — `unknown` / `public` / `private` |

CLI helper:

```bash
go run ./cmd/buyer p2p-debug-peer <PEER_ID> --http http://127.0.0.1:9091
```

## NAT, relay, and bootstrap

- **Default public relays** apply to buyer and seller **only when `--control-url` is omitted** (the implicit default). If you pass `--control-url` explicitly, only env/CLI relays are used. Seller `--server-mode` also skips defaults.
- `--relay` is repeatable; merged with `LIBP2P_STATIC_RELAYS`. Each must include `/p2p/<relayID>`.
- `--bootstrap` is repeatable; use when the seller's `sellerListenAddrs` from the control DB are not reachable from the buyer (NAT or stale list). The seller logs a full `dial this bootstrap: ...` line at startup — copy from there.
- For sellers behind NAT with no other libp2p peers, AutoNAT v2 may not emit reachability updates quickly. Enable Kademlia DHT (`--p2p-dht` / `KM_P2P_DHT=1`) and pass at least one reachable bootstrap peer.
- After relay addresses appear in `host.Addrs()`, **repost seller presence** (`POST /v1/control/sellers/me/presence`) if buyers still cannot dial.

## Manual migrations CLI

The `control api` process applies migrations on startup. You only need the manual CLI for inspection, rollback, or ops scenarios.

```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
# ensure $(go env GOPATH)/bin is on PATH

export DATABASE_URL='postgres://user:pass@127.0.0.1:5432/knowledgemesh?sslmode=disable'
migrate -path internal/control/migrations -database "$DATABASE_URL" up
migrate -path internal/control/migrations -database "$DATABASE_URL" version
migrate -path internal/control/migrations -database "$DATABASE_URL" down 1
```

`migrate force <VERSION>` sets `schema_migrations` without running SQL — use **only** when the DB and files are known to be in sync (e.g. after restoring a backup).

## OllamaSellerConfig schema

Set via `PUT /v1/control/sellers/me/ollama` with seller JWT. JSON shape:

```json
{
  "baseURL": "http://127.0.0.1:11434",
  "models": [
    { "id": "my-chat", "name": "llama3:latest" }
  ]
}
```

- `id` is the marketplace model name as declared in `seller setup --model-id` / `PUT .../models`.
- `name` is the local Ollama tag (`ollama list`).
- Empty `baseURL` defaults to `http://127.0.0.1:11434`.
- Body of `null` clears the config.
- **Restart `seller serve`** after any change — the seller process reads the Ollama profile at startup only.

## Seller backend selection order

`internal/seller/engine.go::ModelEngineFromSellerNode` picks a backend in this order:

1. **OpenAI** — if an OpenAI block is present on the seller node (custom wiring; not provisioned by control).
2. **Anthropic** — if an Anthropic block is present (custom wiring).
3. **Ollama** — loaded from Postgres via the control API after `PUT .../ollama`. This is the path used by `seller setup --ollama-...`.
4. **Mock** — fallback for tests/demos (`MockExecutor`).

`seller serve` uses a `PassthroughExecutor` in the sandbox (`internal/sandbox`) so the buyer's text reaches the backend verbatim. Tests and the `knowledgeMesh serve` mock path use `MockExecutor`.

## Module overview

| Package | Role |
|---|---|
| `internal/control` | Postgres store (buyers, sellers, models, billing, inference matches), HTTP API, JWT, embedded golang-migrate, libp2p control protocol handler |
| `internal/mesh` | Buyer-side runtime: control client (match/track/complete) + libp2p inference dial |
| `internal/buyer` | Buyer session state after control login; CLI commands |
| `internal/seller` | OpenAI / Anthropic / Ollama facades; `serve` loads profile from control and posts inference tracking |
| `internal/matchmaker` | Seller selection by skill, duty, price cap, then price/reputation. **Used inside the control pane** for `/buyers/me/inference/match`. |
| `internal/sandbox` | Request-scoped runner (`PassthroughExecutor` in serve, `MockExecutor` in tests/mocks); also backs `knowledgeMesh serve` |
| `internal/api` | OpenAI / Anthropic style HTTP handlers |
| `internal/network` | libp2p host (QUIC + TCP, Noise, Yamux, NAT, AutoNAT v2, AutoRelay, optional Kademlia DHT, DCUtR hole punching, connmgr, ping); `HolePunchManager`, `ConnectionTypeTracker`, size-aware `MessageRouter`, `NetworkMonitor`, relay→direct upgrade pruning, optional Prometheus metrics, P2P debug |
| `internal/relay` | Stateless circuit relay v2 service with reservation/circuit/bandwidth limits |
| `pkg/types` | Shared types — `InferenceRequest`, `InferenceResponse`, `OllamaSellerConfig`, etc. |
| `pkg/protocol` | Protocol IDs and message envelopes |
| `pkg/config` | Shared config primitives |

## Running as systemd services (Ubuntu)

Recipe for the control API and relay (the two binaries that benefit most from being persistent services).

```bash
# 1. Service account (skip if you already have one)
sudo useradd --system --home /var/lib/knowledgemesh --create-home --shell /usr/sbin/nologin knowledgemesh

# 2. Build binaries
go build -o /usr/local/bin/knowledgemesh-control ./cmd/control
go build -o /usr/local/bin/knowledgemesh-relay   ./cmd/relay

# 3. Control env file
sudo mkdir -p /etc/knowledgemesh
sudo tee /etc/knowledgemesh/control.env >/dev/null <<'EOF'
DATABASE_URL=postgres://user:pass@127.0.0.1:5432/knowledgemesh?sslmode=disable
CONTROL_JWT_SECRET=replace-with-a-long-random-secret
EOF
sudo chmod 600 /etc/knowledgemesh/control.env

# 4. Relay state dir (for stable peer ID)
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

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now knowledgemesh-control.service
sudo systemctl enable --now knowledgemesh-relay.service
sudo journalctl -u knowledgemesh-control.service -f
```

## Docker (relay only — for now)

A `Dockerfile.relay` is checked in. There are no Dockerfiles for `control`, `buyer`, or `seller` yet.

```bash
docker build -f Dockerfile.relay -t knowledgemesh-relay .
docker run --rm -p 4001:4001/tcp -p 4001:4001/udp knowledgemesh-relay
```

For a **stable relay peer ID** across container restarts, mount a volume for the identity file:

```bash
docker run --rm -p 4001:4001/tcp -p 4001:4001/udp \
  -v relay-id:/data \
  -e RELAY_IDENTITY_FILE=/data/relay-identity.key \
  knowledgemesh-relay
```

---

## Known gotchas for agents

1. **Module path mismatch.** `go.mod` declares `github.com/knowledgemeshgrid/knowledgemesh`; the repo is at `github.com/Knowledge-Mesh/knowledgeMesh`. Neither path is `go install`-able — clone and `go build`.
2. **Default `--control-url`.** When omitted, defaults to `http://127.0.0.1:8090` and prints a warning. Pass it explicitly to silence the warning and to make logs unambiguous about which control plane is in use.
3. **Default public relays only when `--control-url` is omitted.** Passing `--control-url` disables the implicit defaults — supply `--relay` or `LIBP2P_STATIC_RELAYS` explicitly if you need them.
4. **`seller serve` reads Ollama config once, at startup.** After `PUT .../ollama`, restart `seller serve`.
5. **Settlement is idempotent per `requestId`.** Retrying a `complete` call with the same `requestId` is safe; it does not double-charge.
6. **`schema_migrations` is auto-managed.** Don't edit by hand. Use `migrate force <VERSION>` only after restoring from backup, when you know the DB and files are in sync.
7. **`old_code_archive` branch on `origin` is not canonical.** Ignore it.
8. **`cmd/demo run` is a placeholder.** It is listed in the CLI overview but doesn't do anything useful yet.
9. **Empty `docs/` on `main`.** All current docs live in this file, [README.md](./README.md), and [ARCHITECTURE.md](./ARCHITECTURE.md). The `docs/` directory exists for future use.
10. **No CI / no Makefile.** Canonical build / test commands live here and in [CONTRIBUTING.md](./CONTRIBUTING.md): `go build ./...` and `go test ./...`. There is no `make build` / `make test`.
