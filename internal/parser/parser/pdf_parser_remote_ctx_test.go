package parser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The remote PDF backends must honour the pipeline context: a cancelled run
// stops the request instead of waiting on the transport's header timeout.

func TestParsePDFWithSoMark_CancelledContextStopsBeforeSubmit(t *testing.T) {
	withSSRFBypass(t)
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-1"}}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	p := NewPDFParser()
	p.SoMarkBaseURL = server.URL
	p.SoMarkAPIKey = "key"
	res := parsePDFWithSoMark(ctx, "sample.pdf", []byte("%PDF-1.4"), p)
	if res.Err == nil {
		t.Fatalf("error = nil, want context cancellation from submit")
	}
	if !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled in chain", res.Err)
	}
	if hits != 0 {
		t.Fatalf("server requests = %d, want 0 after cancellation", hits)
	}
}

func TestSoMarkPoll_CancelledContextStopsRequest(t *testing.T) {
	withSSRFBypass(t)
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"code":0,"data":{"status":"RUNNING"}}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := soMarkPoll(ctx, server.URL, "task-1", "key")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled in chain", err)
	}
	if hits != 0 {
		t.Fatalf("server requests = %d, want 0 after cancellation", hits)
	}
}

func TestParsePDFWithDocling_CancelledContextStopsBeforeFirstEndpoint(t *testing.T) {
	withSSRFBypass(t)
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	p := NewPDFParser()
	p.DoclingServerURL = server.URL
	res := parsePDFWithDocling(ctx, "sample.pdf", []byte("%PDF-1.4"), p)
	if res.Err == nil {
		t.Fatalf("error = nil, want the four endpoint attempts to fail on cancellation")
	}
	// The loop tries four endpoints; every one must fail with the cancelled
	// context rather than reaching the server, proving ctx is threaded.
	if hits != 0 {
		t.Fatalf("server requests = %d, want 0 after cancellation", hits)
	}
	if !strings.Contains(res.Err.Error(), "context canceled") {
		t.Fatalf("error = %v, want context canceled in chain", res.Err)
	}
}
