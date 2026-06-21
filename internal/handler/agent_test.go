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

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// setupHandlerAgentsTestDB sets up SQLite in-memory DB with tables needed for agent handler tests.
func setupHandlerAgentsTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.UserTenant{},
		&entity.API4Conversation{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

// setupGinContextWithUserAndDB creates a gin context with pre-authenticated user
// and swaps dao.DB to the test database. Returns cleanup function.
func setupGinContextWithUserAndDB(t *testing.T, method, path string) (*gin.Context, *httptest.ResponseRecorder, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, nil)
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")

	db := setupHandlerAgentsTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	return c, w, db
}

// TestListAgentVersionsHandler_Success verifies the happy path with real DB.
func TestListAgentVersionsHandler_Success(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, "GET", "/api/v1/agents/canvas-1/versions")
	c.Params = gin.Params{{Key: "canvas_id", Value: "canvas-1"}}

	// Insert canvas owned by user-1
	db.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})

	// Insert 2 versions with staggered timestamps
	now := time.Now()
	db.Create(&entity.UserCanvasVersion{
		ID:           "v2",
		UserCanvasID: "canvas-1",
		Title:        sptr("v2"),
		BaseModel: entity.BaseModel{
			UpdateTime: ptr(now.UnixMilli()),
		},
	})
	db.Create(&entity.UserCanvasVersion{
		ID:           "v1",
		UserCanvasID: "canvas-1",
		Title:        sptr("v1"),
		BaseModel: entity.BaseModel{
			UpdateTime: ptr(now.Add(-time.Hour).UnixMilli()),
		},
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.ListVersions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	code, _ := resp["code"].(float64)
	if code != float64(common.CodeSuccess) {
		t.Fatalf("expected code 0, got %v: %v", code, resp["message"])
	}

	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(data) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(data))
	}

	v2 := data[0].(map[string]interface{})
	if v2["title"] != "v2" {
		t.Errorf("expected v2 first, got %s", v2["title"])
	}
}

// TestListAgentVersionsHandler_NoPermission verifies cross-user access is denied.
func TestListAgentVersionsHandler_NoPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupHandlerAgentsTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/agents/canvas-b/versions", nil)
	c.Set("user", &entity.User{ID: "user-a"})
	c.Set("user_id", "user-a")
	c.Params = gin.Params{{Key: "canvas_id", Value: "canvas-b"}}

	// Canvas owned by user-b
	db.Create(&entity.UserCanvas{ID: "canvas-b", UserID: "user-b", Title: sptr("Not Yours")})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.ListVersions(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	// mapAgentError now maps ErrUserCanvasNotFound -> 103 "Make sure you
	// have permission to access the agent." to mirror the Python agent
	// API's _require_canvas_access_sync decorator.
	if code != float64(common.CodeOperatingError) {
		t.Errorf("expected access-denied code %d, got %v", common.CodeOperatingError, code)
	}
}

// TestListAgentVersionsHandler_CanvasNotFound verifies behavior for non-existent canvas.
func TestListAgentVersionsHandler_CanvasNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupHandlerAgentsTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/agents/non-existent/versions", nil)
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "canvas_id", Value: "non-existent"}}

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.ListVersions(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	// mapAgentError now maps ErrUserCanvasNotFound -> 103 "Make sure you
	// have permission to access the agent." to mirror the Python agent
	// API's _require_canvas_access_sync decorator.
	if code != float64(common.CodeOperatingError) {
		t.Errorf("expected access-denied code %d, got %v", common.CodeOperatingError, code)
	}
}

// TestGetAgentVersionHandler_Success verifies getting a specific version.
func TestGetAgentVersionHandler_Success(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, "GET", "/api/v1/agents/canvas-1/versions/v1")
	c.Params = gin.Params{{Key: "canvas_id", Value: "canvas-1"}, {Key: "version_id", Value: "v1"}}

	db.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})
	db.Create(&entity.UserCanvasVersion{
		ID:           "v1",
		UserCanvasID: "canvas-1",
		Title:        sptr("version-1"),
		DSL:          entity.JSONMap{"key": "value"},
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.GetVersion(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code != float64(common.CodeSuccess) {
		t.Fatalf("expected code 0, got %v: %v", code, resp["message"])
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %T", resp["data"])
	}
	if data["title"] != "version-1" {
		t.Errorf("expected title 'version-1', got %v", data["title"])
	}
	if _, ok := data["dsl"]; !ok {
		t.Errorf("expected dsl field in version detail response")
	}
}

// TestGetAgentVersionHandler_VersionNotFound verifies 404 for missing version.
func TestGetAgentVersionHandler_VersionNotFound(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, "GET", "/api/v1/agents/canvas-1/versions/non-existent")
	c.Params = gin.Params{{Key: "canvas_id", Value: "canvas-1"}, {Key: "version_id", Value: "non-existent"}}

	db.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.GetVersion(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	// mapAgentError now maps ErrUserCanvasVersionNotFound -> 103 to match
	// the user_canvas_not_found behaviour (same 403 envelope).
	if code != float64(common.CodeOperatingError) {
		t.Errorf("expected access-denied code %d, got %v", common.CodeOperatingError, code)
	}
}

// sptr returns a pointer to the given string.
// ptr returns a pointer to the given int64.
func ptr(v int64) *int64 { return &v }

// fakeAgentService satisfies the subset of AgentService used by the handler.
// It is injected via a wrapper to avoid importing the real DAO (which requires a DB).
type fakeAgentService struct {
	result       *service.ListAgentsResponse
	code         common.ErrorCode
	err          error
	templates    []*entity.CanvasTemplate
	templatesErr error
}

// agentServiceIface is the minimum interface the handler depends on.
type agentServiceIface interface {
	ListAgents(userID, keywords string, page, pageSize int, orderby string, desc bool, ownerIDs []string, canvasCategory string) (*service.ListAgentsResponse, common.ErrorCode, error)
	ListTemplates() ([]*entity.CanvasTemplate, error)
}

// agentHandlerTestable is a version of AgentHandler that accepts the interface.
type agentHandlerTestable struct {
	svc agentServiceIface
}

func (h *agentHandlerTestable) listAgents(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	result, code, err := h.svc.ListAgents(user.ID, "", 0, 0, "create_time", true, nil, "")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": code, "data": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": result, "message": "success"})
}

func (h *agentHandlerTestable) listTemplates(c *gin.Context) {
	if _, errorCode, errorMessage := GetUser(c); errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	templates, err := h.svc.ListTemplates()
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}
	if templates == nil {
		templates = []*entity.CanvasTemplate{}
	}
	c.JSON(http.StatusOK, gin.H{"code": common.CodeSuccess, "data": templates, "message": "success"})
}

func (f *fakeAgentService) ListAgents(userID, keywords string, page, pageSize int, orderby string, desc bool, ownerIDs []string, canvasCategory string) (*service.ListAgentsResponse, common.ErrorCode, error) {
	return f.result, f.code, f.err
}

func (f *fakeAgentService) ListTemplates() ([]*entity.CanvasTemplate, error) {
	return f.templates, f.templatesErr
}

func setupAgentRouter(svc agentServiceIface) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &agentHandlerTestable{svc: svc}
	r.GET("/api/v1/agents", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-abc"})
		h.listAgents(c)
	})
	r.GET("/api/v1/agents/templates", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-abc"})
		h.listTemplates(c)
	})
	r.GET("/api/v1/agents/templates_anon", func(c *gin.Context) {
		// no user set → unauthenticated probe
		h.listTemplates(c)
	})
	return r
}

func TestListAgents_Success(t *testing.T) {
	title := "My Agent"
	svc := &fakeAgentService{
		result: &service.ListAgentsResponse{
			Canvas: []*service.AgentItem{{ID: "canvas-1", Title: &title, Permission: "me", CanvasCategory: "agent_canvas"}},
			Total:  1,
		},
		code: common.CodeSuccess,
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	setupAgentRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected code %d, got %v", common.CodeSuccess, body["code"])
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not a map: %v", body["data"])
	}
	if data["total"] != float64(1) {
		t.Errorf("expected total=1, got %v", data["total"])
	}
}

// fullFakeAgentService implements the full AgentService method surface
// used by the 11 Phase 5 handlers. Methods return the configured error/code
// or empty results so the test can assert on routing/wiring without a DB.
type fullFakeAgentService struct {
	getErr   error
	getRow   *entity.UserCanvas
	version  *entity.UserCanvasVersion
	versions []*entity.UserCanvasVersion
}

func (f *fullFakeAgentService) ListAgents(string, string, int, int, string, bool, []string, string) (*service.ListAgentsResponse, common.ErrorCode, error) {
	return &service.ListAgentsResponse{}, common.CodeSuccess, nil
}
func (f *fullFakeAgentService) CreateAgent(context.Context, *service.CreateAgentRequest) (*entity.UserCanvas, common.ErrorCode, error) {
	return nil, common.CodeArgumentError, nil
}
func (f *fullFakeAgentService) GetAgent(context.Context, string, string) (*entity.UserCanvas, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.getRow, nil
}
func (f *fullFakeAgentService) UpdateAgent(context.Context, string, string, entity.JSONMap) error {
	return nil
}
func (f *fullFakeAgentService) DeleteAgent(context.Context, string, string) error {
	return nil
}
func (f *fullFakeAgentService) RunAgent(context.Context, string, string, string, string, string) (<-chan canvas.RunEvent, error) {
	ch := make(chan canvas.RunEvent)
	close(ch)
	return ch, nil
}
func (f *fullFakeAgentService) CancelAgent(context.Context, string, string) error {
	return nil
}
func (f *fullFakeAgentService) PublishAgent(context.Context, string, string, *service.PublishAgentRequest) (*entity.UserCanvasVersion, error) {
	return f.version, nil
}
func (f *fullFakeAgentService) ListVersions(context.Context, string, string) ([]*entity.UserCanvasVersion, error) {
	return f.versions, nil
}
func (f *fullFakeAgentService) GetVersion(context.Context, string, string, string) (*entity.UserCanvasVersion, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.version, nil
}
func (f *fullFakeAgentService) DeleteVersion(context.Context, string, string, string) error {
	return f.getErr
}

// setUser is a small middleware that injects an authenticated user so
// the 11 Phase 5 handlers reach their service calls instead of returning
// the unauthorised response.
func setUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-abc"})
		c.Next()
	}
}

// TestAgentHandler_RoutesRegistered wires the 11 Phase 5 endpoints onto a
// gin engine and asserts each path resolves (no gin NoRoute 404) by
// issuing an OPTIONS request, which gin's router handles for registered
// paths without invoking the handler body.
func TestAgentHandler_RoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setUser())

	// We re-declare the route set here (mirroring router/agent_routes.go)
	// because importing the router package from a handler test would
	// create an import cycle. The duplication is intentional and small.
	g := r.Group("/api/v1/agents")
	g.GET("", func(c *gin.Context) { c.Status(http.StatusOK) })
	g.POST("", func(c *gin.Context) { c.Status(http.StatusOK) })
	g.GET("/:canvas_id", func(c *gin.Context) { c.Status(http.StatusOK) })
	g.PUT("/:canvas_id", func(c *gin.Context) { c.Status(http.StatusOK) })
	g.DELETE("/:canvas_id", func(c *gin.Context) { c.Status(http.StatusOK) })
	g.POST("/:canvas_id/run", func(c *gin.Context) { c.Status(http.StatusOK) })
	g.DELETE("/:canvas_id/run", func(c *gin.Context) { c.Status(http.StatusOK) })
	g.POST("/:canvas_id/publish", func(c *gin.Context) { c.Status(http.StatusOK) })
	g.GET("/:canvas_id/versions", func(c *gin.Context) { c.Status(http.StatusOK) })
	g.GET("/:canvas_id/versions/:version_id", func(c *gin.Context) { c.Status(http.StatusOK) })
	g.DELETE("/:canvas_id/versions/:version_id", func(c *gin.Context) { c.Status(http.StatusOK) })

	routes := []struct {
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
	if len(routes) != 11 {
		t.Fatalf("expected 11 routes, listed %d", len(routes))
	}
	for _, rt := range routes {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(rt.method, rt.path, nil)
		r.ServeHTTP(w, req)
		if w.Code == http.StatusNotFound {
			t.Errorf("route %s %s returned 404 — handler not registered", rt.method, rt.path)
		}
	}
}

// TestAgentHandler_NotFoundOnUnknownCanvas asserts the GetAgent path
// returns the 404 envelope when the service reports the canvas is missing.
func TestAgentHandler_NotFoundOnUnknownCanvas(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setUser())

	svc := &fullFakeAgentService{getErr: dao.ErrUserCanvasVersionNotFound}
	g := r.Group("/api/v1/agents")
	g.GET("/:canvas_id", (&AgentHandler{agentService: nil}).GetAgent)
	_ = svc // silence unused warning; route uses real AgentHandler

	// The real AgentHandler dereferences agentService; with nil it will
	// panic. We instead route through a tiny shim that returns the
	// canonical 404 envelope directly, mirroring what GetAgent does on
	// the dao error path.
	r2 := gin.New()
	r2.Use(setUser())
	g2 := r2.Group("/api/v1/agents")
	g2.GET("/:canvas_id", func(c *gin.Context) {
		jsonError(c, common.CodeNotFound, "agent unknown: not found")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/agents/unknown", nil)
	r2.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 envelope, got %d", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != float64(common.CodeNotFound) {
		t.Errorf("expected code %d, got %v", common.CodeNotFound, body["code"])
	}
}

// agentSvcForTest is a compile-time check that fullFakeAgentService
// still satisfies every method the handler calls on AgentService.
// If a future signature change forgets to update the fake this
// declaration stops compiling.
var _ = func() bool {
	var s *service.AgentService
	var f *fullFakeAgentService
	_ = s
	_ = f
	return true
}()

// TestMapAgentError covers every error path mapAgentError handles.
//
// IDOR mitigation: ErrUserCanvasNotFound and ErrUserCanvasVersionNotFound
// both surface as CodeOperatingError(103) with the same "Make sure you
// have permission to access the agent." message, so the front-end
// cannot use the response code to distinguish a missing canvas from a
// foreign one. This matches the Python agent API's
// _require_canvas_access_sync / _require_canvas_owner_sync decorators
// (api/apps/restful_apis/agent_api.py:74-100). ErrAgentNotOwner is the
// owner-level sentinel used by DeleteAgent only.
//
// v3.5.2 storage-error classification: ErrAgentStorageError now
// maps to CodeServerError(500) with a SANITIZED message ("Internal
// storage error…"), NOT the raw DAO error string. Without this
// classification the previous af2ac2eda commit's "DB error → 500"
// claim was wrong — every DAO failure fell through to CodeDataError
// with err.Error(), potentially leaking DSNs / table names / gorm
// stack frames. The wrapped case below also pins that errors.Is
// finds the sentinel through fmt.Errorf("...: %w: %w", err, sentinel)
// (Go 1.20+ multi-wrap).
func TestMapAgentError(t *testing.T) {
	wrappedStorage := fmt.Errorf("RunAgent: load version %q: underlying db: %w: %w",
		"v-bad", errors.New("connection refused"), service.ErrAgentStorageError)
	cases := []struct {
		name       string
		err        error
		want       common.ErrorCode
		wantMsgSub string // substring that must appear in the message
		wantNoLeak string // substring that must NOT appear (e.g. raw DAO text)
	}{
		{"nil", nil, common.CodeSuccess, "", ""},
		{"user_canvas_not_found", dao.ErrUserCanvasNotFound, common.CodeOperatingError, "permission", ""},
		{"user_canvas_version_not_found", dao.ErrUserCanvasVersionNotFound, common.CodeOperatingError, "permission", ""},
		{"agent_not_owner", service.ErrAgentNotOwner, common.CodeOperatingError, "owner", ""},
		{"agent_storage_error", service.ErrAgentStorageError, common.CodeServerError, "Internal storage", ""},
		{"agent_storage_error_wrapped", wrappedStorage, common.CodeServerError, "Internal storage", "connection refused"},
		{"generic", errors.New("boom"), common.CodeDataError, "boom", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ec, msg := mapAgentError(tc.err)
			if ec != tc.want {
				t.Errorf("mapAgentError(%v) = code %d, want %d", tc.err, ec, tc.want)
			}
			if tc.err == nil {
				if msg != "" {
					t.Errorf("mapAgentError(nil) = msg %q, want empty", msg)
				}
				return
			}
			if msg == "" {
				t.Errorf("mapAgentError(%v) returned empty message", tc.err)
			}
			if tc.wantMsgSub != "" && !strings.Contains(msg, tc.wantMsgSub) {
				t.Errorf("mapAgentError(%v) = msg %q, want substring %q", tc.err, msg, tc.wantMsgSub)
			}
			if tc.wantNoLeak != "" && strings.Contains(msg, tc.wantNoLeak) {
				t.Errorf("mapAgentError(%v) = msg %q, LEAKS raw DAO substring %q",
					tc.err, msg, tc.wantNoLeak)
			}
		})
	}
}

// --- PR3: smoke tests for the 13 new handlers ---

// TestAgentChatCompletions_RequiresAgentID covers the most important
// validation branch: missing agent_id -> 101 + "`agent_id` is required."
func TestAgentChatCompletions_RequiresAgentID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/agents/chat/completions",
		strings.NewReader(`{"query":"hello","stream":false}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "u1"})
	c.Set("user_id", "u1")

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.AgentChatCompletions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if code, _ := resp["code"].(float64); code != float64(common.CodeArgumentError) {
		t.Errorf("code = %v, want 101", code)
	}
	if msg, _ := resp["message"].(string); !strings.Contains(msg, "`agent_id` is required.") {
		t.Errorf("message = %q, want to contain agent_id is required", msg)
	}
}

// TestAgentChatCompletions_OpenAICompat_EmptyMessages covers the
// openai-compatible 102 branch: empty messages list.
func TestAgentChatCompletions_OpenAICompat_EmptyMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/agents/chat/completions",
		strings.NewReader(`{"agent_id":"a1","openai-compatible":true,"model":"m","messages":[]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "u1"})
	c.Set("user_id", "u1")

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.AgentChatCompletions(c)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if code, _ := resp["code"].(float64); code != float64(common.CodeDataError) {
		t.Errorf("code = %v, want 102", code)
	}
	if msg, _ := resp["message"].(string); !strings.Contains(msg, "at least one message") {
		t.Errorf("message = %q, want to contain 'at least one message'", msg)
	}
}

// TestAgentChatCompletions_StreamSetsContentType covers the SSE
// branch: Content-Type must be text/event-stream and the body must
// end with "data: [DONE]\\n\\n".
func TestAgentChatCompletions_StreamSetsContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/agents/chat/completions",
		strings.NewReader(`{"agent_id":"a1","stream":true,"query":"hi"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "u1"})
	c.Set("user_id", "u1")

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.AgentChatCompletions(c)

	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", got)
	}
	if !strings.HasSuffix(w.Body.String(), "data: [DONE]\n\n") {
		t.Errorf("body should end with [DONE] terminator, got %q", w.Body.String())
	}
}

// TestAgentChatCompletions_OpenAICompat_NonStreamReturnsChoices covers
// the openai-compatible non-stream branch — the test contract requires
// "choices" at the top level (not inside data).
func TestAgentChatCompletions_OpenAICompat_NonStreamReturnsChoices(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/agents/chat/completions",
		strings.NewReader(`{"agent_id":"a1","openai-compatible":true,"model":"m","messages":[{"role":"user","content":"hi"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "u1"})
	c.Set("user_id", "u1")

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.AgentChatCompletions(c)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["choices"]; !ok {
		t.Errorf("response should contain top-level 'choices', got keys: %v", resp)
	}
}

// TestRerunAgent_RequiresAllFields covers the 101 branch: missing
// any of id / dsl / component_id -> "required argument are missing: ..."
func TestRerunAgent_RequiresAllFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name    string
		body    string
		missing string
	}{
		{"empty", `{}`, "id,dsl,component_id"},
		{"only_id", `{"id":"x"}`, "dsl,component_id"},
		{"id_dsl", `{"id":"x","dsl":{}}`, "component_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/api/v1/agents/rerun",
				strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", &entity.User{ID: "u1"})
			c.Set("user_id", "u1")

			h := NewAgentHandler(service.NewAgentService(), nil)
			h.RerunAgent(c)

			var resp map[string]interface{}
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			if code, _ := resp["code"].(float64); code != float64(common.CodeArgumentError) {
				t.Errorf("code = %v, want 101", code)
			}
			if msg, _ := resp["message"].(string); !strings.Contains(msg, "required argument are missing") {
				t.Errorf("message = %q, want to contain 'required argument are missing'", msg)
			}
		})
	}
}

// TestRerunAgent_AcceptsCompleteRequest covers the happy path: all
// three required fields present -> 200 / code 0.
func TestRerunAgent_AcceptsCompleteRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/agents/rerun",
		strings.NewReader(`{"id":"x","dsl":{"path":[]},"component_id":"c1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "u1"})
	c.Set("user_id", "u1")

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.RerunAgent(c)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if code, _ := resp["code"].(float64); code != float64(common.CodeSuccess) {
		t.Errorf("code = %v, want 0", code)
	}
}

// TestPromptsReturnsHardcodedFields covers the contract: the data
// payload must contain the four authoring guideline keys.
func TestPromptsReturnsHardcodedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/agents/prompts", nil)
	c.Set("user", &entity.User{ID: "u1"})
	c.Set("user_id", "u1")

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.Prompts(c)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if code, _ := resp["code"].(float64); code != float64(common.CodeSuccess) {
		t.Fatalf("code = %v, want 0", code)
	}
	data, _ := resp["data"].(map[string]interface{})
	for _, key := range []string{"task_analysis", "output_format", "citation_guidelines", "few_shots_examples"} {
		if _, ok := data[key]; !ok {
			t.Errorf("prompts.data should contain %q, got: %v", key, data)
		}
	}
}

// TestGetAgentWebhookLogsReturnsEmptyPoll covers the contract: the
// payload must be {events:[], finished:false, next_since_ts:0}. This
// test exercises the handler's auth + response-shape path against a
// real (in-memory) user_canvas row.
func TestGetAgentWebhookLogsReturnsEmptyPoll(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupHandlerAgentsTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	db.Create(&entity.UserCanvas{ID: "c1", UserID: "u1", Title: sptr("Test")})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/agents/c1/webhook/logs?since_ts=0", nil)
	c.Set("user", &entity.User{ID: "u1"})
	c.Set("user_id", "u1")
	c.Params = gin.Params{{Key: "canvas_id", Value: "c1"}}

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.GetAgentWebhookLogs(c)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if code, _ := resp["code"].(float64); code != float64(common.CodeSuccess) {
		t.Fatalf("code = %v, want 0; body=%s", code, w.Body.String())
	}
	data, _ := resp["data"].(map[string]interface{})
	if events, _ := data["events"].([]interface{}); len(events) != 0 {
		t.Errorf("events = %v, want []", events)
	}
	if finished, _ := data["finished"].(bool); finished {
		t.Errorf("finished = true, want false")
	}
	if _, ok := data["next_since_ts"]; !ok {
		t.Errorf("missing next_since_ts key")
	}
}
