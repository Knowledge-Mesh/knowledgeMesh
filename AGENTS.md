# Agents — start here

This file is the entry point for autonomous coding agents (Claude Code, Cursor, Aider, Codex, etc.) bringing up `knowledgeMesh` on a user's machine. **Read this before the README.** The README is human-skim documentation; this file is the executable runbook.

If you are a human, this file is also a useful index — but you may prefer the [README](./README.md) and [ARCHITECTURE.md](./ARCHITECTURE.md).

---

## What you must do first

Before running any commands, ask the user **exactly one question** and wait for their answer. Use this prompt verbatim (you may reformat it for your UI):

> I see this is **knowledgeMesh** — a marketplace-style mesh for AI inference (buyer ↔ control pane ↔ seller, over a libp2p QUIC overlay). I can set this up three ways. Pick one:
>
> **(1) Connect as a buyer with defaults — hosted public mesh** — fastest path to *real* inference. Registers a buyer account on the hosted control pane at `http://control.p2pinfer.cloud:8090` and runs a prompt against whichever seller is currently on duty there. Needs only outbound internet. ~2 min.
>
> **(2) Connect as a seller with defaults — hosted public mesh** — registers a seller account on `http://control.p2pinfer.cloud:8090` and exposes your local Ollama model to public traffic. Requires Ollama running locally. ~5 min.
>
> **(3) Help me set up my own mesh** — full local stack: PostgreSQL → control API → relay (optional) → seller → buyer, with an end-to-end smoke test. No dependence on the public mesh. ~15–20 min.

After the user answers `1`, `2`, or `3`, jump to the matching path below. Do not proceed without their answer. If they describe a different scenario (e.g. "I just want to read the code"), default to summarising the architecture from [ARCHITECTURE.md](./ARCHITECTURE.md) and ask again.

## Decision matrix

| | (1) Buyer → hosted mesh | (2) Seller → hosted mesh | (3) Own mesh |
|---|---|---|---|
| Wall-clock | ~2 min | ~5 min | ~15–20 min |
| External deps | outbound internet | Ollama + outbound internet | Postgres + Ollama (+ optional Docker) |
| Control plane | `http://control.p2pinfer.cloud:8090` (shared, public) | `http://control.p2pinfer.cloud:8090` (shared, public) | `http://127.0.0.1:8090` (local, private) |
| Long-running processes | 1 (`buyer serve`) | 1 (`seller serve`) | 3–4 |
| What this verifies | end-to-end real inference against a public seller | seller registration + Ollama wiring + public presence | full local match → inference → settlement loop |
| Real LLM output? | yes (whatever the on-duty public seller is serving) | yes (your own Ollama answering public traffic) | yes (your own Ollama answering your own buyer) |
| Touches Postgres? | no (uses hosted DB) | no (uses hosted DB) | yes — local DB, migrations auto-apply |
| Privacy | your prompts traverse a public control pane | your model serves arbitrary public buyers | fully local |

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

## Path 1 — Buyer with defaults (hosted public mesh)

**Goal:** working buyer + a real LLM completion against a public seller in ~2 minutes, with no local Postgres, Ollama, or seller process.

The buyer registers on the hosted control plane at `http://control.p2pinfer.cloud:8090` (the value baked into [`internal/control/defaults.go`](./internal/control/defaults.go) — used whenever `--control-url` is omitted), gets matched to whichever seller is on duty in the public mesh, and runs a real prompt over libp2p QUIC.

**Questions to ask the user (collect all before running):**

| # | Question | Default |
|---|---|---|
| 1 | Buyer email | `buyer-<8-char-random>@example.com` (avoid collisions on the shared DB) |
| 2 | Buyer password | generate a 24-char random password and surface it back to the user — they will need it to re-`serve` later |
| 3 | Buyer display name | `Demo Buyer` |

**Confirm the public control plane is reachable before running anything:**

```bash
curl -s http://control.p2pinfer.cloud:8090/healthz
# expect: {"module":"control","status":"ok"}
```

If that curl fails, fall back to the **offline alternative** below.

**Run (substitute the values you collected):**

```bash
# 1. Register on the public control pane.
go run ./cmd/buyer register \
  --name "<NAME>" \
  --email <EMAIL> \
  --password '<PASSWORD>'
# --control-url is omitted intentionally; code applies the public default
# and prints "warning: no --control-url specified; using default ..." — that warning is expected.

# 2. Start the buyer mesh process (foreground or background).
go run ./cmd/buyer serve \
  --email <EMAIL> \
  --password '<PASSWORD>'
# Logs include a session token line like "session: <opaque>" — capture it; the buyer HTTP API requires it.

# 3. In another terminal, send a real prompt.
go run ./cmd/buyer prompt \
  --email <EMAIL> \
  --password '<PASSWORD>' \
  --api-url http://127.0.0.1:8080 \
  --prompt 'Write a short haiku about distributed systems.'
```

**Success criteria:**
- `register` returns 200 with a `buyerId`.
- `serve` brings up a libp2p host, logs the session token, and stays running.
- `prompt` returns a non-empty completion. The text is whatever the on-duty public seller's model produces (real LLM output, not mock).

**Failure modes:**
| Symptom | Cause / fix |
|---|---|
| `connection refused` / DNS error to `control.p2pinfer.cloud` | network/firewall blocks outbound HTTP to port 8090 — fall back to the offline alternative below |
| `email already registered` | another agent or run used this email — pick a fresh one or skip `register` and just `serve` + `prompt` if you have the password |
| `no eligible sellers` from `/inference/match` | no seller is on duty in the public mesh right now — the user can run **Path 2** to put their own seller up, or use **Path 3** for a fully local mesh, or simply retry later |
| `dial timeout` on libp2p / inference stream | NAT or relay issue dialing the matched seller's listen addresses — see [Advanced § NAT, relay, and bootstrap](#nat-relay-and-bootstrap); often resolved by passing `--relay` or by retrying |
| HTTP 402 from `/inference/match` | wallet / quota check failed on the public DB; surface the message to the user |

**Privacy note to surface to the user before running:** the prompt text and any model metadata pass through the hosted control pane and a third-party seller. Do not send sensitive data on Path 1.

### Offline alternative — mock sandbox (no internet, no real inference)

If the public control plane is unreachable (offline, firewall, outage) and the user just wants to exercise the buyer HTTP API surface, run the mock sandbox instead — zero deps, fake completions, never touches the public mesh:

```bash
go run ./cmd/knowledgeMesh serve
# in another terminal:
curl -s http://127.0.0.1:8080/healthz
curl -s -X POST http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"kmg-mock-1","messages":[{"role":"user","content":"hello"}]}'
```

This exercises `internal/api` + `internal/sandbox` only — no control pane, no real LLM. Useful for offline dev and for verifying the OpenAI/Anthropic-shaped API contract.

---

## Path 2 — Seller with defaults (hosted public mesh)

**Goal:** register a seller on the hosted control plane at `http://control.p2pinfer.cloud:8090`, expose your local Ollama model to the public mesh, and start serving real buyer traffic.

**Questions to ask the user (collect all before running):**

| # | Question | Default |
|---|---|---|
| 1 | Control API URL | omit (uses hosted public default `http://control.p2pinfer.cloud:8090`); pass `--control-url http://127.0.0.1:8090` if you want a local control pane instead — see Path 3 |
| 2 | Seller email | `seller-<8-char-random>@example.com` |
| 3 | Seller password | generate a 24-char random password and surface it back to the user |
| 4 | Seller display name | `Demo Seller` |
| 5 | Catalog model id | `my-chat` |
| 6 | Ollama tag to expose | `llama3:latest` |
| 7 | Ollama base URL | `http://127.0.0.1:11434` |
| 8 | Rate per token | `0.000001` |

**Privacy / safety to surface to the user before running:** registering a seller on the public mesh means **arbitrary public buyers can route prompts to your local Ollama instance** as long as you are on duty. Stop the seller (or `seller duty off`) when you are done.

**Confirm the public control plane is reachable:**

```bash
curl -s http://control.p2pinfer.cloud:8090/healthz
# expect: {"module":"control","status":"ok"}
```

**Confirm Ollama is running and the model is pulled:**

```bash
curl -s http://127.0.0.1:11434/api/tags
ollama pull llama3    # only if the tag isn't already present
```

**Run (substitute the values you collected; omit `--control-url` to use the hosted public default):**

```bash
go run ./cmd/seller register \
  --name "<NAME>" \
  --email <EMAIL> \
  --password '<PASSWORD>'

go run ./cmd/seller setup \
  --email <EMAIL> \
  --password '<PASSWORD>' \
  --model-id <MODEL_ID> \
  --rate-per-token <RATE> \
  --ollama-base-url <OLLAMA_URL> \
  --ollama-map <MODEL_ID>=<OLLAMA_TAG>

go run ./cmd/seller status \
  --email <EMAIL> \
  --password '<PASSWORD>'

go run ./cmd/seller serve \
  --email <EMAIL> \
  --password '<PASSWORD>'
```

> Each command will log `warning: no --control-url specified; using default http://control.p2pinfer.cloud:8090`. That warning is expected and confirms you are on the public mesh. Pass `--control-url http://127.0.0.1:8090` (or whatever local URL) on every command to switch to a private control plane instead.

**Success criteria:**
- `seller status` output includes `onDuty: true` and at least one model.
- `seller serve` log emits a line beginning `dial this bootstrap: /ip4/...` — capture this; buyers may need it as `--bootstrap`.

**Failure modes:**
| Symptom | Fix |
|---|---|
| `connection refused` / DNS error to `control.p2pinfer.cloud` | network / firewall blocks outbound HTTP — switch to Path 3 with a local control pane |
| `ollama: connection refused` | start Ollama (`ollama serve`) |
| `model "<tag>" not found` | `ollama pull <tag>` |
| `email already registered` | skip `register`; proceed to `setup` (requires the original password) |
| `seller serve` exits with `failed to login` | re-check email/password; if Ollama config changed since the last serve, restart `serve` (it reads the profile only at startup) |

**Toggle duty without restarting serve:**

```bash
go run ./cmd/seller duty off --email <EMAIL> --password '<PASSWORD>'
go run ./cmd/seller duty on  --email <EMAIL> --password '<PASSWORD>'
```

**Stop serving the public mesh:** `pkill -f 'go run ./cmd/seller'` or `seller duty off`. While the seller is on duty, public buyers can route real prompts to your Ollama instance.

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
2. **Default `--control-url` is the hosted public mesh.** When omitted, every buyer/seller CLI command resolves to `http://control.p2pinfer.cloud:8090` (see [`internal/control/defaults.go`](./internal/control/defaults.go)) and logs `warning: no --control-url specified; using default ...`. The warning is expected — it tells the user where their request is going. Pass `--control-url` explicitly to switch to a private control pane (e.g. `--control-url http://127.0.0.1:8090` for Path 3).
3. **The README's `--control-url` default is documented incorrectly.** Both `README.md` (on `main` and on `fix/docs`) claim the default is `http://127.0.0.1:8090`. The actual default is the hosted public mesh URL above. Trust the code, not that line.
4. **Default public relays only when `--control-url` is omitted.** Passing `--control-url` disables the implicit defaults — supply `--relay` or `LIBP2P_STATIC_RELAYS` explicitly if you need them.
5. **`seller serve` reads Ollama config once, at startup.** After `PUT .../ollama`, restart `seller serve`.
6. **Settlement is idempotent per `requestId`.** Retrying a `complete` call with the same `requestId` is safe; it does not double-charge.
7. **`schema_migrations` is auto-managed.** Don't edit by hand. Use `migrate force <VERSION>` only after restoring from backup, when you know the DB and files are in sync.
8. **`old_code_archive` branch on `origin` is not canonical.** Ignore it.
9. **`cmd/demo run` is a placeholder.** It is listed in the CLI overview but doesn't do anything useful yet.
10. **Empty `docs/` on `main`.** All current docs live in this file, [README.md](./README.md), and [ARCHITECTURE.md](./ARCHITECTURE.md). The `docs/` directory exists for future use.
11. **No CI / no Makefile.** Canonical build / test commands live here and in [CONTRIBUTING.md](./CONTRIBUTING.md): `go build ./...` and `go test ./...`. There is no `make build` / `make test`.
