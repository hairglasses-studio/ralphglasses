#!/usr/bin/env bash
set -euo pipefail

REPO="hairglasses-studio/ralphglasses"
BINARY="ralphglasses"
VERSION="${1:-}"

# Parse --version flag
for arg in "$@"; do
  case "$arg" in
    --version=*) VERSION="${arg#--version=}" ;;
    --version)   shift; VERSION="${1:-}" ;;
  esac
done

# Detect OS and arch
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
case "$OS" in
  linux|darwin) ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Resolve version
if [ -z "$VERSION" ]; then
  echo "Fetching latest release..."
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')"
fi
if [ -z "$VERSION" ]; then
  echo "Could not determine version. Pass --version=vX.Y.Z to specify." >&2
  exit 1
fi

ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading ${BINARY} ${VERSION} (${OS}/${ARCH})..."
curl -fsSL "${BASE_URL}/${ARCHIVE}" -o "${TMPDIR}/${ARCHIVE}"
curl -fsSL "${BASE_URL}/${BINARY}_${VERSION}_checksums.txt" -o "${TMPDIR}/checksums.txt"

# Verify checksum
echo "Verifying checksum..."
cd "$TMPDIR"
if command -v sha256sum >/dev/null 2>&1; then
  grep "${ARCHIVE}" checksums.txt | sha256sum --check --status
elif command -v shasum >/dev/null 2>&1; then
  grep "${ARCHIVE}" checksums.txt | shasum -a 256 --check --status
else
  echo "Warning: no sha256sum or shasum found, skipping verification" >&2
fi

tar -xzf "${ARCHIVE}"

# Install
if [ -w /usr/local/bin ]; then
  INSTALL_DIR="/usr/local/bin"
elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
  INSTALL_DIR="/usr/local/bin"
  USE_SUDO=1
else
  INSTALL_DIR="${HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

if [ "${USE_SUDO:-}" = "1" ]; then
  sudo install -m 755 "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  install -m 755 "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

echo "Installed ${BINARY} ${VERSION} to ${INSTALL_DIR}/${BINARY}"
if ! echo "$PATH" | grep -q "${INSTALL_DIR}"; then
  echo "Note: add ${INSTALL_DIR} to your PATH"
fi
