package mygomcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type sessionInput struct {
	SessionID string `json:"session_id" jsonschema:"Registered headless session ID"`
}
type clickInput struct {
	SessionID    string  `json:"session_id"`
	ScreenshotID string  `json:"screenshot_id"`
	X            float64 `json:"x" jsonschema:"Horizontal screenshot pixel coordinate"`
	Y            float64 `json:"y" jsonschema:"Vertical screenshot pixel coordinate"`
}
type typeInput struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text" jsonschema:"Committed UTF-8 text; at most 65536 bytes"`
}
type keyInput struct {
	SessionID string   `json:"session_id"`
	Key       string   `json:"key"`
	Modifiers []string `json:"modifiers,omitempty"`
}
type advanceInput struct {
	SessionID    string  `json:"session_id"`
	Milliseconds float64 `json:"milliseconds" jsonschema:"Integer simulated duration from 0 through 1000"`
}

func (in clickInput) validate() error {
	if in.ScreenshotID == "" || !finite(in.X) || !finite(in.Y) || in.X < 0 || in.Y < 0 {
		return errors.New("screenshot_id and finite nonnegative pixel coordinates required")
	}
	return nil
}

func (in typeInput) validate() error {
	if !utf8.ValidString(in.Text) || len(in.Text) > maxTextBytes {
		return errors.New("text must be valid UTF-8 and at most 65536 bytes")
	}
	return nil
}

func (in advanceInput) validate() error {
	if !finite(in.Milliseconds) || in.Milliseconds < 0 || in.Milliseconds > maxAdvanceMS || math.Trunc(in.Milliseconds) != in.Milliseconds {
		return errors.New("milliseconds must be an integer from 0 through 1000")
	}
	return nil
}

type sessionInfo struct {
	ID         string   `json:"session_id"`
	Name       string   `json:"name"`
	Width      int      `json:"content_width"`
	Height     int      `json:"content_height"`
	Scale      float32  `json:"scale"`
	Operations []string `json:"operations"`
	Failed     string   `json:"failed,omitempty"`
}

type status struct {
	SessionID    string    `json:"session_id"`
	Time         time.Time `json:"simulated_time"`
	FramePending bool      `json:"frame_pending"`
	Delivered    bool      `json:"delivered,omitempty"`
}
type screenshotInfo struct {
	status
	ScreenshotID  string  `json:"screenshot_id"`
	ImageWidth    int     `json:"image_width"`
	ImageHeight   int     `json:"image_height"`
	ContentWidth  int     `json:"content_width"`
	ContentHeight int     `json:"content_height"`
	Scale         float32 `json:"scale"`
}
type payload struct {
	metadata any
	png      []byte
}

func addTool[In any](server *mcp.Server, name, description string, fn func(context.Context, In) (payload, error)) {
	mcp.AddTool(server, &mcp.Tool{Name: name, Description: description}, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		p, err := fn(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		text, err := json.Marshal(p.metadata)
		if err != nil {
			return nil, nil, fmt.Errorf("encode result: %w", err)
		}
		content := []mcp.Content{&mcp.TextContent{Text: string(text)}}
		if p.png != nil {
			content = append(content, &mcp.ImageContent{Data: p.png, MIMEType: "image/png"})
		}
		return &mcp.CallToolResult{Content: content, StructuredContent: p.metadata}, nil, nil
	})
}

func currentStatus(x *session, delivered bool) status {
	return status{SessionID: x.id, Time: x.tester.Now(), FramePending: x.tester.FramePending(), Delivered: delivered}
}

func (s *service) operation(ctx context.Context, id string, validate func() error, mutate bool, fn func(*session) (payload, error)) (payload, error) {
	if id == "" {
		return payload{}, errors.New("session_id required")
	}
	if err := validate(); err != nil {
		return payload{}, err
	}
	v, err := s.call(ctx, id, func(x *session) (any, error) {
		if err := validate(); err != nil {
			return nil, err
		}
		if mutate {
			x.shot = nil
		}
		return fn(x)
	})
	if err != nil {
		return payload{}, err
	}
	return v.(payload), nil
}

func validSessionInput() error { return nil }

func (s *service) registerTools(server *mcp.Server) {
	addTool(server, "mygo_sessions", "List registered isolated headless fixtures, not desktop windows.", func(ctx context.Context, _ struct{}) (payload, error) {
		v, err := s.call(ctx, "", func(*session) (any, error) {
			list := make([]sessionInfo, 0, len(s.sessions))
			for _, x := range s.sessions {
				info := sessionInfo{ID: x.id, Name: x.fixture.Name, Width: x.fixture.Width, Height: x.fixture.Height, Scale: x.fixture.Scale, Operations: []string{"mygo_screenshot", "mygo_click", "mygo_type", "mygo_key", "mygo_advance"}}
				if x.failed != nil {
					info.Failed = x.failed.Error()
					info.Operations = []string{}
				}
				list = append(list, info)
			}
			return payload{metadata: struct {
				Sessions []sessionInfo `json:"sessions"`
			}{list}}, nil
		})
		if err != nil {
			return payload{}, err
		}
		return v.(payload), nil
	})
	addTool(server, "mygo_screenshot", "Render current headless state as PNG; supersedes the previous screenshot ID.", func(ctx context.Context, in sessionInput) (payload, error) {
		return s.operation(ctx, in.SessionID, validSessionInput, false, func(x *session) (payload, error) {
			x.shot = nil
			x.tester.Frame()
			im := x.tester.Image()
			if im == nil {
				return payload{}, errors.New("tester returned no image")
			}
			bounds := im.Bounds()
			w, h := bounds.Dx(), bounds.Dy()
			if w <= 0 || h <= 0 || w > maxDimension || h > maxDimension || w*h > maxPixels {
				return payload{}, errors.New("unexpected image geometry")
			}
			var b pngBuffer
			if err := png.Encode(&b, im); err != nil {
				return payload{}, fmt.Errorf("encode screenshot: %w", err)
			}
			x.sequence++
			x.shot = &screenshot{id: fmt.Sprintf("%s-shot-%d", x.id, x.sequence), width: w, height: h}
			info := screenshotInfo{status: currentStatus(x, false), ScreenshotID: x.shot.id, ImageWidth: w, ImageHeight: h, ContentWidth: x.fixture.Width, ContentHeight: x.fixture.Height, Scale: x.fixture.Scale}
			return payload{metadata: info, png: b.Bytes()}, nil
		})
	})
	addTool(server, "mygo_click", "Deliver one left click in pixels of the latest screenshot. Requires a new screenshot after any mutation.", func(ctx context.Context, in clickInput) (payload, error) {
		return s.operation(ctx, in.SessionID, in.validate, false, func(x *session) (payload, error) {
			shot := x.shot
			if shot == nil || shot.id != in.ScreenshotID {
				return payload{}, errors.New("missing, wrong-session, or stale screenshot_id")
			}
			if in.X >= float64(shot.width) || in.Y >= float64(shot.height) {
				return payload{}, errors.New("click outside screenshot")
			}
			dipX := in.X * float64(x.fixture.Width) / float64(shot.width)
			dipY := in.Y * float64(x.fixture.Height) / float64(shot.height)
			x.shot = nil
			// Keep valid near-edge pixels inside content after float32 rounding.
			x.tester.ClickAt(min(float32(dipX), math.Nextafter32(float32(x.fixture.Width), 0)), min(float32(dipY), math.Nextafter32(float32(x.fixture.Height), 0)))
			return payload{metadata: currentStatus(x, true)}, nil
		})
	})
	addTool(server, "mygo_type", "Deliver committed Unicode to the focused control. Delivery does not imply acceptance; inspect a screenshot.", func(ctx context.Context, in typeInput) (payload, error) {
		return s.operation(ctx, in.SessionID, in.validate, true, func(x *session) (payload, error) {
			x.tester.Type(in.Text)
			return payload{metadata: currentStatus(x, true)}, nil
		})
	})
	addTool(server, "mygo_key", "Deliver a complete key chord; printable keys do not insert text. Modifiers: shift, control, alt, meta, primary.", func(ctx context.Context, in keyInput) (payload, error) {
		return s.operation(ctx, in.SessionID, func() error { _, _, err := parseKey(in); return err }, true, func(x *session) (payload, error) {
			key, mods, err := parseKey(in)
			if err != nil {
				return payload{}, err
			}
			x.tester.Key(mods, key)
			return payload{metadata: currentStatus(x, true)}, nil
		})
	})
	addTool(server, "mygo_advance", "Advance frozen tester time by 0..1000 integer milliseconds; does not wait for external services or imply idleness.", func(ctx context.Context, in advanceInput) (payload, error) {
		return s.operation(ctx, in.SessionID, in.validate, true, func(x *session) (payload, error) {
			x.tester.Advance(time.Duration(in.Milliseconds) * time.Millisecond)
			return payload{metadata: currentStatus(x, true)}, nil
		})
	})
}

type pngBuffer struct{ bytes.Buffer }

func (b *pngBuffer) Write(p []byte) (int, error) {
	if len(p) > maxPNGBytes-b.Len() {
		return 0, errors.New("PNG exceeds 16 MiB response limit")
	}
	n, err := b.Buffer.Write(p)
	if err != nil {
		return n, fmt.Errorf("buffer PNG: %w", err)
	}
	return n, nil
}

var keys = map[string]ui.Key{
	"enter": ui.KeyEnter, "escape": ui.KeyEscape, "backspace": ui.KeyBackspace,
	"tab": ui.KeyTab, "space": ui.KeySpace, "delete": ui.KeyDelete, "insert": ui.KeyInsert,
	"home": ui.KeyHome, "end": ui.KeyEnd, "pageup": ui.KeyPageUp, "pagedown": ui.KeyPageDown,
	"left": ui.KeyLeft, "right": ui.KeyRight, "up": ui.KeyUp, "down": ui.KeyDown,
	"back": ui.KeyBack, "forward": ui.KeyForward,
	"f1": ui.KeyF1, "f2": ui.KeyF2, "f3": ui.KeyF3, "f4": ui.KeyF4, "f5": ui.KeyF5, "f6": ui.KeyF6,
	"f7": ui.KeyF7, "f8": ui.KeyF8, "f9": ui.KeyF9, "f10": ui.KeyF10, "f11": ui.KeyF11, "f12": ui.KeyF12,
	"a": ui.KeyA, "b": ui.KeyB, "c": ui.KeyC, "d": ui.KeyD, "e": ui.KeyE, "f": ui.KeyF,
	"g": ui.KeyG, "h": ui.KeyH, "i": ui.KeyI, "j": ui.KeyJ, "k": ui.KeyK, "l": ui.KeyL,
	"m": ui.KeyM, "n": ui.KeyN, "o": ui.KeyO, "p": ui.KeyP, "q": ui.KeyQ, "r": ui.KeyR,
	"s": ui.KeyS, "t": ui.KeyT, "u": ui.KeyU, "v": ui.KeyV, "w": ui.KeyW, "x": ui.KeyX, "y": ui.KeyY, "z": ui.KeyZ,
	"0": ui.Key0, "1": ui.Key1, "2": ui.Key2, "3": ui.Key3, "4": ui.Key4,
	"5": ui.Key5, "6": ui.Key6, "7": ui.Key7, "8": ui.Key8, "9": ui.Key9,
	"minus": ui.KeyMinus, "equal": ui.KeyEqual, "comma": ui.KeyComma, "period": ui.KeyPeriod,
	"slash": ui.KeySlash, "semicolon": ui.KeySemicolon, "quote": ui.KeyQuote,
	"bracketleft": ui.KeyBracketLeft, "bracketright": ui.KeyBracketRight, "backslash": ui.KeyBackslash,
	"backquote": ui.KeyBackquote, "contextmenu": ui.KeyContextMenu,
}

func parseKey(in keyInput) (ui.Key, ui.Modifiers, error) {
	key, ok := keys[strings.ToLower(in.Key)]
	if !ok {
		return 0, 0, errors.New("unsupported key")
	}
	if len(in.Modifiers) > 5 {
		return 0, 0, errors.New("too many modifiers")
	}
	var mods ui.Modifiers
	for _, name := range in.Modifiers {
		switch strings.ToLower(name) {
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
			return 0, 0, errors.New("unsupported modifier")
		}
	}
	return key, mods, nil
}
