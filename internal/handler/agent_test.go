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
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestGetAgentVersionHandler_Success verifies getting a specific version.
func TestGetAgentVersionHandler_Success(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, "GET", "/api/v1/agents/canvas-1/versions/v1")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}, {Key: "version_id", Value: "v1"}}

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
	h.GetAgentVersion(c)

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
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}, {Key: "version_id", Value: "non-existent"}}

	db.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.GetAgentVersion(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code != float64(common.CodeNotFound) {
		t.Errorf("expected not found code %d, got %v", common.CodeNotFound, code)
	}
}

func TestListAgentSessionsHandlerSuccess(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, http.MethodGet, "/api/v1/agents/canvas-1/sessions")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}}

	db.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-1",
		DialogID:  "canvas-1",
		UserID:    "user-1",
		Message:   json.RawMessage(`[{"role":"assistant","content":"hello","prompt":"hidden"}]`),
		Reference: json.RawMessage(`[]`),
		BaseModel: entity.BaseModel{
			UpdateTime: ptr(time.Now().UnixMilli()),
		},
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-2",
		DialogID:  "canvas-1",
		UserID:    "user-1",
		Message:   json.RawMessage(`[{"role":"user","content":"question"}]`),
		Reference: json.RawMessage(`[]`),
		BaseModel: entity.BaseModel{
			UpdateTime: ptr(time.Now().Add(-time.Hour).UnixMilli()),
		},
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-other-agent",
		DialogID:  "canvas-other",
		UserID:    "user-1",
		Message:   json.RawMessage(`[{"role":"assistant","content":"other"}]`),
		Reference: json.RawMessage(`[]`),
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.ListAgentSessions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code %d, got %v: %v", common.CodeSuccess, resp["code"], resp["message"])
	}
	if resp["total"] != float64(2) {
		t.Fatalf("expected total 2, got %v", resp["total"])
	}

	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(data) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(data))
	}

	first := data[0].(map[string]interface{})
	if first["agent_id"] != "canvas-1" {
		t.Fatalf("expected agent_id canvas-1, got %v", first["agent_id"])
	}
	messages := first["message"].([]interface{})
	message := messages[0].(map[string]interface{})
	if _, ok := message["prompt"]; ok {
		t.Fatalf("expected prompt to be stripped from list response")
	}
}

func TestGetAgentSessionHandlerSuccess(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, http.MethodGet, "/api/v1/agents/canvas-1/sessions/session-1")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}, {Key: "session_id", Value: "session-1"}}

	db.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-1",
		DialogID:  "canvas-1",
		UserID:    "user-1",
		Message:   json.RawMessage(`[{"role":"assistant","content":"hello"}]`),
		Reference: json.RawMessage(`[]`),
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.GetAgentSession(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code %d, got %v: %v", common.CodeSuccess, resp["code"], resp["message"])
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %T", resp["data"])
	}
	if data["id"] != "session-1" {
		t.Fatalf("expected session-1, got %v", data["id"])
	}
	if data["dialog_id"] != "canvas-1" {
		t.Fatalf("expected dialog_id canvas-1, got %v", data["dialog_id"])
	}
}

func TestGetAgentSessionHandlerRejectsSessionFromAnotherAgent(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, http.MethodGet, "/api/v1/agents/canvas-1/sessions/session-other")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}, {Key: "session_id", Value: "session-other"}}

	db.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-other",
		DialogID:  "canvas-other",
		UserID:    "user-1",
		Message:   json.RawMessage(`[{"role":"assistant","content":"other"}]`),
		Reference: json.RawMessage(`[]`),
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.GetAgentSession(c)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["code"] == float64(common.CodeSuccess) {
		t.Fatalf("expected non-success for cross-agent session, got response %v", resp)
	}
	if resp["data"] != nil {
		t.Fatalf("expected nil data for cross-agent session, got %v", resp["data"])
	}
}

func TestDeleteAgentSessionItemHandlerDeletesOnlyMatchingAgent(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, http.MethodDelete, "/api/v1/agents/canvas-1/sessions/session-1")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}, {Key: "session_id", Value: "session-1"}}

	db.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-1",
		DialogID:  "canvas-1",
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-other",
		DialogID:  "canvas-other",
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.DeleteAgentSessionItem(c)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code %d, got %v: %v", common.CodeSuccess, resp["code"], resp["message"])
	}
	if resp["data"] != true {
		t.Fatalf("expected data true, got %v", resp["data"])
	}

	var deletedCount int64
	if err := db.Model(&entity.API4Conversation{}).Where("id = ?", "session-1").Count(&deletedCount).Error; err != nil {
		t.Fatalf("failed to count deleted session: %v", err)
	}
	if deletedCount != 0 {
		t.Fatalf("expected session-1 to be deleted, count=%d", deletedCount)
	}

	var otherCount int64
	if err := db.Model(&entity.API4Conversation{}).Where("id = ?", "session-other").Count(&otherCount).Error; err != nil {
		t.Fatalf("failed to count other session: %v", err)
	}
	if otherCount != 1 {
		t.Fatalf("expected session-other to remain, count=%d", otherCount)
	}
}

func TestDeleteAgentSessionItemHandlerIgnoresSessionFromAnotherAgent(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, http.MethodDelete, "/api/v1/agents/canvas-1/sessions/session-other")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}, {Key: "session_id", Value: "session-other"}}

	db.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-other",
		DialogID:  "canvas-other",
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.DeleteAgentSessionItem(c)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code %d, got %v: %v", common.CodeSuccess, resp["code"], resp["message"])
	}
	if resp["data"] != false {
		t.Fatalf("expected data false, got %v", resp["data"])
	}

	var count int64
	if err := db.Model(&entity.API4Conversation{}).Where("id = ?", "session-other").Count(&count).Error; err != nil {
		t.Fatalf("failed to count other session: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected cross-agent session to remain, count=%d", count)
	}
}

func TestDeleteAgentSessionsHandlerDeletesDuplicateIDsPartially(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, http.MethodDelete, "/api/v1/agents/canvas-1/sessions")
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/agents/canvas-1/sessions", strings.NewReader(`{"ids":["session-1","session-1"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}}

	db.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-1",
		DialogID:  "canvas-1",
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.DeleteAgentSessions(c)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code %d, got %v: %v", common.CodeSuccess, resp["code"], resp["message"])
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected partial data object, got %T", resp["data"])
	}
	if data["success_count"] != float64(1) {
		t.Fatalf("expected success_count 1, got %v", data["success_count"])
	}
	errorsList, ok := data["errors"].([]interface{})
	if !ok || len(errorsList) != 1 {
		t.Fatalf("expected one duplicate error, got %v", data["errors"])
	}
	if errorsList[0] != "Duplicate session ids: session-1" {
		t.Fatalf("unexpected duplicate error: %v", errorsList[0])
	}

	var count int64
	if err := db.Model(&entity.API4Conversation{}).Where("id = ?", "session-1").Count(&count).Error; err != nil {
		t.Fatalf("failed to count deleted session: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected session-1 to be deleted, count=%d", count)
	}
}

func TestDeleteAgentSessionsHandlerDeleteAll(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, http.MethodDelete, "/api/v1/agents/canvas-1/sessions")
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/agents/canvas-1/sessions", strings.NewReader(`{"delete_all":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}}

	db.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-1",
		DialogID:  "canvas-1",
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-2",
		DialogID:  "canvas-1",
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-other",
		DialogID:  "canvas-other",
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.DeleteAgentSessions(c)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code %d, got %v: %v", common.CodeSuccess, resp["code"], resp["message"])
	}

	var ownCount int64
	if err := db.Model(&entity.API4Conversation{}).Where("dialog_id = ?", "canvas-1").Count(&ownCount).Error; err != nil {
		t.Fatalf("failed to count own sessions: %v", err)
	}
	if ownCount != 0 {
		t.Fatalf("expected all canvas-1 sessions to be deleted, count=%d", ownCount)
	}

	var otherCount int64
	if err := db.Model(&entity.API4Conversation{}).Where("id = ?", "session-other").Count(&otherCount).Error; err != nil {
		t.Fatalf("failed to count other session: %v", err)
	}
	if otherCount != 1 {
		t.Fatalf("expected other agent session to remain, count=%d", otherCount)
	}
}

func TestDeleteAgentSessionsHandlerRequiresOwner(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, http.MethodDelete, "/api/v1/agents/canvas-1/sessions")
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/agents/canvas-1/sessions", strings.NewReader(`{"ids":["session-1"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}}

	db.Create(&entity.UserCanvas{
		ID:         "canvas-1",
		UserID:     "user-2",
		Permission: "team",
		Title:      sptr("Team Agent"),
	})
	db.Create(&entity.API4Conversation{
		ID:        "session-1",
		DialogID:  "canvas-1",
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.DeleteAgentSessions(c)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["code"] != float64(common.CodeDataError) {
		t.Fatalf("expected code %d, got %v: %v", common.CodeDataError, resp["code"], resp["message"])
	}

	var count int64
	if err := db.Model(&entity.API4Conversation{}).Where("id = ?", "session-1").Count(&count).Error; err != nil {
		t.Fatalf("failed to count session: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected session to remain, count=%d", count)
	}
}

func TestTestDBConnectionHandlerMissingFields(t *testing.T) {
	c, w, _ := setupGinContextWithUserAndDB(t, http.MethodPost, "/api/v1/agents/test_db_connection")
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/agents/test_db_connection", strings.NewReader(`{"db_type":"mysql"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.TestDBConnection(c)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["code"] != float64(common.CodeArgumentError) {
		t.Fatalf("expected code %d, got %v: %v", common.CodeArgumentError, resp["code"], resp["message"])
	}
	if resp["data"] != nil {
		t.Fatalf("expected nil data, got %v", resp["data"])
	}
	want := "required argument are missing: database,username,host,port,password; "
	if resp["message"] != want {
		t.Fatalf("expected message %q, got %v", want, resp["message"])
	}
}

func TestTestDBConnectionHandlerRejectsLocalhost(t *testing.T) {
	c, w, _ := setupGinContextWithUserAndDB(t, http.MethodPost, "/api/v1/agents/test_db_connection")
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/agents/test_db_connection", strings.NewReader(`{
		"db_type":"mysql",
		"database":"rag_flow",
		"username":"root",
		"host":"localhost",
		"port":3306,
		"password":"infini_rag_flow"
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.TestDBConnection(c)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["code"] != float64(common.CodeDataError) {
		t.Fatalf("expected code %d, got %v: %v", common.CodeDataError, resp["code"], resp["message"])
	}
	if resp["data"] != nil {
		t.Fatalf("expected nil data, got %v", resp["data"])
	}
	message, ok := resp["message"].(string)
	if !ok || !strings.Contains(message, "non-public address") {
		t.Fatalf("expected non-public host message, got %v", resp["message"])
	}
}

func TestUpdateAgentTagsHandlerSuccess(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, http.MethodPut, "/api/v1/agents/canvas-1/tags")
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/agents/canvas-1/tags", strings.NewReader(`{"tags":["alpha","beta","alpha"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}}

	db.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.UpdateAgentTags(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	code, _ := resp["code"].(float64)
	if code != float64(common.CodeSuccess) {
		t.Fatalf("expected code %d, got %v: %v", common.CodeSuccess, code, resp["message"])
	}
	if resp["data"] != true {
		t.Fatalf("expected data true, got %v", resp["data"])
	}

	var canvas entity.UserCanvas
	if err := db.Where("id = ?", "canvas-1").First(&canvas).Error; err != nil {
		t.Fatalf("failed to reload canvas: %v", err)
	}
	if canvas.Tags != "alpha,beta" {
		t.Fatalf("expected normalized tags alpha,beta, got %q", canvas.Tags)
	}
}

func TestUpdateAgentTagsHandlerNoPermission(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, http.MethodPut, "/api/v1/agents/canvas-b/tags")
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/agents/canvas-b/tags", strings.NewReader(`{"tags":["alpha"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-b"}}

	db.Create(&entity.UserCanvas{
		ID:         "canvas-b",
		UserID:     "user-b",
		Title:      sptr("Private Agent"),
		Permission: "me",
	})

	h := NewAgentHandler(service.NewAgentService(), nil)
	h.UpdateAgentTags(c)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	code, _ := resp["code"].(float64)
	if code != float64(common.CodeOperatingError) {
		t.Fatalf("expected code %d, got %v: %v", common.CodeOperatingError, code, resp["message"])
	}
	if resp["data"] != false {
		t.Fatalf("expected data false, got %v", resp["data"])
	}
	if resp["message"] != "Agent not found or no permission." {
		t.Fatalf("unexpected message: %v", resp["message"])
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

func TestListAgentTemplates_Success(t *testing.T) {
	cnvType := "agent"
	svc := &fakeAgentService{
		templates: []*entity.CanvasTemplate{
			{
				ID:             "template-1",
				CanvasType:     &cnvType,
				CanvasCategory: "agent_canvas",
				Title:          entity.JSONMap{"en": "Sample"},
				Description:    entity.JSONMap{"en": "Sample desc"},
			},
		},
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/agents/templates", nil)
	setupAgentRouter(svc).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	if body["code"] != float64(common.CodeSuccess) {
		t.Errorf("code=%v want %d", body["code"], common.CodeSuccess)
	}
	data, ok := body["data"].([]interface{})
	if !ok {
		t.Fatalf("data is not an array: %v", body["data"])
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 template, got %d", len(data))
	}
	first := data[0].(map[string]interface{})
	if first["id"] != "template-1" {
		t.Errorf("id=%v want template-1", first["id"])
	}
	if first["canvas_category"] != "agent_canvas" {
		t.Errorf("canvas_category=%v want agent_canvas", first["canvas_category"])
	}
}

func TestListAgentTemplates_EmptyIsArrayNotNull(t *testing.T) {
	svc := &fakeAgentService{templates: nil}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/agents/templates", nil)
	setupAgentRouter(svc).ServeHTTP(w, req)

	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	// JSON shape contract: never null - frontends do .map() on it.
	if _, ok := body["data"].([]interface{}); !ok {
		t.Fatalf("data is not an array when templates empty: %v (raw=%s)", body["data"], w.Body.String())
	}
}

func TestListAgentTemplates_RequiresAuth(t *testing.T) {
	svc := &fakeAgentService{templates: []*entity.CanvasTemplate{}}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/agents/templates_anon", nil)
	setupAgentRouter(svc).ServeHTTP(w, req)

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if code, _ := body["code"].(float64); int(code) == int(common.CodeSuccess) {
		t.Errorf("expected non-success without auth, got body=%v", body)
	}
}

type fakeAgentFileService struct {
	blob []byte
	err  error
}

func (f *fakeAgentFileService) UploadFile(tenantID, parentID string, files []*multipart.FileHeader) ([]map[string]interface{}, error) {
	return nil, nil
}

func (f *fakeAgentFileService) DownloadAgentFile(tenantID, location string) ([]byte, error) {
	return f.blob, f.err
}

func TestDownloadAgentFile_Success(t *testing.T) {
	c, w, _ := setupGinContextWithUserAndDB(t, http.MethodGet, "/api/v1/agents/download?id=test-file.pdf")

	fakeFileSvc := &fakeAgentFileService{
		blob: []byte("test content"),
		err:  nil,
	}

	h := &AgentHandler{
		agentService: service.NewAgentService(),
		fileService:  fakeFileSvc,
	}

	h.DownloadAgentFile(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if w.Header().Get("Content-Type") != "application/pdf" {
		t.Errorf("expected Content-Type application/pdf, got %s", w.Header().Get("Content-Type"))
	}

	if w.Body.String() != "test content" {
		t.Errorf("expected 'test content', got %s", w.Body.String())
	}
}

func TestDownloadAgentFile_MissingID(t *testing.T) {
	c, w, _ := setupGinContextWithUserAndDB(t, http.MethodGet, "/api/v1/agents/download")

	h := &AgentHandler{
		agentService: service.NewAgentService(),
		fileService:  &fakeAgentFileService{},
	}

	h.DownloadAgentFile(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (json error return), got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if code, _ := resp["code"].(float64); code != float64(common.CodeArgumentError) {
		t.Errorf("expected code 102, got %v", code)
	}
}

func TestGetPrompts_Success(t *testing.T) {
	c, w, _ := setupGinContextWithUserAndDB(t, http.MethodGet, "/api/v1/agents/prompts")

	// Create handler with fake or real service.
	h := NewAgentHandler(service.NewAgentService(), nil)
	h.GetPrompts(c)

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

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data map, got %T", resp["data"])
	}

	// Check if keys exist
	expectedKeys := []string{"task_analysis", "plan_generation", "reflection", "citation_guidelines"}
	for _, key := range expectedKeys {
		if _, ok := data[key]; !ok {
			t.Errorf("expected key %s in data", key)
		}
	}
}
