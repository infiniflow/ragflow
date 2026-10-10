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

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// setupGroupHandlerDB migrates the minimal schema for the compilation template
// group handler and pushes it onto dao.DB.
func setupGroupHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()

	// A shared-cache memory database keeps the transaction's connection and the
	// pool's other connections on the same schema.
	dsn := "file:" + url.QueryEscape(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err = db.AutoMigrate(
		&entity.CompilationTemplateGroup{},
		&entity.CompilationTemplate{},
		&entity.Tenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = origDB })
	return db
}

func newCompilationTemplateGroupHandlerRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewCompilationTemplateGroupHandler(service.NewCompilationTemplateGroupService())
	r := gin.New()
	r.POST("/api/v1/compilation-template-groups", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		h.Save(c)
	})
	r.PUT("/api/v1/compilation-template-groups/:group_id", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		h.Update(c)
	})
	return r
}

type groupSaveResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

func postCompilationTemplateGroup(t *testing.T, r *gin.Engine, body string) groupSaveResponse {
	t.Helper()
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/compilation-template-groups", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	var parsed groupSaveResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	return parsed
}

// Saving a group whose name is already used by another group of the tenant
// appends a counter instead of returning "Duplicated compilation template
// group name.".
func TestCompilationTemplateGroupHandlerSaveDedupesDuplicateName(t *testing.T) {
	setupGroupHandlerDB(t)
	r := newCompilationTemplateGroupHandlerRouter()
	body := `{"name":"Group A","templates":[{"name":"tpl","kind":"text","config":{}}]}`

	first := postCompilationTemplateGroup(t, r, body)
	if first.Code != int(common.CodeSuccess) {
		t.Fatalf("first save code = %d, message = %q", first.Code, first.Message)
	}
	if got := first.Data["name"]; got != "Group A" {
		t.Fatalf("first group name = %v, want %q", got, "Group A")
	}

	second := postCompilationTemplateGroup(t, r, body)
	if second.Code != int(common.CodeSuccess) {
		t.Fatalf("second save code = %d, message = %q", second.Code, second.Message)
	}
	if got := second.Data["name"]; got != "Group A(1)" {
		t.Fatalf("second group name = %v, want %q", got, "Group A(1)")
	}
}

func putCompilationTemplateGroup(t *testing.T, r *gin.Engine, id, body string) groupSaveResponse {
	t.Helper()
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/compilation-template-groups/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(resp, req)

	var parsed groupSaveResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	return parsed
}

// After a name fallback the group and its single template share the group name,
// so an update carrying the template name (what the client submits) is accepted;
// reusing another group's name is still rejected.
func TestCompilationTemplateGroupHandlerUpdateAllowsTemplateName(t *testing.T) {
	setupGroupHandlerDB(t)
	r := newCompilationTemplateGroupHandlerRouter()
	body := `{"name":"Group A","templates":[{"name":"tpl","kind":"text","config":{}}]}`

	if first := postCompilationTemplateGroup(t, r, body); first.Code != int(common.CodeSuccess) {
		t.Fatalf("first save code = %d, message = %q", first.Code, first.Message)
	}
	second := postCompilationTemplateGroup(t, r, body)
	if second.Code != int(common.CodeSuccess) {
		t.Fatalf("second save code = %d, message = %q", second.Code, second.Message)
	}
	id, _ := second.Data["id"].(string)
	if id == "" {
		t.Fatalf("second save returned no id: %#v", second.Data)
	}

	updated := putCompilationTemplateGroup(t, r, id, `{"name":"Group A(1)","templates":[{"name":"Group A(1)","kind":"text","config":{}}]}`)
	if updated.Code != int(common.CodeSuccess) {
		t.Fatalf("update code = %d, message = %q", updated.Code, updated.Message)
	}
	if got := updated.Data["name"]; got != "Group A(1)" {
		t.Fatalf("updated group name = %v, want %q", got, "Group A(1)")
	}

	conflict := putCompilationTemplateGroup(t, r, id, `{"name":"Group A","templates":[{"name":"Group A","kind":"text","config":{}}]}`)
	if conflict.Code != int(common.CodeDataError) {
		t.Fatalf("conflicting update code = %d, want %d", conflict.Code, int(common.CodeDataError))
	}
}
