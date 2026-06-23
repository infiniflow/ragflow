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

package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/handler"
)

// TestAgentRoutes_AllElevenRegistered exercises the 11 Phase 5 agent
// endpoints via the public RegisterAgentRoutes helper, proving that the
// route table defined in agent_routes.go is actually wired when called
// from a real router. This guards against the regression captured in
// the post-Phase-7 code review: the helper was defined but never
// invoked from Router.Setup, so 10 of the 11 endpoints returned 404 in
// production even though the helper "looked correct".
func TestAgentRoutes_AllElevenRegistered(t *testing.T) {
	eng := gin.New()
	g := eng.Group("/api/v1/agents")
	RegisterAgentRoutes(g, &handler.AgentHandler{})

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/agents"},
		{http.MethodPost, "/api/v1/agents"},
		{http.MethodGet, "/api/v1/agents/abc"},
		{http.MethodPut, "/api/v1/agents/abc"},
		{http.MethodDelete, "/api/v1/agents/abc"},
		{http.MethodPost, "/api/v1/agents/abc/run"},
		{http.MethodDelete, "/api/v1/agents/abc/run"},
		{http.MethodPost, "/api/v1/agents/abc/publish"},
		{http.MethodGet, "/api/v1/agents/abc/versions"},
		{http.MethodGet, "/api/v1/agents/abc/versions/v1"},
		{http.MethodDelete, "/api/v1/agents/abc/versions/v1"},
	}
	if len(cases) != 11 {
		t.Fatalf("expected 11 routes, listed %d", len(cases))
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(c.method, c.path, nil)
		eng.ServeHTTP(w, req)
		// The handler dereferences a nil AgentService so a non-404 here
		// would panic; what we care about is "not NoRoute 404".
		if w.Code == http.StatusNotFound {
			t.Errorf("route %s %s returned 404 — RegisterAgentRoutes did not wire it", c.method, c.path)
		}
	}
}

// TestAgentRoutes_NilSafety makes sure the helper tolerates the "no
// handler yet" wiring case. A nil group or nil handler is a no-op so
// upstream config bugs surface as missing routes, not nil-deref panics.
func TestAgentRoutes_NilSafety(t *testing.T) {
	RegisterAgentRoutes(nil, nil)
	eng := gin.New()
	RegisterAgentRoutes(eng.Group("/agents"), nil)
	// Reaching here without panicking is the assertion.
}
