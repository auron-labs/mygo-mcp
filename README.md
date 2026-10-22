# MyGo MCP

An opt-in Go package for exercising **isolated, headless MyGo views** using MCP.
It does not discover or control desktop windows. Importing it starts nothing.
MyGo is pinned to unmodified upstream commit
`3dfa48145e523d029ea6959f6e129915eb16499d`; no fork, patch, internal imports,
module replacement, reflection, or unsafe access is used.

## Run the example

Requires Go 1.27.1 or later (the pinned MyGo requirement). Go's automatic
toolchain selection can download it. No desktop session or CGO is required on Linux.

```sh
go run ./examples/headless -listen 127.0.0.1:8080
```

Configure an MCP client for **Streamable HTTP** at `http://127.0.0.1:8080/mcp`.
The client must run on the **same machine** as the harness; this is not a hosted
or remote-control endpoint. The address is explicit, so separate harnesses can
use separate ports. IPv6 loopback (`[::1]:8080`) also works. Hostnames, wildcard
addresses and non-loopback IPs are rejected for listeners.

The pinned official `github.com/modelcontextprotocol/go-sdk` v1.8.0 provides
schemas, tool errors, cancellation, image encoding on the wire, and stateless
HTTP. With its Go client, configure:

```go
transport := &mcp.StreamableClientTransport{
    Endpoint: "http://127.0.0.1:8080/mcp",
    DisableStandaloneSSE: true,
    MaxRetries: -1,
}
session, err := client.Connect(ctx, transport, nil)
```

Stateless MCP protocol connections do **not** reset the persistent headless
fixtures. GET/DELETE return 405. There is no authentication: any local process
reaching the port can operate fixtures and invoke their ordinary handlers.
Use test data and test services. The SDK's localhost Host validation is retained;
an additional loopback/localhost Host check and exact same-origin browser check
reject DNS-rebinding and unexpected browser origins. Cross-origin unsafe requests
are also checked by `http.CrossOriginProtection`. Do not put a proxy or public
port forward in front of this endpoint.

## Integrate an existing view

Add the library to your Go module:

```sh
GOPRIVATE=github.com/auron-labs/mygo-mcp go get github.com/auron-labs/mygo-mcp
```

This repository is currently private, so your Go/Git environment needs access to
`github.com/auron-labs/mygo-mcp`. The command scopes `GOPRIVATE` to this invocation;
include this module in your existing `GOPRIVATE` setting for subsequent module
updates. Import it as:

```go
import mygomcp "github.com/auron-labs/mygo-mcp"
```

Put the harness in a separate entry point, or behind a development build tag,
outside the normal application import graph. Reuse the application's view,
components, state constructors, services and normal handlers:

```go
fixture := mygomcp.Fixture{
    Name: "Seeded editor",
    Width: 800, Height: 600, // DIPs, not pixels
    Scale: 1.5,             // omitted/zero means 1
    Factory: func() (func(*ui.Context), func(), error) {
        state := newTestApplicationState() // project-owned constructor/services
        return state.View, state.Close, nil
    },
}
err := mygomcp.Run(ctx, "127.0.0.1:8080", []mygomcp.Fixture{fixture})
```

The constructor and cleanup in that snippet are project-specific, not package
APIs. A complete compilable example is in [`examples/headless`](examples/headless/main.go).
It and the tests share the same [application view](internal/demo/view.go).
It includes navigation, an editable field, button and primary+Enter submission,
and a delayed visible update using tester time.

Factories run once per fixture on the single owning worker. Construct independent
state there, **not in the render function**, and never share mutable state with a
desktop window. Platform-dependent view construction must be adapted explicitly
in the host project's harness; keep existing components (including Onevra where
used) rather than inventing a second view or replacing handlers.

All tester calls, view callbacks, PNG encoding and cleanup run on that worker,
serialized across fixtures. Background work must synchronize its results and apply
them safely before rendering. Prefer deterministic existing test services. A fresh
screenshot requests a build so synchronized new state can appear. Tester time does
not control goroutines, networking, external services or wall-clock application logic.

Cancel `Run`'s context to stop accepting HTTP requests and cancel queued work.
In-flight synchronous operations finish before cleanup runs once; `Run` waits for
that cleanup. Factories that fail or panic own cleanup of partially created
resources. If a factory returns successfully, its cleanup still runs if tester
construction/rendering later panics. MyGo has no public tester disposal API.
Panics fail only the affected fixture permanently; discovery reports the failure
and other fixtures keep working. Panic values are not reported, to avoid exposing
typed text. Restart the harness for a clean run. There is no remote reset/create tool.

## Tools

Every targeted tool requires a `session_id` from `mygo_sessions`. IDs are opaque,
stable for one run, different between runs, and not derived from fixture names.
Tool results include structured metadata and a text representation; screenshots
also contain SDK `ImageContent` with `image/png`, not a custom base64 envelope.

| Tool | Other inputs | Behavior |
| --- | --- | --- |
| `mygo_sessions` | none | IDs, names, DIP dimensions, scale, available operations and fixture failures |
| `mygo_inspect` | none | `Frame()` then texts/labels, match counts, unique match bounds in DIPs, dimensions, simulated time, `frame_pending`, and a fresh `observation_id`; no PNG |
| `mygo_screenshot` | none | Same observation metadata plus PNG and a legacy `screenshot_id` alias |
| `mygo_click` | `observation_id` or `screenshot_id`, `x`, `y`, optional `screenshot` | One complete left click in the observation's image pixels |
| `mygo_click_label` | `observation_id` or `screenshot_id`, `label`, optional `screenshot` | Click the center of one exact text or explicit MyGo label |
| `mygo_scroll` | `observation_id` or `screenshot_id`, `x`, `y`, `delta_x`, `delta_y`, optional `screenshot` | Immediate scrolling at an image pixel point, with deltas in DIPs |
| `mygo_type` | `text`, optional `screenshot` | Commit valid UTF-8 through normal editable-focus/selection handling |
| `mygo_key` | `key`, optional `modifiers` array and `screenshot` | Complete press/release chord |
| `mygo_advance` | integer `milliseconds` | Advance simulated time by 0–1000 ms |
| `mygo_wait_text` | `text`, integer `timeout_ms`, optional `screenshot` | Check rendered texts/labels for a substring, advancing simulated time up to the requested limit; return `matched`, `advanced_ms`, and a fresh observation |

### Inspect without a PNG

Call `mygo_inspect` with `{"session_id":"<session>"}` to observe a fixture without
encoding or transferring a screenshot. Use its `observation_id` to click or
scroll. `labels` lists MyGo's texts/labels in frame order, including duplicates.
`targets` groups these by exact label and returns `matches`, `bounds` for unique
matches, and `center_in_content`. Bounds contain `x`, `y`, `width`, and `height`
in DIPs. They can describe text rather than the full control, and clipping can
produce zero-size bounds. Bounds are omitted for duplicates because MyGo's
public `Find` API exposes only the first match, and for non-finite geometry.

Inspection renders the view and returns actual image dimensions even though it
does not encode a PNG. It does not advance time. For visual verification, use
`mygo_screenshot` or request a screenshot with an action.

### Click by label

Inspect or take a screenshot, then use an entry from its `labels` metadata:

```json
{"session_id":"<session>","observation_id":"<latest observation>","label":"Edit"}
```

Pass this to `mygo_click_label`. Matching is exact and case-sensitive. Missing,
duplicate, empty, and out-of-content targets return tool errors without consuming
the observation. For controls without useful visible text, give the existing view
an explicit label, for example `ui.TextInput(c, &text).Label("Message")`.
The demo uses this label for its editable field.

Labels come from MyGo's last rendered frame, including ordinary text and possibly
offscreen elements; they are not a list of enabled, unobscured buttons. A click
uses normal pointer dispatch at the matched element's center. Delivery does not
guarantee the intended handler ran, so verify the next observation. For ambiguous
labels, use coordinate clicks or give controls distinct labels. Label clicks use
DIPs internally and work independently of screenshot scale.

### Observation IDs and screenshot coordinates

Only the **latest** observation ID is valid for its fixture. Inspection,
screenshots, and text waits each supersede the previous observation. Screenshots
return identical `observation_id` and `screenshot_id` values; either input name
works for clicks and scrolling. If both are supplied, they must agree.

Every dispatched coordinate/label click, scroll, type, key, or advance invalidates
the previous observation, including no-op inputs and actions whose caller
cancelled after dispatch. Inspect or take a fresh screenshot before another
targeted action, or use the fresh observation returned by `screenshot: true`.
Invalid requests do not invalidate it. Wrong-session IDs, stale IDs, non-finite
and out-of-image points are rejected.
`0 <= x < image_width` and `0 <= y < image_height` are required.

Conversion uses actual PNG dimensions, including scale rounding:
`dip_x = x * content_width / image_width` (likewise Y). Never send desktop-global
coordinates or assume one pixel per DIP. Geometry stays fixed for the run; there
are no resize/scale tools.

### Return a screenshot with an action

Set `screenshot: true` on a click, label click, type, key, or scroll request to
receive the resulting PNG and observation metadata, with `delivered: true`, in
the same call. For example, pass this to `mygo_click_label`:

```json
{"session_id":"<session>","observation_id":"<latest observation>","label":"Edit","screenshot":true}
```

The new observation can target the next action. The default is `false`, returning
only delivery status for these input tools. Capture does not advance time or
wait for application idleness. If PNG capture fails after delivery, the error
states that input was delivered; do not replay it.

### Recover from targeting errors

Targeting failures are ordinary MCP tool errors (`isError: true`) with JSON in
both text content and structured content. `code` is one of `not_found`,
`ambiguous`, `offscreen`, `invalid_geometry`, `stale_observation`,
`invalid_observation`, or `out_of_bounds`. `message` describes recovery. Label
lookup errors include `target` with the label, match count, and bounds when
available. Other validation failures remain ordinary SDK tool errors.

For `stale_observation`, inspect again. For `ambiguous`, use distinct labels or
coordinates. For `offscreen`, scroll and inspect again. These errors do not
consume the current valid observation.

### Scroll to controls

Pass an image pixel point inside the scroll container and bounded DIP deltas to
`mygo_scroll`:

```json
{"session_id":"<session>","observation_id":"<latest observation>","x":100,"y":80,"delta_x":0,"delta_y":200,"screenshot":true}
```

Positive `delta_y` scrolls down; negative scrolls up. Each delta must be finite
and between -4096 and 4096 DIPs. Scrolling uses MyGo's immediate touchpad-style
input, so it does not require a clock advance to finish a wheel animation. It
uses normal pointer routing; verify that the intended container scrolled.

### Wait for rendered text

Pass this to `mygo_wait_text` after an action that schedules a visible update:

```json
{"session_id":"<session>","text":"Ready","timeout_ms":500}
```

The tool renders once and checks for a case-sensitive substring in MyGo's
texts/labels. If absent, it advances simulated time in steps of at most 10 ms,
checking after each step until a match or the requested limit. `timeout_ms`
must be an integer from 0 through 1000. The tool never advances beyond that
budget; an existing match returns immediately with `advanced_ms: 0`.

Results include `matched`, the actual `advanced_ms`, and a fresh observation.
A timeout returns `matched: false`, not a tool error. Set `screenshot: true`
to include the final PNG. Matching does not prove visibility: offscreen labels
can match, and explicit labels can replace a control's displayed text in MyGo's
text queries. This advances the tester clock, not wall-clock services,
networking, or background goroutines, and does not guarantee application idleness.

### Input, clock and completion

`Type` delivers committed Unicode, not IME composition. Delivery does **not** mean
acceptance: MyGo may ignore it without an editable focus target. Verify the next
screenshot. Printable `Key` calls do not insert text; use `mygo_type` for that.

Key names are case-insensitive: `enter`, `escape`, `backspace`, `tab`, `space`,
`delete`, `insert`, `home`, `end`, `pageup`, `pagedown`, `left`, `right`, `up`,
`down`, `back`, `forward`, letters `a`–`z`, digits `0`–`9`, `f1`–`f12`, and
`minus`, `equal`, `comma`, `period`, `slash`, `semicolon`, `quote`, `bracketleft`,
`bracketright`, `backslash`, `backquote`, `contextmenu`.
Modifiers are `shift`, `control`, `alt`, `meta` and `primary` (also case-insensitive).
`meta` maps to MyGo Super; `primary` maps to `ui.Cmd` (Command on macOS, Control
elsewhere). For select-all: `{"key":"a","modifiers":["primary"]}`.

Tester time starts at construction and stays frozen until explicit advances.
Wall-clock sleeps do not advance it. Consecutive clicks at the same frozen time
may count as double-clicks; explicitly advance time when separated clicks matter.
Only `mygo_advance` and `mygo_wait_text` advance time, within their explicit budgets.
Advances reject negative, fractional, non-finite,
overflowing or excessive durations before conversion to `time.Duration`.

Completion means the public tester method returned, **not** application idleness.
Input and screenshot rendering use MyGo's bounded settling (up to 20 frames per
settle); `frame_pending` may remain true for animations/continuously rebuilding
views. We never loop waiting for it to become false. An advance runs frames/timers
due within its bounded simulated interval.

Cancellation is respected while queued and before dispatch. Once a synchronous
tester operation starts, it cannot safely be interrupted: the caller can stop
waiting, but the worker finishes before another operation runs. Cancellation does
not undo input; **never automatically replay actions**. Rendering and input
handlers must be nonblocking, with long work outside them. Deadlines cannot stop
arbitrary blocking application callbacks. Use a disposable harness process for
potentially hanging code; such code can also prevent shutdown from completing.

### Fixed resource limits

- 1–16 fixtures; names are nonempty UTF-8, at most 256 bytes (names need not be unique).
- Positive DIP dimensions and actual pixel dimensions up to 4096 per axis.
- Positive finite scale up to 4 (zero defaults to 1); each scaled axis must be at least one pixel.
- At most 4,194,304 pixels per fixture and 16,777,216 across fixtures, budgeting
  scale-1 construction **and** configured scale before allocating testers.
- 32 queued operations across all fixtures; a full queue returns a tool error.
- 65,536 UTF-8 bytes per text, label, or wait query; at most five modifiers per chord.
- 1000 ms per advance or wait; at most 100 clock steps per wait.
- Scroll deltas between -4096 and 4096 DIPs per axis.
- 16 MiB of PNG bytes per screenshot (about 22 MiB after SDK base64/JSON encoding).
  Encoding is bounded while writing and errors propagate.
- 1 MiB HTTP request bodies, valid UTF-8; 16 KiB headers, 5-second header and
  10-second request-read timeouts in `Run`. Malformed/oversized HTTP bodies are
  rejected before SDK decoding. Parsed tool validation failures are ordinary MCP errors.

These limits bound package-owned work, not arbitrary application allocations or
blocking callbacks. Typed text is not logged by this package.

## Development

[mise](https://mise.jdx.dev/) pins the contributor toolchain and linters in
[`mise.toml`](mise.toml). From a trusted checkout:

```sh
mise trust
mise install
mise exec -- hk install --mise
mise run check
mise run test
mise run build
```

`mise run check` checks all tracked files with the existing hk linters and
validates GitHub Actions syntax. Use `mise run format` to apply formatting and
lint fixes. The local hooks check staged changes and require Conventional Commit
messages, such as `fix: reject stale screenshots`.

CI runs on pull requests and pushes to `main`: the module's minimum Go version
owns tests/builds, while mise supplies the pinned lint toolchain. The Linux checks
include race detection and CGO-disabled tests/builds. Dependabot checks Go modules
and GitHub Actions weekly; the verified MyGo commit is updated manually.

There is no tagged release or publishing workflow yet. The headless command is
an example harness; integrate the library into your own development entry point.

### Verification

```sh
go test ./...
go test -race ./...
CGO_ENABLED=0 go test ./...
go vet ./...
go build ./...
CGO_ENABLED=0 go build ./...
go mod verify
```

Tests use real public testers and SDK clients over `httptest` HTTP, decode PNGs,
exercise UI state/visible output, and check isolation, restart IDs, scale/rounding,
validation, stale screenshots, queue ordering/full queues, cancellation,
shutdown/cleanup, panics, timers, continuous frames and endpoint restrictions.
See [`VERIFICATION.md`](VERIFICATION.md) for the recorded delivery checks.

## Not supported

No live-window discovery/control, OS automation, desktop permissions, OS menus or
dialogs, actual window focus, system webviews or mixed native/webview rendering.
Opening a desktop window does not register another fixture. Headless screenshots
are software-rendered and do not claim GPU framebuffer/font/environment parity.
No role/CSS selectors, assertions, recordings, arbitrary evaluation, shell tools,
remote fixture mutation/reset, launcher/stdio bridge, public hosting or automatic
project rewriting. Views requiring live platform services need explicit harness
adaptation or remain unsupported. The old fork PR is neither a dependency nor a
prerequisite and is not modified by this package.
