#!/bin/bash
# Start KnowledgeMesh in API mode (Anthropic API key)
#
# Usage (standalone):
#   ANTHROPIC_API_KEY=sk-ant-... ./start-api.sh
#
# Usage (with broker — auto-creates Cloudflare tunnel):
#   ANTHROPIC_API_KEY=sk-ant-... KM_BROKER_URL=https://xxx.trycloudflare.com ./start-api.sh
#
# The worker auto-starts a cloudflared tunnel when KM_BROKER_URL is set
# and KM_TUNNEL_URL is not. Set KM_TUNNEL_URL to override with your own URL.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "╔═══════════════════════════════════════════════╗"
echo "║  KnowledgeMesh — API Key Mode                 ║"
echo "╚═══════════════════════════════════════════════╝"
echo ""

export KM_TIER=api
"$SCRIPT_DIR/target/release/km-worker"
