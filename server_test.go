package mygomcp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	s, err := Start(context.Background(), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return s
}

func TestAddressAndHTTPBoundaries(t *testing.T) {
	for _, address := range []string{"localhost:0", "0.0.0.0:0", "[::]:0", "192.0.2.1:7331", ":7331"} {
		if s, err := Start(context.Background(), address); err == nil {
			_ = s.Close(context.Background())
			t.Errorf("accepted %s", address)
		}
	}
	s := testServer(t)
	for _, test := range []struct {
		host, origin string
		want         int
	}{
		{"", "https://evil.example", 403},
		{"evil.example", "", 403},
		{"", "null", 403},
		{"", "", 405},
	} {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, s.URL(), nil)
		req.Host = test.host
		req.Header.Set("Origin", test.origin)
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = r.Body.Close()
		if r.StatusCode != test.want {
			t.Errorf("host %q origin %q: %d", test.host, test.origin, r.StatusCode)
		}
	}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, s.URL(), strings.NewReader(strings.Repeat("x", 300<<10)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Body.Close()
	if r.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body: %d", r.StatusCode)
	}
}

func TestSDKClientSchemasAndErrors(t *testing.T) {
	s := testServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: s.URL()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 5 {
		t.Fatalf("tools: %d", len(listed.Tools))
	}
	for _, tool := range listed.Tools {
		if tool.InputSchema == nil {
			t.Errorf("missing schema: %s", tool.Name)
		}
	}
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"mygo_key", map[string]any{"window_id": "unknown", "key": "not-a-key"}},
		{"mygo_screenshot", map[string]any{"window_id": "unknown"}},
		{"mygo_type", map[string]any{"window_id": "unknown", "text": strings.Repeat("x", (64<<10)+1)}},
		{"mygo_click", map[string]any{"window_id": "unknown", "screenshot_id": "old", "x": 1, "y": 1}},
	} {
		r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: test.name, Arguments: test.args})
		if err != nil {
			t.Fatal(err)
		}
		if !r.IsError {
			t.Errorf("%s accepted invalid input", test.name)
		}
	}
}

func TestQueueOrderingCancellationAndShutdown(t *testing.T) {
	s := testServer(t)
	started, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		_, err := s.execute(context.Background(), func(context.Context) (any, error) { close(started); <-release; return 1, nil })
		first <- err
	}()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancelled := command{ctx: ctx, run: func(context.Context) (any, error) { t.Error("executed cancelled command"); return true, nil }, done: make(chan outcome, 1)}
	s.queue <- cancelled
	cancel()
	for range cap(s.queue) - 1 {
		s.queue <- command{ctx: context.Background(), run: func(context.Context) (any, error) { return true, nil }, done: make(chan outcome, 1)}
	}
	if _, err := s.execute(context.Background(), func(context.Context) (any, error) { return true, nil }); err == nil {
		t.Error("unbounded queue")
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if r := <-cancelled.done; !errors.Is(r.err, context.Canceled) {
		t.Errorf("queued cancellation: %v", r.err)
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.execute(context.Background(), func(context.Context) (any, error) { return true, nil }); err == nil {
		t.Error("executed after close")
	}
}

func TestQueueDoesNotInterleave(t *testing.T) {
	s := testServer(t)
	var sequence []int
	started, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		_, err := s.execute(context.Background(), func(context.Context) (any, error) {
			sequence = append(sequence, 1)
			close(started)
			<-release
			sequence = append(sequence, 2)
			return true, nil
		})
		first <- err
	}()
	<-started
	second := command{ctx: context.Background(), run: func(context.Context) (any, error) { sequence = append(sequence, 3); return true, nil }, done: make(chan outcome, 1)}
	s.queue <- second
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	<-second.done
	if len(sequence) != 3 || sequence[0] != 1 || sequence[1] != 2 || sequence[2] != 3 {
		t.Fatalf("interleaved: %v", sequence)
	}
}
