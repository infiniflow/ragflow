//go:build integration

package api

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type apiObject = map[string]any

type restResult struct {
	Status int
	JSON   map[string]any
	Body   []byte
	Header http.Header
}

var resourceSequence atomic.Uint64

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s_%x_%d", prefix, time.Now().UnixNano(), resourceSequence.Add(1))
}

func restCall(t *testing.T, c *ServerTestConfig, method, path string, body any, query map[string]any) restResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// Cleanup calls must remain usable after the test context has been canceled.
	return restCallContext(t, ctx, c, method, path, body, query)
}

func restCallContext(t *testing.T, ctx context.Context, c *ServerTestConfig, method, path string, body any, query map[string]any) restResult {
	t.Helper()
	started := time.Now()
	headers := map[string]string{"Authorization": "Bearer " + c.APIKey}
	if c.APIKey == "" {
		headers["Authorization"] = " "
	}
	requestBody, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	requestURL, err := url.Parse(c.apiBase() + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		t.Fatal("invalid API URL")
	}
	params := requestURL.Query()
	for key, value := range query {
		params.Set(key, fmt.Sprint(value))
	}
	requestURL.RawQuery = params.Encode()
	resp, err := c.RequestJSONContext(ctx, method, path, body, query, headers)
	if err != nil {
		// url.Error includes the unsanitized URL; diagnostics already include its safe form.
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			err = urlErr.Err
		}
		t.Fatalf("%s %s payload=%s duration=%s transport error=%v", method, diagnosticURL(requestURL.String()), diagnosticBody(requestBody, c.APIKey, c.Password), time.Since(started), err)
	}
	defer resp.Body.Close()
	result := restResult{Status: resp.StatusCode, Header: resp.Header}
	result.Body, err = io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if err != nil {
		t.Fatalf("%s %s HTTP=%d duration=%s read: %v", method, diagnosticURL(resp.Request.URL.String()), resp.StatusCode, time.Since(started), err)
	}
	if len(result.Body) > 4<<20 {
		t.Fatalf("%s %s response exceeds 4 MiB", method, diagnosticURL(resp.Request.URL.String()))
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		_ = json.Unmarshal(result.Body, &result.JSON)
	}
	responseLog := diagnosticBody(result.Body, c.APIKey, c.Password)
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		events, parseErr := parseSSE(strings.NewReader(string(result.Body)))
		preview := make([]any, 0, min(len(events), 5))
		for _, event := range events[:min(len(events), 5)] {
			var data any
			if json.Unmarshal([]byte(event.Data), &data) == nil {
				preview = append(preview, apiObject{"event": event.Event, "data": data})
			} else if event.Data == "[DONE]" {
				preview = append(preview, "[DONE]")
			} else {
				preview = append(preview, "[non-JSON SSE data omitted]")
			}
		}
		encoded, _ := json.Marshal(preview)
		responseLog = fmt.Sprintf("SSE bytes=%d events=%d parse_error=%v preview=%s", len(result.Body), len(events), parseErr, diagnosticBody(encoded, c.APIKey, c.Password))
	}
	t.Logf("%s %s payload=%s HTTP=%d code=%v response=%s duration=%s", method, diagnosticURL(resp.Request.URL.String()), diagnosticBody(requestBody, c.APIKey, c.Password), result.Status, result.JSON["code"], responseLog, time.Since(started))
	return result
}

func restCode(t *testing.T, r restResult, status, code int) {
	t.Helper()
	if r.Status != status {
		t.Fatalf("HTTP status=%d want=%d", r.Status, status)
	}
	actual, ok := r.JSON["code"].(float64)
	if !ok || actual != float64(code) {
		t.Fatalf("application code=%v want=%d (see request diagnostic)", r.JSON["code"], code)
	}
}
func restOK(t *testing.T, r restResult) map[string]any {
	t.Helper()
	restCode(t, r, 200, 0)
	return r.JSON
}

func cleanupResource(t *testing.T, c *ServerTestConfig, family, id string) {
	t.Helper()
	t.Cleanup(func() {
		path := "/" + family + "/" + id
		var body any
		if family == "chats" || family == "datasets" {
			path = "/" + family
			body = apiObject{"ids": []string{id}}
		}
		r := restCall(t, c, http.MethodDelete, path, body, nil)
		// Tests exercise deletion themselves; a subsequently missing resource is expected.
		code, ok := r.JSON["code"].(float64)
		if r.Status != 200 || !ok || (code != 0 && code != 102 && code != 103 && code != 404) {
			t.Errorf("cleanup %s/%s failed (see diagnostic)", family, id)
		}
		// A bulk-delete envelope can report partial failures. Prove absence rather
		// than treating a general error code as evidence that cleanup succeeded.
		switch family {
		case "agents":
			restCode(t, restCall(t, c, http.MethodGet, "/agents/"+id, nil, nil), 200, 103)
		case "chats":
			restCode(t, restCall(t, c, http.MethodGet, "/chats/"+id, nil, nil), 200, 404)
		case "datasets":
			remaining := restOK(t, restCall(t, c, http.MethodGet, "/datasets", nil, apiObject{"id": id}))
			if len(array(t, remaining["data"])) != 0 {
				t.Errorf("cleanup left dataset %s present", id)
			}
		}
	})
}

func TestRESTHelperTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/cancel" {
			<-r.Context().Done()
			return
		}
		if r.Method != http.MethodPost || r.URL.Query().Get("keywords") != "hello & 中文" {
			t.Errorf("method=%s query=%s", r.Method, r.URL.RawQuery)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Errorf("unexpected authentication header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{"id":"created"}}`)
	}))
	defer server.Close()
	c := &ServerTestConfig{HostAddress: server.URL, APIVersion: "v1", APIKey: "test-secret"}
	r := restCall(t, c, http.MethodPost, "/echo", apiObject{"password": "not-for-logs"}, apiObject{"keywords": "hello & 中文"})
	if object(t, restOK(t, r)["data"])["id"] != "created" {
		t.Fatal("response object was lost")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	resp, err := c.RequestJSONContext(ctx, http.MethodGet, "/cancel", nil, nil, nil)
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation error=%v", err)
	}
}
