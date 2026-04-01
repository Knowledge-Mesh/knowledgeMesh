#!/bin/bash
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

PORT="${PORT:-9000}"

echo ""
echo "  ╔══════════════════════════════════════════╗"
echo "  ║   KnowledgeMesh Broker                   ║"
echo "  ╚══════════════════════════════════════════╝"
echo ""

# If BROKER_TUNNEL_URL is already set, skip cloudflared
if [ -n "$BROKER_TUNNEL_URL" ]; then
    echo "[broker] Using provided tunnel URL: $BROKER_TUNNEL_URL"
    echo ""
    cd "$SCRIPT_DIR/fabric"
    ./km-fabric
    exit 0
fi

# Check cloudflared is installed
if ! command -v cloudflared &> /dev/null; then
    echo "ERROR: cloudflared is not installed."
    echo "Install with: brew install cloudflared"
    exit 1
fi

# Start cloudflared tunnel for the broker
echo "[broker] Starting cloudflared tunnel for port $PORT..."
cloudflared tunnel --url "http://127.0.0.1:$PORT" &> /tmp/km-broker-tunnel.log &
CF_PID=$!

# Clean up on exit
cleanup() {
    echo ""
    echo "[broker] Shutting down..."
    kill $CF_PID 2>/dev/null || true
    wait $CF_PID 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# Wait for the trycloudflare.com URL to appear in cloudflared output
TUNNEL_URL=""
echo "[broker] Waiting for tunnel URL..."
for i in $(seq 1 30); do
    sleep 1
    # Look for the URL in cloudflared's log output
    TUNNEL_URL=$(grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/km-broker-tunnel.log 2>/dev/null | head -1 || true)
    if [ -n "$TUNNEL_URL" ]; then
        break
    fi
done

if [ -z "$TUNNEL_URL" ]; then
    echo "ERROR: Failed to get tunnel URL from cloudflared (30s timeout)"
    echo "Check /tmp/km-broker-tunnel.log for details"
    exit 1
fi

echo ""
echo "  ┌──────────────────────────────────────────────────────────┐"
echo "  │  BROKER TUNNEL READY                                     │"
echo "  │                                                          │"
echo "  │  $TUNNEL_URL"
echo "  │                                                          │"
echo "  │  Workers should set:                                     │"
echo "  │  KM_BROKER_URL=$TUNNEL_URL"
echo "  └──────────────────────────────────────────────────────────┘"
echo ""

# Start the Go broker
cd "$SCRIPT_DIR/fabric"
./km-fabric
