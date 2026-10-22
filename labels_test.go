package mygomcp

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestClickLabelWorkflow(t *testing.T) {
	s := testService(t, demoFixture("first", 1.25), demoFixture("second", 1))
	cs := testClient(t, s)
	id := s.sessions[0].id
	shot := takeShot(t, cs, id)
	if !slices.Contains(shot.Labels, "Edit") {
		t.Fatal("screenshot omitted visible labels", shot.Labels)
	}
	click := clickLabelInput{id, shot.ScreenshotID, "Edit"}
	toolError(t, cs, "mygo_click_label", clickLabelInput{s.sessions[1].id, shot.ScreenshotID, "Edit"})
	toolError(t, cs, "mygo_click_label", clickLabelInput{id, shot.ScreenshotID, "edit"})
	callTool(t, cs, "mygo_click_label", click, nil)
	toolError(t, cs, "mygo_click_label", click)
	if !hasText(t, s, id, "Submit") || !hasText(t, s, s.sessions[1].id, "Edit") {
		t.Fatal("label click failed navigation or isolation")
	}
	shot = takeShot(t, cs, id)
	if !slices.Contains(shot.Labels, "Message") {
		t.Fatal("explicit label missing", shot.Labels)
	}
	callTool(t, cs, "mygo_click_label", clickLabelInput{id, shot.ScreenshotID, "Message"}, nil)
	callTool(t, cs, "mygo_type", typeInput{id, "Hello 日本語"}, nil)
	shot = takeShot(t, cs, id)
	callTool(t, cs, "mygo_click_label", clickLabelInput{id, shot.ScreenshotID, "Submit"}, nil)
	if !hasText(t, s, id, "Submitted: Hello 日本語") {
		t.Fatal("label click failed to focus or submit")
	}
}

func TestClickLabelRejectsAmbiguityAndOffscreen(t *testing.T) {
	s := testService(t, Fixture{Name: "labels", Width: 200, Height: 100, Factory: func() (func(*ui.Context), func(), error) {
		return func(c *ui.Context) {
			ui.Column(c).Children(func() {
				ui.Button(c, "Duplicate").Size(100, 40)
				ui.Button(c, "Duplicate").Size(100, 40)
				ui.Button(c, "Spacer").Size(100, 20)
				ui.Button(c, "Outside").Size(100, 40)
			})
		}, nil, nil
	}})
	cs := testClient(t, s)
	id := s.sessions[0].id
	shot := takeShot(t, cs, id)
	for label, code := range map[string]string{"Duplicate": "ambiguous", "Outside": "offscreen", "Missing": "not_found"} {
		err := targetingError(t, cs, "mygo_click_label", clickLabelInput{id, shot.ScreenshotID, label}, code)
		if err.Target == nil || err.Target.Label != label {
			t.Fatal("missing target details", err)
		}
		if label == "Duplicate" && (err.Target.Matches != 2 || err.Target.Bounds != nil) {
			t.Fatal("duplicate counts or bounds misleading", err.Target)
		}
		if label == "Outside" && (err.Target.Bounds == nil || err.Target.CenterInContent) {
			t.Fatal("offscreen geometry missing", err.Target)
		}
	}
	toolError(t, cs, "mygo_click_label", clickLabelInput{id, shot.ScreenshotID, ""})
	// Rejected requests preserve the screenshot for a subsequent valid click.
	callTool(t, cs, "mygo_click_label", clickLabelInput{id, shot.ScreenshotID, "Spacer"}, nil)
}
