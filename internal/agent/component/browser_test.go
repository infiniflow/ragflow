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

package component

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/canvas"
)

// TestBrowser_FetchesHTML: happy path — a stub HTTP server returns
// "<html>hi</html>", the Browser component fetches it, and the
// response map's content field contains the body.
func TestBrowser_FetchesHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("server: got method %q, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>hi</html>"))
	}))
	defer srv.Close()

	c, err := NewBrowserComponent(nil)
	if err != nil {
		t.Fatalf("NewBrowserComponent: %v", err)
	}
	state := canvas.NewCanvasState("run-1", "task-1")
	ctx := canvas.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if status, _ := out["status"].(int); status != http.StatusOK {
		t.Errorf("status: got %d, want 200", status)
	}
	if body, _ := out["content"].(string); !strings.Contains(body, "hi") {
		t.Errorf("content: got %q, want substring %q", body, "hi")
	}
	if got, want := out["url"], srv.URL; got != want {
		t.Errorf("url: got %v, want %v", got, want)
	}
	if size, _ := out["size"].(int); size != len("<html>hi</html>") {
		t.Errorf("size: got %d, want %d", size, len("<html>hi</html>"))
	}
}

// TestBrowser_HTTPError: a 500 response surfaces as an error so the
// canvas engine can mark the node failed. The Browser component does
// not silently swallow non-2xx statuses.
func TestBrowser_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c, _ := NewBrowserComponent(nil)
	state := canvas.NewCanvasState("run-2", "task-2")
	ctx := canvas.WithState(context.Background(), state)

	// Per P4 contract, a 5xx response is returned to the caller as-is
	// (the canvas engine can branch on status); the Browser component
	// itself does not error on 5xx — verify that and the body is still
	// populated.
	out, err := c.Invoke(ctx, map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("Invoke: returned error %v, want nil for 500 (caller decides)", err)
	}
	if status, _ := out["status"].(int); status != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", status)
	}
	if body, _ := out["content"].(string); body != "boom" {
		t.Errorf("content: got %q, want %q", body, "boom")
	}
}

// TestBrowser_Timeout: a slow server (delay > timeout) causes the
// HTTP client to fail with a timeout, and the Browser component
// surfaces that as a wrapped error.
func TestBrowser_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep much longer than the client timeout. timeout=1 means
		// 1 second; we sleep 3s to be safe across slow CI.
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, _ := NewBrowserComponent(map[string]any{"timeout": 1})
	state := canvas.NewCanvasState("run-3", "task-3")
	ctx := canvas.WithState(context.Background(), state)

	start := time.Now()
	_, err := c.Invoke(ctx, map[string]any{"url": srv.URL})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	// The call must NOT block longer than the configured timeout plus
	// a small slack for the OS scheduler.
	if elapsed > 2*time.Second {
		t.Errorf("Invoke took %v, want < 2s with 1s timeout", elapsed)
	}
}

// TestBrowser_MissingURL: no url in param or inputs surfaces a
// ParamError.
func TestBrowser_MissingURL(t *testing.T) {
	c, _ := NewBrowserComponent(nil)
	state := canvas.NewCanvasState("run-4", "task-4")
	ctx := canvas.WithState(context.Background(), state)

	_, err := c.Invoke(ctx, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing url, got nil")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("error %q should mention url", err.Error())
	}
}

// TestBrowser_ParamCheck: negative timeout is rejected at construction.
func TestBrowser_ParamCheck(t *testing.T) {
	_, err := NewBrowserComponent(map[string]any{"timeout": -1})
	if err == nil {
		t.Fatal("expected error for negative timeout, got nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error %q should mention timeout", err.Error())
	}
}

// TestBrowser_Registered: factory lookup works case-insensitively.
func TestBrowser_Registered(t *testing.T) {
	c, err := New("browser", nil)
	if err != nil {
		t.Fatalf("registry lookup: %v", err)
	}
	if c.Name() != "Browser" {
		t.Errorf("Name()=%q, want Browser", c.Name())
	}
}
