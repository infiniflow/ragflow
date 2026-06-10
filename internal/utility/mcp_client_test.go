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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// allowLoopbackForTests overrides the SSRF guard's resolver so 127.0.0.1
// targets used by httptest are accepted by AssertURLSafe.
func allowLoopbackForTests(t *testing.T) func() {
	t.Helper()
	orig := LookupHost
	LookupHost = func(host string) ([]string, error) {
		// Return a public IPv4 so the guard sees the host as global; the
		// httptest server is on loopback but we connect via raw URL.
		return []string{"8.8.8.8"}, nil
	}
	return func() { LookupHost = orig }
}

func TestFetchToolsStreamableHTTPJSON(t *testing.T) {
	defer allowLoopbackForTests(t)()

	var initCount, listCount, notifyCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			// testing.T's Fatal* must run on the test goroutine; surface the
			// failure via Errorf and bail the handler out instead.
			t.Errorf("invalid request body: %v", err)
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		switch req["method"] {
		case "initialize":
			atomic.AddInt32(&initCount, 1)
			w.Header().Set(sessionHeader, "test-session")
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%v,"result":{"capabilities":{}}}`, req["id"])
		case "notifications/initialized":
			atomic.AddInt32(&notifyCount, 1)
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			atomic.AddInt32(&listCount, 1)
			if got := r.Header.Get(sessionHeader); got != "test-session" {
				t.Errorf("expected session header to be propagated, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%v,"result":{"tools":[{"name":"search","description":"Find docs","inputSchema":{"type":"object"}},{"name":"fetch"}]}}`, req["id"])
		default:
			http.Error(w, "unexpected method", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	tools, err := FetchTools(context.Background(), FetchOptions{
		URL:        srv.URL,
		ServerType: TransportStreamableHTTP,
		HTTPClient: srv.Client(),
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(tools); got != 2 {
		t.Fatalf("expected 2 tools, got %d", got)
	}
	if tools[0].Name != "search" || tools[0].Description != "Find docs" {
		t.Errorf("tool 0 = %+v", tools[0])
	}
	if tools[1].Name != "fetch" {
		t.Errorf("tool 1 = %+v", tools[1])
	}
	if atomic.LoadInt32(&initCount) != 1 || atomic.LoadInt32(&notifyCount) != 1 || atomic.LoadInt32(&listCount) != 1 {
		t.Errorf("expected 1 init / 1 notify / 1 list, got %d/%d/%d", initCount, notifyCount, listCount)
	}
}

func TestFetchToolsStreamableHTTPErrorResponse(t *testing.T) {
	defer allowLoopbackForTests(t)()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		_ = json.Unmarshal(body, &req)
		if req["method"] == "initialize" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%v,"error":{"code":-32600,"message":"bad init"}}`, req["id"])
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	_, err := FetchTools(context.Background(), FetchOptions{
		URL:        srv.URL,
		ServerType: TransportStreamableHTTP,
		HTTPClient: srv.Client(),
		Timeout:    2 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "bad init") {
		t.Fatalf("expected MCP error to surface, got %v", err)
	}
}

func TestFetchToolsSSE(t *testing.T) {
	defer allowLoopbackForTests(t)()

	type ssePush struct {
		event string
		data  string
	}
	pushes := make(chan ssePush, 4)

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("response writer is not a flusher")
			http.Error(w, "no flusher", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fmt.Fprintf(w, "event: endpoint\ndata: /messages\n\n")
		flusher.Flush()
		ctx := r.Context()
		for {
			select {
			case p := <-pushes:
				if p.event != "" {
					fmt.Fprintf(w, "event: %s\n", p.event)
				}
				fmt.Fprintf(w, "data: %s\n\n", p.data)
				flusher.Flush()
			case <-ctx.Done():
				return
			}
		}
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("invalid request body: %v", err)
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		switch req["method"] {
		case "initialize":
			pushes <- ssePush{event: "message", data: fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":{"capabilities":{}}}`, req["id"])}
		case "notifications/initialized":
		case "tools/list":
			pushes <- ssePush{event: "message", data: fmt.Sprintf(`{"jsonrpc":"2.0","id":%v,"result":{"tools":[{"name":"alpha"}]}}`, req["id"])}
		}
		w.WriteHeader(http.StatusAccepted)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	tools, err := FetchTools(context.Background(), FetchOptions{
		URL:        srv.URL + "/sse",
		ServerType: TransportSSE,
		HTTPClient: srv.Client(),
		Timeout:    3 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "alpha" {
		t.Fatalf("expected [alpha], got %+v", tools)
	}
}

func TestFetchToolsUnsupportedType(t *testing.T) {
	defer allowLoopbackForTests(t)()
	_, err := FetchTools(context.Background(), FetchOptions{
		URL:        "https://example.com",
		ServerType: "stdio",
		Timeout:    time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "Unsupported MCP server type") {
		t.Fatalf("expected unsupported-type error, got %v", err)
	}
}

func TestFetchToolsEmptyURL(t *testing.T) {
	_, err := FetchTools(context.Background(), FetchOptions{URL: "", ServerType: TransportSSE})
	if err == nil || !strings.Contains(err.Error(), "Invalid url") {
		t.Fatalf("expected Invalid url error, got %v", err)
	}
}

func TestSubstituteTemplate(t *testing.T) {
	vars := map[string]string{"token": "abc123"}
	if got := substituteTemplate("Bearer ${token}", vars); got != "Bearer abc123" {
		t.Errorf("got %q", got)
	}
	if got := substituteTemplate("Bearer ${missing}", vars); got != "Bearer ${missing}" {
		t.Errorf("got %q", got)
	}
	if got := substituteTemplate("no-var", vars); got != "no-var" {
		t.Errorf("got %q", got)
	}
	if got := substituteTemplate("${a}-${token}", map[string]string{"a": "1", "token": "2"}); got != "1-2" {
		t.Errorf("got %q", got)
	}
}

func TestNormalizeID(t *testing.T) {
	if got := normalizeID(json.Number("1")); got != "1" {
		t.Errorf("got %q", got)
	}
	if got := normalizeID(1); got != "1" {
		t.Errorf("got %q", got)
	}
	if got := normalizeID("abc"); got != "abc" {
		t.Errorf("got %q", got)
	}
	if got := normalizeID(nil); got != "" {
		t.Errorf("got %q", got)
	}
}
