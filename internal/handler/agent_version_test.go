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
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func TestAgentVersionActions(t *testing.T) {
	for _, action := range []struct {
		name   string
		method string
		run    func(*AgentHandler, *gin.Context)
	}{
		{"delete", http.MethodDelete, (*AgentHandler).DeleteVersion},
		{"apply", http.MethodPost, (*AgentHandler).ApplyVersion},
	} {
		for _, scenario := range []string{"success", "last_version", "empty_dsl", "draft_version", "missing_agent", "missing_version", "foreign_agent", "foreign_version", "unauthenticated", "nil_dsl"} {
			t.Run(action.name+"/"+scenario, func(t *testing.T) {
				path := "/api/v1/agents/canvas-1/versions/version-1"
				if action.name == "apply" {
					path += "/apply"
				}
				c, w, db := setupGinContextWithUserAndDB(t, action.method, path)
				c.Params = gin.Params{{Key: "canvas_id", Value: "canvas-1"}, {Key: "version_id", Value: "version-1"}}
				if err := db.Create(&entity.User{ID: "user-1", Nickname: "owner", Email: "owner@test.com"}).Error; err != nil {
					t.Fatal(err)
				}
				current := entity.UserCanvas{
					ID: "canvas-1", UserID: "user-1", Title: sptr("Current Agent"),
					Description: sptr("Current description"), Tags: "keep", Release: true,
					DSL: entity.JSONMap{"components": map[string]interface{}{}, "revision": "current"},
				}
				if scenario == "foreign_agent" {
					current.UserID = "user-2"
				}
				if err := db.Create(&current).Error; err != nil {
					t.Fatal(err)
				}
				createdAt := int64(1)
				version := entity.UserCanvasVersion{
					ID: "version-1", UserCanvasID: current.ID, Title: sptr("Historical version title"),
					Release: true, DSL: entity.JSONMap{"components": map[string]interface{}{}, "revision": "old"},
					BaseModel: entity.BaseModel{CreateTime: &createdAt},
				}
				wantCode := common.CodeSuccess
				switch scenario {
				case "empty_dsl":
					version.DSL = entity.JSONMap{}
				case "draft_version":
					version.Release = false
				case "missing_agent":
					c.Params[0].Value = "missing"
					wantCode = common.CodeOperatingError
				case "missing_version":
					c.Params[1].Value = "missing"
					wantCode = common.CodeOperatingError
				case "foreign_agent":
					wantCode = common.CodeOperatingError
				case "foreign_version":
					version.UserCanvasID = "canvas-2"
					wantCode = common.CodeOperatingError
				case "unauthenticated":
					delete(c.Keys, "user")
					wantCode = common.CodeUnauthorized
				case "nil_dsl":
					version.DSL = nil
					if action.name == "apply" {
						wantCode = common.CodeDataError
					}
				}
				if err := db.Create(&version).Error; err != nil {
					t.Fatal(err)
				}
				initialCount := int64(1)
				if scenario != "last_version" {
					latestCreatedAt := int64(2)
					if err := db.Create(&entity.UserCanvasVersion{
						ID: "version-2", UserCanvasID: current.ID, Release: true, DSL: current.DSL,
						BaseModel: entity.BaseModel{CreateTime: &latestCreatedAt},
					}).Error; err != nil {
						t.Fatal(err)
					}
					initialCount++
				}

				action.run(NewAgentHandler(t.Context(), service.NewAgentService(), nil), c)
				if w.Code != http.StatusOK {
					t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
				}
				var response struct {
					Code    common.ErrorCode `json:"code"`
					Data    interface{}      `json:"data"`
					Message string           `json:"message"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response.Code != wantCode || response.Message == "" {
					t.Fatalf("unexpected response: %s", w.Body.String())
				}
				if wantCode == common.CodeSuccess && response.Data != true {
					t.Fatalf("success data = %v, want true", response.Data)
				}
				persisted, err := dao.NewUserCanvasDAO().GetByID(t.Context(), db, current.ID)
				if err != nil {
					t.Fatal(err)
				}
				wantDSL, wantRelease, wantCount := current.DSL, current.Release, initialCount
				if wantCode == common.CodeSuccess {
					if action.name == "apply" {
						wantDSL, wantRelease, wantCount = version.DSL, false, initialCount+1
					} else {
						wantCount--
					}
				}
				if !reflect.DeepEqual(persisted.DSL, wantDSL) || persisted.Release != wantRelease {
					t.Fatalf("unexpected current agent: dsl=%v release=%v", persisted.DSL, persisted.Release)
				}
				if *persisted.Title != *current.Title || *persisted.Description != *current.Description || persisted.Tags != current.Tags || persisted.UserID != current.UserID {
					t.Fatal("agent metadata changed")
				}
				var count int64
				if err := db.Model(&entity.UserCanvasVersion{}).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != wantCount {
					t.Fatalf("version count = %d, want %d", count, wantCount)
				}
				if scenario != "last_version" {
					if _, err := dao.NewUserCanvasVersionDAO().GetByID(t.Context(), db, "version-2"); err != nil {
						t.Fatalf("latest version was removed: %v", err)
					}
				}
				if action.name == "apply" || wantCode != common.CodeSuccess {
					retained, err := dao.NewUserCanvasVersionDAO().GetByID(t.Context(), db, version.ID)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(retained.DSL, version.DSL) || retained.Release != version.Release || *retained.Title != *version.Title {
						t.Fatal("source version changed")
					}
				}
			})
		}
	}
}
