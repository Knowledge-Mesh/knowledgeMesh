# knowledgeMesh architecture

This document describes how the main components fit together and how requests flow through the system. For a **command-by-command CLI table** and copy-paste examples, see [README.md](./README.md) (sections *CLI reference* and *Examples and quick binary check*).

## Visual architecture (diagrams)

### System context (who talks to what)

Big-picture: operators and apps use **HTTPS** to the control API and buyer API; buyer and seller nodes also use **QUIC/libp2p** for inference traffic.

```mermaid
flowchart TB
  subgraph actors["People / apps"]
    OP[Operators / CLIs]
    APP[Client apps curl SDKs]
  end

  subgraph http["HTTPS"]
    CTRL[Control API :8090]
    BUY[Buyer mesh API :8080]
  end

  subgraph data["Data"]
    DB[(PostgreSQL)]
  end

  subgraph p2p["QUIC / libp2p overlay"]
    BH[Buyer libp2p host]
    SH[Seller libp2p host]
  end

  OP --> CTRL
  OP --> BUY
  APP --> BUY
  CTRL --- DB
  BH <-.->|bootstrap + inference| SH
  BUY --- BH
```

### Runtime processes (one machine view)

Typical dev setup: three long-running processes. **Registration** and **billing** use HTTP to the control API; **prompt execution** uses P2P between mesh and seller.

```mermaid
flowchart LR
  subgraph P1["Process: control api"]
    A1[HTTP server]
    A2[(Postgres)]
    A1 --- A2
  end

  subgraph P2["Process: mesh serve / buyer start"]
    B1[OpenAI-style API]
    B2[Mesh runtime]
    B3[libp2p QUIC]
    B1 --> B2 --> B3
  end

  subgraph P3["Process: seller serve"]
    C1[Inference service]
    C2[libp2p QUIC]
    C1 --- C2
  end

  B2 -->|HTTPS JWT| A1
  C1 -->|HTTPS JWT| A1
  B3 <-->|inference stream| C2
```

### Code layers (`cmd` → `internal` → `pkg`)

```mermaid
flowchart TB
  subgraph cmd["cmd/ binaries"]
    c1[knowledgeMesh]
    c2[buyer]
    c3[seller]
    c4[control]
  end

  subgraph internal["internal/"]
    i1[control — API store JWT billing]
    i2[mesh — runtime + control client]
    i3[api — HTTP handlers]
    i4[seller — inference sandbox engines]
    i5[buyer — session state]
    i6[network — libp2p QUIC]
    i7[matchmaker — used inside control]
  end

  subgraph pkg["pkg/"]
    p1[types]
  end

  c4 --> i1
  c1 --> i2
  c2 --> i2
  c3 --> i4
  i2 --> i3
  i2 --> i6
  i2 --> i5
  i2 --> i1
  i4 --> i6
  i4 --> i1
  i1 --> i7
  i3 --> p1
  i4 --> p1
  i2 --> p1
```

### Billing and match (control plane)

```mermaid
flowchart LR
  subgraph match["Match path"]
    M1[InferenceRequest meta]
    M2[Load sellers from PG]
    M3[matchmaker.Match]
    M4[Insert inference_matches]
    M1 --> M2 --> M3 --> M4
  end

  subgraph bill["Settlement path"]
    S1[POST .../complete]
    S2[Debit buyer wallet]
    S3[Credit seller wallet]
    S4[billing_transactions]
    S1 --> S2 --> S3 --> S4
  end

  match --> bill
```

---

## High-level view

```mermaid
flowchart LR
  subgraph clients["CLIs / processes"]
    KM[knowledgeMesh]
    BuyerCLI[buyer]
    SellerCLI[seller]
    CtlBin[control]
  end

  subgraph control["Control pane HTTP API"]
    API[HTTPServer]
    PG[(PostgreSQL)]
    API --> PG
  end

  subgraph mesh["Buyer mesh process"]
    BuyerAPI[Buyer HTTP API]
    P2PB[libp2p host]
    BuyerAPI --> P2PB
  end

  subgraph sellerProc["Seller process"]
    P2PS[libp2p host]
    INF[Inference handler]
    P2PS --> INF
  end

  KM --> BuyerAPI
  BuyerCLI --> BuyerAPI
  SellerCLI --> API
  CtlBin --> API
  BuyerAPI -->|"JWT: match / tracking / complete"| API
  INF -->|"JWT: tracking"| API
  P2PB <-->|"QUIC / inference protocol"| P2PS
```

- **Control pane** (`control api`) is the source of truth for accounts (buyers and sellers), seller models and duty, **billing** (wallets, quotas, transaction ledger), and **matchmaking** inputs (on-duty sellers with presence loaded from PostgreSQL).
- **Buyer mesh** (`knowledgeMesh mesh serve` or `buyer start`) exposes OpenAI/Anthropic-style HTTP APIs, holds the buyer’s libp2p host, and calls the control API for **match → track → settle** around each inference.
- **Seller** (`seller serve`) exposes inference over libp2p and reports execution metadata back to the control pane.
- **`knowledgeMesh serve`** (no mesh) runs the same HTTP API with **mock inference** only (no `MeshRuntime`); useful for local UI tests without PostgreSQL.

## Components

| Area | Package / binary | Role |
|------|------------------|------|
| Control HTTP | `cmd/control`, `internal/control` | REST API, JWT issuance, Postgres persistence, billing, inference orchestration metadata |
| Matchmaking | `internal/matchmaker` | Selects a seller from a candidate list (skill, duty, price cap; then lowest price, then reputation). Invoked **inside** the control pane for `/buyers/me/inference/match`. |
| Buyer mesh | `internal/mesh`, `internal/api` | Session from control login; calls control for match and billing completion; runs libp2p inference streams |
| Seller runtime | `internal/seller` | Sandbox + model engines; inference over `/knowledgemesh/inference/1.0.0`; optional control tracking callbacks |
| Network | `internal/network` | QUIC libp2p host, bootstrap connect, request/response streams |

### `cmd/` binaries (entrypoints)

| Binary | Main commands | Maps to |
|--------|----------------|--------|
| `knowledgeMesh` | `serve`, `mesh serve` | `cmd/knowledgeMesh` → `internal/sandbox`, `internal/mesh` |
| `buyer` | `register`, `start`, `prompt` | `cmd/buyer` → `internal/buyer`, `internal/mesh` |
| `seller` | `register`, `serve` | `cmd/seller` → `internal/seller`, `internal/control` client |
| `control` | `api`, `start` | `cmd/control` → `internal/control` HTTP server or libp2p control protocol |
| `demo` | `run` | placeholder |

Registration and login for buyers and sellers in production flows go through **`control api`** (PostgreSQL), invoked via `buyer register`, `seller register`, or the HTTP routes.

## Data stores (PostgreSQL)

Schema changes are **versioned migrations** (`internal/control/migrations/*.sql`, [golang-migrate](https://github.com/golang-migrate/migrate)), applied on `control api` startup. Migration history is stored in **`schema_migrations`** (managed by the tool).

| Data | Tables (conceptual) |
|------|---------------------|
| Identity | `buyer_users`, `seller_users` |
| Seller offers | `seller_models` (skills, limits, rates, active flag) |
| Presence | `seller_users.peer_id` (updated via presence API) |
| Billing | `buyer_billing`, `seller_billing` (wallet, quota, tokens used) |
| Ledger | `billing_transactions` (typed debits/credits and audit entries) |
| Inference | `inference_matches` (request id, buyer/seller, peer id, settlement flag) |

JWTs distinguish **buyer** vs **seller** subjects so tokens are not interchangeable across roles.

## End-to-end inference flow

### 1. Prerequisites

1. **Control API** running with `DATABASE_URL` (migrations on startup).
2. **Buyer** registered (`POST /v1/control/buyers/register` or `buyer register`).
3. **Seller** registered, models declared, **on duty**, and **presence** posted so PostgreSQL has a routable libp2p peer id.
4. **Buyer mesh** (`mesh serve`) logged in to control; **seller** (`seller serve`) logged in to control.
5. Buyer mesh process started with `--bootstrap` so it can reach the seller’s multiaddr (dial path).

### 2. Sequence (happy path)

```mermaid
sequenceDiagram
  participant App as App / CLI
  participant BAPI as Buyer HTTP API
  participant Mesh as Mesh runtime
  participant Ctrl as Control API
  participant PG as PostgreSQL
  participant P2P as libp2p QUIC
  participant Sell as Seller inference

  App->>BAPI: POST /v1/chat/completions (session header)
  BAPI->>Mesh: RunInference
  Mesh->>Ctrl: POST .../inference/match (buyer JWT, InferenceRequest meta)
  Ctrl->>PG: load sellers, check billing, matchmaker
  Ctrl-->>Mesh: sellerPeerId, sellerId, requestId, ...
  Mesh->>Ctrl: POST .../inference/tracking (e.g. started)
  Mesh->>P2P: inference stream to sellerPeerId
  P2P->>Sell: JSON InferenceRequest
  Sell-->>P2P: JSON InferenceResponse
  Mesh->>Ctrl: POST .../inference/tracking (completed)
  Mesh->>Ctrl: POST .../inference/complete (tokens, success)
  Ctrl->>PG: settle wallets + ledger (if success)
  Sell->>Ctrl: POST sellers/.../inference/tracking (audit)
  Mesh-->>BAPI: response
  BAPI-->>App: completion + usage
```

Each inference uses a **short-lived stream**: the connection is used for one request/response pair, then released.

### 3. What “matchmaking” means here

The matchmaker does **not** compute geographic distance. It filters candidates that are on duty, within price constraints, and expose the requested skill, then ranks by **lowest price** and **higher reputation** as a tie-breaker. Candidates come from PostgreSQL (sellers with `peer_id` and active models).

### 4. Billing (control pane)

- **Match** may reject the buyer if wallet/quota checks fail (HTTP 402 from control).
- **Complete** (buyer) applies settlement: debit buyer wallet, credit seller wallet, append `billing_transactions`, mark the `inference_matches` row settled (idempotent per `requestId`).
- **Tracking** endpoints append **audit** ledger rows (e.g. `inference_tracking`) with zero amount and JSON details where applicable.

## Related protocols

| Protocol ID | Use |
|-------------|-----|
| `/knowledgemesh/inference/1.0.0` | JSON `InferenceRequest` / `InferenceResponse` over a single stream |
| `/knowledgemesh/control/1.0.0` | Optional libp2p control node (`control start`), separate from HTTP control API |

## CLI registration

Seller and buyer **account** registration uses the **control pane** only:

- **CLI:** `buyer register`, `seller register` (both call `POST /v1/control/.../register`).
- **Buyer mesh** does not register users by itself; run `buyer register` (or the HTTP route) before `mesh serve` / `buyer start`.

There is no separate local-file registration CLI. The [README.md](./README.md) *CLI reference* lists every command.

### Optional libp2p control stream

`control start` exposes a small JSON ping handler on `/knowledgemesh/control/1.0.0`. This is **orthogonal** to the HTTP control API and is not required for buyer/seller registration or inference.
