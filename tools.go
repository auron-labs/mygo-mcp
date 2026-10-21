package mygomcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type targetArgs struct {
	WindowID string `json:"window_id"`
}
type clickArgs struct {
	WindowID     string  `json:"window_id"`
	ScreenshotID string  `json:"screenshot_id"`
	X            float64 `json:"x"`
	Y            float64 `json:"y"`
}
type typeArgs struct {
	WindowID string `json:"window_id"`
	Text     string `json:"text"`
}
type keyArgs struct {
	WindowID  string   `json:"window_id"`
	Key       string   `json:"key"`
	Modifiers []string `json:"modifiers,omitempty"`
}

type screenshot struct {
	Revision      uint64  `json:"-"`
	ID            string  `json:"screenshot_id"`
	WindowID      string  `json:"window_id"`
	ContentWidth  float64 `json:"content_width"`
	ContentHeight float64 `json:"content_height"`
	Width         int     `json:"image_width"`
	Height        int     `json:"image_height"`
	Scale         float64 `json:"scale"`
}

func result(v any, img []byte) *mcp.CallToolResult {
	b, _ := json.Marshal(v)
	r := &mcp.CallToolResult{StructuredContent: v, Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
	if img != nil {
		r.Content = append(r.Content, &mcp.ImageContent{MIMEType: "image/png", Data: img})
	}
	return r
}

func (s *Server) register(protocol *mcp.Server) {
	mcp.AddTool(protocol, &mcp.Tool{Name: "mygo_windows", Description: "List supported live native windows in this application."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		v, err := s.execute(ctx, func(context.Context) (any, error) { return map[string]any{"windows": s.refresh()}, nil })
		if err != nil {
			return nil, nil, err
		}
		return result(v, nil), nil, nil
	})
	mcp.AddTool(protocol, &mcp.Tool{Name: "mygo_screenshot", Description: "Capture a fresh native scene as PNG; use its screenshot_id and image pixels for clicks."}, func(ctx context.Context, _ *mcp.CallToolRequest, a targetArgs) (*mcp.CallToolResult, any, error) {
		v, err := s.execute(ctx, func(ctx context.Context) (any, error) {
			w, err := s.target(a.WindowID)
			if err != nil {
				return nil, err
			}
			w.shot = nil
			capture, err := w.driver.Capture(ctx, 16_000_000)
			if err != nil {
				return nil, fmt.Errorf("capture: %w", err)
			}
			if len(capture.PNG) > 32<<20 {
				return nil, errors.New("mygo-mcp: PNG exceeds size limit")
			}
			g := capture.Geometry
			w.shot = &screenshot{g.Revision, newID(), a.WindowID, g.Width, g.Height, capture.Width, capture.Height, g.Scale}
			return result(w.shot, capture.PNG), nil
		})
		if err != nil {
			return nil, nil, err
		}
		return v.(*mcp.CallToolResult), nil, nil
	})
	mcp.AddTool(protocol, &mcp.Tool{Name: "mygo_click", Description: "One left click in pixels from the latest screenshot. Rejects stale geometry."}, func(ctx context.Context, _ *mcp.CallToolRequest, a clickArgs) (*mcp.CallToolResult, any, error) {
		_, err := s.execute(ctx, func(ctx context.Context) (any, error) {
			w, err := s.target(a.WindowID)
			if err != nil {
				return nil, err
			}
			g, err := w.driver.Geometry()
			if err != nil {
				return nil, fmt.Errorf("geometry: %w", err)
			}
			x, y, err := clickPoint(w.shot, a, g)
			if err != nil {
				return nil, err
			}
			return nil, w.driver.ClickSnapshot(ctx, x, y, g)
		})
		if err != nil {
			return nil, nil, err
		}
		return result(map[string]bool{"ok": true}, nil), nil, nil
	})
	mcp.AddTool(protocol, &mcp.Tool{Name: "mygo_type", Description: "Commit Unicode text to the focused editable native control; does not simulate IME composition."}, func(ctx context.Context, _ *mcp.CallToolRequest, a typeArgs) (*mcp.CallToolResult, any, error) {
		if !utf8.ValidString(a.Text) || len(a.Text) > 64<<10 {
			return nil, nil, errors.New("mygo-mcp: text must be valid UTF-8 and at most 64 KiB")
		}
		_, err := s.execute(ctx, func(ctx context.Context) (any, error) {
			w, err := s.target(a.WindowID)
			if err != nil {
				return nil, err
			}
			return nil, w.driver.Type(ctx, a.Text)
		})
		if err != nil {
			return nil, nil, err
		}
		return result(map[string]bool{"ok": true}, nil), nil, nil
	})
	mcp.AddTool(protocol, &mcp.Tool{Name: "mygo_key", Description: "Press and release one native UI key with optional Shift, Control, Alt, Meta, or Primary modifiers. Does not type printable text."}, func(ctx context.Context, _ *mcp.CallToolRequest, a keyArgs) (*mcp.CallToolResult, any, error) {
		key, mods, err := parseKey(a.Key, a.Modifiers)
		if err != nil {
			return nil, nil, err
		}
		_, err = s.execute(ctx, func(ctx context.Context) (any, error) {
			w, err := s.target(a.WindowID)
			if err != nil {
				return nil, err
			}
			return nil, w.driver.Key(ctx, key, mods)
		})
		if err != nil {
			return nil, nil, err
		}
		return result(map[string]bool{"ok": true}, nil), nil, nil
	})
}

func clickPoint(shot *screenshot, a clickArgs, g mygo.InputGeometry) (float64, float64, error) {
	if shot == nil || shot.ID != a.ScreenshotID || shot.WindowID != a.WindowID || shot.ContentWidth != g.Width || shot.ContentHeight != g.Height || shot.Scale != g.Scale || shot.Revision != g.Revision {
		return 0, 0, errors.New("mygo-mcp: stale screenshot; capture again")
	}
	if math.IsNaN(a.X) || math.IsNaN(a.Y) || math.IsInf(a.X, 0) || math.IsInf(a.Y, 0) || a.X < 0 || a.Y < 0 || a.X >= float64(shot.Width) || a.Y >= float64(shot.Height) {
		return 0, 0, errors.New("mygo-mcp: coordinates outside screenshot")
	}
	return a.X * g.Width / float64(shot.Width), a.Y * g.Height / float64(shot.Height), nil
}

func parseKey(name string, modifiers []string) (mygo.InputKey, mygo.InputModifiers, error) {
	var key ui.Key
	name = strings.ToLower(name)
	switch {
	case len(name) == 1 && name[0] >= 'a' && name[0] <= 'z':
		key = ui.KeyA + ui.Key(name[0]-'a')
	case len(name) == 1 && name[0] >= '0' && name[0] <= '9':
		key = ui.Key0 + ui.Key(name[0]-'0')
	default:
		names := map[string]ui.Key{"enter": ui.KeyEnter, "escape": ui.KeyEscape, "backspace": ui.KeyBackspace, "tab": ui.KeyTab, "space": ui.KeySpace, "delete": ui.KeyDelete, "insert": ui.KeyInsert, "home": ui.KeyHome, "end": ui.KeyEnd, "pageup": ui.KeyPageUp, "pagedown": ui.KeyPageDown, "left": ui.KeyLeft, "right": ui.KeyRight, "up": ui.KeyUp, "down": ui.KeyDown, "minus": ui.KeyMinus, "equal": ui.KeyEqual, "comma": ui.KeyComma, "period": ui.KeyPeriod, "slash": ui.KeySlash, "semicolon": ui.KeySemicolon, "quote": ui.KeyQuote, "bracketleft": ui.KeyBracketLeft, "bracketright": ui.KeyBracketRight, "backslash": ui.KeyBackslash, "backquote": ui.KeyBackquote, "contextmenu": ui.KeyContextMenu, "back": ui.KeyBack, "forward": ui.KeyForward}
		key = names[name]
		for i := 1; i <= 12; i++ {
			if name == fmt.Sprintf("f%d", i) {
				key = ui.KeyF1 + ui.Key(i-1)
				break
			}
		}
	}
	if key == ui.KeyUnknown {
		return 0, 0, errors.New("mygo-mcp: unsupported key")
	}
	var mods ui.Modifiers
	for _, m := range modifiers {
		switch strings.ToLower(m) {
		case "shift":
			mods |= ui.Shift
		case "control":
			mods |= ui.Ctrl
		case "alt":
			mods |= ui.Alt
		case "meta":
			mods |= ui.Super
		case "primary":
			mods |= ui.Cmd
		default:
			return 0, 0, errors.New("mygo-mcp: unsupported modifier")
		}
	}
	return mygo.InputKey(key), mygo.InputModifiers(mods), nil
}
