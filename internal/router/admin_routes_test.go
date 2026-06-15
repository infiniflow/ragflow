//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//

package router

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
	"ragflow/internal/handler"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestAdminRuntimeRoutes_Registered(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	selector := runtime.NewSelector(rdb, nil)
	h := handler.NewAdminRuntimeHandler(selector)

	eng := gin.New()
	v1 := eng.Group("/api/v1")
	admin := v1.Group("/admin")
	RegisterAdminRuntimeRoutes(admin, h)

	body, _ := json.Marshal(map[string]string{"runtime": "go"})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/admin/canvas-runtime/tenant_123", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"runtime":"go"`)) {
		t.Errorf("response body missing runtime:go: %s", w.Body.String())
	}
}

func TestAdminRuntimeRoutes_NilSafety(t *testing.T) {
	// A nil router group or handler must not panic; the helper is
	// documented as a no-op in that case so wiring bugs surface as
	// missing routes rather than nil-deref panics.
	RegisterAdminRuntimeRoutes(nil, nil)
	// Just reaching here without panicking is the test.
}

// TestAdminRuntimeRoutes_StaysRegisteredWithNilSelector locks in the
// review follow-up: when the server starts before Redis is reachable
// the handler is constructed with a nil selector. The route MUST
// still be registered and MUST return ErrSelectorNotConfigured (HTTP
// 500), not a 404. The previous version of the wiring made the route
// vanish in this scenario, which stranded canary operators with an
// opaque 404 until the next process restart.
func TestAdminRuntimeRoutes_StaysRegisteredWithNilSelector(t *testing.T) {
	h := handler.NewAdminRuntimeHandler(nil) // nil selector — Redis unavailable

	eng := gin.New()
	v1 := eng.Group("/api/v1")
	admin := v1.Group("/admin")
	RegisterAdminRuntimeRoutes(admin, h)

	body, _ := json.Marshal(map[string]string{"runtime": "go"})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/admin/canvas-runtime/tenant_123", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("route returned 404 — the route must stay registered even when the selector is nil; body=%s", w.Body.String())
	}
	if w.Code != http.StatusOK {
		// 200/500 both acceptable; the contract is "not 404" so the
		// operator sees a uniform surface and can read the error in the
		// body. The handler currently returns 500 with
		// ErrSelectorNotConfigured; we assert the body contains that
		// string for a useful diagnostic.
		if !bytes.Contains(w.Body.Bytes(), []byte("selector not configured")) {
			t.Errorf("body missing 'selector not configured' diagnostic; got %s", w.Body.String())
		}
	}
}
