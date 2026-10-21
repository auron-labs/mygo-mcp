package mygomcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func validateAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("explicit loopback IP and port required: %w", err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.IsLoopback() || ip.Zone() != "" {
		return errors.New("listener must use an explicit loopback IP")
	}
	return nil
}

func (s *service) handler() http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "mygo-mcp", Version: "0.1.0"}, nil)
	s.registerTools(server)
	transport := http.NewCrossOriginProtection().Handler(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Keep SDK Host checks too; this also covers in-process HTTP test clients.
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		ip, err := netip.ParseAddr(strings.Trim(host, "[]"))
		if host != "localhost" && (err != nil || !ip.IsLoopback()) {
			http.Error(w, "invalid Host", http.StatusForbidden)
			return
		}
		if origins := r.Header.Values("Origin"); len(origins) != 0 {
			origin, err := url.Parse(origins[0])
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			if len(origins) != 1 || err != nil || origin.User != nil || origin.Host != r.Host || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.Scheme != scheme {
				http.Error(w, "unexpected browser origin", http.StatusForbidden)
				return
			}
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024*1024))
		if err != nil {
			http.Error(w, "request body exceeds limit or could not be read", http.StatusBadRequest)
			return
		}
		if !utf8.Valid(body) {
			http.Error(w, "request body must be UTF-8", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		transport.ServeHTTP(w, r)
	})
}

// Run binds address (for example 127.0.0.1:8080) and serves /mcp until ctx is
// cancelled. It waits for in-flight UI work and fixture cleanup before returning.
// No server is started by importing this package. Handlers must not block.
func Run(ctx context.Context, address string, fixtures []Fixture) error {
	if err := validateAddress(address); err != nil {
		return err
	}
	if _, err := validateFixtures(fixtures); err != nil {
		return err
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer func() { _ = listener.Close() }()
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	s, err := newService(workerCtx, fixtures)
	if err != nil {
		return err
	}
	defer func() { cancel(); <-s.done }()
	mux := http.NewServeMux()
	mux.Handle("/mcp", s.handler())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, MaxHeaderBytes: 16 * 1024}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	select {
	case err = <-finished:
	case <-ctx.Done():
		// Close transports promptly; the worker still finishes synchronous input.
		err = server.Close()
		serveErr := <-finished
		if err == nil && !errors.Is(serveErr, http.ErrServerClosed) {
			err = serveErr
		}
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("serve MCP: %w", err)
	}
	return nil
}
