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
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func setupSearchHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Search{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("failed to migrate test schema: %v", err)
	}

	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = origDB })

	return db
}

func TestSearchHandlerCreateRejectsEmptyName(t *testing.T) {
	setupSearchHandlerTestDB(t)

	h := NewSearchHandler(service.NewSearchService(), service.NewUserService())
	c, w := setupGinContextWithUser("POST", "/api/v1/searches", `{"name": "   "}`)

	h.CreateSearch(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["code"] != float64(common.CodeDataError) {
		t.Fatalf("expected code 102, got %v", resp["code"])
	}
	if !strings.Contains(resp["message"].(string), "empty") {
		t.Fatalf("expected message containing 'empty', got %v", resp["message"])
	}
}

func TestSearchHandlerUpdateRejectsInvalidSearchID(t *testing.T) {
	setupSearchHandlerTestDB(t)

	h := NewSearchHandler(service.NewSearchService(), service.NewUserService())
	c, w := setupGinContextWithUser("PUT", "/api/v1/searches/invalid_search_id", `{"name": "invalid", "search_config": {}}`)
	c.Params = []gin.Param{{Key: "search_id", Value: "invalid_search_id"}}

	h.UpdateSearch(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["code"] != float64(common.CodeAuthenticationError) {
		t.Fatalf("expected code 109, got %v", resp["code"])
	}
	if !strings.Contains(resp["message"].(string), "No authorization") {
		t.Fatalf("expected 'No authorization' in message, got %v", resp["message"])
	}
}
