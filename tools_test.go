package mygomcp

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/auron-labs/mygo-mcp/internal/demo"
	"github.com/egoist/mygo/ui"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func demoFixture(name string, scale float32) Fixture {
	return Fixture{Name: name, Width: 321, Height: 201, Scale: scale, Factory: func() (func(*ui.Context), func(), error) {
		s := demo.New()
		return s.View, nil, nil
	}}
}

func testService(t *testing.T, fixtures ...Fixture) *service {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	s, err := newService(ctx, fixtures)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			t.Error("worker failed to stop")
		}
	})
	return s
}

func testClient(t *testing.T, s *service) *mcp.ClientSession {
	t.Helper()
	httpServer := httptest.NewServer(s.handler())
	t.Cleanup(httpServer.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	cs, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cs.Close(); err != nil {
			t.Error(err)
		}
	})
	return cs
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args any, out any) *mcp.CallToolResult {
	t.Helper()
	r, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if r.IsError {
		t.Fatalf("%s: %v", name, r.Content)
	}
	if out != nil {
		b, err := json.Marshal(r.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func toolError(t *testing.T, cs *mcp.ClientSession, name string, args any) {
	t.Helper()
	r, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("expected ordinary tool error, got protocol error: %v", err)
	}
	if !r.IsError {
		t.Fatalf("%s accepted invalid arguments: %+v", name, args)
	}
}

func takeShot(t *testing.T, cs *mcp.ClientSession, id string) screenshotInfo {
	t.Helper()
	var out screenshotInfo
	r := callTool(t, cs, "mygo_screenshot", sessionInput{id}, &out)
	if len(r.Content) != 2 {
		t.Fatalf("screenshot content: %v", r.Content)
	}
	im, ok := r.Content[1].(*mcp.ImageContent)
	if !ok || im.MIMEType != "image/png" {
		t.Fatal("expected PNG ImageContent")
	}
	decoded, err := png.Decode(bytes.NewReader(im.Data))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != out.ImageWidth || decoded.Bounds().Dy() != out.ImageHeight {
		t.Fatal("image metadata mismatch")
	}
	// A real rendered view contains differing pixels, not a blank placeholder.
	first := decoded.At(0, 0)
	different := false
	for y := 0; y < out.ImageHeight && !different; y++ {
		for x := range out.ImageWidth {
			if decoded.At(x, y) != first {
				different = true
				break
			}
		}
	}
	if !different {
		t.Fatal("blank screenshot")
	}
	return out
}

func clickDIP(t *testing.T, cs *mcp.ClientSession, shot screenshotInfo, x, y float64) {
	t.Helper()
	callTool(t, cs, "mygo_click", clickInput{SessionID: shot.SessionID, ScreenshotID: shot.ScreenshotID, X: x * float64(shot.ImageWidth) / float64(shot.ContentWidth), Y: y * float64(shot.ImageHeight) / float64(shot.ContentHeight)}, nil)
}

func hasText(t *testing.T, s *service, id, text string) bool {
	t.Helper()
	v, err := s.call(t.Context(), id, func(x *session) (any, error) { return x.tester.HasText(text), nil })
	if err != nil {
		t.Fatal(err)
	}
	return v.(bool)
}

func TestHeadlessMCPWorkflow(t *testing.T) {
	var state *demo.State
	first := demoFixture("first", 1.25)
	first.Factory = func() (func(*ui.Context), func(), error) { state = demo.New(); return state.View, nil, nil }
	s := testService(t, first, demoFixture("second", 1))
	cs := testClient(t, s)
	var list struct {
		Sessions []sessionInfo `json:"sessions"`
	}
	callTool(t, cs, "mygo_sessions", struct{}{}, &list)
	if len(list.Sessions) != 2 || list.Sessions[0].ID == list.Sessions[1].ID {
		t.Fatal(list)
	}
	id, other := list.Sessions[0].ID, list.Sessions[1].ID
	tools, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 6 {
		t.Fatal("expected six tools")
	}
	for _, tool := range tools.Tools {
		b, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(b, []byte(`"object"`)) || (tool.Name != "mygo_sessions" && !bytes.Contains(b, []byte(`"session_id"`))) {
			t.Fatalf("bad schema for %s: %s", tool.Name, b)
		}
	}
	shot := takeShot(t, cs, id)
	if shot.Scale != 1.25 || shot.ImageWidth == shot.ContentWidth {
		t.Fatal(shot)
	}
	clickDIP(t, cs, shot, 100, 20)
	if !hasText(t, s, id, "Submit") || !hasText(t, s, other, "Edit") {
		t.Fatal("navigation or isolation failed")
	}
	shot = takeShot(t, cs, id)
	clickDIP(t, cs, shot, 100, 20)
	callTool(t, cs, "mygo_type", typeInput{id, "Hello 日本語 😀"}, nil)
	value, err := s.call(t.Context(), id, func(*session) (any, error) { return state.Text, nil })
	if err != nil || value != "Hello 日本語 😀" {
		t.Fatalf("Unicode input: %v %v", value, err)
	}
	callTool(t, cs, "mygo_key", keyInput{id, "a", []string{"primary"}}, nil)
	callTool(t, cs, "mygo_type", typeInput{id, "Replacement café"}, nil)
	callTool(t, cs, "mygo_key", keyInput{id, "enter", []string{"primary"}}, nil)
	shot = takeShot(t, cs, id)
	if !hasText(t, s, id, "Submitted: Replacement café") || hasText(t, s, id, "Ready") {
		t.Fatal("shortcut submission failed")
	}
	var after status
	callTool(t, cs, "mygo_advance", advanceInput{id, 100}, &after)
	if after.Time.Sub(shot.Time) != 100*time.Millisecond || !hasText(t, s, id, "Ready") {
		t.Fatal("advance did not fire timer")
	}
	shot = takeShot(t, cs, id)
	clickDIP(t, cs, shot, 100, 60)
	if !hasText(t, s, id, "Submitted: Replacement café") {
		t.Fatal("button submission failed")
	}
	_ = takeShot(t, cs, id)
	if hasText(t, s, other, "Submitted: Replacement café") {
		t.Fatal("state leaked between fixtures")
	}
	newRun := testService(t, demoFixture("first", 1))
	if newRun.sessions[0].id == id {
		t.Fatal("run reused identity")
	}
	if _, err := newRun.call(t.Context(), id, func(*session) (any, error) { return true, nil }); err == nil {
		t.Fatal("accepted previous-run ID")
	}
}

func TestScreenshotValidationAndInvalidation(t *testing.T) {
	s := testService(t, demoFixture("one", 1), demoFixture("two", 2))
	cs := testClient(t, s)
	id := s.sessions[0].id
	shot := takeShot(t, cs, id)
	latest := takeShot(t, cs, id)
	toolError(t, cs, "mygo_click", clickInput{id, shot.ScreenshotID, 10, 10})
	toolError(t, cs, "mygo_click", clickInput{s.sessions[1].id, latest.ScreenshotID, 10, 10})
	for _, xy := range [][2]float64{{-1, 0}, {0, -1}, {float64(latest.ImageWidth), 0}, {0, float64(latest.ImageHeight)}, {math.MaxFloat64, 0}} {
		toolError(t, cs, "mygo_click", clickInput{id, latest.ScreenshotID, xy[0], xy[1]})
	}
	// Rejections preserve the still-valid screenshot.
	clickDIP(t, cs, latest, 100, 20)
	toolError(t, cs, "mygo_click", clickInput{id, latest.ScreenshotID, 10, 10})
	for _, action := range []struct {
		name string
		args any
	}{
		{"mygo_type", typeInput{id, ""}},         // No editable focus still counts as delivery.
		{"mygo_key", keyInput{id, "shift", nil}}, // Invalid input must not invalidate.
		{"mygo_key", keyInput{id, "tab", nil}},
		{"mygo_advance", advanceInput{id, 0}},
	} {
		shot = takeShot(t, cs, id)
		if action.name == "mygo_key" && action.args.(keyInput).Key == "shift" {
			toolError(t, cs, action.name, action.args)
			clickDIP(t, cs, shot, 10, 10)
		} else {
			callTool(t, cs, action.name, action.args, nil)
			toolError(t, cs, "mygo_click", clickInput{id, shot.ScreenshotID, 10, 10})
		}
	}
}

func TestToolInputErrors(t *testing.T) {
	s := testService(t, demoFixture("one", 1))
	cs := testClient(t, s)
	id := s.sessions[0].id
	for _, name := range []string{"mygo_screenshot", "mygo_type", "mygo_key", "mygo_advance", "mygo_click"} {
		toolError(t, cs, name, map[string]any{})
		toolError(t, cs, name, map[string]any{"session_id": "old-or-unknown"})
	}
	for _, ms := range []float64{-1, 0.5, 1001, math.MaxFloat64} {
		toolError(t, cs, "mygo_advance", advanceInput{id, ms})
	}
	toolError(t, cs, "mygo_type", typeInput{id, strings.Repeat("x", maxTextBytes+1)})
	for _, in := range []keyInput{{id, "unknown", nil}, {id, "a", []string{"hyper"}}, {id, "a", []string{"shift", "shift", "shift", "shift", "shift", "shift"}}} {
		toolError(t, cs, "mygo_key", in)
	}
	for name, want := range map[string]ui.Key{"Enter": ui.KeyEnter, "a": ui.KeyA, "9": ui.Key9, "F12": ui.KeyF12, "PageDown": ui.KeyPageDown, "backquote": ui.KeyBackquote} {
		got, mods, err := parseKey(keyInput{Key: name, Modifiers: []string{"Shift", "Control", "Alt", "Meta", "primary"}})
		if err != nil || got != want || mods != ui.Shift|ui.Ctrl|ui.Alt|ui.Super {
			t.Fatalf("parse %s: %v %v %v", name, got, mods, err)
		}
	}
	var b pngBuffer
	if _, err := b.Write(make([]byte, maxPNGBytes+1)); err == nil || b.Len() != 0 {
		t.Fatal("PNG allocation was not bounded")
	}
}

func TestContinuousFramesAreBounded(t *testing.T) {
	frames := 0
	s := testService(t, Fixture{Name: "animated", Width: 100, Height: 50, Factory: func() (func(*ui.Context), func(), error) {
		return func(c *ui.Context) {
			frames++
			ui.Text(c, "animation").Animate("pulse", float32(frames%2), time.Second)
		}, nil, nil
	}})
	cs := testClient(t, s)
	id := s.sessions[0].id
	shot := takeShot(t, cs, id)
	if !shot.FramePending {
		t.Fatal("continuously requesting view must report frame_pending")
	}
	before := frames
	callTool(t, cs, "mygo_key", keyInput{id, "a", nil}, nil)
	if frames-before > 100 {
		t.Fatal("unbounded frame loop")
	}
	next := takeShot(t, cs, id)
	if !next.Time.Equal(shot.Time) || !next.FramePending {
		t.Fatal("wall time advanced clock or pending flag lost")
	}
}
