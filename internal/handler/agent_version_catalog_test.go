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
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/permission/testsupport"
	"ragflow/internal/service"
)

func TestListVersionCatalog(t *testing.T) {
	for _, category := range []string{"dataflow", "agent"} {
		t.Run(category, func(t *testing.T) {
			c, w, db := setupGinContextWithUserAndDB(t, "GET", "/api/v1/agents/versions?category="+category)
			canvases := []entity.UserCanvas{
				{ID: "pipe-1", UserID: "user-1", Title: sptr("Invoices"), Description: sptr("Invoice parser"), CanvasCategory: "dataflow_canvas"},
				{ID: "pipe-empty", UserID: "user-1", CanvasCategory: "dataflow_canvas"},
				{ID: "agent-1", UserID: "user-1", Title: sptr("Assistant"), CanvasCategory: "agent_canvas"},
				{ID: "foreign", UserID: "user-2", CanvasCategory: "dataflow_canvas"},
				{ID: "foreign-agent", UserID: "user-2", CanvasCategory: "agent_canvas"},
			}
			if err := db.Create(&canvases).Error; err != nil {
				t.Fatal(err)
			}
			oldTime, newTime := int64(1), int64(2)
			versions := []entity.UserCanvasVersion{
				{ID: "v1", UserCanvasID: "pipe-1", Title: sptr("Version one"), Description: sptr("First parser"), Alias: sptr("production"), Release: true, BaseModel: entity.BaseModel{CreateTime: &oldTime}},
				{ID: "v2", UserCanvasID: "pipe-1", Title: sptr("Version two"), Release: false, BaseModel: entity.BaseModel{CreateTime: &newTime}},
				{ID: "foreign-version", UserCanvasID: "foreign"},
				{ID: "foreign-agent-version", UserCanvasID: "foreign-agent"},
				{ID: "agent-version", UserCanvasID: "agent-1", Title: sptr("Agent version"), Alias: sptr("agent alias")},
			}
			for i := range versions {
				versions[i].DSL = entity.JSONMap{"secret": "must not be loaded"}
			}
			if err := db.Create(&versions).Error; err != nil {
				t.Fatal(err)
			}
			var queries []string
			if err := db.Callback().Query().After("gorm:query").Register("capture_catalog", func(tx *gorm.DB) {
				queries = append(queries, tx.Statement.SQL.String())
			}); err != nil {
				t.Fatal(err)
			}
			(&AgentHandler{agentService: service.NewAgentService()}).ListVersionCatalog(c)
			var response struct {
				Code  common.ErrorCode                  `json:"code"`
				Data  []service.AgentVersionCatalogItem `json:"data"`
				Total *int64                            `json:"total"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			wantTotal := int64(2)
			if category == "agent" {
				wantTotal = 1
			}
			if response.Code != common.CodeSuccess || int64(len(response.Data)) != wantTotal || response.Total == nil || *response.Total != wantTotal {
				t.Fatalf("unexpected response: %s", w.Body.String())
			}
			byID := make(map[string]service.AgentVersionCatalogItem)
			for _, item := range response.Data {
				byID[item.ID] = item
			}
			want := service.AgentVersionCatalogItem{
				ID: "pipe-1", Name: "Invoices",
				Versions: []service.AgentVersionMetadata{
					{ID: "v2", Name: "Version two"},
					{ID: "v1", Name: "Version one", Alias: "production"},
				},
			}
			if category == "dataflow" {
				if !reflect.DeepEqual(byID["pipe-1"], want) {
					t.Fatalf("pipeline = %#v, want %#v", byID["pipe-1"], want)
				}
				if empty, ok := byID["pipe-empty"]; !ok || empty.Versions == nil || len(empty.Versions) != 0 {
					t.Fatalf("empty pipeline missing or versions not []: %#v", byID)
				}
			} else {
				want = service.AgentVersionCatalogItem{
					ID: "agent-1", Name: "Assistant",
					Versions: []service.AgentVersionMetadata{{ID: "agent-version", Name: "Agent version", Alias: "agent alias"}},
				}
				if !reflect.DeepEqual(byID["agent-1"], want) {
					t.Fatalf("agent = %#v, want %#v", byID["agent-1"], want)
				}
			}
			var raw struct {
				Data []map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			for _, item := range raw.Data {
				if len(item) != 3 || item["id"] == nil || item["name"] == nil || item["versions"] == nil {
					t.Fatalf("unexpected pipeline fields: %v", item)
				}
				var rows []map[string]json.RawMessage
				if err := json.Unmarshal(item["versions"], &rows); err != nil {
					t.Fatal(err)
				}
				for _, row := range rows {
					if len(row) != 3 || row["id"] == nil || row["name"] == nil || row["alias"] == nil {
						t.Fatalf("unexpected version fields: %v", row)
					}
				}
			}
			versionQueries := 0
			for _, query := range queries {
				if strings.Contains(query, "user_canvas") && (strings.Contains(query, "SELECT *") || strings.Contains(query, "`dsl`") || strings.Contains(query, "`description`")) {
					t.Errorf("catalog loaded unnecessary fields: %s", query)
				}
				if strings.Contains(query, "FROM `user_canvas_version`") {
					versionQueries++
				}
			}
			if versionQueries != 1 {
				t.Fatalf("version queries = %d, want 1", versionQueries)
			}

		})
	}
}

func TestListVersionCatalogEmpty(t *testing.T) {
	c, w, _ := setupGinContextWithUserAndDB(t, "GET", "/api/v1/agents/versions?category=dataflow")
	(&AgentHandler{agentService: service.NewAgentService()}).ListVersionCatalog(c)
	var response struct {
		Code  common.ErrorCode `json:"code"`
		Data  json.RawMessage  `json:"data"`
		Total *int64           `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != common.CodeSuccess || string(response.Data) != "[]" || response.Total == nil || *response.Total != 0 {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}

func TestListVersionCatalogSharedPipeline(t *testing.T) {
	c, w, db := setupGinContextWithUserAndDB(t, "GET", "/api/v1/agents/versions?category=dataflow")
	c.Request = c.Request.WithContext(permission.WithIdentity(c.Request.Context(), "user-1", "team"))
	if err := db.Create(&entity.UserTenant{
		ID: "membership", UserID: "user-1", TenantID: "team", Role: "normal", Status: sptr(string(entity.TenantMembershipStatusJoined)),
	}).Error; err != nil {
		t.Fatal(err)
	}
	canvases := []entity.UserCanvas{
		{ID: "shared", UserID: "user-2", TenantID: sptr("team"), Permission: "team", CanvasCategory: "dataflow_canvas"},
		{ID: "private", UserID: "user-2", TenantID: sptr("team"), Permission: "me", CanvasCategory: "dataflow_canvas"},
	}
	if err := db.Create(&canvases).Error; err != nil {
		t.Fatal(err)
	}
	if err := testsupport.PrepareEnterprisePermissionFixture(db, "team"); err != nil {
		t.Fatal(err)
	}
	(&AgentHandler{agentService: service.NewAgentService()}).ListVersionCatalog(c)
	var response struct {
		Code common.ErrorCode                  `json:"code"`
		Data []service.AgentVersionCatalogItem `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != common.CodeSuccess || len(response.Data) != 1 || response.Data[0].ID != "shared" {
		t.Fatalf("unexpected shared pipeline response: %s", w.Body.String())
	}
}

func TestListVersionCatalogFailure(t *testing.T) {
	for _, scenario := range []string{"canceled", "version_query"} {
		t.Run(scenario, func(t *testing.T) {
			c, w, db := setupGinContextWithUserAndDB(t, "GET", "/api/v1/agents/versions?category=dataflow")
			if scenario == "canceled" {
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			} else {
				if err := db.Create(&entity.UserCanvas{ID: "pipeline", UserID: "user-1", CanvasCategory: "dataflow_canvas"}).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Migrator().DropTable(&entity.UserCanvasVersion{}); err != nil {
					t.Fatal(err)
				}
			}
			(&AgentHandler{agentService: service.NewAgentService()}).ListVersionCatalog(c)
			var response struct {
				Code common.ErrorCode `json:"code"`
				Data json.RawMessage  `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != common.CodeServerError || response.Data != nil {
				t.Fatalf("failure reported as catalog data: %s", w.Body.String())
			}
		})
	}
}

func TestListVersionCatalogRejectsInvalidRequests(t *testing.T) {
	for _, scenario := range []string{"missing_category", "invalid_category", "unauthenticated"} {
		t.Run(scenario, func(t *testing.T) {
			c, w, _ := setupGinContextWithUserAndDB(t, "GET", "/api/v1/agents/versions?category=dataflow")
			want := common.CodeBadRequest
			switch scenario {
			case "missing_category":
				c.Request = httptest.NewRequest("GET", "/api/v1/agents/versions", nil)
			case "invalid_category":
				c.Request = httptest.NewRequest("GET", "/api/v1/agents/versions?category=anything", nil)
			case "unauthenticated":
				c.Keys = nil
				c.Request = c.Request.WithContext(t.Context())
				want = common.CodeUnauthorized
			}
			(&AgentHandler{agentService: service.NewAgentService()}).ListVersionCatalog(c)
			var response struct {
				Code common.ErrorCode `json:"code"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != want {
				t.Fatalf("unexpected response: %s", w.Body.String())
			}
		})
	}
}
