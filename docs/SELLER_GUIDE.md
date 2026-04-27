# Seller Guide

## Quick onboarding (Ollama)

```bash
go run ./cmd/seller register \
  --name "Demo Seller" \
  --email seller@example.com \
  --password 'your-password'

go run ./cmd/seller setup \
  --email seller@example.com \
  --password 'your-password' \
  --model-id my-chat \
  --rate-per-token 0.000001 \
  --ollama-base-url http://127.0.0.1:11434 \
  --ollama-map my-chat=llama3:latest

go run ./cmd/seller status --email seller@example.com --password 'your-password'
go run ./cmd/seller serve --email seller@example.com --password 'your-password'
```

## Duty control

```bash
go run ./cmd/seller duty on  --email seller@example.com --password 'your-password'
go run ./cmd/seller duty off --email seller@example.com --password 'your-password'
```

## Useful flags

- `--control-url` to use non-local control API
- `--p2p-addr` on `seller serve` for fixed listen addr
- `--relay` / `LIBP2P_STATIC_RELAYS` for explicit relay multiaddrs
- `--server-mode` for public-server reachability behavior

## Common checks

- Validate seller profile: `seller status`
- Ensure at least one active model exists
- Ensure seller is on-duty before expecting matches

