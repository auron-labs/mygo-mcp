package mygomcp

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestScrollReachesOffscreenControl(t *testing.T) {
	var scroll ui.ScrollState
	clicks := 0
	s := testService(t, Fixture{Name: "scroll", Width: 200, Height: 100, Scale: 1.5, Factory: func() (func(*ui.Context), func(), error) {
		return func(c *ui.Context) {
			ui.Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
				ui.Column(c).Height(500).Shrink(0).Children(func() {
					ui.Box(c).Size(150, 300).Shrink(0)
					if ui.Button(c, "Bottom").Size(150, 40).Shrink(0).Clicked() {
						clicks++
					}
				})
			})
		}, nil, nil
	}})
	cs := testClient(t, s)
	id := s.sessions[0].id
	obs := inspect(t, cs, id)
	targetingError(t, cs, "mygo_click_label", map[string]any{"session_id": id, "observation_id": obs.ObservationID, "label": "Bottom"}, "offscreen")
	args := map[string]any{"session_id": id, "observation_id": obs.ObservationID, "x": 150, "y": 75, "delta_x": 0, "delta_y": 4097}
	toolError(t, cs, "mygo_scroll", args)
	args["delta_y"] = 250
	args["screenshot"] = true
	var result screenshotInfo
	r := callTool(t, cs, "mygo_scroll", args, &result)
	checkActionShot(t, r, result)
	if !result.Time.Equal(obs.Time) {
		t.Fatal("scroll advanced simulated time")
	}
	targetingError(t, cs, "mygo_scroll", args, "stale_observation")
	callTool(t, cs, "mygo_click_label", clickLabelInput{id, result.ScreenshotID, "Bottom"}, nil)
	v, err := s.call(t.Context(), id, func(*session) (any, error) { return scroll.Y == 250 && clicks == 1, nil })
	if err != nil || !v.(bool) {
		t.Fatal("scroll failed to reach and click control", err)
	}
	obs = inspect(t, cs, id)
	args = map[string]any{"session_id": id, "observation_id": obs.ObservationID, "x": 150, "y": 75, "delta_x": 0, "delta_y": 0}
	callTool(t, cs, "mygo_scroll", args, nil)
	targetingError(t, cs, "mygo_scroll", args, "stale_observation")
}

func TestWaitTextBoundsAndObservations(t *testing.T) {
	s := testService(t, demoFixture("one", 1), demoFixture("two", 1))
	cs := testClient(t, s)
	id := s.sessions[0].id
	otherStart := inspect(t, cs, s.sessions[1].id).Time
	obs := inspect(t, cs, id)
	callTool(t, cs, "mygo_click_label", map[string]any{"session_id": id, "observation_id": obs.ObservationID, "label": "Edit"}, nil)
	callTool(t, cs, "mygo_key", keyInput{id, "enter", []string{"primary"}}, nil)
	obs = inspect(t, cs, id)
	var out waitTextInfo
	r := callTool(t, cs, "mygo_wait_text", waitTextInput{SessionID: id, Text: "Ready", TimeoutMS: 200, Screenshot: true}, &out)
	if !out.Matched || out.AdvancedMS != 100 || out.Time.Sub(obs.Time) != 100*time.Millisecond || len(r.Content) != 2 || out.ObservationID == "" {
		t.Fatal("wait did not stop at expected simulated update", out)
	}
	targetingError(t, cs, "mygo_click_label", map[string]any{"session_id": id, "observation_id": obs.ObservationID, "label": "Submit"}, "stale_observation")
	callTool(t, cs, "mygo_click_label", clickLabelInput{id, out.ScreenshotID, "Message"}, nil)
	obs = inspect(t, cs, id)
	out = waitTextInfo{}
	r = callTool(t, cs, "mygo_wait_text", waitTextInput{SessionID: id, Text: "does not exist", TimeoutMS: 37}, &out)
	if out.Matched || out.AdvancedMS != 37 || out.Time.Sub(obs.Time) != 37*time.Millisecond || out.ScreenshotID != "" || len(r.Content) != 1 {
		t.Fatal("timeout exceeded budget or claimed a match", out)
	}
	start := out.Time
	out = waitTextInfo{}
	callTool(t, cs, "mygo_wait_text", waitTextInput{SessionID: id, Text: "Submitted:", TimeoutMS: 1000}, &out)
	if !out.Matched || out.AdvancedMS != 0 || !out.Time.Equal(start) {
		t.Fatal("existing substring should return immediately", out)
	}
	out = waitTextInfo{}
	callTool(t, cs, "mygo_wait_text", waitTextInput{SessionID: id, Text: "missing", TimeoutMS: 0}, &out)
	if out.Matched || out.AdvancedMS != 0 || !out.Time.Equal(start) {
		t.Fatal("zero timeout advanced time", out)
	}
	if other := inspect(t, cs, s.sessions[1].id); !other.Time.Equal(otherStart) {
		t.Fatal("waiting advanced another fixture's clock", other)
	}
}

func TestScrollAndWaitValidationPreservesObservation(t *testing.T) {
	s := testService(t, demoFixture("one", 1))
	cs := testClient(t, s)
	id := s.sessions[0].id
	obs := inspect(t, cs, id)
	for _, ms := range []float64{-1, .5, 1001, math.MaxFloat64} {
		toolError(t, cs, "mygo_wait_text", waitTextInput{SessionID: id, Text: "Edit", TimeoutMS: ms})
	}
	toolError(t, cs, "mygo_wait_text", waitTextInput{SessionID: id, Text: "", TimeoutMS: 0})
	for _, delta := range []float64{math.NaN(), math.Inf(1), -4097, 4097} {
		in := scrollInput{DeltaY: delta}
		in.ObservationID, in.SessionID, in.X, in.Y = obs.ObservationID, id, 50, 20
		if in.validate() == nil {
			t.Fatal("accepted invalid scroll delta", delta)
		}
	}
	targetingError(t, cs, "mygo_scroll", map[string]any{"session_id": id, "observation_id": obs.ObservationID, "x": 500, "y": 20, "delta_x": 0, "delta_y": 20}, "out_of_bounds")
	callTool(t, cs, "mygo_click_label", map[string]any{"session_id": id, "observation_id": obs.ObservationID, "label": "Edit"}, nil)
}
