package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestNativeFlow attaches to the real example; ordinary test runs skip it.
// Start: go run -tags mygo_mcp ./examples/native
// Run with MYGO_MCP_URL=http://127.0.0.1:7331/mcp and go test ./integration -v.
func TestNativeFlow(t *testing.T) {
	endpoint := os.Getenv("MYGO_MCP_URL")
	if endpoint == "" {
		t.Skip("start the native example and set MYGO_MCP_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "native-flow", Version: "1"}, nil)
	s, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	call := func(name string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		r, err := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if r.IsError {
			t.Fatalf("%s: %v", name, r.Content)
		}
		return r
	}
	var listed struct{ Windows []struct{ ID, Title string } }
	decode(t, call("mygo_windows", map[string]any{}).StructuredContent, &listed)
	if len(listed.Windows) != 2 {
		t.Fatalf("want two windows, got %d", len(listed.Windows))
	}
	var id string
	for _, w := range listed.Windows {
		if w.Title == "MCP form" {
			id = w.ID
		}
	}
	if id == "" {
		t.Fatal("missing primary window")
	}
	shot := func(label string) (string, []byte) {
		r := call("mygo_screenshot", map[string]any{"window_id": id})
		var meta struct {
			ID     string `json:"screenshot_id"`
			Width  int    `json:"image_width"`
			Height int    `json:"image_height"`
		}
		decode(t, r.StructuredContent, &meta)
		for _, content := range r.Content {
			if img, ok := content.(*mcp.ImageContent); ok {
				decoded, err := png.Decode(bytes.NewReader(img.Data))
				if err != nil {
					t.Fatal(err)
				}
				if decoded.Bounds().Dx() != meta.Width || decoded.Bounds().Dy() != meta.Height {
					t.Fatal("PNG metadata mismatch")
				}
				if meta.Width < 500 || meta.Height < 300 {
					t.Fatalf("example content too small: %dx%d", meta.Width, meta.Height)
				}
				if out := os.Getenv("MYGO_MCP_ARTIFACTS"); out != "" {
					if err := os.MkdirAll(out, 0o750); err != nil { // #nosec G703 -- Output directory is explicitly selected by the developer running this test.
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(out, label+".png"), img.Data, 0o600); err != nil { // #nosec G703 -- Label is a literal in this test; output root is developer-selected.
						t.Fatal(err)
					}
				}
				return meta.ID, img.Data
			}
		}
		t.Fatal("missing image content")
		return "", nil
	}
	click := func(shot string, x, y float64) {
		call("mygo_click", map[string]any{"window_id": id, "screenshot_id": shot, "x": x, "y": y})
	}
	initial, before := shot("01-initial")
	click(initial, 140, 84)
	form, afterNav := shot("02-form")
	if bytes.Equal(before, afterNav) {
		t.Fatal("navigation did not change scene")
	}
	click(form, 140, 84)
	call("mygo_type", map[string]any{"window_id": id, "text": "Hello 世界 🌍"})
	call("mygo_key", map[string]any{"window_id": id, "key": "a", "modifiers": []string{"Primary"}})
	call("mygo_type", map[string]any{"window_id": id, "text": "Selected café 世界"})
	call("mygo_key", map[string]any{"window_id": id, "key": "Enter", "modifiers": []string{"Primary"}})
	_, final := shot("03-result")
	if bytes.Equal(afterNav, final) {
		t.Fatal("editing did not change scene")
	}
	id = listed.Windows[0].ID
	if id == "" {
		t.Fatal("empty second ID")
	}
	for _, w := range listed.Windows {
		if w.Title == "MCP second window" {
			id = w.ID
		}
	}
	second, _ := shot("04-second-window")
	click(second, 140, 84)
	shot("05-second-form")
	t.Log("decoded real native PNGs; navigated, typed Unicode, replaced selection, submitted shortcut, and operated second window")
}

func decode(t *testing.T, v, dst any) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, dst); err != nil {
		t.Fatal(err)
	}
}
