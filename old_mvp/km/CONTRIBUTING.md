# Contributing to KnowledgeMesh

Thanks for your interest in contributing! KnowledgeMesh is a peer-to-peer AI inference marketplace with two main components:

- **Broker** (`fabric/`) — Go HTTP server that matches buyers to sellers
- **Worker** (`worker/`) — Rust binary that proxies inference requests

## Quick Setup

### Broker (Go)

```bash
cd fabric
go build -o km-fabric .
KM_ADMIN_SECRET=dev-secret ./km-fabric
# Open http://127.0.0.1:9000
```

### Worker (Rust)

```bash
cargo build --release -p km-worker

# Run with an API key
ANTHROPIC_API_KEY=sk-ant-xxx KM_TIER=api KM_NODE_SECRET=km-sec-xxx ./target/release/km-worker

# Or with Ollama (must be running locally)
KM_TIER=ollama KM_NODE_SECRET=km-sec-xxx ./target/release/km-worker
```

### Integration Tests

```bash
./test/integration.sh
```

## Project Structure

```
fabric/                Go broker
  main.go              HTTP server + routes
  handlers.go          Shared types, middleware, rate limiting
  handlers_auth.go     Registration, recovery, whoami, node-config
  handlers_task.go     Task execution, OpenAI-compatible endpoint
  handlers_admin.go    Admin: invites, reset, reset-tokens
  handlers_node.go     Node: register, heartbeat, deregister, status, models
  email.go             Recovery email sending (Resend)
  capacity.go          Concurrency and token budget tracking
  validation.go        Tunnel URL validation (SSRF prevention)
  registry.go          Node registry + heartbeat reaper
  matcher.go           Model-aware routing
  escrow.go            Credit escrow
  ledger.go            Append-only JSONL ledger
  state.go             Persistent state
  web/                 Dashboard (embedded HTML)

worker/              Rust worker
  src/main.rs        HTTP server + bridge creation
  src/config.rs      Broker bootstrap config
  src/setup.rs       Self-registration wizard
  src/models.rs      Model discovery + pricing
  src/handlers.rs    HTTP request handlers
  src/fabric.rs      Broker registration + heartbeat
  src/tunnel.rs      Cloudflare Tunnel auto-management
  src/bridge/        AI provider bridges
    anthropic.rs     Anthropic API
    openai.rs        OpenAI API
    ollama.rs        Ollama (local models)
    session.rs       Claude.ai session
    subscription.rs  Legacy subscription bridge

cli/                 CLI tool
sdk/python/          Python SDK
```

## How to Contribute

1. **Fork** the repo and create a branch from `main`
2. **Make your changes** — keep PRs focused on one thing
3. **Test** — run the integration tests, make sure `go build` and `cargo build` pass
4. **Submit a PR** with a clear description of what and why

### Good First Issues

- Add new AI provider bridges (e.g., Google Gemini, Mistral)
- Improve error messages in the worker
- Add more integration test cases
- Dashboard UI improvements
- Python SDK enhancements

### Adding a New Bridge

1. Create `worker/src/bridge/yourprovider.rs`
2. Implement the bridge (see `anthropic.rs` for reference):
   - `new()` constructor
   - `run()` that takes `InferenceRequest` and returns `InferenceResponse`
   - `health_check()` for startup validation
3. Add the variant to `BridgeKind` in `bridge/mod.rs`
4. Add the tier to `create_bridge()` in `main.rs`
5. Add model discovery to `build_models_map()` in `main.rs`

## Code Style

- **Go**: standard `gofmt`, no external linters required
- **Rust**: standard `cargo fmt`, `cargo clippy` should be clean
- Keep dependencies minimal — the worker should compile fast
- No frameworks for the dashboard — vanilla HTML/JS only

## Security

- Never log API keys, session tokens, or node secrets
- Never store emails in plaintext (HMAC-SHA256 hash only)
- All POST endpoints must use `http.MaxBytesReader` (100KB limit)
- Rate limiting on all public endpoints

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
