package common

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestDriverHTTPClientLogsProviderRequestAndResponseWhenEnabled(t *testing.T) {
	t.Setenv(EnvLLMDebug, "true")
	core, logs := observer.New(zapcore.InfoLevel)
	previousLogger := Logger
	Logger = zap.New(core)
	t.Cleanup(func() {
		Logger = previousLogger
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rerank" {
			t.Fatalf("path = %q, want /v1/rerank", r.URL.Path)
		}
		fmt.Fprint(w, `{"answer":"ok","access_token":"response-secret"}`)
	}))
	defer server.Close()

	client := GetSchemeSafeHTTPClient()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/v1/rerank?key=request-secret", strings.NewReader(`{"query":"hello","api_key":"payload-secret"}`))
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if _, err = io.ReadAll(resp.Body); err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err = resp.Body.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("provider log count = %d, want 1", len(entries))
	}
	message := entries[0].Message
	if strings.Contains(message, `\"`) {
		t.Errorf("provider log contains escaped JSON: %q", message)
	}
	if !strings.Contains(message, `payload={"api_key":"[REDACTED]","query":"hello"}`) {
		t.Errorf("provider log payload is not raw redacted JSON: %q", message)
	}
	if !strings.Contains(message, `response_code=200 took=`) || !strings.Contains(message, ` first-token=`) || !strings.Contains(message, ` response_body={"access_token":"[REDACTED]","answer":"ok"}`) {
		t.Errorf("provider log response is not raw redacted JSON: %q", message)
	}
	if strings.Contains(message, "request-secret") || strings.Contains(message, "payload-secret") || strings.Contains(message, "response-secret") {
		t.Errorf("provider log contains an unredacted secret: %q", message)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGetMaxDocumentUploadBytes(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int64
	}{
		{name: "default", want: DefaultMaxDocumentUploadBytes},
		{name: "configured", value: "4096", want: 4096},
		{name: "invalid uses default", value: "invalid", want: DefaultMaxDocumentUploadBytes},
		{name: "nonpositive uses default", value: "0", want: DefaultMaxDocumentUploadBytes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvMaxDocumentUploadSize, tt.value)
			if got := GetMaxDocumentUploadBytes(); got != tt.want {
				t.Fatalf("GetMaxDocumentUploadBytes() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestProviderLoggingRecordsTimingsAndTruncatesVectors(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	previousLogger := Logger
	Logger = zap.New(core)
	t.Cleanup(func() {
		Logger = previousLogger
	})

	startedAt := time.Unix(100, 0)
	times := []time.Time{
		startedAt,
		startedAt.Add(time.Second),
		startedAt.Add(3 * time.Second),
	}
	timeIndex := 0
	transport := &providerLoggingTransport{
		debug: true,
		base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"data":[{"embedding":[0.1,0.2,0.3,0.4,0.5]}],"vectors":[[1,2,3,4],[5,6]]}`)),
				Request:    req,
			}, nil
		}),
		now: func() time.Time {
			current := times[timeIndex]
			timeIndex++
			return current
		},
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://provider.example/v1/embeddings", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if _, err = io.ReadAll(resp.Body); err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err = resp.Body.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("provider log count = %d, want 1", len(entries))
	}
	message := entries[0].Message
	if !strings.Contains(message, "took=3s first-token=1s") {
		t.Errorf("provider log timings = %q, want took=3s first-token=1s", message)
	}
	if !strings.Contains(message, `response_body={"data":[{"embedding":[0.1,0.2,0.3]}],"vectors":[[1,2,3],[5,6]]}`) {
		t.Errorf("provider log did not truncate vectors: %q", message)
	}
	if strings.Contains(message, "0.4") || strings.Contains(message, "0.5") || strings.Contains(message, "[1,2,3,4]") {
		t.Errorf("provider log contains vector values after the first three: %q", message)
	}
}

type trackingReadCloser struct {
	reader io.Reader
	reads  int
}

func (r *trackingReadCloser) Read(p []byte) (int, error) {
	r.reads++
	return r.reader.Read(p)
}

func (r *trackingReadCloser) Close() error {
	return nil
}

func TestProviderLoggingDisabledAvoidsPayloadWork(t *testing.T) {
	t.Setenv(EnvLLMDebug, "")
	core, logs := observer.New(zapcore.InfoLevel)
	previousLogger := Logger
	Logger = zap.New(core)
	t.Cleanup(func() {
		Logger = previousLogger
	})

	payload := &trackingReadCloser{reader: strings.NewReader(`{"query":"hello"}`)}
	client := &http.Client{Transport: NewProviderLoggingTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     make(http.Header),
			Body:       http.NoBody,
			Request:    req,
		}, nil
	}))}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://provider.example/v1/rerank", payload)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	// The body is still wrapped so the call can be timed, but nothing is
	// captured and nothing is logged while LLM_DEBUG is disabled.
	if wrapped, ok := resp.Body.(*providerResponseBody); ok && wrapped.capture {
		t.Fatalf("response body was captured while LLM_DEBUG is disabled")
	}
	_ = resp.Body.Close()

	if payload.reads != 0 {
		t.Fatalf("request body reads = %d, want 0 while LLM_DEBUG is disabled", payload.reads)
	}
	if count := logs.Len(); count != 0 {
		t.Fatalf("provider log count = %d, want 0", count)
	}
}

// TestProviderSlowStreamReportsTimingsWithoutDebug covers the always-on timing
// line: a slow streaming call reports took/firstToken even with LLM_DEBUG off,
// and never carries payloads.
func TestProviderSlowStreamReportsTimingsWithoutDebug(t *testing.T) {
	t.Setenv(EnvLLMDebug, "")
	core, logs := observer.New(zapcore.InfoLevel)
	previousLogger := Logger
	Logger = zap.New(core)
	t.Cleanup(func() {
		Logger = previousLogger
	})

	startedAt := time.Unix(100, 0)
	times := []time.Time{
		startedAt,
		startedAt.Add(time.Second),
		startedAt.Add(7 * time.Second),
	}
	timeIndex := 0
	transport := &providerLoggingTransport{
		base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			header := make(http.Header)
			header.Set("Content-Type", "text/event-stream")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     header,
				Body:       io.NopCloser(strings.NewReader("data: {\"choices\":[]}\n\n")),
				Request:    req,
			}, nil
		}),
		now: func() time.Time {
			current := times[timeIndex]
			timeIndex++
			return current
		},
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://provider.example/v1/chat/completions", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if _, err = io.ReadAll(resp.Body); err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	_ = resp.Body.Close()

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("provider log count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	durationField := func(key string) (time.Duration, bool) {
		switch v := fields[key].(type) {
		case time.Duration:
			return v, true
		case int64:
			return time.Duration(v), true
		}
		return 0, false
	}
	if took, ok := durationField("took"); !ok || took != 7*time.Second {
		t.Errorf("took = %v, want 7s (fields %v)", fields["took"], fields)
	}
	if firstToken, ok := durationField("firstToken"); !ok || firstToken != time.Second {
		t.Errorf("firstToken = %v, want 1s (fields %v)", fields["firstToken"], fields)
	}
	if _, ok := fields["payload"]; ok {
		t.Errorf("timing log must not carry payloads while LLM_DEBUG is disabled: %v", fields)
	}
}

// resetSharedHTTPClients drops the process-wide client cache so the next Get*
// call rebuilds it under the test's environment, and restores the previous
// cache afterwards. A test that changed the env must not leave a client built
// from it behind for later tests.
func resetSharedHTTPClients(t *testing.T) {
	t.Helper()
	previousSSRF, previousSchemeSafe := ssrfHttpClient, schemeSafeHttpClient
	ssrfHttpClient, schemeSafeHttpClient = nil, nil
	t.Cleanup(func() {
		ssrfHttpClient, schemeSafeHttpClient = previousSSRF, previousSchemeSafe
	})
}

func responseHeaderTimeoutOf(t *testing.T, client *http.Client) time.Duration {
	t.Helper()
	logging, ok := client.Transport.(*providerLoggingTransport)
	if !ok {
		t.Fatalf("client transport = %T, want *providerLoggingTransport", client.Transport)
	}
	var inner http.RoundTripper
	switch tr := logging.base.(type) {
	case *schemeSafeTransport:
		inner = tr.base
	case *strictSSRFTransport:
		inner = tr.base
	default:
		t.Fatalf("inner transport = %T, want scheme-safe or SSRF transport", logging.base)
	}
	transport, ok := inner.(*http.Transport)
	if !ok {
		t.Fatalf("inner transport base = %T, want *http.Transport", inner)
	}
	return transport.ResponseHeaderTimeout
}

func TestProviderHTTPClientResponseHeaderTimeoutDefaults(t *testing.T) {
	t.Setenv(EnvLLMHTTPResponseHeaderTimeout, "")
	resetSharedHTTPClients(t)

	if got := responseHeaderTimeoutOf(t, GetSchemeSafeHTTPClient()); got != schemeSafeResponseHeaderTimeoutDefault {
		t.Errorf("scheme-safe response header timeout = %v, want %v", got, schemeSafeResponseHeaderTimeoutDefault)
	}
	if got := responseHeaderTimeoutOf(t, GetSSRFHTTPClient()); got != ssrfResponseHeaderTimeoutDefault {
		t.Errorf("SSRF response header timeout = %v, want %v", got, ssrfResponseHeaderTimeoutDefault)
	}
}

func TestProviderHTTPClientResponseHeaderTimeoutOverride(t *testing.T) {
	t.Setenv(EnvLLMHTTPResponseHeaderTimeout, "15s")
	resetSharedHTTPClients(t)

	if got := responseHeaderTimeoutOf(t, GetSchemeSafeHTTPClient()); got != 15*time.Second {
		t.Errorf("scheme-safe response header timeout = %v, want 15s", got)
	}
	if got := responseHeaderTimeoutOf(t, GetSSRFHTTPClient()); got != 15*time.Second {
		t.Errorf("SSRF response header timeout = %v, want 15s", got)
	}
}

func TestProviderHTTPClientResponseHeaderTimeoutIgnoresInvalidValues(t *testing.T) {
	for _, value := range []string{"not-a-duration", "0s", "-5s", "30"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(EnvLLMHTTPResponseHeaderTimeout, value)
			resetSharedHTTPClients(t)

			if got := responseHeaderTimeoutOf(t, GetSchemeSafeHTTPClient()); got != schemeSafeResponseHeaderTimeoutDefault {
				t.Errorf("scheme-safe response header timeout = %v, want default %v for %q", got, schemeSafeResponseHeaderTimeoutDefault, value)
			}
			if got := responseHeaderTimeoutOf(t, GetSSRFHTTPClient()); got != ssrfResponseHeaderTimeoutDefault {
				t.Errorf("SSRF response header timeout = %v, want default %v for %q", got, ssrfResponseHeaderTimeoutDefault, value)
			}
		})
	}
}
