package mygomcp_test

import (
	"bytes"
	"image/png"
	"testing"
	"time"

	"github.com/auron-labs/mygo-mcp/internal/demo"
	"github.com/egoist/mygo/ui"
)

func TestUnmodifiedPublicTester(t *testing.T) {
	s := demo.New()
	tt := ui.NewTester(s.View, 320, 200)
	tt.SetFocused(true)
	tt.ClickAt(100, 20)
	if !s.Editing {
		t.Fatal("navigation handler was not called")
	}
	tt.ClickAt(100, 20)
	tt.Type("Hello 日本語 😀")
	if s.Text != "Hello 日本語 😀" {
		t.Fatalf("Unicode input: %q", s.Text)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Replacement café")
	if s.Text != "Replacement café" {
		t.Fatalf("selection replacement: %q", s.Text)
	}
	tt.Key(ui.Cmd, ui.KeyEnter)
	if s.Submitted != s.Text || !tt.HasText("Submitted: Replacement café") {
		t.Fatal("shortcut submission not rendered")
	}
	tt.Advance(100 * time.Millisecond)
	if !s.Ready || !tt.HasText("Ready") {
		t.Fatal("simulated timer not rendered")
	}
	tt.Frame()
	var b bytes.Buffer
	if err := png.Encode(&b, tt.Image()); err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(&b)
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Dx() != 320 || im.Bounds().Dy() != 200 {
		t.Fatal(im.Bounds())
	}
	tt.ClickAt(100, 60)
	if s.Submitted != s.Text {
		t.Fatal("button submission failed")
	}
}
