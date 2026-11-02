package parser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The remote PDF backends must honour the pipeline context: a cancelled run
// stops the request instead of waiting on the transport's header timeout.

// holdRequest starts a server that records the request and reports entry, then
// keeps the response open until release closes. Cancelling before the release
// is what proves the call aborts mid-flight: a call that ignored the context
// would read the released response and return without context.Canceled.
func holdRequest(t *testing.T) (*httptest.Server, *atomic.Int64, chan struct{}, chan struct{}) {
	t.Helper()
	var hits atomic.Int64
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	return server, &hits, entered, release
}

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

func TestSoMarkSubmit_CancelDuringRequestAbortsIt(t *testing.T) {
	withSSRFBypass(t)
	server, hits, entered, release := holdRequest(t)
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseHandler()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// The call must return on cancellation before the server writes its
	// response: releasing first would let a context-ignoring client read the
	// 503 instead of failing with context.Canceled, hiding the regression.
	done := make(chan error, 1)
	go func() {
		_, err := soMarkSubmit(ctx, server.URL, "sample.pdf", []byte("%PDF-1.4"), NewPDFParser(), "key")
		done <- err
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the SoMark submit request never reached the server")
	}
	cancel()

	var err error
	select {
	case err = <-done:
	case <-time.After(2 * time.Second):
		releaseHandler()
		t.Fatal("soMarkSubmit returned nothing after cancellation: the in-flight request kept running")
	}
	releaseHandler()

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled in chain", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server requests = %d, want 1 aborted request", got)
	}
}

// Cancelling while the poll request is in flight must abort that request. This
// is the case that proves parsePDFWithSoMark hands its own context to soMarkPoll:
// a variant that derived a fresh background context there would read the
// released response instead of failing with context.Canceled.
func TestParsePDFWithSoMark_CancelDuringPollAbortsPollRequest(t *testing.T) {
	withSSRFBypass(t)
	var pollHits atomic.Int64
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/parse/async_check" {
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"task-1"}}`))
			return
		}
		pollHits.Add(1)
		once.Do(func() { close(entered) })
		<-release
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseHandler()

	p := NewPDFParser()
	p.SoMarkBaseURL = server.URL
	p.SoMarkAPIKey = "key"
	done := make(chan ParseResult, 1)
	go func() { done <- parsePDFWithSoMark(ctx, "sample.pdf", []byte("%PDF-1.4"), p) }()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the SoMark poll request never reached the server")
	}
	cancel()

	var res ParseResult
	select {
	case res = <-done:
	case <-time.After(2 * time.Second):
		releaseHandler()
		t.Fatal("parsePDFWithSoMark returned nothing after cancellation: the in-flight poll kept running")
	}
	releaseHandler()

	if !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled from the in-flight poll", res.Err)
	}
	if got := pollHits.Load(); got != 1 {
		t.Fatalf("poll requests = %d, want the single in-flight request aborted", got)
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

// Cancelling during the first Docling attempt must abort that request and keep
// the remaining endpoint attempts from reaching the server. The parser has to
// return before the held request is released: releasing on cancel would let a
// call that ignored ctx read the released response, and its later endpoint
// attempts would then supply the context.Canceled error the assertion looks
// for.
func TestParsePDFWithDocling_CancelDuringRequestAbortsItAndSkipsRetries(t *testing.T) {
	withSSRFBypass(t)
	server, hits, entered, release := holdRequest(t)
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseHandler()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan ParseResult, 1)
	go func() {
		p := NewPDFParser()
		p.DoclingServerURL = server.URL
		done <- parsePDFWithDocling(ctx, "sample.pdf", []byte("%PDF-1.4"), p)
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the first Docling request never reached the server")
	}
	cancel()

	var res ParseResult
	select {
	case res = <-done:
	case <-time.After(2 * time.Second):
		releaseHandler()
		t.Fatal("parsePDFWithDocling returned nothing after cancellation: the in-flight request kept running")
	}
	releaseHandler()

	if res.Err == nil {
		t.Fatal("error = nil, want the cancelled request to fail the conversion")
	}
	if !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled in chain", res.Err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("server requests = %d, want only the aborted in-flight attempt", got)
	}
}
