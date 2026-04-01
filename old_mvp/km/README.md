# KnowledgeMesh

**Peer-to-peer AI inference marketplace.** Share AI compute, earn credits, access any model at a fraction of retail cost.

```
Buyer                              Seller (your GPU / API key / subscription)
  |                                  |
  |  "Give me gpt-4o, cheap"        |  "I have a spare OpenAI key"
  |                                  |
  +-------->  KM Broker  <----------+
              (matchmaker)
              Routes to cheapest
              node offering gpt-4o
```

## Why

- You pay **$20/mo for Claude** but use 10% of it. Sell the rest.
- You have a **beefy GPU** running Llama 3. Earn money from it.
- You want **GPT-4o** but don't want to pay $10/M tokens. Get it for $2/M.
- You need **private inference** — data never leaves your network.

## Quick Start

### Sell: Share your compute (60 seconds)

**Option A — Install from release:**
```bash
curl -fsSL https://raw.githubusercontent.com/ArchieIndian/km/main/install.sh | sh
```

**Option B — Build from source:**
```bash
git clone https://github.com/ArchieIndian/km.git && cd km
cargo build --release
# Binary is at ./target/release/km-worker
# Optional: copy to PATH
sudo cp target/release/km-worker /usr/local/bin/
```

Then run:
```bash
# First time — self-register with one command:
KM_INVITE_CODE=km-xxx KM_NODE_NAME=my-node KM_EMAIL=me@test.com \
  KM_TIER=ollama ./target/release/km-worker

# Subsequent runs — secret is saved to ~/.km/configs/, just supply tier + creds:
KM_NODE_NAME=my-node KM_TIER=ollama ./target/release/km-worker

# If installed via install.sh (binary is in /usr/local/bin/):
KM_INVITE_CODE=km-xxx KM_NODE_NAME=my-node KM_EMAIL=me@test.com KM_TIER=ollama km-worker
# ...and subsequent runs:
KM_NODE_NAME=my-node KM_TIER=ollama km-worker
```

Set the credential env var for your tier:
- `api` → `ANTHROPIC_API_KEY=sk-ant-...`
- `openai` → `OPENAI_API_KEY=sk-...`
- `subscription` → `KM_SESSION_KEY=...` (session cookie from claude.ai)
- `ollama` → no key needed, just have `ollama serve` running

> **Note:** `cargo build --release` puts the binary at `./target/release/km-worker`, not `./km-worker`. The install script copies it to `/usr/local/bin/km-worker`.

> **Alternative:** Register through the [dashboard](https://km-broker.onrender.com) and use `KM_NODE_SECRET` directly.

### Buy: Access any model (3 lines)

```bash
curl -X POST https://km-broker.onrender.com/v1/chat/completions \
  -H "Authorization: Bearer km-sec-xxxxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Hello!"}]}'
```

Or with Python:
```python
from knowledgemesh import KM
km = KM(secret="km-sec-xxxxx")
result = km.chat("What is 2+2?", model="gpt-4o")
print(result["content"])  # "4"
print(result["savings"])  # "96.7%"
```

Or use it as an **OpenAI drop-in replacement**:
```python
from openai import OpenAI
client = OpenAI(base_url="https://km-broker.onrender.com/v1", api_key="km-sec-xxxxx")
resp = client.chat.completions.create(model="gpt-4o", messages=[{"role":"user","content":"Hi"}])
```

## How It Works

```
                         KnowledgeMesh
 ┌─────────┐      ┌──────────────────┐      ┌──────────────┐
 │  Buyer   │─────>│     Broker       │─────>│  Worker A    │
 │ (curl/   │      │  - Match buyer   │      │  Anthropic   │
 │  Python/ │      │    to cheapest   │      │  API key     │
 │  OpenAI  │      │    seller        │      │  claude-*    │
 │  SDK)    │      │  - Per-model     │      │  $3/M        │
 └─────────┘      │    pricing       │      └──────────────┘
                   │  - Escrow +      │
                   │    settlement    │      ┌──────────────┐
                   │  - Model         │─────>│  Worker B    │
                   │    verification  │      │  OpenAI key  │
                   │  - Rate limiting │      │  gpt-4o      │
                   └──────────────────┘      │  $2/M        │
                                             └──────────────┘
                          Cloudflare
                          Tunnels           ┌──────────────┐
                          (auto-managed)───>│  Worker C    │
                                            │  Ollama      │
                                            │  llama3.2    │
                                            │  $0.10/M     │
                                            └──────────────┘
```

1. **Sellers** run `km-worker` with their AI credentials. Workers auto-tunnel via Cloudflare.
2. **Broker** receives task requests, picks the cheapest node offering the requested model.
3. **Escrow** locks buyer credits before execution, settles actual cost after.
4. **Model verification** checks the response model matches what the seller advertised.
5. **Buyers** use curl, Python SDK, or any OpenAI-compatible client.

## Supply Tiers

> **Tier names refer to the backend, not the brand.** `api` = Anthropic API, `openai` = OpenAI API.

| Tier | Backend | Credential Env Var | Models | Token Accuracy |
|------|---------|-------------------|--------|----------------|
| `api` | **Anthropic API** | `ANTHROPIC_API_KEY` | claude-sonnet-4, claude-haiku-4 | Exact |
| `openai` | **OpenAI API** | `OPENAI_API_KEY` | gpt-4o, gpt-4o-mini | Exact |
| `subscription` | **Claude Pro/MAX session** | `KM_SESSION_KEY` | claude-sonnet | Estimated |
| `ollama` | **Local Ollama** | *(none — just run `ollama serve`)* | Any installed model | Exact |
| `buyer` | *(buy only)* | *(none)* | — | — |

## Per-Model Pricing

Each seller sets prices per model. Buyers pick the model, broker finds the cheapest:

```bash
# See what's available
curl https://km-broker.onrender.com/models

# Request a specific model
curl -X POST .../task -d '{"model":"gpt-4o-mini", "buyer":"me", "buyer_secret":"km-sec-xxx", "messages":[...]}'

# Or cheapest anything
curl -X POST .../task -d '{"buyer":"me", "buyer_secret":"km-sec-xxx", "messages":[...]}'
```

## API Reference

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | - | Broker health check |
| `/status` | GET | - | Network status: nodes, balances, stats |
| `/models` | GET | - | Available models with best prices |
| `/register-user` | POST | Invite code | Register new user |
| `/task` | POST | `buyer_secret` | Submit inference task |
| `/v1/chat/completions` | POST | Bearer token | OpenAI-compatible endpoint |
| `/whoami` | GET | Bearer token | Your identity, balance, tier |
| `/recover` | POST | name + email | Initiate account recovery |
| `/reset-secret` | POST | Reset token | Get new node secret |
| `/admin/reset-tokens` | GET | `KM_ADMIN_SECRET` | List pending reset tokens (account recovery) |
| `/update-price` | POST | `node_secret` | Change your pricing |
| `/update-tier` | POST | `node_secret` | Change your tier |
| `/update-limits` | POST | `node_secret` | Set token budget, concurrency |
| `/deregister` | POST | `node_secret` | Go offline (graceful) |
| `/node-config` | GET | Bearer token | Worker bootstrap config |
| `/register` | POST | `node_secret` | Worker self-registration |
| `/heartbeat` | POST | `node_id` | Worker liveness (every 10s) |

## Architecture

```
km/
├── fabric/                  Go broker (matchmaker + ledger)
│   ├── main.go              HTTP server + routes
│   ├── handlers.go          Shared handler utilities
│   ├── handlers_auth.go     Registration, recovery, whoami
│   ├── handlers_task.go     Task + OpenAI-compatible endpoints
│   ├── handlers_admin.go    Admin endpoints (invites, reset)
│   ├── handlers_node.go     Node registration, heartbeat, config
│   ├── registry.go          Node registry + heartbeat reaper
│   ├── matcher.go           Model-aware routing (cheapest first)
│   ├── escrow.go            Credit lock / settle / refund
│   ├── ledger.go            Append-only JSONL ledger
│   ├── capacity.go          Token budgets + concurrency tracking
│   ├── validation.go        Request validation helpers
│   ├── email.go             Recovery email sending (Resend)
│   ├── state.go             Persistent state (secrets, configs, emails)
│   ├── github_sync.go       State backup to GitHub
│   ├── forwarder.go         HTTP client for worker routing
│   └── web/index.html       Dashboard
├── worker/                  Rust worker (inference proxy)
│   └── src/
│       ├── main.rs          HTTP server + bridge creation
│       ├── config.rs         Broker bootstrap config
│       ├── setup.rs          Self-registration wizard
│       ├── models.rs         Model discovery + pricing
│       ├── handlers.rs       HTTP request handlers
│       ├── bridge/
│       │   ├── anthropic.rs  Anthropic API
│       │   ├── openai.rs     OpenAI API
│       │   ├── ollama.rs     Ollama (local models)
│       │   ├── session.rs    Claude.ai session
│       │   └── subscription.rs  Legacy subscription bridge
│       ├── fabric.rs         Broker registration + heartbeat
│       └── tunnel.rs         Cloudflare Tunnel auto-management
├── cli/                     CLI tool (`mesh status`, `mesh task`)
├── sdk/python/              Python SDK
├── install.sh               One-line installer
└── LICENSE                  MIT
```

## Web Search

Some models support web search natively through KnowledgeMesh:

- **OpenAI:** Request model `gpt-4o-search-preview` — web search is automatic.
- **Anthropic:** Add `"web_search_options":{}` to your request body alongside `messages`.

```bash
# OpenAI web search
curl -X POST https://km-broker.onrender.com/v1/chat/completions \
  -H "Authorization: Bearer km-sec-xxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-search-preview","messages":[{"role":"user","content":"Latest news on AI"}]}'

# Anthropic web search
curl -X POST https://km-broker.onrender.com/v1/chat/completions \
  -H "Authorization: Bearer km-sec-xxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet","messages":[{"role":"user","content":"Latest news on AI"}],"web_search_options":{}}'
```

## Security

- **Credentials never leave the worker.** API keys and session cookies stay on the seller's machine. The broker never sees them.
- **Buyer authentication** via `buyer_secret` on every request.
- **Email recovery** — hashed with HMAC-SHA256, never stored in plaintext.
- **Rate limiting** — per-IP on all endpoints.
- **Max payload** — 100KB limit.
- **Model verification** — broker checks response model matches advertised model.
- **Graceful shutdown** — Ctrl+C deregisters from broker immediately.

## Environment Variables

### Worker

**Identity & Registration**

| Variable | Default | Description |
|----------|---------|-------------|
| `KM_NODE_NAME` | **required** | Your node name |
| `KM_NODE_SECRET` | auto (from `~/.km/configs/`) | Node secret (from dashboard or self-registration) |
| `KM_INVITE_CODE` | — | Invite code for self-registration (first run only) |
| `KM_EMAIL` | — | Email for account recovery (first run only) |

**Tier & Credentials**

| Variable | Default | Description |
|----------|---------|-------------|
| `KM_TIER` | from broker | `api` (Anthropic), `openai` (OpenAI), `subscription` (Claude Pro/MAX), `ollama` (local) |
| `ANTHROPIC_API_KEY` | — | Required for `api` tier |
| `OPENAI_API_KEY` | — | Required for `openai` tier |
| `KM_SESSION_KEY` | — | Required for `subscription` tier (claude.ai session cookie) |
| `OLLAMA_URL` | `http://localhost:11434` | Ollama server URL (`ollama` tier) |
| `OLLAMA_MODEL` | `llama3.2` | Default Ollama model (`ollama` tier) |
| `KM_CLAUDE_MODELS` | — | Comma-separated list of Claude models to advertise |

**Pricing & Limits**

| Variable | Default | Description |
|----------|---------|-------------|
| `KM_PRICE` | from broker | Price per million tokens (e.g. `1.00`) |
| `KM_TOKEN_BUDGET` | unlimited | Max tokens to serve per rolling window |
| `KM_BUDGET_WINDOW_HOURS` | unlimited (`0`) | Rolling window duration in hours |
| `KM_MAX_CONCURRENT` | 1 (subscription), 5 (api), 3 (local) | Max concurrent requests the worker accepts. The broker also enforces its own per-node concurrency limit (set during registration); this var controls the worker side only. |

**Network**

| Variable | Default | Description |
|----------|---------|-------------|
| `KM_BROKER_URL` | `https://km-broker.onrender.com` | Broker URL |
| `KM_PORT` | auto (8000–8010) | Worker HTTP port |
| `KM_BIND` | `127.0.0.1` | Bind address |

### Broker
| Variable | Default | Description |
|----------|---------|-------------|
| `KM_ADMIN_SECRET` | **required** | Admin auth for invite generation and reset |
| `KM_EMAIL_HMAC_KEY` | insecure default | HMAC key for email hashing. Set this in production (default is insecure) |
| `KM_LEDGER_PATH` | `ledger.txt` | Path to ledger file |
| `KM_GITHUB_TOKEN` | - | GitHub PAT for state persistence |
| `KM_GITHUB_REPO` | - | GitHub repo for state sync (e.g. `user/km-state`) |
| `KM_SYNC_ENCRYPTION_KEY` | - | AES-256 key for encrypting synced state |
| `RESEND_API_KEY` | - | Resend.com API key for recovery emails |
| `RESEND_FROM_EMAIL` | `noreply@knowledgemesh.ai` | From address for emails |
| `PORT` | `9000` | HTTP port |

## License

MIT
