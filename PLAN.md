# MyGo MCP implementation plan

Build an opt-in **Go package** that lets an MCP agent operate a running MyGo project's **native UI**: discover its windows, take screenshots, click to navigate, type into the focused control, and send keys with modifiers. Reuse MyGo's window lifecycle, capture, input routing, focus handling, and shortcut machinery. Keep MCP transport and dependencies in this repository.

The target application imports and starts the package in a development or test build. The agent connects to that application and tests its actual view and application state. A small upstream MyGo addition is required because the current public API does not expose input injection into live native windows.

Scope follows the requested native UI first approach. The initial keybind contract covers native UI controls and `Context.Shortcut`; native OS menu accelerators require the separate platform work described below. All proposed API and file names are provisional. Source findings refer to the local `.references/mygo` snapshot inspected on 2026-10-21; it has no Git metadata, so pin a real upstream revision before implementation.

## Existing behavior to reuse

| Requirement | Existing MyGo implementation | Planned use |
| --- | --- | --- |
| Application startup | `App.WhenReady`, `App.Run` in [app.go](.references/mygo/app.go) | Start the endpoint after readiness; keep the application event loop on the main goroutine. |
| Window discovery | `Windows`, `FocusedWindow`, `App.OnWindowCreated`, `Window.OnClosed` in [window.go](.references/mygo/window.go) and [app.go](.references/mygo/app.go) | Track supported live windows and invalidate their IDs on close. |
| Screenshots | `Window.CapturePage` in [window.go](.references/mygo/window.go), native capture in [content.go](.references/mygo/content.go) and [ui/window.go](.references/mygo/ui/window.go) | Return the existing PNG through MCP. Native capture rasterizes the completed MyGo scene on the CPU. |
| Live input | `windowHandler.SurfaceEvent` in [content.go](.references/mygo/content.go), `windowHost.event` in [ui/window.go](.references/mygo/ui/window.go), engine routing in [ui/input.go](.references/mygo/ui/input.go) | Expose a narrow opt-in public entry point into the existing path. These entry points and event types are currently internal. |
| Keys and modifiers | Public definitions in [ui/keys.go](.references/mygo/ui/keys.go), internal platform definitions and accelerator parser | Keep the same key vocabulary and modifier semantics; preserve package dependency direction. |
| Interaction tests | `ui.Tester` in [ui/headless.go](.references/mygo/ui/headless.go) | Reuse for focused tests and event-sequence reference. It constructs a separate headless runtime and cannot control a live window. |
| Optional real GUI checks | [internal/e2e](.references/mygo/internal/e2e) and [.github/workflows/ci.yml](.references/mygo/.github/workflows/ci.yml) | Reuse the existing desktop test setup when the implementing agent's machine supports it. |

Useful source anchors: `Window.CapturePage` at `window.go:1296`, live dispatch at `content.go:225`, native attachment at `ui/window.go:75`, native capture at `ui/window.go:537`, keys at `ui/keys.go:11`, and the content/window connection at `internal/surface/surface.go:15`.

## Package and integration design

Use one embedded MCP server with **Streamable HTTP on loopback, without authentication**. The application starts it explicitly from `App.WhenReady`; the agent's MCP configuration supplies only its URL. The package adds no authorization prompts or per-call approvals. This fits an already-running desktop application and keeps ordinary application stdout separate from protocol messages.

Use `github.com/modelcontextprotocol/go-sdk/mcp` for tools, schemas, transport, cancellation, and image results. Pin a compatible release in `go.mod` and commit `go.sum`. Use the SDK's HTTP handler plus Go's `net/http`; build no custom JSON-RPC layer. [Official Go SDK transport documentation](https://go.sdk.modelcontextprotocol.io/protocol/).

Keep the public package surface to a constructor/start operation and context-driven shutdown. Its configuration needs only a loopback address. The address is explicit so the developer can put the same endpoint in the agent configuration and run multiple projects on different ports. A tagged application integration file, such as `mcp_dev.go` with `//go:build mygo_mcp`, keeps the import and server startup out of ordinary builds. Starting an endpoint must never happen merely by importing the package.

The initial layout should stay small:

- `server.go`: exported lifecycle, HTTP endpoint, tool registration, and shutdown.
- `windows.go`: supported-window registry, immutable IDs, and close handling.
- `tools.go`: the five tool handlers, validation, and serialized access to the native driver.
- Focused `*_test.go` files; reuse a native example for optional GUI checks.
- `README.md`: application integration, MCP client configuration, tool semantics, and supported MyGo revision.

Keep all native runtime access behind MyGo's public API. The external package cannot import `github.com/egoist/mygo/internal/...`. Implement the necessary framework seam in a writable MyGo checkout and upstream it; use a temporary local module replacement during development. Do not modify the cached `.references/mygo` snapshot or publish a dependency on its local path.

## MCP tools

Every tool targets only the application hosting this server. Window IDs are opaque, stable for that window's lifetime, and never reused within a server run. A restarted server must reject previous IDs. Input and capture calls require an explicit `window_id`.

| Tool | Input | Result and semantics |
| --- | --- | --- |
| `mygo_windows` | None | Supported native windows with ID, title, readiness, focus, content dimensions, and supported operations. Discover newly created windows and remove closed ones. |
| `mygo_screenshot` | `window_id` | A PNG image content block plus content dimensions, image dimensions, scale, and a screenshot ID. Capture a fresh completed scene. |
| `mygo_click` | `window_id`, `screenshot_id`, `x`, `y` | One left click at coordinates in that screenshot's pixels, translated to content DIPs by the server. Return after the resulting scene has been built. |
| `mygo_type` | `window_id`, `text` | Commit Unicode text into the currently focused editable native control. Preserve normal selection replacement and change notifications. This is text insertion, not a simulation of an IME session. |
| `mygo_key` | `window_id`, `key`, optional `modifiers` | One complete key press and release through native UI routing. Support named navigation/editing keys, letters, digits, and function keys represented by MyGo. Support Shift, Control, Alt, Meta, and the platform's primary shortcut modifier. |

Image bytes belong in the SDK's `ImageContent` with `image/png`; the SDK handles wire encoding. Return metadata as structured content and a concise text representation for clients that need it. [Official Go SDK tool results](https://go.sdk.modelcontextprotocol.io/server/#tool-result-content).

Keep only the latest screenshot geometry and ID per window. A click must match that ID and the current content size and scale; reject old IDs after a newer screenshot, resize, DPI change, or window replacement. Reject non-finite and out-of-image coordinates. Geometry validation cannot guarantee that an animated control has remained in the same position; the agent should observe again after navigation or asynchronous changes.

For a native surface, derive DIP coordinates using the actual captured image dimensions and content dimensions, including scaling/rounding, rather than assuming one pixel per DIP. Store those dimensions together on the UI thread for the captured scene. Do not accept desktop-global coordinates.

Use ordinary MCP tool errors for closed windows, unsupported content, invalid keys, unfocused/non-editable text targets, stale screenshots, cancellation, and capture failures. Avoid arbitrary evaluation, application-state mutation, or shell tools: navigation should exercise the application's normal handlers.

## Required MyGo changes

**1. Add an opt-in native input driver.** Add a narrow constructor or equivalent public entry point in `package mygo`, tentatively `NewInputDriver(window)`. It should support click, committed text, complete key presses, and a frame fence. Keep the implementation adjacent to `content.go`, where the window-to-content connection already belongs.

Translate public input values to the existing internal `platform.SurfaceEvent` representation and deliver through `windowHandler.SurfaceEvent`. Let MyGo continue owning hit testing, focus, disabled controls, clipping, selection, and `Context.Shortcut` processing. Do not invoke button callbacks directly or create a second widget tree.

Reuse the existing key/modifier definitions and internal conversion/parser code where they fit. The public API must not force callers to import internal packages or add a dependency from core `mygo` to the entire `ui` package. Choose public input types following the existing `ui.Key` to platform-key mapping; keep any MCP string-to-key lookup small and exhaustive. No new shortcut registry is needed.

Click delivery must include pointer positioning and a balanced press/release sequence. Text delivery uses the existing committed-text path. Keys must use balanced press/release and modifier state, without converting printable keys into text implicitly. Validate that the target is a live supported native content window and ready to receive input. Establish target window focus through MyGo's existing focus path without changing the focused control except as the action requires.

**2. Add completion at the native scene boundary.** Input dispatch currently has no completion acknowledgment. `Window.Update` and `Invalidate` schedule work asynchronously. Native `capture()` normally rasterizes `h.last`, so calling capture immediately after scheduling input can return the previous scene.

Extend the existing `internal/surface.Conn` and `ui.windowHost` connection with the smallest completion hook needed. Arm completion before dispatching the input, request the next frame, and signal when a scene reflecting the dispatched sequence has completed its build and become the capture source. The relevant owners are [internal/surface/surface.go](.references/mygo/internal/surface/surface.go), [ui/window.go](.references/mygo/ui/window.go), and `engine.runFrame` in [ui/runtime.go](.references/mygo/ui/runtime.go).

Completion means a fresh scene for the action is available; it does not mean the application is idle or that every asynchronous task has finished. Account for callbacks that schedule another build before their result is reflected. Reuse MyGo's existing input/frame scheduling to reach that scene, without duplicating its render loop or waiting for perpetual animations to stop. Screenshot requests also use the fence so pending changes are reflected before capture.

Keep waits in MCP worker goroutines. The UI thread dispatches input and sends completion; it never blocks waiting for a frame that requires itself. Closing a window or shutting down the application must resolve pending work with an error. If a window cannot currently produce a frame, return a bounded not-ready error rather than hanging. A cancelled action already dispatched cannot be rolled back; finish releasing its keys/buttons and never replay it automatically.

**3. Preserve MyGo's dependency and build invariants.** Keep the driver opt-in, with no MCP import or automatic server branch in normal app startup. Reuse the current pure-Go event path; no cgo or OS automation dependency is necessary for native content input. Any internal hook should carry no active work when unused. Keep disabled builds free of MCP code; check builds and size baselines when the local environment supports them. Follow the architecture in [AGENTS.md](.references/mygo/AGENTS.md) and [architecture.md](.references/mygo/docs/architecture.md); the user's best-effort verification policy below takes precedence over mandatory check lists.

## Concurrency and endpoint behavior

Serialize input and capture operations through one application-level command queue. This prevents two MCP requests from interleaving a key chord, changing focus mid-action, or taking a screenshot before the preceding action completes. Discovering windows may read a safely marshalled snapshot. Validate the window again when executing a queued command because it may have closed while waiting.

Bound request bodies, text length, queued commands, and capture dimensions/PNG size. Enforce capture allocation limits before rasterizing. Use request contexts and cancellation, with deadlines guarding hangs; never use fixed sleeps as a synchronization mechanism. Keep the queue lock off the UI thread and release resources on shutdown. Shut down HTTP and wait for workers after `App.Run` returns, rather than blocking inside MyGo's synchronous quit callback.

Bind only to an explicit loopback IP address, such as `127.0.0.1` or `::1`; reject wildcard and non-loopback listeners. Use no tokens, login, or authorization middleware. Retain the SDK's localhost Host validation and reject unexpected browser origins to prevent a remote website from reaching the local control endpoint. Do not log typed text. Any local process that can reach the endpoint can control the enabled application; that is the intended access model. [Go SDK HTTP implementation](https://github.com/modelcontextprotocol/go-sdk/blob/main/mcp/streamable.go).

Use the SDK's supported stateless HTTP configuration for current protocol compatibility; window and screenshot state belongs to the application and its explicit handles. Check the chosen SDK/client combination when an integration test can run locally. [Go SDK transport configuration](https://go.sdk.modelcontextprotocol.io/protocol/#stateless-mode).

## Delivery sequence and optional verification

Verification is best effort and must not block the implementing agent because its machine lacks a desktop session, platform runtime, toolchain, or test dependency. Run relevant checks that are available locally; skip unavailable checks and report the omissions briefly. Do not require a GUI run, a cross-platform matrix, hardware access, or benchmark results to complete the work, and do not ask the user to provision an environment just to satisfy this plan. Fix defects found by checks that do run; unavailable verification is not evidence of a defect.

1. **Implement the upstream native seam.** In a writable MyGo checkout, connect live input and scene completion to the existing native runtime. Add focused checks for event ordering and pending-frame behavior, reusing `ui.Tester` where appropriate. If a native example runs on the agent's machine, exercise click, text, a modified key press, and `CapturePage`. If it cannot run, continue with the MCP package without a GUI feasibility gate.

2. **Implement the Go MCP package.** Add the official SDK, lifecycle, window registry, serialized execution, and the five tools. Use the real native driver directly; introduce no generic automation-provider framework. Add focused tests for schemas, unknown/closed window IDs, invalid input, stale screenshot geometry, cancellation, queue ordering, and shutdown. Where runnable, use `httptest` and an SDK client to check token-free requests and loopback/origin restrictions.

3. **Optionally exercise one complete agent interaction.** If the local machine supports native GUI execution, reuse or adapt a native example with navigation, a text field, a submit button, a shortcut, and an observable result. Retain existing application components, including Onevra where used; MCP should require no replacement widgets or testing-only event handlers. Through an MCP client: list windows → screenshot → click navigation → screenshot → click the field → type Unicode → send a selection/editing chord → submit via a key or click → screenshot the changed result. Check the expected state and visible output, decode the PNG, and exercise a second window when available. Skip this step if the environment cannot run it.

4. **Check available edge cases.** Where the environment permits, check resize/DPI invalidation, disabled controls, focus traversal, text selection, balanced modifiers after cancellation, close during a pending action, background/minimized window behavior, concurrent calls, and an animating view that still completes actions. Use condition/event waits and MyGo's test helpers; no sleep-based assertions. Fake backend PNG placeholders cannot prove screenshot correctness. Unavailable checks do not hold up delivery.

5. **Run locally supported build checks.** Use `CGO_ENABLED=0`, `mygo_noinspector`, production-isolation checks, cross-compilation, and size comparisons only where the toolchain and dependencies are available. A macOS/Linux/Windows GUI matrix is not a delivery requirement. Keep the implementation independent of the visual inspector and distinguish intended platform support from platforms actually tested.

6. **Document and pin the result.** Publish integration instructions, the supported MyGo revision, endpoint URL configuration, the five tool schemas, coordinate rules, and the example interaction flow. Replace any development-only module replacement with the released/pinned upstream dependency. Complete delivery when the implementation and documentation are finished, reporting checks run and checks skipped without treating missing verification as unfinished work.

## Explicit boundaries and later work

- Native screenshots are CPU-rendered MyGo content. They do not prove GPU framebuffer parity or include OS window chrome. A native window containing embedded system webviews has capture holes; such mixed windows are outside the initial support contract. Use `WebView.CapturePage` only when a later design defines how to represent or compose those surfaces.
- Native UI shortcut handling is included. OS menu accelerators are matched by the Cocoa, GTK, and Windows menu backends, outside `SurfaceEvent`. Supporting them needs an opt-in entry point into each platform's existing key dispatch, with its precedence and disabled-item behavior preserved. Do not build a parallel menu matcher in MCP. See [Windows surface key dispatch](.references/mygo/internal/windows/surface.go), [Linux menus](.references/mygo/internal/linux/menu.go), and [macOS menus](.references/mygo/internal/darwin/menu.go).
- Webview windows, browser/WASM targets, OS dialogs, IME composition, global shortcuts, and arbitrary desktop control are later scopes. JavaScript-generated web events are synthetic and cannot establish full native-input parity.
- Begin with screenshot-guided coordinate navigation. Add semantic inspection/selectors only when a real test needs them, drawing from MyGo's existing accessibility tree with unambiguous identity and visibility rules. A visual inspector or `Tester.Find` by first matching label is insufficient for that contract.
- Defer a standalone launcher, stdio bridge, remote hosting, recordings, custom assertions, and automatic project rewriting. The developer starts the instrumented project with its usual workflow; the agent operates it through MCP.

This is an implementation plan, not a completed package. Four GPT-6 Luna scouts reviewed native input, capture/platforms, lifecycle/integration, and webviews. Source inspection and official SDK documentation support the design. Runtime checks are optional according to the implementing agent's available environment; report what ran without making unavailable verification a completion gate.
