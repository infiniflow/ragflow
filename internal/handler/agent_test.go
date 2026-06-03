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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

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

// draw a box with a slot for the old TestListAgents test.
// TestListAgents_Success verifies the ListAgents handler returns a valid response.

// TestListAgentVersionsHandler_Success verifies the happy path with real DB.
func TestListAgentVersionsHandler_Success(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, "GET", "/api/v1/agents/canvas-1/versions")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}}

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
	h.ListAgentVersions(c)

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
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-b"}}

	// Canvas owned by user-b
	db.Create(&entity.UserCanvas{ID: "canvas-b", UserID: "user-b", Title: sptr("Not Yours")})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.ListAgentVersions(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code != float64(common.CodeOperatingError) {
		t.Errorf("expected operating error code %d, got %v", common.CodeOperatingError, code)
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
	c.Params = gin.Params{{Key: "agent_id", Value: "non-existent"}}

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.ListAgentVersions(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code != float64(common.CodeOperatingError) {
		t.Errorf("expected operating error code %d, got %v", common.CodeOperatingError, code)
	}
}
// sptr returns a pointer to the given string.
// ptr returns a pointer to the given int64.
func ptr(v int64) *int64 { return &v }

func TestAgentPrompts_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Stub out the prompt loader so tests are independent of rag/prompts/
	// being discoverable from the test working directory.
	orig := loadPromptFunc
	defer func() { loadPromptFunc = orig }()
	loadPromptFunc = func(name string) (string, error) {
		return "PROMPT[" + name + "]", nil
	}

	r := gin.New()
	h := NewAgentHandler(nil, nil) // service.AgentService unused by Prompts
	r.GET("/api/v1/agents/prompts", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-abc"})
		h.Prompts(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/agents/prompts", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	if body["code"] != float64(common.CodeSuccess) {
		t.Errorf("code=%v want %d", body["code"], common.CodeSuccess)
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %v", body["data"])
	}
	if data["task_analysis"] != "PROMPT[analyze_task_system]\n\nPROMPT[analyze_task_user]" {
		t.Errorf("task_analysis=%q want concat of system+\\n\\n+user", data["task_analysis"])
	}
	if data["plan_generation"] != "PROMPT[next_step]" {
		t.Errorf("plan_generation=%q want PROMPT[next_step]", data["plan_generation"])
	}
	if data["reflection"] != "PROMPT[reflect]" {
		t.Errorf("reflection=%q want PROMPT[reflect]", data["reflection"])
	}
	if data["citation_guidelines"] != "PROMPT[citation_prompt]" {
		t.Errorf("citation_guidelines=%q want PROMPT[citation_prompt]", data["citation_guidelines"])
	}
	for _, k := range []string{"task_analysis", "plan_generation", "reflection", "citation_guidelines"} {
		if _, ok := data[k]; !ok {
			t.Errorf("missing key %q in response data; keys=%v", k, data)
		}
	}
}

func TestAgentPrompts_RequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	orig := loadPromptFunc
	defer func() { loadPromptFunc = orig }()
	loadPromptFunc = func(name string) (string, error) {
		t.Errorf("loadPromptFunc must not be called when unauthenticated, was called with %q", name)
		return "", nil
	}

	r := gin.New()
	r.GET("/api/v1/agents/prompts", NewAgentHandler(nil, nil).Prompts)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/agents/prompts", nil)
	r.ServeHTTP(w, req)

	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if code, _ := body["code"].(float64); int(code) == int(common.CodeSuccess) {
		t.Errorf("expected non-success code for unauthenticated request, body=%v", body)
	}
}

func TestAgentPrompts_PropagatesLoaderError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	orig := loadPromptFunc
	defer func() { loadPromptFunc = orig }()
	loadPromptFunc = func(name string) (string, error) {
		if name == "next_step" {
			return "", errPromptMissing
		}
		return "ok", nil
	}

	r := gin.New()
	r.GET("/api/v1/agents/prompts", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-abc"})
		NewAgentHandler(nil, nil).Prompts(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/agents/prompts", nil)
	r.ServeHTTP(w, req)

	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if code, _ := body["code"].(float64); int(code) == int(common.CodeSuccess) {
		t.Errorf("missing prompt should not return success, body=%v", body)
	}
}

var errPromptMissing = pErr("prompt file 'next_step.md' not found")

type pErr string

func (e pErr) Error() string { return string(e) }
