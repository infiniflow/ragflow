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
	"ragflow/internal/ingestion/component/knowledge_compiler/structure"
	"ragflow/internal/permission"
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
		c.Request = c.Request.WithContext(permission.WithIdentity(c.Request.Context(), "user-1", "user-1"))
		h.Save(c)
	})
	r.PUT("/api/v1/compilation-template-groups/:group_id", func(c *gin.Context) {
		c.Request = c.Request.WithContext(permission.WithIdentity(c.Request.Context(), "user-1", "user-1"))
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

// Malformed sections must fail before an unusable graph template replaces the
// saved configuration. A relation string used to be accepted and then silently
// omitted from the compiler's edge-extraction prompt.
func TestCompilationTemplateGroupHandlerRejectsMalformedSections(t *testing.T) {
	validConfig := `{"kind":"knowledge_graph","entity":{"fields":[{"type":"person","description":"A person"}]},"relation":{"fields":[{"type":"knows","description":"A relationship"}]}}`
	payload := func(config string) string {
		return `{"name":"Graph","templates":[{"name":"Graph","kind":"knowledge_graph","config":` + config + `}]}`
	}
	for _, tc := range []struct {
		name, config, message string
	}{
		{"relation_object", `{"kind":"knowledge_graph","relation":"not-an-object"}`, "Relation must be an object"},
		{"relation_fields", `{"kind":"knowledge_graph","relation":{"fields":{"type":"knows","description":"A relationship"}}}`, "Relation fields must be a list"},
		{"entity_fields", `{"kind":"knowledge_graph","entity":{"fields":"person"}}`, "Entity fields must be a list"},
	} {
		for _, operation := range []string{"create", "update"} {
			t.Run(tc.name+"/"+operation, func(t *testing.T) {
				db := setupGroupHandlerDB(t)
				router := newCompilationTemplateGroupHandlerRouter()
				var before entity.CompilationTemplate
				var response groupSaveResponse
				if operation == "create" {
					response = postCompilationTemplateGroup(t, router, payload(tc.config))
				} else {
					created := postCompilationTemplateGroup(t, router, payload(validConfig))
					if created.Code != int(common.CodeSuccess) {
						t.Fatalf("create valid graph: %+v", created)
					}
					id, _ := created.Data["id"].(string)
					if err := db.Where("group_id = ?", id).Take(&before).Error; err != nil {
						t.Fatal(err)
					}
					response = putCompilationTemplateGroup(t, router, id, payload(tc.config))
				}
				if response.Code != int(common.CodeDataError) || response.Message != tc.message {
					// Include the actual saved compiler input when the regression
					// accepts bad configuration, so the failure shows its effect.
					var saved entity.CompilationTemplate
					if err := db.Take(&saved).Error; err == nil {
						_, edgePrompt := structure.HypergraphPrompts(map[string]any(saved.Config), "en")
						t.Logf("persisted config=%v; edge extraction enabled=%v", saved.Config, edgePrompt != "")
					}
					t.Fatalf("response = code:%d message:%q, want data error %q", response.Code, response.Message, tc.message)
				}
				var count int64
				if err := db.Model(&entity.CompilationTemplate{}).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if operation == "create" {
					var groupCount int64
					if err := db.Model(&entity.CompilationTemplateGroup{}).Count(&groupCount).Error; err != nil {
						t.Fatal(err)
					}
					if count != 0 || groupCount != 0 {
						t.Fatalf("rejected create persisted %d templates and %d groups", count, groupCount)
					}
				} else {
					var after entity.CompilationTemplate
					if err := db.Take(&after, "id = ?", before.ID).Error; err != nil {
						t.Fatal(err)
					}
					oldConfig, _ := json.Marshal(before.Config)
					newConfig, _ := json.Marshal(after.Config)
					if count != 1 || string(oldConfig) != string(newConfig) {
						t.Fatalf("rejected update changed saved configuration: before=%s after=%s count=%d", oldConfig, newConfig, count)
					}
				}
			})
		}
	}
}
