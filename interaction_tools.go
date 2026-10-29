package mygomcp

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type scrollInput struct {
	clickRequest
	DeltaX float64 `json:"delta_x" jsonschema:"Horizontal scroll delta in DIPs from -4096 through 4096"`
	DeltaY float64 `json:"delta_y" jsonschema:"Vertical scroll delta in DIPs from -4096 through 4096; positive scrolls down"`
}

func (in scrollInput) validate() error {
	if err := in.clickRequest.validate(); err != nil {
		return err
	}
	if !finite(in.DeltaX) || !finite(in.DeltaY) || math.Abs(in.DeltaX) > maxDimension || math.Abs(in.DeltaY) > maxDimension {
		return errors.New("scroll deltas must be finite and between -4096 and 4096 DIPs")
	}
	return nil
}

type waitTextInput struct {
	SessionID  string  `json:"session_id"`
	Text       string  `json:"text" jsonschema:"Nonempty case-sensitive substring to find in rendered text or labels"`
	TimeoutMS  float64 `json:"timeout_ms" jsonschema:"Maximum simulated time to advance, integer from 0 through 1000"`
	Screenshot bool    `json:"screenshot,omitempty" jsonschema:"Include a PNG of the final observation"`
}

func (in waitTextInput) validate() error {
	if in.Text == "" {
		return errors.New("nonempty text required")
	}
	if err := (typeInput{Text: in.Text}).validate(); err != nil {
		return err
	}
	return (advanceInput{Milliseconds: in.TimeoutMS}).validate()
}

type waitTextInfo struct {
	screenshotInfo
	Matched    bool `json:"matched"`
	AdvancedMS int  `json:"advanced_ms"`
}

func (s *service) registerScrollAndWaitTools(server *mcp.Server) {
	addTool(server, "mygo_scroll", "Deliver immediate touchpad-style scrolling at an observation image pixel point; delta_x/delta_y are DIPs. Requires a fresh observation; set inspect=true for resulting targets without a PNG, or screenshot=true to include a PNG. Does not advance time.", func(ctx context.Context, in scrollInput) (payload, error) {
		return s.operation(ctx, in.SessionID, in.validate, false, func(x *session) (payload, error) {
			cx, cy, err := pointTarget(x, in.clickRequest)
			if err != nil {
				return payload{}, err
			}
			x.shot = nil
			x.tester.Scroll(cx, cy, float32(in.DeltaX), float32(in.DeltaY))
			return finishAction(x, in.Inspect, in.Screenshot)
		})
	})
	addTool(server, "mygo_wait_text", "Check a case-sensitive substring in rendered texts/labels, advancing simulated time in steps of at most 10 ms up to timeout_ms (0..1000). Returns matched, advanced_ms and a fresh observation; timeout is matched=false. Does not wait for external services or guarantee visibility.", func(ctx context.Context, in waitTextInput) (payload, error) {
		return s.operation(ctx, in.SessionID, in.validate, true, func(x *session) (payload, error) {
			x.tester.Frame()
			advanced := 0
			matched := x.tester.HasText(in.Text)
			for !matched && advanced < int(in.TimeoutMS) {
				step := min(10, int(in.TimeoutMS)-advanced)
				x.tester.Advance(time.Duration(step) * time.Millisecond)
				advanced += step
				matched = x.tester.HasText(in.Text)
			}
			p, err := captureObservation(x, in.Screenshot, false)
			if err != nil {
				return payload{}, err
			}
			p.metadata = waitTextInfo{screenshotInfo: p.metadata.(screenshotInfo), Matched: matched, AdvancedMS: advanced}
			return p, nil
		})
	})
}
