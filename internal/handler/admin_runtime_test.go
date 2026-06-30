//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//

package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"ragflow/internal/agent/runtime"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newAdminRuntimeTestRig wires a Selector backed by miniredis and returns
// a fully-mounted gin engine with the route registered, so tests can issue
// real HTTP requests against it.
func newAdminRuntimeTestRig(t *testing.T) (*gin.Engine, *runtime.Selector, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	selector := runtime.NewSelector(rdb, nil)
	h := NewAdminRuntimeHandler(selector)

	eng := gin.New()
	g := eng.Group("/api/v1/admin")
	g.POST("/canvas-runtime/:tenant_id", h.SetTenantRuntime)
	return eng, selector, mr
}

func TestAdminRuntime_SetGo(t *testing.T) {
	eng, selector, _ := newAdminRuntimeTestRig(t)

	body, _ := json.Marshal(map[string]string{"runtime": "go"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/canvas-runtime/tenant_123", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var resp setRuntimeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Code != 0 || resp.TenantID != "tenant_123" || resp.Runtime != "go" {
		t.Errorf("unexpected response: %+v", resp)
	}

	// Round-trip: the selector should now report the override.
	mode, err := selector.Select(req.Context(), "tenant_123")
	if err != nil {
		t.Fatalf("Select(): %v", err)
	}
	if mode != runtime.RuntimeGo {
		t.Errorf("Select() after SetGo = %q, want %q", mode, runtime.RuntimeGo)
	}
}

func TestAdminRuntime_SetPython(t *testing.T) {
	eng, selector, _ := newAdminRuntimeTestRig(t)

	body, _ := json.Marshal(map[string]string{"runtime": "python"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/canvas-runtime/tenant_xyz", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	mode, err := selector.Select(req.Context(), "tenant_xyz")
	if err != nil {
		t.Fatalf("Select(): %v", err)
	}
	if mode != runtime.RuntimePython {
		t.Errorf("Select() after SetPython = %q, want %q", mode, runtime.RuntimePython)
	}
}

func TestAdminRuntime_BadRequest(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"unknown_mode", `{"runtime":"rust"}`},
		{"empty_mode", `{"runtime":""}`},
		{"malformed_json", `{"runtime":`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng, _, _ := newAdminRuntimeTestRig(t)
			req := httptest.NewRequest(http.MethodPost,
				"/api/v1/admin/canvas-runtime/tenant_1",
				bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			eng.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 envelope", w.Code)
			}
			var env map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
				t.Fatalf("decode: %v", err)
			}
			// 101 == CodeArgumentError, the only acceptable error for bad input.
			if code, _ := env["code"].(float64); code != 101 {
				t.Errorf("code = %v, want 101 (CodeArgumentError); body=%s", env["code"], w.Body.String())
			}
		})
	}
}
