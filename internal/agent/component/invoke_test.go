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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestInvoke_GET exercises the happy path: a GET request to a stub
// server returns the canned body, and the response map carries the
// expected status / body / headers.
func TestInvoke_GET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("server: got method %q, want GET", r.Method)
		}
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	c, _ := NewInvokeComponent(nil)
	out, err := c.Invoke(context.Background(), map[string]any{
		"method": "GET",
		"url":    srv.URL,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if status, _ := out["status"].(int); status != http.StatusOK {
		t.Errorf("status: got %d, want 200", status)
	}
	if body, _ := out["body"].(string); body != "hello" {
		t.Errorf("body: got %q, want %q", body, "hello")
	}
	hdr, _ := out["headers"].(map[string]string)
	if hdr["X-Test"] != "ok" {
		t.Errorf("headers[X-Test]: got %q, want %q", hdr["X-Test"], "ok")
	}
}

// TestInvoke_POST verifies that POST with a body echoes the body back
// from the server. The Content-Type defaults to application/json when
// not specified; we confirm that default in the test.
func TestInvoke_POST(t *testing.T) {
	var seenCT, seenBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("echo:" + seenBody))
	}))
	defer srv.Close()

	c, _ := NewInvokeComponent(nil)
	out, err := c.Invoke(context.Background(), map[string]any{
		"method": "POST",
		"url":    srv.URL,
		"body":   `{"k":"v"}`,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if status, _ := out["status"].(int); status != http.StatusCreated {
		t.Errorf("status: got %d, want 201", status)
	}
	if seenCT != "application/json" {
		t.Errorf("server saw Content-Type %q, want application/json (default)", seenCT)
	}
	if seenBody != `{"k":"v"}` {
		t.Errorf("server saw body %q, want %q", seenBody, `{"k":"v"}`)
	}
	if body, _ := out["body"].(string); body != `echo:{"k":"v"}` {
		t.Errorf("body: got %q, want %q", body, `echo:{"k":"v"}`)
	}
}

// TestInvoke_BadMethod ensures invalid HTTP methods are rejected
// before any network I/O happens.
func TestInvoke_BadMethod(t *testing.T) {
	c, _ := NewInvokeComponent(nil)
	_, err := c.Invoke(context.Background(), map[string]any{
		"method": "PATCH",
		"url":    "http://localhost:1",
	})
	if err == nil {
		t.Fatal("expected error for PATCH method, got nil")
	}
	if !strings.Contains(err.Error(), "invalid method") {
		t.Errorf("error %q should mention invalid method", err.Error())
	}
}

// TestInvoke_MissingURL confirms url is required.
func TestInvoke_MissingURL(t *testing.T) {
	c, _ := NewInvokeComponent(nil)
	_, err := c.Invoke(context.Background(), map[string]any{
		"method": "GET",
	})
	if err == nil {
		t.Fatal("expected error for missing url, got nil")
	}
	if !strings.Contains(err.Error(), "url is required") {
		t.Errorf("error %q should mention url is required", err.Error())
	}
}
