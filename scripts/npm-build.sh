#!/usr/bin/env bash
# Builds the npm packages of ccw into OUT (default dist/npm): the main package and one
# package per platform. Publish with: for d in OUT/*/; do npm publish "$d"; done (platforms first).
# NPM_SCOPE=@owner/ prefixes every name, for GitHub Packages.
set -euo pipefail
cd "$(dirname "$0")/.."
OUT=${1:-dist/npm}
# The binaries build inside be/, so OUT becomes absolute before that cd.
case "$OUT" in /*) ;; *) OUT="$PWD/$OUT" ;; esac
VERSION=$(node -p "require('./npm/ccw-gateway/package.json').version")
rm -rf "$OUT" && mkdir -p "$OUT"
for target in linux/amd64/linux/x64 linux/arm64/linux/arm64 darwin/amd64/darwin/x64 \
              darwin/arm64/darwin/arm64 windows/amd64/win32/x64 windows/arm64/win32/arm64; do
  IFS=/ read -r goos goarch os cpu <<<"$target"
  name="${NPM_SCOPE:-}ccw-gateway-$os-$cpu"
  # npm refused the name intact-proxy-win32-x64 (this project's earlier name) as spam, so Windows x64 keeps a separate name; bin/ccw.js maps win32-x64 to this name.
  [ "$os-$cpu" = win32-x64 ] && name="${NPM_SCOPE:-}ccw-gateway-windows-x64"
  dir="$OUT/${name#"${NPM_SCOPE:-}"}"
  exe=ccw; [ "$goos" = windows ] && exe=ccw.exe
  mkdir -p "$dir/bin"
  (cd be && GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$dir/bin/$exe" ./cmd/ccw)
  cat > "$dir/package.json" <<JSON
{
  "name": "$name",
  "version": "$VERSION",
  "description": "The ccw binary for $os $cpu. Install ccw-gateway instead.",
  "license": "MIT",
  "repository": { "type": "git", "url": "git+https://github.com/naniiluja/ccw.git" },
  "os": ["$os"],
  "cpu": ["$cpu"],
  "files": ["bin"]
}
JSON
  cp LICENSE "$dir/"
done
cp -r npm/ccw-gateway "$OUT/ccw-gateway"
cp LICENSE "$OUT/ccw-gateway/"
if [ -n "${NPM_SCOPE:-}" ]; then
  (cd "$OUT/ccw-gateway" && node -e '
    const fs = require("fs"), scope = process.argv[1], p = JSON.parse(fs.readFileSync("package.json", "utf8"));
    p.name = scope + p.name;
    p.optionalDependencies = Object.fromEntries(Object.entries(p.optionalDependencies).map(([k, v]) => [scope + k, v]));
    fs.writeFileSync("package.json", JSON.stringify(p, null, 2) + "\n");' "$NPM_SCOPE")
fi
echo "built $VERSION into $OUT"
