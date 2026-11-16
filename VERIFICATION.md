# Delivery verification

## MyGo upgrade — 2026-11-16

Environment: Linux/amd64, Go 1.27.3.

- MyGo now resolves to **v0.9.2**, upstream commit
  `25ce62f4d357cd6d29da04300e3fbc352d79daa4`, from v0.7.4
  (`9bd34e1eb9525d9a1fe698aadc964c49c8ff8e7e`). Upstream's Go minimum is
  unchanged (1.27.3). No `replace`, fork, patch or internal import. Verified as
  the latest stable on the [GitHub releases](https://github.com/egoist/mygo/releases)
  (the `native-sqlitecore` tag is not a release of the module).
- Reviewed the v0.8.x and v0.9.x release notes and the `v0.7.4..v0.9.2` diff of
  `ui` (Tester API in `ui/headless.go`, `ui/tester_access.go`). No breaking change
  touched this library: it builds and all existing tests pass unchanged.
- Implemented: no new MCP tool. Existing tools are unchanged.
- New headless Tester capabilities left for a decision, not implemented:
  `Access`/`AccessByRole`/`AccessAll` (accessibility tree; `ui.Role` has no
  string form), `RightClick`/`ClickWith` modifiers, `Menu`/`ChooseMenuItem`,
  `PressButton`/`ReleaseButton`, touch and pen (`TouchDown`, `UsePointer`),
  `Pinch`, `Compose`, `Command`, `KeyRepeat`/`CodeDown`, `UseScreenReader`/`Alerts`/
  `Announcements`, `SetWindowActive`/`SetWindowHidden`, `NewTesterWith`.
- Out of scope: video/webview widgets, window/titlebar and vibrancy, global
  shortcuts, Linux/Windows/macOS platform fixes, updater and packaging, and the
  live-app automation plugin.
- mise tools bumped: betterleaks 1.10.1, hk 2.12.0, pkl 0.33.0, tombi 1.11.2.
  GitHub Actions majors (`jdx/mise-action` v5, `googleapis/release-please-action`
  v5) are newer but were not bumped, since CI cannot be exercised here.

Checks passed:

```sh
go mod verify && go mod tidy -diff
go test -race -p 2 ./...
CGO_ENABLED=0 go test -p 2 ./... && CGO_ENABLED=0 go build -p 2 ./...
go vet ./... && go build -p 2 ./...
go run github.com/egoist/mygo/cmd/mygo@v0.9.2 vet .
mise run check
```

Smoke test: `go run ./examples/headless -listen 127.0.0.1:8080` answered real
MCP HTTP calls to `mygo_sessions`, `mygo_inspect` and `mygo_click_label`
(labels changed to `Message`, `Submit`, `Submitted:`).

## MyGo upgrade — 2026-11-02

Environment: Linux/amd64, Go 1.27.3. The clean baseline at
`6733a2771c6701188da6d463ed78ef41fd8c8fe0` passed `go test ./...` before the upgrade.

- MyGo now resolves to **v0.7.4**, upstream commit
  `9bd34e1eb9525d9a1fe698aadc964c49c8ff8e7e`, from
  `v0.5.6-0.20261020235557-3dfa48145e52`. The module's Go minimum is now 1.27.3,
  matching upstream and the existing mise toolchain.
- Verified the latest stable version on the [MyGo site](https://mygo.egoist.dev/)
  and [GitHub release](https://github.com/egoist/mygo/releases/tag/v0.7.4).
  Reviewed the intervening 0.6.x and 0.7.x changes.
- There is no active `go.work`, module replacement, custom dependency patch,
  pinned MyGo CLI, JavaScript runtime, generated binding, or Onevra dependency.
  The library and example remain headless; no desktop renderer was introduced.
- Existing MCP tools already support the relevant new native APIs.
  `compatibility_test.go` exercises zoomed target bounds and pixel/label clicks
  at fractional screenshot scale, plus exact-decimal typing, keyboard stepping,
  and invalid-input preservation through real SDK HTTP calls. No production API
  or demo change was needed.
- Ready-to-show windows, webview scrolling, themed previews, destructive dialogs,
  Markdown anchors, updater state, and custom scrollbar controls have no matching
  flow in this headless library. The upstream live-app automation plugin also
  falls outside its isolated-fixture contract.

Checks passed:

```sh
go test -p 2 ./...
go test -race -p 2 ./...
CGO_ENABLED=0 go test -p 2 ./...
go vet ./...
go build -p 2 ./...
CGO_ENABLED=0 go build -p 2 ./...
go run github.com/egoist/mygo/cmd/mygo@v0.7.4 vet .
go mod tidy -diff
go mod verify
golangci-lint run --fix=false ./...
golangci-lint fmt --diff
hk check --all --check --exclude '.upgrade-validation/**'
actionlint
```

Checks used the pinned installed tools and task-local caches; the temporary cache
directory was excluded from repository linting. An initial full `/tmp` prevented
baseline compilation; relocating task caches resolved it. No application test
regressions remain. Desktop/GPU, macOS and Windows checks were not run: this
repository has no desktop application entry point or frontend build.

## Initial delivery — 2026-10-21

The following records describe the original delivery, before the upgrade above.

Environment: Linux/amd64, Go 1.27.3; checked 2026-10-21.

### Implemented plan

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

### Checks

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

### Not checked / residual risk

No GUI, hardware, cross-platform desktop matrix, external host application's
platform-service integration or size benchmark was run; these are not headless
delivery gates. Screenshots do not promise desktop/GPU parity. There is no hard
deadline for blocking application callbacks: they can hold the single worker and
delay shutdown indefinitely. Use nonblocking handlers or a disposable harness
process. Loopback has no authentication, so local processes can invoke fixture
handlers; use isolated state and test services/data.
