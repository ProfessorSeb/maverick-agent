#!/usr/bin/env bash
# Maverick Agent installer — detects OS/arch and downloads the right binary.
# Usage: curl -fsSL https://raw.githubusercontent.com/ProfessorSeb/maverick-agent/main/agent/install.sh | bash
set -euo pipefail

REPO="ProfessorSeb/maverick-agent"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)      echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)              echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

echo "Detected platform: ${OS}/${ARCH}"

# Find latest agent release tag
LATEST_TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases" \
  | grep -o '"tag_name": *"[^"]*"' \
  | head -1 \
  | sed 's/.*"tag_name": *"//;s/"//')"

if [ -z "$LATEST_TAG" ]; then
  echo "Error: could not find a maverick-agent release" >&2
  exit 1
fi

echo "Latest release: ${LATEST_TAG}"

BINARY="maverick-agent-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/${BINARY}"

echo "Downloading ${URL}..."
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

curl -fSL --progress-bar -o "$TMP" "$URL"
chmod +x "$TMP"

if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP" "${INSTALL_DIR}/maverick-agent"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "$TMP" "${INSTALL_DIR}/maverick-agent"
fi

echo "maverick-agent installed to ${INSTALL_DIR}/maverick-agent"
maverick-agent --version
