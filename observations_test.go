package mygomcp

import (
	"bytes"
	"encoding/json"
	"image/png"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func inspect(t *testing.T, cs *mcp.ClientSession, id string) screenshotInfo {
	t.Helper()
	var out screenshotInfo
	r := callTool(t, cs, "mygo_inspect", sessionInput{id}, &out)
	if len(r.Content) != 1 || out.ObservationID == "" || out.ScreenshotID != "" {
		t.Fatal("inspection must have a fresh observation ID and no PNG", out)
	}
	return out
}

func checkActionShot(t *testing.T, r *mcp.CallToolResult, out screenshotInfo) {
	t.Helper()
	if !out.Delivered || out.ObservationID == "" || out.ScreenshotID != out.ObservationID || len(r.Content) != 2 {
		t.Fatal("action must return delivery status, observation and PNG", out)
	}
	im, ok := r.Content[1].(*mcp.ImageContent)
	if !ok || im.MIMEType != "image/png" {
		t.Fatal("expected PNG ImageContent")
	}
	decoded, err := png.Decode(bytes.NewReader(im.Data))
	if err != nil || decoded.Bounds().Dx() != out.ImageWidth || decoded.Bounds().Dy() != out.ImageHeight {
		t.Fatal("invalid action PNG or dimensions", err)
	}
}

func targetingError(t *testing.T, cs *mcp.ClientSession, name string, args any, code string) targetError {
	t.Helper()
	r, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil || !r.IsError {
		t.Fatal("expected ordinary targeting tool error", err)
	}
	b, err := json.Marshal(r.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out targetError
	if err := json.Unmarshal(b, &out); err != nil || out.Code != code || out.Message == "" {
		t.Fatalf("targeting error: %s, expected %s: %v", b, code, err)
	}
	return out
}

func TestInspectAndActionScreenshots(t *testing.T) {
	s := testService(t, demoFixture("one", 1.25), demoFixture("two", 1))
	cs := testClient(t, s)
	id := s.sessions[0].id
	shot := takeShot(t, cs, id)
	obs := inspect(t, cs, id)
	if obs.ImageWidth != shot.ImageWidth || obs.ImageHeight != shot.ImageHeight || obs.Time != shot.Time || obs.FramePending {
		t.Fatal("inspection geometry, clock or frame status", obs)
	}
	if len(obs.Targets) != 1 || obs.Targets[0].Label != "Edit" || obs.Targets[0].Matches != 1 || obs.Targets[0].Bounds == nil || !obs.Targets[0].CenterInContent {
		t.Fatal("inspection missing usable target", obs.Targets)
	}
	b := obs.Targets[0].Bounds
	if b.X+b.Width/2 != 100 || b.Y+b.Height/2 != 20 || b.Width <= 0 || b.Height <= 0 {
		t.Fatal("bounds must be content DIPs", b)
	}
	targetingError(t, cs, "mygo_click", clickInput{id, shot.ScreenshotID, 100, 20}, "stale_observation")
	targetingError(t, cs, "mygo_click_label", map[string]any{"session_id": s.sessions[1].id, "observation_id": obs.ObservationID, "label": "Edit"}, "stale_observation")
	targetingError(t, cs, "mygo_click_label", map[string]any{"session_id": id, "observation_id": obs.ObservationID, "screenshot_id": shot.ScreenshotID, "label": "Edit"}, "invalid_observation")
	var result screenshotInfo
	r := callTool(t, cs, "mygo_click_label", map[string]any{"session_id": id, "observation_id": obs.ObservationID, "label": "Edit", "screenshot": true}, &result)
	checkActionShot(t, r, result)
	targetingError(t, cs, "mygo_click_label", map[string]any{"session_id": id, "observation_id": obs.ObservationID, "label": "Edit"}, "stale_observation")
	// Pixel clicks also accept inspection IDs and use the observation's scale.
	obs = inspect(t, cs, id)
	r = callTool(t, cs, "mygo_click", map[string]any{"session_id": id, "observation_id": obs.ObservationID, "x": 100 * float64(obs.ImageWidth) / float64(obs.ContentWidth), "y": 20 * float64(obs.ImageHeight) / float64(obs.ContentHeight), "screenshot": true}, &result)
	checkActionShot(t, r, result)
	r = callTool(t, cs, "mygo_type", map[string]any{"session_id": id, "text": "Hello", "screenshot": true}, &result)
	checkActionShot(t, r, result)
	r = callTool(t, cs, "mygo_key", map[string]any{"session_id": id, "key": "enter", "modifiers": []string{"primary"}, "screenshot": true}, &result)
	checkActionShot(t, r, result)
	if !hasText(t, s, id, "Submitted: Hello") {
		t.Fatal("key screenshot failed to observe submitted text")
	}
	// The returned screenshot can target the next action without another call.
	callTool(t, cs, "mygo_click_label", clickLabelInput{id, result.ScreenshotID, "Message"}, nil)
	if !result.Time.Equal(shot.Time) {
		t.Fatal("inspection and action capture advanced time")
	}
}
