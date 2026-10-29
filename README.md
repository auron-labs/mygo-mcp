# MyGo MCP

Give an MCP client a way to inspect, click, type into, and capture screenshots of
your MyGo views. Register isolated headless fixtures that reuse your application's
views and handlers, then exercise them without opening a desktop window.

MyGo MCP is an opt-in Go library. Importing it starts nothing, and it does not
discover or control running desktop applications.

## Install

Requires **Go 1.27.1 or later** and a client that supports MCP Streamable HTTP.
Linux headless operation requires neither a desktop session nor CGO.

From your application's Go module:

```sh
go get github.com/auron-labs/mygo-mcp@latest
```

This installs the latest released module version. See [releases](https://github.com/auron-labs/mygo-mcp/releases)
for version history; pin a specific tag in `go get` when you need reproducible setup.
If accessing a private copy, configure Git authentication and include
`github.com/auron-labs/mygo-mcp` in your existing `GOPRIVATE` setting first.

## Start a harness

Create a separate development entry point, such as `cmd/harness/main.go`.
This complete example exposes an editable field and a button:

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"

    mygomcp "github.com/auron-labs/mygo-mcp"
    "github.com/egoist/mygo/ui"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()

    fixture := mygomcp.Fixture{
        Name: "Greeting", Width: 800, Height: 600,
        Factory: func() (func(*ui.Context), func(), error) {
            var name, greeting string
            view := func(c *ui.Context) {
                ui.Column(c).Children(func() {
                    ui.TextInput(c, &name).Label("Name")
                    if ui.Button(c, "Greet").Clicked() {
                        greeting = "Hello, " + name
                    }
                    ui.Text(c, greeting)
                })
            }
            return view, nil, nil
        },
    }
    if err := mygomcp.Run(ctx, "127.0.0.1:8080", []mygomcp.Fixture{fixture}); err != nil {
        log.Fatal(err)
    }
}
```

Run it from your module root:

```sh
go run ./cmd/harness
```

Configure your MCP client to connect using **Streamable HTTP** to
`http://127.0.0.1:8080/mcp`. Run the client on the same machine as the harness.
Stop the harness with Ctrl+C.

In an existing application, replace the example view with your real view and
construct independent seeded state inside `Factory`. Return its cleanup function
alongside the view when needed. Keep the harness outside the normal application
entry point, and keep mutable state separate from desktop windows.

## Use it

Ask your connected agent to list fixtures, inspect the Greeting fixture, focus
the Name field, type a name, click Greet, and verify the greeting in a screenshot.
The tools support:

- **Discovery and inspection:** `mygo_sessions`, `mygo_inspect`, `mygo_screenshot`.
- **Interaction:** `mygo_click_label`, `mygo_click`, `mygo_type`, `mygo_key`, `mygo_scroll`.
- **Simulated time:** `mygo_advance`, `mygo_wait_text`.

Screenshots contain PNG images. Label clicks use exact text or explicit MyGo
labels; coordinate clicks use image pixels. Actions invalidate the current
observation, so inspect again before targeting another action, or request
`inspect: true` with the action for resulting targets and a fresh observation ID.
Use `screenshot: true` when you also need an image (the `inspect` option is
available in the development checkout).

Give your agent the [agent guide](docs/AGENT_GUIDE.md) for tool inputs, a working
loop, observation rules, error recovery, timing, and integration details.

## Configuration and limits

`Run` takes a context, an explicit loopback IP and port, and 1–16 fixtures.
Use a different port for each harness; `[::1]:8080` also works. Fixture dimensions
are in DIPs, and `Scale` defaults to 1. Geometry stays fixed for the run.
Cancel the context to stop the server and run fixture cleanup.

The endpoint has no authentication. Local processes can invoke your fixtures'
ordinary handlers, so use test data and test services. Keep it on loopback and
do not expose it through a proxy or public port forward.

Headless views cannot exercise OS dialogs, system webviews, desktop focus, or
other native window behavior. Screenshots use software rendering. Views requiring
platform services need explicit adaptation in the harness.

## Troubleshooting

- **Connection fails:** start the harness first, check its address and port, and
  select Streamable HTTP in your client. This package does not provide a stdio server.
- **Listener rejected:** use an explicit loopback IP, such as `127.0.0.1`, rather
  than `localhost`, a wildcard, or a non-loopback address.
- **Stale observation:** inspect again and use the new observation ID. A reconnect
  preserves fixture state; restart the harness for a fresh run.
- **Text does not change:** focus the editable field before typing and verify the
  next observation. Successful delivery does not prove a handler accepted input.
- **Delayed UI stays unchanged:** use `mygo_wait_text` or `mygo_advance` for tester
  timers. Wall-clock sleeps do not advance simulated time.

// refused_by: modify_delete_guard · collision: none (no common ancestor text)
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
| `mygo_click` | `observation_id` or `screenshot_id`, `x`, `y`, optional `inspect` and `screenshot` | One complete left click in the observation's image pixels |
| `mygo_click_label` | `observation_id` or `screenshot_id`, `label`, optional `inspect` and `screenshot` | Click the center of one exact text or explicit MyGo label |
| `mygo_scroll` | `observation_id` or `screenshot_id`, `x`, `y`, `delta_x`, `delta_y`, optional `inspect` and `screenshot` | Immediate scrolling at an image pixel point, with deltas in DIPs |
| `mygo_type` | `text`, optional `inspect` and `screenshot` | Commit valid UTF-8 through normal editable-focus/selection handling |
| `mygo_key` | `key`, optional `modifiers` array, `inspect`, and `screenshot` | Complete press/release chord |
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

### Inspect after an action without a PNG

In the development checkout, set `inspect: true` on `mygo_click`,
`mygo_click_label`, `mygo_type`, `mygo_key`, or `mygo_scroll` to receive
the resulting labels, targets, dimensions, and a fresh `observation_id` in
the same call, without a PNG:

```json
{"session_id":"<session>","observation_id":"<latest observation>","label":"Edit","inspect":true}
```

Pass this to `mygo_click_label`, then use the returned observation ID for the
next targeted action. This avoids a separate inspection call when visual
verification is unnecessary. Results include `delivered: true`; verify labels
and targets to check the outcome. Inspection does not advance simulated time.
If both `inspect` and `screenshot` are true, the result also includes a PNG.
Both default to false, returning delivery status only. If observation capture
fails after input, the error says input was delivered; do not replay it.

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

// refused_by: modify_delete_guard · collision: none (no common ancestor text)
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

// refused_by: modify_delete_guard · collision: none (no common ancestor text)
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

From a trusted checkout, use the toolchain pinned in [mise.toml](mise.toml):

```sh
mise trust
mise install
mise exec -- hk install --mise
mise run check
mise run test
mise run build
```

Use `mise run format` to apply formatting and lint fixes. To try the included
demo, run `go run ./examples/headless -listen 127.0.0.1:8080`.
See [verification notes](VERIFICATION.md) for recorded checks.
Maintainers can follow the [release guide](docs/RELEASING.md) to publish versions.
// refused_by: modify_delete_guard · collision: none (no common ancestor text)
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
