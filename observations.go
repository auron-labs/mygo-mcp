package mygomcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"math"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type targetBounds struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
}

type targetInfo struct {
	Label           string        `json:"label"`
	Matches         int           `json:"matches"`
	Bounds          *targetBounds `json:"bounds,omitempty"`
	CenterInContent bool          `json:"center_in_content"`
}

type targetError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Target  *targetInfo `json:"target,omitempty"`
}

func (e *targetError) Error() string { return e.Message }

func observationID(screenshotID, observation string) (string, error) {
	if screenshotID != "" && observation != "" && screenshotID != observation {
		return "", &targetError{Code: "invalid_observation", Message: "screenshot_id and observation_id disagree"}
	}
	if observation != "" {
		return observation, nil
	}
	if screenshotID == "" {
		return "", &targetError{Code: "invalid_observation", Message: "observation_id or screenshot_id required"}
	}
	return screenshotID, nil
}

func requireObservation(x *session, screenshotID, observation string) error {
	id, err := observationID(screenshotID, observation)
	if err != nil {
		return err
	}
	if x.shot == nil || x.shot.id != id {
		return &targetError{Code: "stale_observation", Message: "missing, wrong-session, or stale observation; inspect or take a screenshot again"}
	}
	return nil
}

func pointTarget(x *session, in clickRequest) (float32, float32, error) {
	if err := requireObservation(x, in.ScreenshotID, in.ObservationID); err != nil {
		return 0, 0, err
	}
	if !finite(in.X) || !finite(in.Y) || in.X < 0 || in.Y < 0 || in.X >= float64(x.shot.width) || in.Y >= float64(x.shot.height) {
		return 0, 0, &targetError{Code: "out_of_bounds", Message: "coordinates must be finite and inside observation image dimensions"}
	}
	dipX := in.X * float64(x.fixture.Width) / float64(x.shot.width)
	dipY := in.Y * float64(x.fixture.Height) / float64(x.shot.height)
	// Keep valid near-edge pixels inside content after float32 rounding.
	return min(float32(dipX), math.Nextafter32(float32(x.fixture.Width), 0)), min(float32(dipY), math.Nextafter32(float32(x.fixture.Height), 0)), nil
}

func labelTarget(x *session, in clickLabelRequest) (float32, float32, error) {
	if err := requireObservation(x, in.ScreenshotID, in.ObservationID); err != nil {
		return 0, 0, err
	}
	for _, target := range x.shot.targets {
		if target.Label != in.Label {
			continue
		}
		if target.Matches != 1 {
			return 0, 0, &targetError{Code: "ambiguous", Message: "label matches multiple elements; use coordinates or distinct labels", Target: &target}
		}
		if target.Bounds == nil {
			return 0, 0, &targetError{Code: "invalid_geometry", Message: "label has no finite positive bounds", Target: &target}
		}
		if !target.CenterInContent {
			return 0, 0, &targetError{Code: "offscreen", Message: "label center is outside content; scroll before clicking", Target: &target}
		}
		b := target.Bounds
		return b.X + b.Width/2, b.Y + b.Height/2, nil
	}
	return 0, 0, &targetError{Code: "not_found", Message: "no element has this exact text or label", Target: &targetInfo{Label: in.Label}}
}

func inspectTargets(x *session, labels []string) []targetInfo {
	targets := make([]targetInfo, 0, len(labels))
	indices := make(map[string]int, len(labels))
	for _, label := range labels {
		if index, ok := indices[label]; ok {
			targets[index].Matches++
		} else {
			indices[label] = len(targets)
			targets = append(targets, targetInfo{Label: label, Matches: 1})
		}
	}
	for i := range targets {
		target := &targets[i]
		// Find exposes only the first match, so duplicate bounds would be misleading.
		if target.Matches != 1 {
			continue
		}
		r, ok := x.tester.Find(target.Label)
		if !ok || !finite(float64(r.X)) || !finite(float64(r.Y)) || !finite(float64(r.W)) || !finite(float64(r.H)) {
			continue
		}
		target.Bounds = &targetBounds{X: r.X, Y: r.Y, Width: r.W, Height: r.H}
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		target.CenterInContent = r.W > 0 && r.H > 0 && cx >= 0 && cy >= 0 && cx < float32(x.fixture.Width) && cy < float32(x.fixture.Height)
	}
	return targets
}

func captureObservation(x *session, withPNG, delivered bool) (payload, error) {
	x.shot = nil
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
	if withPNG {
		if err := png.Encode(&b, im); err != nil {
			return payload{}, fmt.Errorf("encode screenshot: %w", err)
		}
	}
	labels := x.tester.Texts()
	targets := inspectTargets(x, labels)
	x.sequence++
	x.shot = &screenshot{id: fmt.Sprintf("%s-shot-%d", x.id, x.sequence), width: w, height: h, targets: targets}
	info := screenshotInfo{status: currentStatus(x, delivered), ObservationID: x.shot.id, ImageWidth: w, ImageHeight: h, ContentWidth: x.fixture.Width, ContentHeight: x.fixture.Height, Scale: x.fixture.Scale, Labels: labels, Targets: targets}
	if withPNG {
		info.ScreenshotID = info.ObservationID
	}
	return payload{metadata: info, png: b.Bytes()}, nil
}

func finishAction(x *session, withPNG bool) (payload, error) {
	if !withPNG {
		return payload{metadata: currentStatus(x, true)}, nil
	}
	p, err := captureObservation(x, true, true)
	if err != nil {
		return payload{}, fmt.Errorf("input delivered; screenshot failed, do not replay input: %w", err)
	}
	return p, nil
}

func (s *service) observe(ctx context.Context, in sessionInput, withPNG bool) (payload, error) {
	return s.operation(ctx, in.SessionID, validSessionInput, false, func(x *session) (payload, error) {
		x.shot = nil
		x.tester.Frame()
		return captureObservation(x, withPNG, false)
	})
}

func (s *service) registerObservationTools(server *mcp.Server) {
	addTool(server, "mygo_screenshot", "Render current headless state as PNG and structured targets; supersedes the previous observation. screenshot_id and observation_id are aliases.", func(ctx context.Context, in sessionInput) (payload, error) {
		return s.observe(ctx, in, true)
	})
	addTool(server, "mygo_inspect", "Render and inspect texts/labels, match counts and unique bounds in DIPs without encoding a PNG. Returns a fresh observation_id for clicks and scrolling. Targets may be disabled, covered or offscreen.", func(ctx context.Context, in sessionInput) (payload, error) {
		return s.observe(ctx, in, false)
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
