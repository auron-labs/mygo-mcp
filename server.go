// Package mygomcp embeds opt-in loopback MCP control in a native MyGo app.
package mygomcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type command struct {
	ctx  context.Context //nolint:containedctx // Queued work retains request cancellation.
	run  func(context.Context) (any, error)
	done chan outcome
}
type outcome struct {
	value any
	err   error
}

// Server owns one bounded application command queue and an HTTP endpoint.
type Server struct {
	ctx        context.Context //nolint:containedctx // Owns the server lifetime, not one request.
	cancel     context.CancelFunc
	queue      chan command
	workerDone chan struct{}
	http       *http.Server
	listener   net.Listener
	closeOnce  sync.Once
	closeErr   error
	windows    map[string]*window
}

// Start explicitly enables control at address (for example 127.0.0.1:7331).
// Call from App.WhenReady. Cancelling ctx starts shutdown; call Close after
// App.Run returns, never from a synchronous UI quit callback.
func Start(ctx context.Context, address string) (*Server, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, errors.New("mygo-mcp: address must use an explicit loopback IP")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("mygo-mcp: start: %w", err)
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("mygo-mcp: listen: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Server{ctx: ctx, cancel: cancel, queue: make(chan command, 32), workerDone: make(chan struct{}), listener: ln, windows: make(map[string]*window)}
	protocol := mcp.NewServer(&mcp.Implementation{Name: "mygo-mcp", Version: "0.1.0"}, nil)
	s.register(protocol)
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return protocol }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 256 << 10, PropagateRequestCancellation: true})
	s.http = &http.Server{Handler: protect(h), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	go s.work() //nolint:contextcheck // The worker inherits each queued request context.
	go func() { _ = s.http.Serve(ln); cancel() }()
	go func() {
		<-ctx.Done()
		shutdown, done := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer done()
		_ = s.Close(shutdown)
	}()
	return s, nil
}

func protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" {
			http.NotFound(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || u.Scheme != "http" || u.Host != r.Host || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
				http.Error(w, "unexpected origin", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// URL is the local MCP endpoint for client configuration.
func (s *Server) URL() string { return "http://" + s.listener.Addr().String() + "/mcp" }

// Close stops accepting work and waits for the worker, bounded by ctx.
func (s *Server) Close(ctx context.Context) error {
	s.closeOnce.Do(func() {
		s.cancel()
		s.closeErr = s.http.Shutdown(ctx)
		if s.closeErr != nil {
			_ = s.http.Close()
		}
		select {
		case <-s.workerDone:
		case <-ctx.Done():
			s.closeErr = ctx.Err()
		}
	})
	return s.closeErr
}

func (s *Server) work() {
	defer close(s.workerDone)
	for {
		select {
		case <-s.ctx.Done():
			return
		case cmd := <-s.queue:
			if err := cmd.ctx.Err(); err != nil {
				cmd.done <- outcome{err: err}
				continue
			}
			ctx, cancel := context.WithTimeout(cmd.ctx, 10*time.Second)
			stop := context.AfterFunc(s.ctx, cancel)
			if s.ctx.Err() != nil {
				cancel()
			}
			v, err := cmd.run(ctx)
			stop()
			cancel()
			cmd.done <- outcome{v, err}
		}
	}
}

func (s *Server) execute(ctx context.Context, run func(context.Context) (any, error)) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("mygo-mcp: queue: %w", err)
	}
	if s.ctx.Err() != nil {
		return nil, errors.New("mygo-mcp: server closed")
	}
	cmd := command{ctx: ctx, run: run, done: make(chan outcome, 1)}
	select {
	case s.queue <- cmd:
	default:
		return nil, errors.New("mygo-mcp: command queue full")
	}
	select {
	case r := <-cmd.done:
		return r.value, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("mygo-mcp: wait: %w", ctx.Err())
	case <-s.ctx.Done():
		return nil, errors.New("mygo-mcp: server closed")
	}
}
