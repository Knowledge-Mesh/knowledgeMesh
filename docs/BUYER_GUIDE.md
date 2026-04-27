# Buyer Guide

## Quick onboarding

Prerequisite: seller is configured and running (see `docs/SELLER_GUIDE.md`).

```bash
go run ./cmd/buyer register \
  --name "Demo Buyer" \
  --email buyer@example.com \
  --password 'your-password'

go run ./cmd/buyer serve \
  --email buyer@example.com \
  --password 'your-password'
```

## Send a test prompt

```bash
go run ./cmd/buyer prompt \
  --email buyer@example.com \
  --password 'your-password' \
  --api-url http://127.0.0.1:8080 \
  --prompt 'Write a short haiku about distributed systems.'
```

## Useful flags

- `--control-url` for non-local control API
- `--bootstrap` when seller addresses in control are not reachable
- `--relay` / `LIBP2P_STATIC_RELAYS` for explicit relay routing
- `--p2p-debug` and `--p2p-debug-http` for connectivity diagnostics

## Quick checks

- Buyer API health: `curl http://127.0.0.1:8080/healthz`
- Control API health: `curl http://127.0.0.1:8090/healthz`

