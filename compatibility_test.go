package mygomcp

import (
	"fmt"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestZoomedFixtureCoordinates(t *testing.T) {
	s := testService(t, Fixture{Name: "zoom", Width: 321, Height: 201, Scale: 1.25, Factory: func() (func(*ui.Context), func(), error) {
		clicks := 0
		return func(c *ui.Context) {
			c.SetZoom(1.5)
			ui.Column(c).Padding(10).Children(func() {
				if ui.Button(c, "Increment").Label("Counter button").Size(100, 40).Clicked() {
					clicks++
				}
				ui.Text(c, fmt.Sprintf("Clicks: %d", clicks))
			})
		}, nil, nil
	}})
	cs := testClient(t, s)
	id := s.sessions[0].id
	shot := takeShot(t, cs, id)
	i := slices.IndexFunc(shot.Targets, func(target targetInfo) bool { return target.Label == "Counter button" })
	if i < 0 || shot.Targets[i].Bounds == nil {
		t.Fatal("zoomed button missing from observation", shot.Targets)
	}
	if b := *shot.Targets[i].Bounds; b != (targetBounds{X: 15, Y: 15, Width: 150, Height: 60}) {
		t.Fatal("target bounds must be zoomed window DIPs", b)
	}
	// This point is inside the zoomed button, but outside its unzoomed bounds.
	clickDIP(t, cs, shot, 150, 60)
	if !hasText(t, s, id, "Clicks: 1") {
		t.Fatal("pixel click missed zoomed button")
	}
	shot = takeShot(t, cs, id)
	callTool(t, cs, "mygo_click_label", clickLabelInput{id, shot.ScreenshotID, "Counter button"}, nil)
	if !hasText(t, s, id, "Clicks: 2") {
		t.Fatal("label click missed zoomed button")
	}
}

func TestDecimalInputThroughMCP(t *testing.T) {
	s := testService(t, Fixture{Name: "decimal", Width: 400, Height: 150, Factory: func() (func(*ui.Context), func(), error) {
		amount := "0.00"
		return func(c *ui.Context) {
			ui.Column(c).AlignItems(ui.Start).Children(func() {
				ui.DecimalInput(c, &amount, "0", "", "0.01").Label("Amount")
				ui.Text(c, "Value: "+amount)
			})
		}, nil, nil
	}})
	cs := testClient(t, s)
	id := s.sessions[0].id
	shot := takeShot(t, cs, id)
	callTool(t, cs, "mygo_click_label", clickLabelInput{id, shot.ScreenshotID, "Amount"}, nil)
	callTool(t, cs, "mygo_key", keyInput{id, "a", []string{"primary"}}, nil)
	callTool(t, cs, "mygo_type", typeInput{id, "9007199254740993.01"}, nil)
	if !hasText(t, s, id, "Value: 9007199254740993.01") {
		t.Fatal("decimal input lost precision or did not receive typed text", inspect(t, cs, id))
	}
	callTool(t, cs, "mygo_key", keyInput{id, "up", nil}, nil)
	if !hasText(t, s, id, "Value: 9007199254740993.02") {
		t.Fatal("decimal step did not preserve precision")
	}
	callTool(t, cs, "mygo_key", keyInput{id, "a", []string{"primary"}}, nil)
	callTool(t, cs, "mygo_type", typeInput{id, "1.001"}, nil)
	if !hasText(t, s, id, "Value: 9007199254740993.02") {
		t.Fatal("invalid decimal changed the committed value")
	}
}
