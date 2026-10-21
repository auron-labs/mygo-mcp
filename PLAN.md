# MyGo MCP implementation plan

Build an opt-in **Go package** that lets an MCP agent exercise a project's native MyGo views through the **public headless `ui.Tester` API**. Support registered test sessions, screenshots, coordinate clicks, text input, key chords, and simulated time. Reuse application views and their normal handlers with isolated test state.

**No MyGo patch or fork is required or permitted.** Depend on an unmodified revision of `github.com/egoist/mygo` and use only exported APIs. Do not bypass this constraint with internal imports, unsafe access, reflection into internals, `go:linkname`, copied framework internals, or module replacements.

This supersedes the live-window plan and its required upstream input driver. The user chose headless testing over OS automation: the server operates an in-memory UI instance, not the running desktop application's windows or state. This checkout currently contains a plan, not an implemented MCP package.

## Existing behavior to reuse

The exported constructor is in `ui/api_gen.go`; tester behavior is in `ui/headless.go`. Verified against unmodified upstream commit `3dfa48145e523d029ea6959f6e129915eb16499d`, the base of the discarded fork PR. Pin that revision, or another explicitly verified unmodified upstream revision, during implementation. The read-only `.references/mygo` snapshot is reference material, not a dependency path.

| Requirement | Public API | Planned use |
| --- | --- | --- |
| Construct a headless view | `ui.NewTester(view, width, height)` | Construct each registered session once with persistent, independent state. |
| Screenshot | `Tester.Frame()`, `Tester.Image()` | Build current state and encode the RGBA image using standard-library `image/png`. |
| Coordinate click | `Tester.ClickAt(x, y)` | Translate screenshot pixels to DIPs; retain normal hit testing and handlers. |
| Committed text | `Tester.Type(text)` | Deliver Unicode through the existing focused-control input path. |
| Complete key chord | `Tester.Key(mods, key)`, `ui.Key`, `ui.Modifiers`, `ui.Cmd` | Reuse press/release and shortcut handling. |
| Timers and animations | `Tester.Advance(duration)`, `Tester.Now()` | Advance simulated time explicitly. |
| Frame status | `Tester.FramePending()` | Report pending frames without claiming application idleness. |
| Focus and geometry | `Tester.SetFocused`, `Tester.SetSize`, `Tester.SetScale` | Initialize focus and bounded geometry. |
| Integration assertions | `Tester.HasText`, `Tester.Texts`, fixture state | Verify behavior without adding MCP assertion tools. |

Sources: [public constructor](https://github.com/egoist/mygo/blob/3dfa48145e523d029ea6959f6e129915eb16499d/ui/api_gen.go), [headless implementation](https://github.com/egoist/mygo/blob/3dfa48145e523d029ea6959f6e129915eb16499d/ui/headless.go).

## Package and integration

Use an embedded MCP server with **Streamable HTTP on an explicit loopback IP, without authentication**. Keep transport and dependencies in this repository. Use the official `github.com/modelcontextprotocol/go-sdk/mcp` SDK for tools, schemas, transport, cancellation, and image results, plus `net/http` for lifecycle. Pin compatible versions and commit `go.sum`.

The developer supplies an explicit list of named headless fixtures at startup. Each supplies a view factory, positive width/height in DIPs, and positive finite scale, defaulting to 1. The factory creates isolated session state and returns its `func(*ui.Context)` view. Construct one tester per session and retain it across requests. Never recreate application state inside rendering.

Reuse existing application view and state constructors, components, and normal handlers, including Onevra where used. Fixtures may supply existing test services and seeded data. Do not replace handlers or implement a second version of the view. Do not share mutable state with a running desktop window. Project-specific adaptation of platform-dependent view construction belongs in the host project's harness, not a MyGo fork.

A small project-owned harness starts the server under a context; cancellation shuts it down. It needs no `App.WhenReady`, `App.Run`, or desktop window. Keep integration outside ordinary builds through a separate harness entry point or development build tag. Importing the package never starts a server.

Register fixtures at startup; restart the harness for a clean run. Do not add remote creation, reset, arbitrary execution, or automatic project rewriting. Allow a cleanup callback for fixture-owned workers/resources; call it once after operations finish. The tester has no public disposal method: do not invent an internal one.

Keep the public surface to fixture configuration and context-driven startup/shutdown. Provisional layout:

- `server.go`: lifecycle, SDK registration, HTTP listener, shutdown.
- `sessions.go`: fixtures, construction, immutable IDs, worker ownership, cleanup.
- `tools.go`: validation, coordinate conversion, tester calls, PNG results.
- Focused tests, one minimal harness example, and `README.md`.

Do not retain a dormant live-window backend or introduce a generic automation-provider abstraction.

## MCP tools

Tools target only registered headless sessions. Use `session_id`, not `window_id`. IDs are opaque, stable during a run, never reused, and include a server-run identity to reject IDs from previous runs. Names are descriptive, not identities.

| Tool | Input | Result and semantics |
| --- | --- | --- |
| `mygo_sessions` | None | Registered sessions with ID, name, content dimensions, scale, and operations. Fixture discovery, not OS window discovery. |
| `mygo_screenshot` | `session_id` | Call `Frame()`, encode `Image()` as PNG, return screenshot ID, image/content dimensions, scale, simulated time, and `frame_pending`. |
| `mygo_click` | `session_id`, `screenshot_id`, `x`, `y` | One left click in screenshot pixels, converted to DIPs and delivered by `ClickAt`. |
| `mygo_type` | `session_id`, `text` | Commit Unicode with `Type`, preserving normal selection replacement and change handling. |
| `mygo_key` | `session_id`, `key`, optional `modifiers` | Complete press/release through `Key`. Explicitly map public navigation/editing, letter, digit, and function keys. Support Shift, Control, Alt, Meta, and primary mapped to `ui.Cmd`. |
| `mygo_advance` | `session_id`, `milliseconds` | Advance simulated time by a bounded nonnegative integer duration through `Advance`. Return time and `frame_pending`. |

Use SDK `ImageContent` with `image/png`; the SDK handles wire encoding. Return structured metadata and concise text for clients needing it. No custom JSON-RPC or base64 wrapper.

### Screenshot and coordinate contract

Retain only the latest screenshot ID and geometry per session. Derive DIPs from actual image dimensions and configured content dimensions, including rounding. Never assume one pixel per DIP or accept desktop-global coordinates.

Reject missing, wrong-session, or superseded IDs, non-finite coordinates, and out-of-image points. Validate before converting to `float32`. Invalidate the saved screenshot after every dispatched click, type, key, or time advance, including an action whose caller cancelled after dispatch. Require another screenshot before the next click. New screenshots supersede older IDs.

Geometry is fixed by startup fixtures in the initial tool API. Test non-unit-scale conversion without adding resize tools. A future resize/scale API must validate allocations first and invalidate screenshots.

Render and encode on the owning worker before another operation touches the tester. Treat `Image()` as tester-owned. Bound dimensions, scale, and resulting pixel allocations before `NewTester` or `SetScale`; checking encoded size afterward cannot prevent large allocations. Also bound encoded response size and propagate PNG errors.

### Input and completion contract

Use `ClickAt`, `Type`, and `Key` directly. Do not invoke callbacks, duplicate routing, or introduce the fork's `InputDriver`, flush hook, or geometry epochs. `Key` already sends press/release; printable keys do not implicitly insert text. `Type` commits text without simulating IME composition.

The public `Type` method has no success result and may do nothing without an editable focus target. Report delivery, not acceptance; the agent verifies the screenshot. `TextCaret()` also reflects input-method policy, so it cannot reliably supply a general editable-focus check. Do not require a patch to manufacture that error contract.

Input methods synchronously settle requested frames with an upstream bound of 20 frames. `Frame()` requests another build and settles it; `Image()` alone returns the previous rendered frame. Report `FramePending()`, including true. Never loop until false: an animated or continuously rebuilding view may not settle.

Completion means the public tester operation returned after bounded rendering work. It does not mean all timers, network activity, or background work completed. The tester clock stays frozen until `mygo_advance`. Sleeps and wall-clock passage do not advance it.

Bound each advance to avoid excessive animation work. Reject negative, fractional, non-finite, over-limit, or overflowing durations before converting to `time.Duration`. Consecutive clicks at the same frozen time may count as a double click; document explicit advances for interactions requiring separated clicks. Do not secretly advance time after actions.

## Concurrency and lifecycle

One worker owns construction, tester methods, view callbacks, screenshot encoding, and cleanup. Serialize operations across sessions through a bounded queue; no worker pool is needed. Do not expose the server-owned tester for concurrent use.

Fixture mutable state belongs to this execution path. Background work must synchronize its results and apply them safely before rendering; screenshot requests can call `Frame()` to display newly available state. The test clock does not control goroutines or external services. Prefer deterministic fixtures using existing project test services.

Validate before queueing and again during execution. Return ordinary MCP tool errors for unknown/failed sessions, invalid keys/modifiers, invalid UTF-8, excessive text, stale screenshots, full queues, and shutdown. Do not log typed text. Use documented fixed resource limits instead of configuration for every bound.

Respect cancellation while queued and before dispatch. Once a synchronous tester call starts, the public API cannot safely interrupt it. The caller can stop waiting, but the worker must finish before executing another command. Cancellation cannot undo input; never replay automatically or cancel between parts of a balanced sequence.

Deadlines cannot stop arbitrary blocking application callbacks. Require nonblocking rendering and input handlers, with long work outside them. Do not promise hard execution deadlines or race a second goroutine against the tester. Use a disposable harness process for potentially hanging application code.

On shutdown, stop accepting requests, cancel queued commands, allow in-flight synchronous work to finish, then clean up fixtures. Catch fixture-construction/rendering panics at the worker boundary, mark the affected session failed, reject subsequent operations, and never reuse a tester after a panic. Run cleanup for successfully constructed fixtures; the fixture factory owns cleanup of partially constructed resources if it fails. Preserve useful diagnostics without logging typed text.

## Endpoint behavior

Bind only to explicit loopback IPs such as `127.0.0.1` or `::1`; reject wildcard and non-loopback listeners. An explicit address supports different harnesses on different ports and stable client URL configuration.

Use no tokens, login, authorization middleware, or per-call approvals. Retain SDK localhost Host validation and reject unexpected browser origins. Local processes reaching the endpoint can operate fixtures and trigger application handlers; use test data/services accordingly. Do not add shell, arbitrary evaluation, or application-state mutation tools.

Use the SDK's supported stateless HTTP configuration. Protocol sessions are distinct from headless test sessions: fixture state and screenshot IDs belong to this server run. Verify SDK/client configuration against the pinned version during implementation.

SDK references: [transport](https://go.sdk.modelcontextprotocol.io/protocol/), [tool results](https://go.sdk.modelcontextprotocol.io/server/#tool-result-content), [HTTP implementation](https://github.com/modelcontextprotocol/go-sdk/blob/main/mcp/streamable.go).

## Delivery and verification

1. **Prove the unmodified public API path.** Pin upstream and compile an external-package headless interaction using `ui.NewTester`. Use a real view with navigation, a text field, button, and `Context.Shortcut`. Exercise screenshots, coordinate clicks, Unicode input, selection/replacement, and submission. Decode actual PNGs and verify state and visible output. No fork or desktop session is needed. Adapt project-specific platform dependencies in the harness explicitly.

2. **Implement the MCP package.** Add the SDK, fixtures, worker, queue, six tools, HTTP lifecycle, and cleanup. Test schemas, session isolation/restart IDs, invalid input, stale screenshots after mutation, non-unit scales, allocation limits, cancellation before/after dispatch, queue ordering, shutdown, and panic failure handling. Use real testers for UI behavior and small controlled fixtures for concurrency checks.

3. **Exercise the full headless MCP path.** Use `httptest` and an SDK client: discover sessions → screenshot → click navigation → screenshot → click field → type Unicode → key chord → screenshot → submit → screenshot. Include explicit time advances for delayed visible updates and a continuously requesting view proving bounded completion with `frame_pending`. Decode real images, not placeholders. Check loopback, Host, and browser-origin restrictions.

4. **Document and check dependency isolation.** Add integration/example, client URL configuration, tools, coordinates, clock rules, cancellation limits, and unsupported behavior. Inspect `go.mod`, `go.sum`, module resolution, source imports, and example builds: MyGo must resolve to unmodified upstream without a fork or replacement. Keep MCP imports out of normal host application builds.

Run relevant available Go tests and builds, including `CGO_ENABLED=0` where supported by pinned dependencies. Verification is best effort if the machine lacks a required toolchain/native dependency: report unavailable checks and fix failures in checks that run. A GUI run, hardware access, desktop platform matrix, or size benchmark is not a delivery gate. Headless testing is the product path and should be verified whenever the toolchain is available.

## Boundaries

- No live desktop-window discovery/control, OS automation, desktop permissions, OS menus/dialogs, or OS window focus. Multiple fixtures represent independent views; opening a real window does not register another session.
- Screenshots are software-rendered headless views, without claims of GPU framebuffer or desktop font/environment parity. System webviews and mixed native/webview rendering are unsupported initially.
- Tester time governs its timers/animations, not network work, goroutines, or wall-clock application logic. No claim of full application idleness or desktop fidelity.
- Keep existing components and handlers. Views requiring live platform services need explicit harness adaptation or remain unsupported; never silently replace the view.
- Defer semantic selectors, remote fixture creation/reset, recordings, assertions, launcher/stdio bridge, remote hosting, and automatic project rewriting.
- The [fork PR](https://github.com/aaronflorey/mygo/pull/1) is not a dependency or prerequisite. This plan supersedes its input-driver stage. Do not modify or close that PR without separate instruction.

Delivery means an implemented headless package, example, and documentation against unmodified upstream, with available verification reported. This document is the revised plan and does not claim implementation is complete.
