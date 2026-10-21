# mygo-mcp

Opt-in MCP control of the **hosting application's native MyGo UI**, using the
official Go MCP SDK's stateless Streamable HTTP transport. No shell, JavaScript
evaluation, OS automation, authentication, or automatic startup.

Requires Go 1.27.3 and the native input seam in
[MyGo fork PR #1](https://github.com/aaronflorey/mygo/pull/1), based on upstream
`3dfa48145e523d029ea6959f6e129915eb16499d`. The dependency is pinned to fork commit
`145da750b27cfa48879f79238798271b9b7f37ce`; no filesystem replacement is needed.
An upstream PR was attempted but GitHub denied creation against `egoist/mygo`.
Until upstream accepts the seam, consuming applications must also copy this
repository's **remote** `replace github.com/egoist/mygo` directive into their own
`go.mod` (Go does not inherit dependency replacements).

## Integration

In a development-only file with `//go:build mygo_mcp`, import this package and
call `mygomcp.Start(ctx, "127.0.0.1:7331")` from `mygo.App.WhenReady`. Keep
`App.Run` on the main goroutine. Cancel the context and call `server.Close(ctx)`
**after** `App.Run` returns, not inside a synchronous quit callback.

Configure the client with URL `http://127.0.0.1:7331/mcp`. Do not publicly expose
this endpoint: **any local process can control the instrumented application**.
Only explicit loopback IP listeners are accepted. The SDK's localhost Host
validation remains enabled; browser origins must match the endpoint exactly.

## Tools

| Tool | Required arguments | Result |
| --- | --- | --- |
| `mygo_windows` | none | `{windows:[{id,title,ready,focused,width,height,scale,operations}]}` |
| `mygo_screenshot` | `window_id` | PNG plus `{screenshot_id,window_id,content_width,content_height,image_width,image_height,scale}` |
| `mygo_click` | `window_id,screenshot_id,x,y` | One primary click, then a fresh scene |
| `mygo_type` | `window_id,text` | Commit Unicode to the focused editable native control |
| `mygo_key` | `window_id,key`; optional `modifiers` | Balanced key press/release, then a fresh scene |

Clicks use **image pixels**, not desktop coordinates or content DIPs. Translation
uses the actual image/content dimensions, including rounding. Only the latest
screenshot handle per window is accepted. Resize (even away and back), scale
change, closure, replacement, and server restart invalidate handles. Ordinary
animation may move a target without changing geometry: observe again after
navigation or asynchronous changes.

Keys are case-insensitive: letters, digits, F1–F12, Enter, Escape, Backspace, Tab,
Space, Delete, Insert, Home, End, PageUp, PageDown, Left, Right, Up, Down, Minus,
Equal, Comma, Period, Slash, Semicolon, Quote, BracketLeft, BracketRight,
Backslash, Backquote, ContextMenu, Back, Forward. Modifiers: Shift, Control, Alt,
Meta, Primary (Command on macOS, Control elsewhere). Printable keys do not
implicitly insert text; use `mygo_type`.

The queue holds at most 32 waiting commands; bodies are limited to 256 KiB,
text to 64 KiB UTF-8, captures to 16 million pixels before raster allocation and
32 MiB encoded PNG. Queued cancellation prevents dispatch. Dispatched input
cannot be rolled back or automatically retried. Completion means a fresh scene
exists, not that application background work or animations have stopped.

Screenshots are CPU-rendered content, not GPU framebuffer copies or OS chrome.
Webviews (including mixed native/webview windows), OS dialogs, OS menu
accelerators, IME composition, WASM, semantic selectors, and arbitrary desktop
control are outside this initial contract.

## Checks

`CGO_ENABLED=0 go test ./...` exercises the real SDK HTTP client, token-free
requests, schemas, input validation, queue bounds/order, cancellation, shutdown,
Host/Origin/body restrictions and screenshot coordinate validation. These tests
do not need a running app; real native GUI evidence is separate.
