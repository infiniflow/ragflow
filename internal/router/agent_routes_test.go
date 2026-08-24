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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/handler"
)

// TestAgentRoutesRegistered exercises the ordinary Agent route set
// endpoints via the public RegisterAgentRoutes helper, proving that the
// route table defined in agent_routes.go is actually wired when called
// from a real router. This guards against the regression captured in
// the post-Phase-7 code review: the helper was defined but never
// invoked from Router.Setup, so 10 of the 11 endpoints returned 404 in
// production even though the helper "looked correct".
func TestAgentRoutesRegistered(t *testing.T) {
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
		{http.MethodPost, "/api/v1/agents/abc/publish"},
		{http.MethodGet, "/api/v1/agents/abc/versions"},
		{http.MethodGet, "/api/v1/agents/abc/versions/v1"},
		{http.MethodDelete, "/api/v1/agents/abc/versions/v1"},
		{http.MethodPost, "/api/v1/agents/abc/reset"},
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

func TestAgentSessionCancelRouteRegistered(t *testing.T) {
	eng := gin.New()
	RegisterAgentCancelRoutes(eng.Group("/api/v1/tasks"), &handler.AgentHandler{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/session-1/cancel", nil)
	eng.ServeHTTP(w, req)
	if w.Code == http.StatusNotFound {
		t.Fatal("POST /api/v1/tasks/:session_id/cancel was not registered")
	}
}

// TestAgentUploadAndAttachmentDownloadOnBetaAuth pins the wiring of the
// two agent endpoints that python declares with
// @login_required(auth_types=[AUTH_JWT, AUTH_API, AUTH_BETA])
// (api/apps/restful_apis/agent_api.py upload_agent_file / download_attachment).
// They serve the embedded/shared agent page, whose only credential is the
// share (beta) APIToken, so Router.Setup must register them on the
// beta-auth group — NOT on the JWT-only authorized group. With no
// Authorization header the beta middleware answers HTTP 200 + code 102
// ("Authorization is not valid!"), while the JWT-only AuthMiddleware
// answers a bare HTTP 401 — which is exactly what makes the embedded
// page redirect to /login after an upload attempt (reported issue).
func TestAgentUploadAndAttachmentDownloadOnBetaAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	r := &Router{
		authHandler:  handler.NewAuthHandler(),
		agentHandler: &handler.AgentHandler{},
	}
	r.Setup(engine)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"agent file upload", http.MethodPost, "/api/v1/agents/canvas-1/upload"},
		{"agent attachment download", http.MethodGet, "/api/v1/agents/attachments/att-1/download"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			engine.ServeHTTP(resp, req)

			if resp.Code == http.StatusNotFound {
				t.Fatalf("%s %s returned 404; route is not registered", tt.method, tt.path)
			}
			var body struct {
				Code common.ErrorCode `json:"code"`
			}
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}
			if resp.Code != http.StatusOK || body.Code != common.CodeDataError {
				t.Fatalf("status=%d body=%s; want beta auth middleware to handle the route (HTTP 200 + code %d)", resp.Code, resp.Body.String(), common.CodeDataError)
			}
		})
	}

	// Parity pin: attachments preview stays JWT-only in python
	// (plain @login_required, agent_api.py:2656), so it must keep
	// answering 401 from the authorized group when unauthenticated.
	t.Run("agent attachment preview stays JWT-only", func(t *testing.T) {
		resp := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/attachments/att-1/preview", nil)
		engine.ServeHTTP(resp, req)

		if resp.Code == http.StatusNotFound {
			t.Fatal("GET /api/v1/agents/attachments/:attachment_id/preview was not registered")
		}
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d body=%s; want the JWT-only AuthMiddleware to keep guarding the preview route", resp.Code, resp.Body.String())
		}
	})
}
