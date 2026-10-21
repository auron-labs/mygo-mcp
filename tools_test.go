package mygomcp

import (
	"math"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestScreenshotCoordinates(t *testing.T) {
	g := mygo.InputGeometry{Width: 100, Height: 50, Scale: 1.5}
	shot := &screenshot{ID: "shot", WindowID: "win", ContentWidth: 100, ContentHeight: 50, Width: 151, Height: 75, Scale: 1.5}
	a := clickArgs{WindowID: "win", ScreenshotID: "shot", X: 75.5, Y: 37.5}
	x, y, err := clickPoint(shot, a, g)
	if err != nil || x != 50 || y != 25 {
		t.Fatalf("rounding: %v %v %v", x, y, err)
	}
	for _, bad := range []float64{-1, 151, math.Inf(1), math.NaN()} {
		a.X = bad
		if _, _, err := clickPoint(shot, a, g); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	a.X = 1
	for _, changed := range []mygo.InputGeometry{{Width: 101, Height: 50, Scale: 1.5}, {Width: 100, Height: 50, Scale: 2}} {
		if _, _, err := clickPoint(shot, a, changed); err == nil {
			t.Error("accepted stale geometry")
		}
	}
	a.ScreenshotID = "previous"
	if _, _, err := clickPoint(shot, a, g); err == nil {
		t.Error("accepted old screenshot")
	}
	if _, _, err := clickPoint(nil, a, g); err == nil {
		t.Error("accepted absent screenshot")
	}
}

func TestKeys(t *testing.T) {
	for _, name := range []string{"Enter", "Escape", "Backspace", "Tab", "Space", "Delete", "Insert", "Home", "End", "PageUp", "PageDown", "Left", "Right", "Up", "Down", "F1", "F12", "A", "z", "0", "9", "Minus", "Equal", "Comma", "Period", "Slash", "Semicolon", "Quote", "BracketLeft", "BracketRight", "Backslash", "Backquote", "ContextMenu", "Back", "Forward"} {
		if _, _, err := parseKey(name, []string{"Shift", "Control", "Alt", "Meta", "Primary"}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, name := range []string{"", "F13", "A+B", "😀"} {
		if _, _, err := parseKey(name, nil); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	if _, _, err := parseKey("a", []string{"typo"}); err == nil {
		t.Error("accepted modifier")
	}
	_, mods, err := parseKey("a", []string{"Primary"})
	if err != nil || mods != mygo.InputModifiers(ui.Cmd) {
		t.Fatal("primary modifier mismatch")
	}
}
