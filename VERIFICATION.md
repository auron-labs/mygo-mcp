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
