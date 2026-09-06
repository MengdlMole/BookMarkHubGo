#!/usr/bin/env sh
set -eu

if [ "$#" -ne 5 ]; then
  echo "usage: make-update.sh <version> <macos|linux|windows> <arch> <core-binary> <output.zip>" >&2
  exit 1
fi

VERSION=$1
PLATFORM=$2
ARCH=$3
CORE=$4
OUTPUT=$5
case "$OUTPUT" in /*) ;; *) OUTPUT="$(pwd)/$OUTPUT" ;; esac
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
BINARY=bookmarkhub-core
if [ "$PLATFORM" = "windows" ]; then BINARY=bookmarkhub-core.exe; fi
mkdir -p "$WORK/payload"
cp "$CORE" "$WORK/payload/$BINARY"
if command -v shasum >/dev/null 2>&1; then
  HASH=$(shasum -a 256 "$CORE" | awk '{print $1}')
else
  HASH=$(sha256sum "$CORE" | awk '{print $1}')
fi
printf '{"version":"%s","platform":"%s","arch":"%s","sha256":"%s","binary":"%s"}\n' "$VERSION" "$PLATFORM" "$ARCH" "$HASH" "$BINARY" > "$WORK/manifest.json"
if [ -n "${UPDATE_PRIVATE_KEY:-}" ]; then
  openssl pkeyutl -sign -inkey "$UPDATE_PRIVATE_KEY" -rawin -in "$WORK/manifest.json" | openssl base64 -A > "$WORK/manifest.sig"
  (cd "$WORK" && zip -qr "$OUTPUT" manifest.json manifest.sig payload)
else
  (cd "$WORK" && zip -qr "$OUTPUT" manifest.json payload)
fi
echo "Created $OUTPUT"
