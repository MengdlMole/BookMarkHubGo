#!/bin/sh
set -eu

PACKAGE_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
LAUNCHER="$PACKAGE_DIR/bookmarkhub"
VERSIONS_DIR="$PACKAGE_DIR/versions"

if [ "$(uname -s)" != "Darwin" ]; then
  echo "This helper is only for macOS." >&2
  exit 1
fi

if [ ! -f "$PACKAGE_DIR/current.json" ] || [ ! -f "$LAUNCHER" ] || [ ! -d "$VERSIONS_DIR" ]; then
  echo "BookmarkHub package files are incomplete. Extract the complete ZIP before running this helper." >&2
  exit 1
fi

chmod u+x "$LAUNCHER"
xattr -d com.apple.quarantine "$LAUNCHER" 2>/dev/null || true

find "$VERSIONS_DIR" -type f -name bookmarkhub-core -print | while IFS= read -r core; do
  chmod u+x "$core"
  xattr -d com.apple.quarantine "$core" 2>/dev/null || true
done

echo "BookmarkHub executable permissions and quarantine attributes are ready."
echo "Starting BookmarkHub..."
exec "$LAUNCHER"
