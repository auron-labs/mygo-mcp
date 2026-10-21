package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestNativeResize waits for an external desktop resize, then checks stale and
// fresh screenshot handles. It is explicitly enabled for coordinated CUA runs.
func TestNativeResize(t *testing.T) {
	endpoint := os.Getenv("MYGO_MCP_URL")
	if endpoint == "" || os.Getenv("MYGO_MCP_RESIZE") == "" {
		t.Skip("set MYGO_MCP_URL and MYGO_MCP_RESIZE, then resize a native window when prompted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "resize-check", Version: "1"}, nil)
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
		return r
	}
	type info struct {
		ID            string
		Width, Height float64
	}
	var windows struct{ Windows []info }
	decode(t, call("mygo_windows", map[string]any{}).StructuredContent, &windows)
	if len(windows.Windows) == 0 {
		t.Fatal("no windows")
	}
	w := windows.Windows[0]
	var initial struct {
		ID string `json:"screenshot_id"`
	}
	r := call("mygo_screenshot", map[string]any{"window_id": w.ID})
	if r.IsError {
		t.Fatal(r.Content)
	}
	decode(t, r.StructuredContent, &initial)
	t.Log("READY TO RESIZE: resize either example window using the desktop")
	for {
		decode(t, call("mygo_windows", map[string]any{}).StructuredContent, &windows)
		changed := false
		for _, current := range windows.Windows {
			if current.ID == w.ID && (current.Width != w.Width || current.Height != w.Height) {
				changed = true
			}
		}
		if changed {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("no resize observed")
		}
	}
	r = call("mygo_click", map[string]any{"window_id": w.ID, "screenshot_id": initial.ID, "x": 10, "y": 10})
	if !r.IsError {
		t.Fatal("accepted pre-resize screenshot")
	}
	r = call("mygo_screenshot", map[string]any{"window_id": w.ID})
	if r.IsError {
		t.Fatal(r.Content)
	}
	var fresh struct {
		ID string `json:"screenshot_id"`
	}
	decode(t, r.StructuredContent, &fresh)
	if fresh.ID == initial.ID {
		t.Fatal("reused screenshot handle")
	}
	r = call("mygo_click", map[string]any{"window_id": w.ID, "screenshot_id": fresh.ID, "x": 10, "y": 10})
	if r.IsError {
		t.Fatal(r.Content)
	}
	t.Log("rejected pre-resize handle and accepted fresh capture coordinates")
}
