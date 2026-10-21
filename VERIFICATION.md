# Delivery verification

Environment: Linux/amd64, Go 1.27.3; checked 2026-10-21.

## Implemented plan

- Public-API proof in `public_api_test.go`: unmodified `ui.NewTester`, real
  navigation/input/selection replacement/button/shortcut handlers, Unicode,
  simulated timers, visible text and decoded PNGs.
- `sessions.go`: isolated factories, run-scoped IDs, one owning worker, fixed
  allocation/queue bounds, cancellation, failure isolation and once-only cleanup.
- `tools.go`: six SDK tools with validated inputs, actual PNG image content,
  geometry-aware screenshot clicks, stale-ID protection, explicit frozen-clock
  advancement and pending-frame reporting.
- `server.go`: explicit loopback binding, stateless SDK HTTP at `/mcp`, Host/origin
  protection, request bounds and context-driven lifecycle.
- `tools_test.go`, `sessions_test.go`, `server_test.go`: real SDK HTTP workflows,
  fractional scales/rounding, boundaries, isolation/restart IDs, stale screenshots,
  cancellation before/after dispatch, queue saturation/ordering across fixtures,
  nonblocking caller cancellation, shutdown waiting for synchronous rendering,
  partial-construction ownership, render/factory/cleanup panics, delayed updates,
  continuous frames and endpoint restrictions.
- Separate `examples/headless` harness sharing `internal/demo`'s application
  view; integration, client setup, limitations and limits in `README.md`.

## Checks

All of the following pass:

```sh
go test ./...
go test -race -count=10 ./...
CGO_ENABLED=0 go test ./...
go vet ./...
go build ./...
CGO_ENABLED=0 go build ./...
go mod tidy -diff
go mod verify
mise exec -- golangci-lint run --fix=false ./...
mise exec -- golangci-lint fmt --diff
mise exec -- hk check --all --check
```

The full race suite was also repeated during development. No required toolchain
or native dependency was unavailable for these Linux checks.

Dependency/import inspection confirms:

- `github.com/egoist/mygo` resolves to
  `v0.5.6-0.20261020235557-3dfa48145e52` (upstream commit
  `3dfa48145e523d029ea6959f6e129915eb16499d`), with checksums in `go.sum`.
- The official MCP SDK is pinned to `github.com/modelcontextprotocol/go-sdk v1.8.0`.
- There are no module replacements or local reference dependencies. `go mod verify`
  verifies downloaded dependency contents against their checksums.
- Project source imports only public MyGo packages, with no `unsafe`, `reflect`,
  internal framework imports or `go:linkname` directives.
- `go list -deps ./internal/demo` contains no MCP SDK packages. The application
  view has no dependency on the opt-in MCP integration; only the separate harness
  imports the root server package.

## Not checked / residual risk

No GUI, hardware, cross-platform desktop matrix, external host application's
platform-service integration or size benchmark was run; these are not headless
delivery gates. Screenshots do not promise desktop/GPU parity. There is no hard
deadline for blocking application callbacks: they can hold the single worker and
delay shutdown indefinitely. Use nonblocking handlers or a disposable harness
process. Loopback has no authentication, so local processes can invoke fixture
handlers; use isolated state and test services/data.
