# mygo-mcp

Opt-in MCP control of the **hosting application's native MyGo UI**, using the
official Go MCP SDK's stateless Streamable HTTP transport. No shell, JavaScript
evaluation, OS automation, authentication, or automatic startup.

Requires Go 1.27.3 and the native input seam in
[MyGo fork PR #1](https://github.com/aaronflorey/mygo/pull/1), based on upstream
`3dfa48145e523d029ea6959f6e129915eb16499d`. The dependency is pinned to fork commit
`df8d39b4a28fc83e09cea4a467962cab2caece36`; no filesystem replacement is needed.
An upstream PR was attempted but GitHub denied creation against `egoist/mygo`.
Until upstream accepts the seam, consuming applications must also copy this
repository's **remote** `replace github.com/egoist/mygo` directive into their own
`go.mod` (Go does not inherit dependency replacements).

## Integration

### Try the example

From this repository, with Go 1.27.3 on PATH:

```sh
go run -tags mygo_mcp ./examples/native
```

This starts two native windows and the local MCP endpoint on port 7331. Running
without `-tags mygo_mcp` builds the same UI without importing or starting MCP.
The enabled and disabled startup implementations are
[`mcp_dev.go`](examples/native/mcp_dev.go) and
[`mcp_disabled.go`](examples/native/mcp_disabled.go); the main lifecycle is in
[`main.go`](examples/native/main.go).

### Add it to an existing application

Until the upstream seam is merged, first add the pinned remote replacement to
the **application's** module, then install the reviewed package revision:

```sh
go mod edit -replace github.com/egoist/mygo=github.com/aaronflorey/mygo@v0.0.0-20261021030745-df8d39b4a28f
GOPRIVATE=github.com/auron-labs/mygo-mcp go get github.com/auron-labs/mygo-mcp@1cc489657fc170855ea5de49ae7ef35453fcbe93
```

That package commit contains the completed implementation, example and checks;
this documentation is delivered separately. The fork is based on the upstream
revision named above, not a moving branch. Rebuild and test your application
against that revision, especially if it currently uses a newer MyGo API. Once
the seam is accepted upstream, a follow-up can remove the remote replacement
and pin the accepting upstream release/revision.

This repository is private: use your existing GitHub Git credentials. The scoped
`GOPRIVATE` setting avoids asking the public module proxy/checksum service to
resolve it; it does not bypass GitHub authorization. If you already use
`GOPRIVATE`, append this module path rather than replacing your other entries.

In a development-only file with `//go:build mygo_mcp`, import this package and
call `mygomcp.Start(ctx, "127.0.0.1:7331")` from `mygo.App.WhenReady`. Keep
`App.Run` on the main goroutine. Cancel the context and call `server.Close(ctx)`
**after** `App.Run` returns, not inside a synchronous quit callback.

Copy the two build-selected startup helpers from the example (adjust the package
and loopback port) and integrate them into the application's existing lifecycle:

```go
stop := func() {}
mygo.App.WhenReady(func() {
    // Keep your existing window creation, components and startup work here.
    stop = startAutomation()
})
err := mygo.App.Run() // stays on the main goroutine
stop()              // shuts down HTTP and waits for the worker off the UI loop
// Handle err using the application's existing error policy.
```

The tagged file's import keeps the server out of ordinary builds. It does not
remove module requirements from go.mod; Go's dependency graph and linked code
are different things. No application components or normal event handlers need
to be replaced. Do not call blocking driver waits directly from a UI callback.

### Client configuration

Configure the client with URL `http://127.0.0.1:7331/mcp`. Do not publicly expose
this endpoint: **any local process can control the instrumented application**.
Only explicit loopback IP listeners are accepted. The SDK's localhost Host
validation remains enabled; browser origins must match the endpoint exactly.

For clients using an `mcpServers` JSON configuration, the entry is typically:

```json
{"mcpServers":{"mygo":{"url":"http://127.0.0.1:7331/mcp"}}}
```

Choose **Streamable HTTP**, not stdio or legacy SSE. Some clients additionally
require a transport/type field; follow that client's format. No token or custom
authorization headers are needed. Give each running project its own explicit
loopback port. IPv6 loopback (`[::1]:7331`) is also accepted. Hostnames such as
`localhost`, wildcard addresses and non-loopback addresses are rejected at startup.
Only `/mcp` serves the protocol. Do not put a public preview/proxy in front of it.

The transport and image/tool result conventions follow the official
[SDK transport](https://go.sdk.modelcontextprotocol.io/protocol/) and
[SDK server](https://go.sdk.modelcontextprotocol.io/server/) documentation.

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

With the native example running, in a second terminal:

```sh
MYGO_MCP_URL=http://127.0.0.1:7331/mcp \
MYGO_MCP_ARTIFACTS=/tmp/mygo-mcp-evidence \
go test ./integration -run TestNativeFlow -count=1 -v
```

Start with fresh example windows for each flow. The client lists windows,
captures PNGs, clicks Open form, focuses the input, types Unicode, sends
Primary+A, replaces the selection, submits using Primary+Enter, captures the
visible result and operates the second window. Expected output:
`Result: Selected café 世界`. The example also prints its submitted result;
the MCP server itself never logs typed text.

`TestNativeResize` is separately opt-in: set `MYGO_MCP_RESIZE=1`, run that test
with `-v`, and resize **both** example windows when it prints READY TO RESIZE.
It waits on observed geometry, rejects the old handle and checks a fresh one.
The smoke coordinates assume this example at scale 1; actual agents should
choose coordinates from each returned image and inspect new scenes as needed.

Run `sh scripts/check-builds.sh` for reproducible pure-Go build/isolation checks.
See [VERIFICATION.md](VERIFICATION.md) for checks actually executed, real PNG
evidence, hardware limitations, upstream submission restrictions and stage PRs.
