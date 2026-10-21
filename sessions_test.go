package mygomcp

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func waitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for worker")
	}
}

func TestFixtureLimits(t *testing.T) {
	base := demoFixture("valid", 1)
	for _, change := range []func(*Fixture){
		func(f *Fixture) { f.Factory = nil }, func(f *Fixture) { f.Name = "" }, func(f *Fixture) { f.Name = "\xff" }, func(f *Fixture) { f.Name = strings.Repeat("x", 257) },
		func(f *Fixture) { f.Width = 0 }, func(f *Fixture) { f.Height = -1 }, func(f *Fixture) { f.Width = 4097 },
		func(f *Fixture) { f.Scale = -1 }, func(f *Fixture) { f.Scale = float32(math.NaN()) }, func(f *Fixture) { f.Scale = float32(math.Inf(1)) }, func(f *Fixture) { f.Scale = 5 },
		func(f *Fixture) { f.Scale = 1e-8 }, func(f *Fixture) { f.Width = 3000; f.Height = 3000 }, func(f *Fixture) { f.Width = 2000; f.Scale = 3 },
	} {
		f := base
		change(&f)
		if _, err := validateFixtures([]Fixture{f}); err == nil {
			t.Fatalf("accepted invalid fixture: %+v", f)
		}
	}
	if _, err := validateFixtures(nil); err == nil {
		t.Fatal("accepted empty registration")
	}
	if _, err := validateFixtures(make([]Fixture, maxSessions+1)); err == nil {
		t.Fatal("accepted excess sessions")
	}
	big := base
	big.Width = 2048
	big.Height = 2048
	if _, err := validateFixtures([]Fixture{big, big, big, big, big}); err == nil {
		t.Fatal("accepted excess total allocation")
	}
	base.Scale = 0
	got, err := validateFixtures([]Fixture{base})
	if err != nil || got[0].Scale != 1 || base.Scale != 0 {
		t.Fatal("default scale or immutable input failed")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := newService(ctx, []Fixture{base}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestBoundaryValidation(t *testing.T) {
	for _, n := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), math.MaxFloat64, -1, 0.5, 1001} {
		if (advanceInput{Milliseconds: n}).validate() == nil {
			t.Fatalf("accepted duration %v", n)
		}
	}
	for _, n := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1} {
		if (clickInput{ScreenshotID: "id", X: n}).validate() == nil || (clickInput{ScreenshotID: "id", Y: n}).validate() == nil {
			t.Fatalf("accepted coordinate %v", n)
		}
	}
	for _, text := range []string{"\xff", strings.Repeat("x", maxTextBytes+1)} {
		if (typeInput{Text: text}).validate() == nil {
			t.Fatal("accepted invalid text")
		}
	}
	if (typeInput{Text: strings.Repeat("x", maxTextBytes)}).validate() != nil {
		t.Fatal("rejected text boundary")
	}
	for _, n := range []float64{0, 1000} {
		if (advanceInput{Milliseconds: n}).validate() != nil {
			t.Fatal("rejected duration boundary")
		}
	}
}

func TestQueueCancellationOrderingAndFull(t *testing.T) {
	s := testService(t, demoFixture("one", 1), demoFixture("two", 1))
	id := s.sessions[0].id
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	firstDone := make(chan error, 1)
	go func() {
		_, err := s.call(t.Context(), id, func(*session) (any, error) { close(started); <-release; return true, nil })
		firstDone <- err
	}()
	waitSignal(t, started)
	var order []int
	var lastResult chan outcome
	for i := range queueSize {
		ctx := t.Context()
		if i == 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			cancel()
		}
		lastResult = make(chan outcome, 1)
		s.queue <- command{ctx: ctx, session: s.sessions[i%2], fn: func() (any, error) { order = append(order, i); return true, nil }, result: lastResult}
	}
	if _, err := s.call(t.Context(), id, func(*session) (any, error) { return true, nil }); err == nil || !strings.Contains(err.Error(), "full") {
		t.Fatal("full queue did not reject", err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.call(cancelled, id, func(*session) (any, error) { t.Error("cancelled operation ran"); return true, nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// Release without closing twice in deferred cleanup.
	release <- struct{}{}
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if result := <-lastResult; result.err != nil {
		t.Fatal(result.err)
	}
	_, err := s.call(t.Context(), id, func(*session) (any, error) {
		if len(order) != queueSize-1 {
			t.Errorf("queued cancellation executed: %v", order)
		}
		for i, n := range order {
			if n != i+1 {
				t.Errorf("queue reordered: %v", order)
				break
			}
		}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCancellationAfterDispatchInvalidatesScreenshot(t *testing.T) {
	s := testService(t, demoFixture("one", 1))
	cs := testClient(t, s)
	id := s.sessions[0].id
	_ = takeShot(t, cs, id)
	ctx, cancel := context.WithCancel(t.Context())
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	returned := make(chan error, 1)
	go func() {
		_, err := s.operation(ctx, id, validSessionInput, true, func(x *session) (payload, error) {
			x.tester.Type("delivered without editable focus")
			close(started)
			<-release
			return payload{metadata: currentStatus(x, true)}, nil
		})
		returned <- err
	}()
	waitSignal(t, started)
	cancel()
	if err := <-returned; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var secondRan atomic.Bool
	secondDone := make(chan error, 1)
	go func() {
		_, err := s.call(t.Context(), id, func(x *session) (any, error) { secondRan.Store(true); return x.shot == nil, nil })
		secondDone <- err
	}()
	if secondRan.Load() {
		t.Fatal("second operation ran concurrently")
	}
	release <- struct{}{}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	v, err := s.call(t.Context(), id, func(x *session) (any, error) { return x.shot == nil, nil })
	if err != nil || v != true {
		t.Fatal("cancelled dispatched action retained screenshot", err)
	}
}

func TestShutdownWaitsForSynchronousRenderAndCleansOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	var block bool
	var cleaned atomic.Int32
	f := Fixture{Name: "blocking", Width: 100, Height: 50, Factory: func() (func(*ui.Context), func(), error) {
		return func(c *ui.Context) {
			if block {
				close(started)
				<-release
				block = false
			}
			ui.Text(c, "view")
		}, func() { cleaned.Add(1) }, nil
	}}
	s, err := newService(ctx, []Fixture{f})
	if err != nil {
		t.Fatal(err)
	}
	requestDone := make(chan error, 1)
	go func() {
		_, err := s.call(t.Context(), s.sessions[0].id, func(x *session) (any, error) { block = true; x.tester.Frame(); return true, nil })
		requestDone <- err
	}()
	waitSignal(t, started)
	var queuedRan atomic.Bool
	s.queue <- command{ctx: t.Context(), session: s.sessions[0], fn: func() (any, error) { queuedRan.Store(true); return true, nil }, result: make(chan outcome, 1)}
	cancel()
	if err := <-requestDone; err == nil {
		t.Fatal("waiting caller did not observe shutdown")
	}
	if cleaned.Load() != 0 {
		t.Fatal("cleanup raced with synchronous render")
	}
	select {
	case <-s.done:
		t.Fatal("worker stopped while render was running")
	default:
	}
	if _, err := s.call(t.Context(), s.sessions[0].id, func(*session) (any, error) { return true, nil }); err == nil {
		t.Fatal("accepted operation after shutdown")
	}
	release <- struct{}{}
	waitSignal(t, s.done)
	if cleaned.Load() != 1 || queuedRan.Load() {
		t.Fatal("cleanup count or queued shutdown failed")
	}
}

func TestSessionPanicsAndConstructionFailures(t *testing.T) {
	var cleaned atomic.Int32
	t.Cleanup(func() {
		if cleaned.Load() != 2 {
			t.Errorf("successful factory cleanup count: %d", cleaned.Load())
		}
	})
	var renderPanic bool
	fixtures := []Fixture{
		{Name: "factory panic", Width: 100, Height: 50, Factory: func() (func(*ui.Context), func(), error) { panic("secret text") }},
		{Name: "factory error", Width: 100, Height: 50, Factory: func() (func(*ui.Context), func(), error) {
			return nil, func() { t.Error("partial cleanup belongs to factory") }, errors.New("service unavailable")
		}},
		{Name: "render panic", Width: 100, Height: 50, Factory: func() (func(*ui.Context), func(), error) {
			return func(*ui.Context) { panic("secret text") }, func() { cleaned.Add(1) }, nil
		}},
		{Name: "later panic", Width: 100, Height: 50, Factory: func() (func(*ui.Context), func(), error) {
			return func(c *ui.Context) {
				if renderPanic {
					panic("secret text")
				}
				ui.Text(c, "safe")
			}, func() { cleaned.Add(1); panic("cleanup panic") }, nil
		}},
		demoFixture("healthy", 1),
	}
	s := testService(t, fixtures...)
	cs := testClient(t, s)
	for _, x := range s.sessions[:3] {
		if x.failed == nil || strings.Contains(x.failed.Error(), "secret text") {
			t.Fatal("panic was not isolated or leaked text", x.failed)
		}
		toolError(t, cs, "mygo_screenshot", sessionInput{x.id})
	}
	x := s.sessions[3]
	if _, err := s.call(t.Context(), x.id, func(x *session) (any, error) { renderPanic = true; x.tester.Frame(); return true, nil }); err == nil {
		t.Fatal("render panic escaped")
	}
	toolError(t, cs, "mygo_key", keyInput{x.id, "a", nil})
	_ = takeShot(t, cs, s.sessions[4].id)
	var list struct {
		Sessions []sessionInfo `json:"sessions"`
	}
	callTool(t, cs, "mygo_sessions", struct{}{}, &list)
	if len(list.Sessions) != 5 || list.Sessions[3].Failed == "" || len(list.Sessions[3].Operations) != 0 {
		t.Fatal("failed session not discoverable")
	}
	if cleaned.Load() != 0 {
		t.Fatal("cleanup ran before shutdown")
	}
}
