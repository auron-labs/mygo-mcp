// Package mygomcp serves explicitly registered, isolated MyGo headless fixtures over MCP.
package mygomcp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

const (
	maxSessions    = 16
	maxDimension   = 4096
	maxPixels      = 4 * 1024 * 1024
	maxTotalPixels = 16 * 1024 * 1024
	queueSize      = 32
	maxTextBytes   = 64 * 1024
	maxPNGBytes    = 16 * 1024 * 1024
	maxAdvanceMS   = 1000
)

// Fixture constructs isolated state on the UI worker. Factory must return a
// nonblocking view. Cleanup runs once on that same worker after operations finish.
// A factory that fails or panics must clean up its own partially built resources.
type Fixture struct {
	Name          string
	Width, Height int
	Scale         float32 // Zero defaults to 1.
	Factory       func() (view func(*ui.Context), cleanup func(), err error)
}

type screenshot struct {
	id            string
	width, height int
}

type session struct {
	id       string
	fixture  Fixture
	tester   *ui.Tester
	cleanup  func()
	failed   error
	shot     *screenshot
	sequence uint64
}

type command struct {
	ctx     context.Context //nolint:containedctx // Queued work retains the caller's cancellation until dispatch.
	session *session
	fn      func() (any, error)
	result  chan outcome
}

type outcome struct {
	value any
	err   error
}

type service struct {
	sessions []*session
	byID     map[string]*session // Immutable after startup.
	queue    chan command
	stop     <-chan struct{}
	done     chan struct{}
}

func validateFixtures(fixtures []Fixture) ([]Fixture, error) {
	if len(fixtures) == 0 || len(fixtures) > maxSessions {
		return nil, errors.New("register between 1 and 16 fixtures")
	}
	copyFixtures := append([]Fixture(nil), fixtures...)
	total := 0.0
	for i := range copyFixtures {
		f := &copyFixtures[i]
		if f.Scale == 0 {
			f.Scale = 1
		}
		if f.Factory == nil || f.Name == "" || !utf8.ValidString(f.Name) || len(f.Name) > 256 {
			return nil, fmt.Errorf("fixture %d: valid name and factory required", i)
		}
		scale := float64(f.Scale)
		if f.Width <= 0 || f.Height <= 0 || f.Width > maxDimension || f.Height > maxDimension || !finite(scale) || scale <= 0 || scale > 4 {
			return nil, fmt.Errorf("fixture %d: invalid dimensions or scale", i)
		}
		w, h := math.Ceil(float64(f.Width)*scale), math.Ceil(float64(f.Height)*scale)
		// NewTester renders at scale 1 before SetScale; budget both geometries.
		pixels := math.Max(w*h, float64(f.Width)*float64(f.Height))
		if float64(f.Width)*scale < 1 || float64(f.Height)*scale < 1 || w > maxDimension || h > maxDimension || pixels > maxPixels {
			return nil, fmt.Errorf("fixture %d: pixel allocation exceeds limits", i)
		}
		total += pixels
	}
	if total > maxTotalPixels {
		return nil, errors.New("total fixture pixel allocation exceeds limits")
	}
	return copyFixtures, nil
}

func newService(ctx context.Context, fixtures []Fixture) (*service, error) {
	fixtures, err := validateFixtures(fixtures)
	if err != nil {
		return nil, fmt.Errorf("start worker: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("start worker: %w", err)
	}
	run := rand.Text()
	s := &service{byID: make(map[string]*session), queue: make(chan command, queueSize), stop: ctx.Done(), done: make(chan struct{})}
	for i, f := range fixtures {
		x := &session{id: fmt.Sprintf("%s-%d", run, i), fixture: f}
		s.sessions = append(s.sessions, x)
		s.byID[x.id] = x
	}
	ready := make(chan struct{})
	go s.work(ready)
	<-ready
	return s, nil
}

func protect(x *session, fn func() (any, error)) (r outcome) {
	defer func() {
		if p := recover(); p != nil {
			// Panic values may contain committed text; report the type, not the value.
			r.err = fmt.Errorf("session failed after panic (%T); restart harness", p)
			if x != nil {
				x.failed = r.err
				x.shot = nil
			}
		}
	}()
	r.value, r.err = fn()
	return r
}

func (s *service) work(ready chan<- struct{}) {
	defer close(s.done)
	defer func() {
		for _, x := range s.sessions {
			if x.cleanup != nil {
				protect(x, func() (any, error) { x.cleanup(); return struct{}{}, nil })
			}
		}
	}()
	for _, x := range s.sessions {
		r := protect(x, func() (any, error) {
			view, cleanup, err := x.fixture.Factory()
			if err != nil {
				return nil, fmt.Errorf("fixture construction failed: %w", err)
			}
			x.cleanup = cleanup
			if view == nil {
				return nil, errors.New("fixture returned a nil view")
			}
			x.tester = ui.NewTester(view, x.fixture.Width, x.fixture.Height)
			x.tester.SetScale(x.fixture.Scale)
			x.tester.SetFocused(true)
			return struct{}{}, nil
		})
		x.failed = r.err
	}
	close(ready)
	for {
		select {
		case <-s.stop:
			return
		default:
		}
		select {
		case <-s.stop:
			return
		case c := <-s.queue:
			select {
			case <-s.stop:
				return
			default:
			}
			if err := c.ctx.Err(); err != nil {
				c.result <- outcome{err: err}
				continue
			}
			if c.session != nil && c.session.failed != nil {
				c.result <- outcome{err: c.session.failed}
				continue
			}
			c.result <- protect(c.session, c.fn)
		}
	}
}

func (s *service) call(ctx context.Context, id string, fn func(*session) (any, error)) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("queue operation: %w", err)
	}
	var x *session
	if id != "" {
		x = s.byID[id]
		if x == nil {
			return nil, errors.New("unknown session_id")
		}
	}
	select {
	case <-s.stop:
		return nil, errors.New("server shutting down")
	default:
	}
	c := command{ctx: ctx, session: x, fn: func() (any, error) { return fn(x) }, result: make(chan outcome, 1)}
	select {
	case s.queue <- c:
	default:
		return nil, errors.New("operation queue full")
	}
	select {
	case r := <-c.result:
		return r.value, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for operation: %w", ctx.Err())
	case <-s.stop:
		return nil, errors.New("server shutting down")
	}
}

func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }
