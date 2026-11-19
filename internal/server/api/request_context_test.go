package api

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAPIRequestDrainsAfterProcessCancellation(t *testing.T) {
	processCtx, stop := context.WithCancel(t.Context())
	defer stop()
	requestCtx, cancel := newRequestContext(processCtx)
	started := make(chan struct{})
	release := make(chan struct{})
	accepted := make(chan string, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
			w.WriteHeader(http.StatusServiceUnavailable)
		case <-release:
			_, _ = io.WriteString(w, "ok")
		}
	}))
	srv.Config.BaseContext = func(net.Listener) context.Context { return requestCtx }
	srv.Start()
	t.Cleanup(func() {
		cancel()
		srv.Close()
	})
	go func() {
		resp, err := srv.Client().Get(srv.URL)
		if err != nil {
			accepted <- err.Error()
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			accepted <- err.Error()
			return
		}
		accepted <- string(body)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	stop()
	if err := requestCtx.Err(); err != nil {
		t.Fatalf("process shutdown canceled the request context: %v", err)
	}
	close(release)
	shutdownCtx, finish := context.WithTimeout(t.Context(), 3*time.Second)
	defer finish()
	if err := shutdownHTTPServer(shutdownCtx, "API", srv.Config); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-accepted:
		if result != "ok" {
			t.Fatalf("accepted request returned %q", result)
		}
	case <-shutdownCtx.Done():
		t.Fatal("request did not complete during shutdown")
	}
}

func TestAPIRequestCanceledAfterDrainDeadline(t *testing.T) {
	processCtx, stop := context.WithCancel(t.Context())
	defer stop()
	requestCtx, cancel := newRequestContext(processCtx)
	started := make(chan struct{})
	canceled := make(chan struct{})
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	srv.Config.BaseContext = func(net.Listener) context.Context { return requestCtx }
	srv.Start()
	t.Cleanup(func() {
		cancel()
		srv.Close()
	})
	go func() {
		if resp, err := srv.Client().Get(srv.URL); err == nil {
			resp.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	stop()
	if err := requestCtx.Err(); err != nil {
		t.Fatalf("process shutdown canceled the request context: %v", err)
	}
	shutdownCtx, finish := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer finish()
	if err := shutdownHTTPServer(shutdownCtx, "API", srv.Config); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v, want deadline exceeded", err)
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("request was still active after the drain deadline")
	}
}
