# Verification record

## Stage 1 — native framework seam

Base MyGo: `3dfa48145e523d029ea6959f6e129915eb16499d`.
Driver revision: `145da750b27cfa48879f79238798271b9b7f37ce`.

Passed with Go 1.27.3 on Linux amd64 and `CGO_ENABLED=0`:

- MyGo core and UI tests, including driver event sequence, shortcuts, cancellation,
  closed windows, non-editable target rejection, capture pixel limits and PNG decode.
- Core/UI vet, full `mygo_noinspector` build, macOS and Windows core/UI cross-builds.
- Stripped native counter size: 9,347,335 bytes both before and after the seam.

Upstream PR creation was attempted and refused by GitHub integration permissions.
The reviewable change is [fork PR #1](https://github.com/aaronflorey/mygo/pull/1).

## Stage 2 — package

Passed unit/SDK HTTP tests and repository commit hooks. Tests cover five schemas,
ordinary tool errors, unknown handles, invalid keys/text, stale geometry and scale,
non-finite/out-of-image coordinates, fractional image rounding, bounded FIFO
execution, cancellation, shutdown, token-free HTTP, hostile Host/Origin headers
and oversized request bodies. These are not fake-backend screenshot evidence.

## Stage 3 — real native application

On 2026-10-21, ran `examples/native` on the sandbox's actual X11 desktop with
the tagged MCP integration, then connected the official SDK v1.8.0 HTTP client:

1. Listed two independent live native windows.
2. Captured and decoded real CPU-rendered PNGs (638×445 content pixels at scale 1;
   OS chrome accounts for the difference from the requested 640×480 window).
3. Clicked Open form, observed a different scene, then clicked the input.
4. Typed `Hello 世界 🌍`, sent Primary+A, and typed `Selected café 世界`.
5. Sent Primary+Enter through the application's ordinary shortcut handler.
6. Captured the result: manually verified visible `Result: Selected café 世界`
   and application stdout `Submitted: Selected café 世界`.
7. Captured the second window, clicked its navigation, and captured its form.
8. Used CUA window discovery and screenshots to verify the actual desktop UI.

`integration/TestNativeFlow` passed. Native screenshot evidence is in
`evidence/result.png` and `evidence/second-window.png`. The integration test
validates PNG decoding, metadata dimensions and changed scenes; the visible
expected text and stdout were checked separately, not inferred from a PNG hash.

The example uses MyGo's normal UI components, adapted from the native counter;
there are no replacement widgets or testing-only input handlers. Without the
`mygo_mcp` build tag it does not import this package.

Limitations: no GPU was available, so native presentation and capture used CPU
rendering. IBUS reported that no input-method service was running; committed
Unicode text worked, but IME composition is outside scope. The flow's fixed
coordinates assume scale 1 and the example layout; it is a reproducible smoke
test, not a selector API. macOS/Windows GUI execution is unavailable on this
Linux machine.

## Stage 4 — edge cases

Updated the pinned framework to `df8d39b4a28fc83e09cea4a467962cab2caece36`
(additional tests, unchanged driver implementation). Its focused tests pass for:

- Unicode selection replacement, disabled button suppression, Tab traversal past
  a disabled control, and Shift+Tab returning to the editor.
- Cancellation during key-down still delivering key-up and cleared modifiers.
- Resize epochs rejecting a previous geometry snapshot, even with unchanged size.
- Closing a window during shortcut handling resolving the pending action.
- Nested pending `Window.Update` calls becoming visible to the scene fence.
- A view scheduling continuous timed updates, and capture while minimized under
  the framework's fake surface (not a claim about a native OS minimized window).

The package tests passed ten consecutive runs. A queue test was made deterministic:
it now inserts a cancelled queued command before filling the remaining capacity,
rather than racing cancellation against the queue fill. HTTP cleanup uses a
10-second deadline, accommodating net/http's delayed connection teardown for an
oversized unread request body, rather than a too-short one-second deadline.

Ran `TestNativeResize` against the real desktop app: CUA changed the native window
frame, the SDK client observed changed content geometry, rejected a click using
the pre-resize screenshot, captured again, and accepted the fresh coordinates.
No sleep-based synchronization was used. Background-window capture also worked
in stage 3 while the second window covered the first.

Not verified on hardware: DPI/monitor changes, native OS minimization, native
shutdown during a platform event, or GPU animations. Scale/revision invalidation,
minimized capture and close/cancellation are covered by focused checks, not
misrepresented as native hardware evidence. The desktop provides one scale-1
X11 display and no GPU; macOS/Windows runtimes are not present.

## Stage 5 — builds and production isolation

Passed on Linux amd64 with Go 1.27.3:

- `CGO_ENABLED=0 go test ./...`, vet with default and
  `mygo_mcp,mygo_noinspector` tags, production-tag build.
- Tagged package/example cross-builds for Linux, macOS and Windows.
- Repository lint, including the tagged integration build.
- Race detector package tests (`CGO_ENABLED=1` only for Go's race instrumentation;
  production builds above remain pure Go with `CGO_ENABLED=0`).
- Final upstream size comparison with identical release flags:

  | Uninstrumented app | Base revision | Driver revision |
  | --- | ---: | ---: |
  | Native counter | 9,347,335 bytes | 9,347,335 bytes |
  | Hello webview | 9,433,351 bytes | 9,433,351 bytes |

`scripts/check-builds.sh` repeats the supported checks and asserts that the
default native example neither imports the MCP package/SDK nor records the SDK
in its compiled module metadata. The enabled build must contain the SDK. CI runs
that script without needing a desktop.

Example production binaries: 9,355,527 bytes without MCP and 13,349,127 bytes
with MCP. Tagged-build lint found an init function in the development example;
it was removed in favor of build-selected enabled/disabled startup functions.
Importing the package still never opens a listener.

These are compilation checks, not macOS/Windows runtime certification. GPU
framebuffer parity, cross-platform GUI execution and hardware benchmarks were
not run because the available machine is a Linux software-rendered desktop;
the plan makes those unavailable checks non-blocking.

## Stage 6 — documentation and pinned delivery

The README now includes existing-app lifecycle integration, enabled/disabled
build files, a pinned install command, local HTTP client configuration, every
tool's argument/result contract, coordinate and stale-handle rules, resource
bounds, cancellation semantics and the complete native interaction flow.

No development-only filesystem replacement remains. The official SDK is pinned
at v1.8.0 and MyGo's remote fork at
`v0.0.0-20261021030745-df8d39b4a28f`, with go.sum committed. Upstream acceptance
cannot be claimed: submission to egoist/mygo was attempted and denied by GitHub
integration permissions. Removing the remote fork replacement therefore remains
conditional on upstream acceptance; downstream apps must copy it explicitly.

Verified the documented install from a separate consumer module with no local
path replacements: fetched the pinned package commit, copied the native example
files, ran tidy, and built both default and MCP-enabled binaries. The first
unscoped fetch failed at the public checksum service because this repository is
private. Using the narrowly scoped `GOPRIVATE` setting and existing GitHub Git
credentials succeeded; the README includes that requirement. No credentials or
token values are needed in the documentation or MCP client configuration.

Final recheck: rebuilt the example against the remote pinned dependency and
build-selected startup helpers, reran default tests and tagged lint, then reran
the complete native SDK flow successfully on fresh desktop windows. The final
application again printed `Submitted: Selected café 世界`.

### Review order

1. [Native framework seam](https://github.com/aaronflorey/mygo/pull/1).
2. [Embedded MCP package](https://github.com/auron-labs/mygo-mcp/pull/1).
3. [Native example and full interaction](https://github.com/auron-labs/mygo-mcp/pull/2).
4. [Edge-case verification](https://github.com/auron-labs/mygo-mcp/pull/3).
5. [Builds and production isolation](https://github.com/auron-labs/mygo-mcp/pull/4).
6. Final documentation: the `docs/integration-guide` PR.

The MCP PRs are stacked on the preceding stage to keep each review limited to
its own changes. Merge in that order and retarget each next PR to main after its
base merges. No PR was merged automatically. All six delivery steps were
attempted; unavailable verification and the upstream-publication exception are
recorded above rather than hidden or treated as proof of defects.
