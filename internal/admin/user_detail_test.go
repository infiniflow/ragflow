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

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"

	"github.com/gin-gonic/gin"
)

type adminTestResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

func setupAdminUserDetailTest(t *testing.T) *Handler {
	t.Helper()
	db := testutil.SetupTestDB(t,
		&entity.User{},
		&entity.UserTenant{},
		&entity.Knowledgebase{},
		&entity.UserCanvas{},
	)
	t.Cleanup(testutil.ReplaceDBForTest(t, db))

	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	isSuperuser := false
	statusValid := string(entity.StatusValid)
	language := "English"
	loginChannel := "password"
	user := entity.User{
		ID:              "user-1",
		Nickname:        "User One",
		Email:           "user@example.com",
		Language:        &language,
		IsAuthenticated: "1",
		IsActive:        "1",
		IsAnonymous:     "0",
		LoginChannel:    &loginChannel,
		Status:          &statusValid,
		IsSuperuser:     &isSuperuser,
		BaseModel:       entity.BaseModel{CreateDate: &now, UpdateDate: &now},
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	membership := entity.UserTenant{
		ID:        "membership-1",
		UserID:    user.ID,
		TenantID:  "tenant-2",
		Role:      "normal",
		InvitedBy: "tenant-2",
		Status:    &statusValid,
	}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}

	datasets := []entity.Knowledgebase{
		{ID: "dataset-own", TenantID: user.ID, Name: "Own dataset", EmbdID: "embedding", Permission: "me", CreatedBy: user.ID, Status: &statusValid},
		{ID: "dataset-team", TenantID: "tenant-2", Name: "Team dataset", EmbdID: "embedding", Permission: "team", CreatedBy: "tenant-2", Status: &statusValid},
		{ID: "dataset-private", TenantID: "tenant-2", Name: "Private dataset", EmbdID: "embedding", Permission: "me", CreatedBy: "tenant-2", Status: &statusValid},
		{ID: "dataset-invalid", TenantID: user.ID, Name: "Invalid dataset", EmbdID: "embedding", Permission: "me", CreatedBy: user.ID, Status: testutil.StrPtr(string(entity.StatusInvalid))},
	}
	if err := db.Create(&datasets).Error; err != nil {
		t.Fatalf("create datasets: %v", err)
	}

	agents := []entity.UserCanvas{
		{ID: "agent-own", UserID: user.ID, Title: testutil.StrPtr("Own agent"), Permission: "me", CanvasCategory: "agent_canvas"},
		{ID: "agent-team", UserID: "tenant-2", Title: testutil.StrPtr("Team agent"), Permission: "team", CanvasCategory: "agent_canvas"},
		{ID: "agent-private", UserID: "tenant-2", Title: testutil.StrPtr("Private agent"), Permission: "me", CanvasCategory: "agent_canvas"},
	}
	if err := db.Create(&agents).Error; err != nil {
		t.Fatalf("create agents: %v", err)
	}

	return NewHandler(NewService())
}

func callAdminUserHandler(t *testing.T, handler func(*gin.Context)) adminTestResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "username", Value: "user@example.com"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handler(ctx)

	var response adminTestResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != int(common.CodeSuccess) {
		t.Fatalf("response code = %d, want %d; body = %s", response.Code, common.CodeSuccess, recorder.Body.String())
	}
	return response
}

func TestGetUserReturnsPythonCompatibleDetailList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupAdminUserDetailTest(t)
	response := callAdminUserHandler(t, handler.GetUser)

	var details []struct {
		Email       string `json:"email"`
		IsActive    string `json:"is_active"`
		CreateDate  string `json:"create_date"`
		IsAnonymous string `json:"is_anonymous"`
	}
	if err := json.Unmarshal(response.Data, &details); err != nil {
		t.Fatalf("decode user details: %v", err)
	}
	if len(details) != 1 {
		t.Fatalf("user details length = %d, want 1", len(details))
	}
	if details[0].Email != "user@example.com" || details[0].IsActive != "1" || details[0].CreateDate == "" || details[0].IsAnonymous != "0" {
		t.Fatalf("unexpected user details: %+v", details[0])
	}
}

func TestListUserDatasetsReturnsAccessibleDatasets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupAdminUserDetailTest(t)
	response := callAdminUserHandler(t, handler.ListUserDatasets)

	var datasets []struct {
		Name       string `json:"name"`
		Permission string `json:"permission"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(response.Data, &datasets); err != nil {
		t.Fatalf("decode datasets: %v", err)
	}
	if len(datasets) != 2 {
		t.Fatalf("datasets length = %d, want 2: %+v", len(datasets), datasets)
	}
	datasetsByName := make(map[string]struct {
		Permission string
		Status     string
	}, len(datasets))
	for _, dataset := range datasets {
		datasetsByName[dataset.Name] = struct {
			Permission string
			Status     string
		}{Permission: dataset.Permission, Status: dataset.Status}
	}
	if dataset := datasetsByName["Own dataset"]; dataset.Permission != "me" || dataset.Status != "1" {
		t.Fatalf("unexpected own dataset: %+v", dataset)
	}
	if dataset := datasetsByName["Team dataset"]; dataset.Permission != "team" || dataset.Status != "1" {
		t.Fatalf("unexpected team dataset: %+v", dataset)
	}
}

func TestListUserAgentsReturnsAccessibleAgents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupAdminUserDetailTest(t)
	response := callAdminUserHandler(t, handler.ListUserAgents)

	var agents []struct {
		Title          string `json:"title"`
		Permission     string `json:"permission"`
		CanvasCategory string `json:"canvas_category"`
	}
	if err := json.Unmarshal(response.Data, &agents); err != nil {
		t.Fatalf("decode agents: %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("agents length = %d, want 2: %+v", len(agents), agents)
	}
	agentsByTitle := make(map[string]struct {
		Permission     string
		CanvasCategory string
	}, len(agents))
	for _, agent := range agents {
		agentsByTitle[agent.Title] = struct {
			Permission     string
			CanvasCategory string
		}{Permission: agent.Permission, CanvasCategory: agent.CanvasCategory}
	}
	if agent := agentsByTitle["Own agent"]; agent.Permission != "me" || agent.CanvasCategory != "agent" {
		t.Fatalf("unexpected own agent: %+v", agent)
	}
	if agent := agentsByTitle["Team agent"]; agent.Permission != "team" || agent.CanvasCategory != "agent" {
		t.Fatalf("unexpected team agent: %+v", agent)
	}
}
