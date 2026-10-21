#!/bin/sh
# Run from the repository root with the pinned Go toolchain on PATH.
set -eu
export CGO_ENABLED=0

go test ./...
go vet ./...
go vet -tags mygo_mcp,mygo_noinspector ./...
go build -tags mygo_noinspector ./...
for platform in linux darwin windows; do
  GOOS="$platform" go build -tags mygo_mcp,mygo_noinspector ./...
done

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
go list -deps ./examples/native > "$tmp/default-deps"
if grep -Eq '^github.com/(auron-labs/mygo-mcp|modelcontextprotocol/go-sdk/mcp)$' "$tmp/default-deps"; then
  echo 'ERROR: default example imports MCP' >&2
  exit 1
fi
go list -tags mygo_mcp -deps ./examples/native > "$tmp/enabled-deps"
grep -q '^github.com/modelcontextprotocol/go-sdk/mcp$' "$tmp/enabled-deps"
go build -trimpath -tags mygo_noinspector -ldflags '-s -w' -o "$tmp/default" ./examples/native
go build -trimpath -tags mygo_noinspector,mygo_mcp -ldflags '-s -w' -o "$tmp/enabled" ./examples/native
go version -m "$tmp/default" > "$tmp/default-modules"
if grep -q 'github.com/modelcontextprotocol/go-sdk' "$tmp/default-modules"; then
  echo 'ERROR: default binary contains the MCP SDK module' >&2
  exit 1
fi
echo 'Production isolation passed. Native example binary sizes (default, enabled):'
wc -c "$tmp/default" "$tmp/enabled"
