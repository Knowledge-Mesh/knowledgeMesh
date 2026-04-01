#!/bin/bash
# KnowledgeMesh worker installer
# Usage: curl -fsSL https://raw.githubusercontent.com/ArchieIndian/km/main/install.sh | sh

set -e

BINARY="km-worker"
INSTALL_DIR="/usr/local/bin"
BASE_URL="https://github.com/ArchieIndian/km/releases/latest/download"

echo "==> KnowledgeMesh Worker Installer"
echo ""

# Detect OS
OS="$(uname -s)"
case "$OS" in
  Darwin) OS="darwin" ;;
  Linux)  OS="linux" ;;
  *)      echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="x86_64" ;;
  arm64|aarch64) ARCH="aarch64" ;;
  *)             echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

ASSET="${BINARY}-${OS}-${ARCH}"
URL="${BASE_URL}/${ASSET}"

echo "==> Detected: ${OS}/${ARCH}"
echo "==> Downloading ${ASSET}..."

curl -fsSL -o /tmp/${BINARY} "$URL" || { echo "Download failed from ${URL}"; exit 1; }

chmod +x /tmp/${BINARY}

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv /tmp/${BINARY} "${INSTALL_DIR}/${BINARY}"
else
  echo "==> Need sudo to install to ${INSTALL_DIR}"
  sudo mv /tmp/${BINARY} "${INSTALL_DIR}/${BINARY}"
fi

echo ""
echo "==> Installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "First time? Self-register:"
echo "  KM_INVITE_CODE=<your-code> KM_NODE_NAME=my-node KM_EMAIL=me@example.com KM_TIER=ollama km-worker"
echo ""
echo "Already registered? Just run:"
echo "  KM_NODE_NAME=my-node KM_TIER=ollama km-worker"
echo ""
echo "Set credentials for your tier:"
echo "  api          -> ANTHROPIC_API_KEY=sk-ant-..."
echo "  openai       -> OPENAI_API_KEY=sk-..."
echo "  subscription -> KM_SESSION_KEY=..."
echo "  ollama       -> (none, just run 'ollama serve')"
