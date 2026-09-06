#!/usr/bin/env sh
set -eu

VERSION="${1:-0.1.0}"
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DIST="$ROOT/dist"

command -v go >/dev/null 2>&1 || { echo "Go 1.23 or newer is required" >&2; exit 1; }
rm -rf "$DIST"
mkdir -p "$DIST"

cd "$ROOT"
go test ./...

build_target() {
  target_os=$1
  target_arch=$2
  platform=$target_os
  launcher=bookmarkhub
  core=bookmarkhub-core
  if [ "$target_os" = "darwin" ]; then platform=macos; fi
  if [ "$target_os" = "windows" ]; then launcher=bookmarkhub.exe; core=bookmarkhub-core.exe; fi
  package="$DIST/bookmarkhub-$platform-$target_arch"
  mkdir -p "$package/versions/$VERSION"
  CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$package/versions/$VERSION/$core" ./cmd/bookmarkhub-core
  CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags="-s -w" -o "$package/$launcher" ./cmd/bookmarkhub-launcher
  printf '{\n  "version": "%s"\n}\n' "$VERSION" > "$package/current.json"
  if command -v zip >/dev/null 2>&1; then
    (cd "$DIST" && zip -qr "bookmarkhub-$platform-$target_arch.zip" "bookmarkhub-$platform-$target_arch")
  fi
}

build_target darwin amd64
build_target darwin arm64
build_target linux amd64
build_target linux arm64
build_target windows amd64
build_target windows arm64

extension_package="$DIST/bookmarkhub-extension-$VERSION"
mkdir -p "$extension_package"
cp -R extension/. "$extension_package/"
if command -v zip >/dev/null 2>&1; then
  (cd "$DIST" && zip -qr "bookmarkhub-extension-$VERSION.zip" "bookmarkhub-extension-$VERSION")
fi

echo "Portable packages created in $DIST"
