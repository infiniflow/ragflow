package models

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestDriverHTTPClientLogsProviderRequestAndResponseWhenEnabled(t *testing.T) {
	t.Setenv(common.EnvLLMDebug, "true")
	core, logs := observer.New(zapcore.InfoLevel)
	previousLogger := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() {
		common.Logger = previousLogger
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rerank" {
			t.Fatalf("path = %q, want /v1/rerank", r.URL.Path)
		}
		fmt.Fprint(w, `{"answer":"ok","access_token":"response-secret"}`)
	}))
	defer server.Close()

	client := NewDriverHTTPClient(true)
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

	entries := logs.FilterMessage(providerLogMessage).All()
	if len(entries) != 1 {
		t.Fatalf("provider log count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["response_code"] != int64(http.StatusOK) {
		t.Errorf("response_code = %#v, want %d", fields["response_code"], http.StatusOK)
	}
	if strings.Contains(fields["provider_url"].(string), "request-secret") {
		t.Errorf("provider_url contains the unredacted API key: %q", fields["provider_url"])
	}
	assertRedactedJSONField(t, fields["payload"].(string), "api_key")
	assertRedactedJSONField(t, fields["response_body"].(string), "access_token")
}

func TestProviderLoggingDisabledAvoidsPayloadWork(t *testing.T) {
	t.Setenv(common.EnvLLMDebug, "")
	core, logs := observer.New(zapcore.InfoLevel)
	previousLogger := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() {
		common.Logger = previousLogger
	})

	payload := &trackingReadCloser{reader: strings.NewReader(`{"query":"hello"}`)}
	client := &http.Client{Transport: newProviderLoggingTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
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
	if resp.Body != http.NoBody {
		t.Fatalf("response body was wrapped while LLM_DEBUG is disabled")
	}
	_ = resp.Body.Close()

	if payload.reads != 0 {
		t.Fatalf("request body reads = %d, want 0 while LLM_DEBUG is disabled", payload.reads)
	}
	if count := logs.FilterMessage(providerLogMessage).Len(); count != 0 {
		t.Fatalf("provider log count = %d, want 0", count)
	}
}

func TestIsLLMDebugEnabled(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: ""},
		{value: "false"},
		{value: "1", want: true},
		{value: "true", want: true},
		{value: " TRUE ", want: true},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv(common.EnvLLMDebug, test.value)
			if got := isLLMDebugEnabled(); got != test.want {
				t.Errorf("isLLMDebugEnabled() = %v, want %v", got, test.want)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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

func assertRedactedJSONField(t *testing.T, body, key string) {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatalf("Unmarshal(%q) error = %v", body, err)
	}
	if got := value[key]; got != redactedLogValue {
		t.Errorf("%s = %#v, want %q", key, got, redactedLogValue)
	}
}

func TestBaseModelDoRequestAuthorizationHeader(t *testing.T) {
	tests := []struct {
		name       string
		apiConfig  *APIConfig
		wantHeader string
	}{
		{name: "nil api config"},
		{name: "nil api key", apiConfig: &APIConfig{}},
		{name: "blank api key", apiConfig: &APIConfig{ApiKey: modelFamilyTestString("  ")}},
		{name: "api key", apiConfig: &APIConfig{ApiKey: modelFamilyTestString(" key-1 ")}, wantHeader: "Bearer key-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != tt.wantHeader {
					t.Fatalf("Authorization = %q, want %q", got, tt.wantHeader)
				}
				if got := r.Header.Get("Content-Type"); got != "application/json" {
					t.Fatalf("Content-Type = %q, want application/json", got)
				}
				fmt.Fprint(w, `{"ok":true}`)
			}))
			defer server.Close()

			model := &BaseModel{httpClient: server.Client(), AllowEmptyAPIKey: true}
			if _, err := model.doRequest(t.Context(), server.URL, tt.apiConfig, map[string]any{"ok": true}, time.Second); err != nil {
				t.Fatalf("doRequest() error = %v", err)
			}
		})
	}
}

func TestBaseModelDoStreamRequestAllowsMissingAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization = %q, want empty", got)
		}
		fmt.Fprint(w, `data: {"ok":true}`)
	}))
	defer server.Close()

	model := &BaseModel{httpClient: server.Client(), AllowEmptyAPIKey: true}
	err := model.doStreamRequest(t.Context(), server.URL, nil, map[string]any{"ok": true}, time.Second, func(body io.ReadCloser) error {
		_, err := io.ReadAll(body)
		return err
	})
	if err != nil {
		t.Fatalf("doStreamRequest() error = %v", err)
	}
}

func modelFamilyTestString(value string) *string {
	return &value
}

// captureEmbedBodies serves an embedding endpoint that records every request
// body and replies with respBody. Drivers that read an OpenAI-shaped response
// can use `{"data":[{"embedding":[0.1],"index":0}]}`; Cohere needs its own
// `{"embeddings":{"float":[[0.1]]}}` shape.
func captureEmbedBodies(t *testing.T, respBody string) (baseURL string, bodies *[]map[string]interface{}) {
	t.Helper()
	captured := &[]map[string]interface{}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		_ = json.Unmarshal(raw, &body)
		*captured = append(*captured, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, captured
}

const openAIShapeEmbeddingBody = `{"data":[{"embedding":[0.1],"index":0}]}`
