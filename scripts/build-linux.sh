#!/bin/sh
set -eu

# Native Linux build; no CGO, X11 development headers, root, or system install.
cd "$(dirname "$0")/.."
if [ "$(uname -s)" != Linux ]; then
  echo 'Run this script on Linux.' >&2
  exit 2
fi
arch=$(go env GOARCH)
out="dist/linux/$arch"
mkdir -p "$out"
export CGO_ENABLED=0
for name in krasis krasis-cli krasis-agent krasis-clipboard; do
  go build -trimpath -o "$out/$name" "./cmd/$name"
done
cp docs/linux.md "$out/README.md"
tar -czf "dist/linux/krasis-linux-$arch.tar.gz" -C "$out" \
  krasis krasis-cli krasis-agent krasis-clipboard README.md
sha256sum "dist/linux/krasis-linux-$arch.tar.gz"
