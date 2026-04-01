#!/bin/bash
# Start KnowledgeMesh in subscription mode (direct Claude.ai session API)
#
# Usage:
#   KM_SESSION_KEY="sessionKey=..." ./start-subscription.sh
#   KM_SESSION_KEY="sk-ant-sid01-..." ./start-subscription.sh
#   KM_SESSION_KEY="..." KM_BROKER_URL=https://xxx.trycloudflare.com ./start-subscription.sh
#
# For the old Chrome extension path (legacy):
#   KM_TIER=subscription-legacy ./start-subscription.sh
#
# How to get your session cookie:
#   1. Open https://claude.ai in Chrome and log in
#   2. Open DevTools (F12) → Application → Cookies → claude.ai
#   3. Find 'sessionKey' and copy its value
#
# Prerequisites:
#   1. A Claude MAX subscription
#   2. cloudflared installed (brew install cloudflared) — only if using broker

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Legacy mode: use old Chrome extension bridge
if [ "${KM_TIER}" = "subscription-legacy" ]; then
    echo "╔═══════════════════════════════════════════════╗"
    echo "║  KnowledgeMesh — Subscription Bridge (Legacy)  ║"
    echo "╚═══════════════════════════════════════════════╝"
    echo ""

    echo "[1/2] Starting bridge server on localhost:8100..."
    node "$SCRIPT_DIR/extension/bridge-server.js" &
    BRIDGE_PID=$!
    sleep 1

    export KM_TIER=subscription-legacy
    echo "[2/2] Starting worker on localhost:${KM_PORT:-8000}..."
    echo ""
    "$SCRIPT_DIR/target/release/km-worker"

    trap "kill $BRIDGE_PID 2>/dev/null" EXIT
    exit 0
fi

# New default: direct session API
echo "╔═══════════════════════════════════════════════╗"
echo "║  KnowledgeMesh — Session Mode (Direct API)    ║"
echo "╚═══════════════════════════════════════════════╝"
echo ""

if [ -z "$KM_SESSION_KEY" ]; then
    echo "No KM_SESSION_KEY set."
    echo ""
    echo "  Get your session cookie from Chrome DevTools:"
    echo "  1. Open https://claude.ai in Chrome"
    echo "  2. DevTools (F12) → Application → Cookies → claude.ai"
    echo "  3. Find 'sessionKey' and copy its value"
    echo "  4. Run:"
    echo ""
    echo "     KM_SESSION_KEY=\"paste-value-here\" ./start-subscription.sh"
    echo ""
    exit 1
fi

export KM_TIER=subscription
echo "Starting worker on localhost:${KM_PORT:-8000}..."
echo ""
"$SCRIPT_DIR/target/release/km-worker"
