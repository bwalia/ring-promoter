#!/bin/sh
# Build the static export and copy it into the Go embed directory
# (internal/web/static), replacing whatever was there. Run from web/.
set -eu

cd "$(dirname "$0")/.."

NEXT_OUTPUT=export npx next build

DEST="../internal/web/static"
rm -rf "$DEST"
mkdir -p "$DEST"
cp -R out/. "$DEST/"

echo "Embedded UI updated: $(du -sh "$DEST" | cut -f1) in internal/web/static"
