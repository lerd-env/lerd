#!/bin/sh
# Builds lerd.exe for amd64 and arm64 into dist-windows/, where .goreleaser.yml
# reads it. The version resource names the product and version, which SignPath
# Foundation requires of every file it signs.
set -eu

version=${1:?usage: build-windows.sh <version>}

(cd cmd/lerd-windows && go run github.com/tc-hib/go-winres@v0.3.3 simply \
  --arch amd64,arm64 --manifest none \
  --product-name Lerd --file-description Lerd --original-filename lerd.exe \
  --product-version "$version" --file-version "$version")

for arch in amd64 arm64; do
  CGO_ENABLED=0 GOOS=windows GOARCH=$arch go build -ldflags="-s -w" \
    -o "dist-windows/$arch/lerd.exe" ./cmd/lerd-windows
done
