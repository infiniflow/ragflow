//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package utility

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestParseCallResult_TextBlocksConcatenated: text content
// blocks are concatenated into Result.Text with a newline
// separator (matches the "multiple text blocks → one
// human-readable result" convention used by the Python
// implementation).
func TestParseCallResult_TextBlocksConcatenated(t *testing.T) {
	raw := json.RawMessage(`{
		"content": [
			{"type": "text", "text": "first"},
			{"type": "text", "text": "second"}
		]
	}`)
	res, err := parseCallResult(raw)
	if err != nil {
		t.Fatalf("parseCallResult: %v", err)
	}
	if res.Text != "first\nsecond" {
		t.Errorf("Text=%q, want 'first\\nsecond'", res.Text)
	}
	if res.IsError {
		t.Errorf("IsError should be false")
	}
	if len(res.Content) != 2 {
		t.Errorf("Content len=%d, want 2", len(res.Content))
	}
}

// TestParseCallResult_IsErrorFlag: the isError flag is surfaced.
func TestParseCallResult_IsErrorFlag(t *testing.T) {
	raw := json.RawMessage(`{
		"content": [{"type": "text", "text": "tool said no"}],
		"isError": true
	}`)
	res, err := parseCallResult(raw)
	if err != nil {
		t.Fatalf("parseCallResult: %v", err)
	}
	if !res.IsError {
		t.Errorf("IsError should be true")
	}
	if res.Text != "tool said no" {
		t.Errorf("Text=%q, want 'tool said no'", res.Text)
	}
}

// TestParseCallResult_NonTextSkipped: non-text content blocks
// (image / audio / resource) are kept in Content but not
// concatenated into Text. This keeps the contract narrow
// while preserving the full envelope.
func TestParseCallResult_NonTextSkipped(t *testing.T) {
	raw := json.RawMessage(`{
		"content": [
			{"type": "text", "text": "see image"},
			{"type": "image", "data": "...", "mimeType": "image/png"}
		]
	}`)
	res, err := parseCallResult(raw)
	if err != nil {
		t.Fatalf("parseCallResult: %v", err)
	}
	if res.Text != "see image" {
		t.Errorf("Text=%q, want 'see image'", res.Text)
	}
	if len(res.Content) != 2 {
		t.Errorf("Content len=%d, want 2", len(res.Content))
	}
}

// TestParseCallResult_Empty: empty / null result returns an
// empty CallResult with no error.
func TestParseCallResult_Empty(t *testing.T) {
	res, err := parseCallResult(nil)
	if err != nil {
		t.Fatalf("parseCallResult(nil): %v", err)
	}
	if res.Text != "" || res.IsError || len(res.Content) != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}

// TestCallTool_StreamableHTTP: drive the full session
// (initialize → notifications/initialized → tools/call → DELETE) against
// a local httptest server. Verifies the request shape, the
// rendered headers, session id propagation, request order, and
// response parsing.
func TestCallTool_StreamableHTTP(t *testing.T) {
	defer allowLoopbackForTests(t)()
	var deleteCount int32
	var mu sync.Mutex
	var requestOrder []string
	recordRequest := func(method string) {
		mu.Lock()
		defer mu.Unlock()
		requestOrder = append(requestOrder, method)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization=%q, want rendered header", got)
		}
		if r.Method == http.MethodDelete {
			atomic.AddInt32(&deleteCount, 1)
			recordRequest(http.MethodDelete)
			if got := r.Header.Get(sessionHeader); got != "test-session-42" {
				t.Errorf("DELETE session header=%q, want test-session-42", got)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("request method=%s, want POST or DELETE", r.Method)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req jsonRPCRequest
		_ = json.Unmarshal(body, &req)
		recordRequest(req.Method)
		w.Header().Set("Content-Type", "application/json")
		// First call (initialize) returns a session id.
		if req.Method == "initialize" {
			w.Header().Set(sessionHeader, "test-session-42")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":"2025-03-26"}}`))
			return
		}
		if got := r.Header.Get(sessionHeader); got != "test-session-42" {
			t.Errorf("%s session header=%q, want test-session-42", req.Method, got)
		}
		// tools/call returns the canned result.
		if req.Method == "tools/call" {
			_, _ = w.Write([]byte(`{
				"jsonrpc":"2.0","id":2,
				"result":{"content":[{"type":"text","text":"hello from mcp"}],"isError":false}
			}`))
			return
		}
		// notifications/initialized + others: 202 with no body.
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	res, err := CallTool(t.Context(), CallOptions{
		URL:        srv.URL,
		ServerType: TransportStreamableHTTP,
		ToolName:   "echo",
		Arguments:  json.RawMessage(`{"msg":"hi"}`),
		Headers: map[string]string{
			"${header_name}": "Bearer ${token}",
			sessionHeader:    "stale-session",
		},
		Variables: map[string]string{
			"header_name": "Authorization",
			"token":       "test-token",
		},
		HTTPClient: srv.Client(),
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.Text != "hello from mcp" {
		t.Errorf("Text=%q, want 'hello from mcp'", res.Text)
	}
	if res.IsError {
		t.Errorf("IsError should be false")
	}
	if got := atomic.LoadInt32(&deleteCount); got != 1 {
		t.Errorf("DELETE count=%d, want 1", got)
	}
	mu.Lock()
	gotOrder := strings.Join(requestOrder, ",")
	mu.Unlock()
	if want := "initialize,notifications/initialized,tools/call,DELETE"; gotOrder != want {
		t.Errorf("request order=%q, want %q", gotOrder, want)
	}
}

func TestCallTool_StreamableHTTPSessionTerminationStatusPreservesResult(t *testing.T) {
	defer allowLoopbackForTests(t)()

	for _, status := range []int{http.StatusMethodNotAllowed, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var deleteCount int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					atomic.AddInt32(&deleteCount, 1)
					w.WriteHeader(status)
					return
				}
				body, _ := io.ReadAll(r.Body)
				var req jsonRPCRequest
				_ = json.Unmarshal(body, &req)
				w.Header().Set("Content-Type", "application/json")
				switch req.Method {
				case "initialize":
					w.Header().Set(sessionHeader, "test-session")
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{}}`))
				case "tools/call":
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"kept"}]}}`))
				default:
					w.WriteHeader(http.StatusAccepted)
				}
			}))

			res, err := CallTool(t.Context(), CallOptions{
				URL:        srv.URL,
				ServerType: TransportStreamableHTTP,
				ToolName:   "echo",
				Arguments:  json.RawMessage(`{}`),
				HTTPClient: srv.Client(),
				Timeout:    2 * time.Second,
			})
			srv.Close()
			if err != nil {
				t.Fatalf("CallTool returned cleanup error: %v", err)
			}
			if res.Text != "kept" {
				t.Errorf("Text=%q, want kept", res.Text)
			}
			if got := atomic.LoadInt32(&deleteCount); got != 1 {
				t.Errorf("DELETE count=%d, want 1", got)
			}
		})
	}
}

func TestCallTool_StreamableHTTPDeletesAfterMalformedInitializeResponse(t *testing.T) {
	defer allowLoopbackForTests(t)()
	var deleteCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			atomic.AddInt32(&deleteCount, 1)
			if got := r.Header.Get(sessionHeader); got != "test-session" {
				t.Errorf("DELETE session header=%q, want test-session", got)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set(sessionHeader, "test-session")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":`))
	}))
	defer srv.Close()

	_, err := CallTool(t.Context(), CallOptions{
		URL:        srv.URL,
		ServerType: TransportStreamableHTTP,
		ToolName:   "echo",
		Arguments:  json.RawMessage(`{}`),
		HTTPClient: srv.Client(),
		Timeout:    2 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "parse MCP response") {
		t.Fatalf("CallTool error=%v, want initialize response parse error", err)
	}
	if got := atomic.LoadInt32(&deleteCount); got != 1 {
		t.Errorf("DELETE count=%d after malformed initialize response, want 1", got)
	}
}

func TestCallTool_StreamableHTTPSessionTerminationDoesNotFollowRedirect(t *testing.T) {
	defer allowLoopbackForTests(t)()

	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var sourceDeleteCount int32
			var targetDeleteCount int32
			redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					atomic.AddInt32(&targetDeleteCount, 1)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer redirectTarget.Close()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					atomic.AddInt32(&sourceDeleteCount, 1)
					w.Header().Set("Location", redirectTarget.URL)
					w.WriteHeader(status)
					return
				}
				body, _ := io.ReadAll(r.Body)
				var req jsonRPCRequest
				_ = json.Unmarshal(body, &req)
				w.Header().Set("Content-Type", "application/json")
				switch req.Method {
				case "initialize":
					w.Header().Set(sessionHeader, "test-session")
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{}}`))
				case "tools/call":
					_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"kept"}]}}`))
				default:
					w.WriteHeader(http.StatusAccepted)
				}
			}))
			defer srv.Close()

			res, err := CallTool(t.Context(), CallOptions{
				URL:        srv.URL,
				ServerType: TransportStreamableHTTP,
				ToolName:   "echo",
				Arguments:  json.RawMessage(`{}`),
				HTTPClient: srv.Client(),
				Timeout:    2 * time.Second,
			})
			if err != nil {
				t.Fatalf("CallTool returned cleanup error: %v", err)
			}
			if res.Text != "kept" {
				t.Errorf("Text=%q, want kept", res.Text)
			}
			if got := atomic.LoadInt32(&sourceDeleteCount); got != 1 {
				t.Errorf("source DELETE count=%d, want 1", got)
			}
			if got := atomic.LoadInt32(&targetDeleteCount); got != 0 {
				t.Errorf("redirect target DELETE count=%d, want 0", got)
			}
		})
	}
}

func TestCallTool_StreamableHTTPDeletesAfterCallerCancellation(t *testing.T) {
	defer allowLoopbackForTests(t)()
	var deleteCount int32
	callStarted := make(chan struct{}, 1)
	deleteStarted := make(chan struct{}, 1)
	deleteStopped := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			atomic.AddInt32(&deleteCount, 1)
			deleteStarted <- struct{}{}
			<-r.Context().Done()
			deleteStopped <- struct{}{}
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req jsonRPCRequest
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			w.Header().Set(sessionHeader, "test-session")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{}}`))
		case "tools/call":
			callStarted <- struct{}{}
			<-r.Context().Done()
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		_, err := CallTool(ctx, CallOptions{
			URL:        srv.URL,
			ServerType: TransportStreamableHTTP,
			ToolName:   "echo",
			Arguments:  json.RawMessage(`{}`),
			HTTPClient: srv.Client(),
			Timeout:    250 * time.Millisecond,
		})
		errCh <- err
	}()

	select {
	case <-callStarted:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("tools/call did not start")
	}
	select {
	case <-deleteStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("session DELETE did not start after caller cancellation")
	}
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("CallTool error=nil after caller cancellation")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("CallTool error=%v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CallTool did not return within the session cleanup budget")
	}
	select {
	case <-deleteStopped:
	case <-time.After(2 * time.Second):
		t.Fatal("session DELETE context was not canceled at the cleanup deadline")
	}
	if got := atomic.LoadInt32(&deleteCount); got != 1 {
		t.Errorf("DELETE count=%d after cancellation, want 1", got)
	}
}

func TestCallTool_StreamableHTTPWithoutSessionIDSkipsDelete(t *testing.T) {
	defer allowLoopbackForTests(t)()
	var deleteCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			atomic.AddInt32(&deleteCount, 1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req jsonRPCRequest
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{}}`))
		case "tools/call":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"ok"}]}}`))
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer srv.Close()

	res, err := CallTool(t.Context(), CallOptions{
		URL:        srv.URL,
		ServerType: TransportStreamableHTTP,
		ToolName:   "echo",
		Arguments:  json.RawMessage(`{}`),
		HTTPClient: srv.Client(),
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.Text != "ok" {
		t.Errorf("Text=%q, want ok", res.Text)
	}
	if got := atomic.LoadInt32(&deleteCount); got != 0 {
		t.Errorf("DELETE count=%d without session id, want 0", got)
	}
}

// TestCallTool_ServerError: a JSON-RPC error response surfaces
// as a Go error so callers can react (ReAct loop will route
// it as a tool failure).
func TestCallTool_ServerError(t *testing.T) {
	defer allowLoopbackForTests(t)()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req jsonRPCRequest
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		if req.Method == "initialize" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{}}`))
			return
		}
		if req.Method == "tools/call" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"method not found"}}`))
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	_, err := CallTool(t.Context(), CallOptions{
		URL:        srv.URL,
		ServerType: TransportStreamableHTTP,
		ToolName:   "missing",
		Arguments:  json.RawMessage(`{}`),
		HTTPClient: srv.Client(),
		Timeout:    srv.Client().Timeout,
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "tools/call") {
		t.Errorf("error should reference tools/call method, got %q", err.Error())
	}
}

// TestCallTool_MissingURL: an empty URL is rejected up front
// before any network I/O.
func TestCallTool_MissingURL(t *testing.T) {
	_, err := CallTool(t.Context(), CallOptions{ToolName: "x"})
	if err == nil {
		t.Fatalf("expected error for empty URL")
	}
	if !strings.Contains(err.Error(), "Invalid url") {
		t.Errorf("got %v, want URL error", err)
	}
}

// TestCallTool_MissingToolName: an empty tool name is rejected
// up front.
func TestCallTool_MissingToolName(t *testing.T) {
	_, err := CallTool(t.Context(), CallOptions{URL: "http://localhost:0"})
	if err == nil {
		t.Fatalf("expected error for empty tool name")
	}
	if !strings.Contains(err.Error(), "tool name") {
		t.Errorf("got %v, want tool-name error", err)
	}
}

// TestCallTool_InvalidArgumentsJSON: non-JSON arguments surface
// a clear error before hitting the network.
func TestCallTool_InvalidArgumentsJSON(t *testing.T) {
	defer allowLoopbackForTests(t)()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{}}`))
	}))
	defer srv.Close()
	_, err := CallTool(t.Context(), CallOptions{
		URL:        srv.URL,
		ServerType: TransportStreamableHTTP,
		ToolName:   "x",
		Arguments:  json.RawMessage(`{not json}`),
		HTTPClient: srv.Client(),
		Timeout:    srv.Client().Timeout,
	})
	if err == nil {
		t.Fatalf("expected error for invalid arguments JSON")
	}
	// The session initialize call still goes out, so the
	// error message references the post-initialize path. The
	// important property is "non-nil error".
	if !strings.Contains(err.Error(), "json") && !strings.Contains(err.Error(), "JSON") {
		t.Errorf("error should mention JSON, got %v", err)
	}
}
