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

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// fakeAgentService satisfies the subset of AgentService used by the handler.
// It is injected via a wrapper to avoid importing the real DAO (which requires a DB).
type fakeAgentService struct {
	result *service.ListAgentsResponse
	code   common.ErrorCode
	err    error
}

// agentServiceIface is the minimum interface the handler depends on.
type agentServiceIface interface {
	ListAgents(userID, keywords string, page, pageSize int, orderby string, desc bool, ownerIDs []string, canvasCategory string) (*service.ListAgentsResponse, common.ErrorCode, error)
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

func (f *fakeAgentService) ListAgents(userID, keywords string, page, pageSize int, orderby string, desc bool, ownerIDs []string, canvasCategory string) (*service.ListAgentsResponse, common.ErrorCode, error) {
	return f.result, f.code, f.err
}

func setupAgentRouter(svc agentServiceIface) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &agentHandlerTestable{svc: svc}
	r.GET("/api/v1/agents", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-abc"})
		h.listAgents(c)
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
