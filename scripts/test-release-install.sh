#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

case "$(uname -s)" in
  Darwin) OS=darwin ;;
  Linux) OS=linux ;;
  *) echo "unsupported test OS" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "unsupported test architecture" >&2; exit 1 ;;
esac

VERSION="0.0.0-test"
RELEASE_DIR="$TMP_DIR/release"
PAYLOAD_DIR="$TMP_DIR/payload"
INSTALL_DIR="$TMP_DIR/install"
mkdir -p "$RELEASE_DIR" "$PAYLOAD_DIR"
mkdir -p "$TMP_DIR/home"

cd "$ROOT"
LDFLAGS="-s -w -X github.com/solo-ai/solo/pkg/version.Version=$VERSION -X github.com/solo-ai/solo/pkg/version.Commit=test -X github.com/solo-ai/solo/pkg/version.Date=test"
go build -ldflags "$LDFLAGS" -o "$PAYLOAD_DIR/solo" ./cmd/solo
go build -ldflags "$LDFLAGS" -o "$PAYLOAD_DIR/solo-daemon" ./cmd/daemon

ASSET="solo_${VERSION}_${OS}_${ARCH}.tar.gz"
tar -czf "$RELEASE_DIR/$ASSET" -C "$PAYLOAD_DIR" solo solo-daemon
if command -v sha256sum >/dev/null 2>&1; then
  (cd "$RELEASE_DIR" && sha256sum "$ASSET" > checksums.txt)
else
  (cd "$RELEASE_DIR" && shasum -a 256 "$ASSET" > checksums.txt)
fi

HOME="$TMP_DIR/home" \
SHELL=/bin/sh \
SOLO_VERSION="$VERSION" \
SOLO_BIN_DIR="$INSTALL_DIR" \
SOLO_RELEASE_BASE_URL="file://$RELEASE_DIR" \
bash "$ROOT/scripts/install.sh"

test -x "$INSTALL_DIR/solo"
test -x "$INSTALL_DIR/solo-daemon"
"$INSTALL_DIR/solo" version | grep -F "solo $VERSION" >/dev/null
printf 'Release archive, checksum, installer, and linked version verified.\n'
