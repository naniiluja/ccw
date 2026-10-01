#!/usr/bin/env bash
# Builds the dashboard SPA (fe/) and copies it into be/internal/webui/static/, where
# go:embed picks it up. The copy is gitignored except .gitkeep. Safe to re-run.
set -euo pipefail
cd "$(dirname "$0")/.."
DEST=be/internal/webui/static
pnpm -C fe install --frozen-lockfile
pnpm -C fe build
mkdir -p "$DEST"
find "$DEST" -mindepth 1 -not -name .gitkeep -delete
cp -R fe/dist/. "$DEST/"
echo "UI built into $DEST"
