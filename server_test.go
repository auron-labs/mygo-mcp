package mygomcp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLoopbackAddressValidation(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8080", "[::1]:8080", "127.0.0.2:0", "[::ffff:127.0.0.1]:0"} {
		if err := validateAddress(address); err != nil {
			t.Errorf("%s: %v", address, err)
		}
	}
	for _, address := range []string{"", ":8080", "0.0.0.0:8080", "[::]:8080", "localhost:8080", "192.168.1.1:8080", "example.com:8080", "[::1%lo]:8080", "127.0.0.1"} {
		if err := Run(t.Context(), address, []Fixture{demoFixture("one", 1)}); err == nil {
			t.Errorf("accepted listener %s", address)
		}
	}
	if err := Run(t.Context(), "127.0.0.1:0", nil); err == nil {
		t.Fatal("accepted no fixtures")
	}
}

func TestHTTPRestrictions(t *testing.T) {
	s := testService(t, demoFixture("one", 1))
	h := s.handler()
	for _, tc := range []struct {
		host, origin, site string
		want               int
	}{
		{"127.0.0.1:8080", "", "", http.StatusMethodNotAllowed},
		{"[::1]:8080", "", "", http.StatusMethodNotAllowed},
		{"localhost:8080", "", "", http.StatusMethodNotAllowed},
		{"evil.example:8080", "", "", http.StatusForbidden},
		{"127.0.0.1:8080", "https://evil.example", "", http.StatusForbidden},
		{"127.0.0.1:8080", "null", "", http.StatusForbidden},
		{"127.0.0.1:8080", "https://127.0.0.1:8080", "", http.StatusForbidden},
		{"127.0.0.1:8080", "http://127.0.0.1:8080/path", "", http.StatusForbidden},
		{"127.0.0.1:8080", "http://127.0.0.1:8080", "cross-site", http.StatusForbidden},
		{"127.0.0.1:8080", "http://127.0.0.1:8080", "same-origin", http.StatusMethodNotAllowed},
	} {
		method := http.MethodGet
		if tc.site == "cross-site" {
			method = http.MethodPost
		}
		req := httptest.NewRequestWithContext(t.Context(), method, "http://"+tc.host+"/mcp", nil)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		if tc.site != "" {
			req.Header.Set("Sec-Fetch-Site", tc.site)
		}
		r := httptest.NewRecorder()
		h.ServeHTTP(r, req)
		if r.Code != tc.want {
			t.Errorf("%+v: status %d", tc, r.Code)
		}
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://127.0.0.1:8080/mcp", nil)
	req.Header.Add("Origin", "http://127.0.0.1:8080")
	req.Header.Add("Origin", "http://127.0.0.1:8080")
	r := httptest.NewRecorder()
	h.ServeHTTP(r, req)
	if r.Code != http.StatusForbidden {
		t.Fatal("multiple origins accepted")
	}
	req = httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://127.0.0.1:8080/mcp", strings.NewReader("{\"text\":\"\xff\"}"))
	r = httptest.NewRecorder()
	h.ServeHTTP(r, req)
	if r.Code != http.StatusBadRequest {
		t.Fatal("invalid UTF-8 body accepted")
	}
	// Also verify SDK Host protection in a real listener context, not only Recorder.
	httpServer := httptest.NewServer(h)
	defer httpServer.Close()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, httpServer.URL, strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "evil.example"
	resp, err := httpServer.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatal("real listener accepted invalid Host")
	}
}

func TestRunLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := Run(t.Context(), address, []Fixture{demoFixture("occupied", 1)}); err == nil {
		t.Fatal("accepted occupied listener")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	var cleaned atomic.Int32
	f := demoFixture("real harness", 1)
	original := f.Factory
	f.Factory = func() (func(*ui.Context), func(), error) {
		view, _, err := original()
		close(started)
		return view, func() { cleaned.Add(1) }, err
	}
	finished := make(chan error, 1)
	go func() { finished <- Run(ctx, address, []Fixture{f}) }()
	waitSignal(t, started)
	client := mcp.NewClient(&mcp.Implementation{Name: "lifecycle", Version: "1"}, nil)
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{Endpoint: "http://" + address + "/mcp", DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	callTool(t, cs, "mygo_sessions", struct{}{}, nil)
	if err := cs.Close(); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	if cleaned.Load() != 1 {
		t.Fatal("Run did not clean up before returning")
	}
	// The cancelled harness releases its port.
	listener, err = (&net.ListenConfig{}).Listen(t.Context(), "tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPRequestBodyLimit(t *testing.T) {
	s := testService(t, demoFixture("one", 1))
	httpServer := httptest.NewServer(s.handler())
	defer httpServer.Close()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"mygo_type","arguments":{"session_id":"` + s.sessions[0].id + `","text":"` + strings.Repeat("x", 1024*1024) + `"}}}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, httpServer.URL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := httpServer.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	response, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode < 400 && !strings.Contains(string(response), `"error"`) {
		t.Fatalf("oversize body accepted: %d %s", resp.StatusCode, response)
	}
}
