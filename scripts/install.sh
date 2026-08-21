#!/usr/bin/env bash
set -euo pipefail

REPO="solo-agent/solo"
BIN_DIR="${SOLO_BIN_DIR:-$HOME/.local/bin}"
VERSION="${SOLO_VERSION:-}"
CONNECT=0
SERVER=""
COMPUTER_ID=""
TOKEN=""

fail() { printf 'solo installer: %s\n' "$*" >&2; exit 1; }

while [ "$#" -gt 0 ]; do
  case "$1" in
    connect) CONNECT=1; shift ;;
    --server) [ "$#" -ge 2 ] || fail "--server needs a value"; SERVER="$2"; shift 2 ;;
    --computer-id) [ "$#" -ge 2 ] || fail "--computer-id needs a value"; COMPUTER_ID="$2"; shift 2 ;;
    --token) [ "$#" -ge 2 ] || fail "--token needs a value"; TOKEN="$2"; shift 2 ;;
    --version) [ "$#" -ge 2 ] || fail "--version needs a value"; VERSION="$2"; shift 2 ;;
    --bin-dir) [ "$#" -ge 2 ] || fail "--bin-dir needs a value"; BIN_DIR="$2"; shift 2 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"
command -v install >/dev/null 2>&1 || fail "install is required"

case "$(uname -s)" in
  Darwin) OS=darwin ;;
  Linux) OS=linux ;;
  *) fail "only macOS and Linux are supported" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) fail "unsupported architecture: $(uname -m)" ;;
esac

if [ -z "$VERSION" ]; then
  RELEASE_URL="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest")"
  VERSION="${RELEASE_URL##*/}"
fi
case "$VERSION" in
  v*) TAG="$VERSION"; ASSET_VERSION="${VERSION#v}" ;;
  *) TAG="v$VERSION"; ASSET_VERSION="$VERSION" ;;
esac
case "$ASSET_VERSION" in
  *[!A-Za-z0-9._-]*|'') fail "invalid release version" ;;
esac

ASSET="solo_${ASSET_VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="${SOLO_RELEASE_BASE_URL:-https://github.com/$REPO/releases/download/$TAG}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

printf 'Downloading Solo %s for %s/%s...\n' "$ASSET_VERSION" "$OS" "$ARCH"
curl -fsSL "$BASE_URL/$ASSET" -o "$TMP_DIR/$ASSET"
curl -fsSL "$BASE_URL/checksums.txt" -o "$TMP_DIR/checksums.txt"
EXPECTED="$(awk -v name="$ASSET" '$2 == name {print $1}' "$TMP_DIR/checksums.txt")"
[ -n "$EXPECTED" ] || fail "release checksum not found"
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL="$(sha256sum "$TMP_DIR/$ASSET" | awk '{print $1}')"
else
  ACTUAL="$(shasum -a 256 "$TMP_DIR/$ASSET" | awk '{print $1}')"
fi
[ "$EXPECTED" = "$ACTUAL" ] || fail "checksum verification failed"

tar -xzf "$TMP_DIR/$ASSET" -C "$TMP_DIR"
[ -x "$TMP_DIR/solo" ] || fail "release is missing solo"
[ -x "$TMP_DIR/solo-daemon" ] || fail "release is missing solo-daemon"
mkdir -p "$BIN_DIR"
install -m 0755 "$TMP_DIR/solo" "$BIN_DIR/solo"
install -m 0755 "$TMP_DIR/solo-daemon" "$BIN_DIR/solo-daemon"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *)
    SHELL_RC="$HOME/.profile"
    case "${SHELL:-}" in
      */zsh) SHELL_RC="$HOME/.zshrc" ;;
      */bash) SHELL_RC="$HOME/.bashrc" ;;
    esac
    PATH_LINE="export PATH=\"$BIN_DIR:\$PATH\""
    if ! grep -Fq "$PATH_LINE" "$SHELL_RC" 2>/dev/null; then
      printf '\n# Solo CLI\n%s\n' "$PATH_LINE" >> "$SHELL_RC"
      printf 'Added %s to PATH in %s.\n' "$BIN_DIR" "$SHELL_RC"
    fi
    ;;
esac

printf 'Installed solo and solo-daemon %s in %s.\n' "$ASSET_VERSION" "$BIN_DIR"
if [ "$CONNECT" -eq 1 ]; then
  [ -n "$SERVER" ] || fail "connect requires --server"
  [ -n "$COMPUTER_ID" ] || fail "connect requires --computer-id"
  [ -n "$TOKEN" ] || fail "connect requires --token"
  exec "$BIN_DIR/solo" daemon connect --server "$SERVER" --computer-id "$COMPUTER_ID" --token "$TOKEN" --profile "$COMPUTER_ID"
fi
