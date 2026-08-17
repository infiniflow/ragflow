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

func newPluginRouter(authenticate bool) (*gin.Engine, *PluginHandler) {
	gin.SetMode(gin.TestMode)
	h := NewPluginHandler(service.NewPluginService())
	r := gin.New()
	r.GET("/api/v1/plugin/tools", func(c *gin.Context) {
		if authenticate {
			c.Set("user", &entity.User{ID: "tenant-1"})
		}
		h.ListLLMTools(c)
	})
	return r, h
}

func TestPluginHandlerListLLMToolsReturnsEmbeddedMetadata(t *testing.T) {
	r, _ := newPluginRouter(true)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugin/tools", nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Code    int                       `json:"code"`
		Message string                    `json:"message"`
		Data    []service.LLMToolMetadata `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	if body.Code != int(common.CodeSuccess) {
		t.Fatalf("code=%d want=%d body=%s", body.Code, common.CodeSuccess, resp.Body.String())
	}
	if body.Message != "success" {
		t.Errorf("message=%q want=%q", body.Message, "success")
	}
	if len(body.Data) == 0 {
		t.Fatalf("data should contain at least one embedded plugin, got 0")
	}

	// Verify bad_calculator is present with the exact metadata shape the Python
	// endpoint returns, so existing clients can swap backends transparently.
	var bad *service.LLMToolMetadata
	for i := range body.Data {
		if body.Data[i].Name == "bad_calculator" {
			bad = &body.Data[i]
			break
		}
	}
	if bad == nil {
		t.Fatalf("bad_calculator missing from data=%+v", body.Data)
	}
	if bad.DisplayName != "$t:bad_calculator.name" {
		t.Errorf("displayName=%q want %q", bad.DisplayName, "$t:bad_calculator.name")
	}
	if bad.Description == "" {
		t.Errorf("description must be non-empty")
	}
	for _, k := range []string{"a", "b"} {
		p, ok := bad.Parameters[k]
		if !ok {
			t.Errorf("parameter %q missing", k)
			continue
		}
		if p.Type != "number" {
			t.Errorf("param %q type=%q want number", k, p.Type)
		}
		if !p.Required {
			t.Errorf("param %q must be required", k)
		}
	}
}

func TestPluginHandlerListLLMToolsResponseFieldNamesMatchPython(t *testing.T) {
	// Defensive check: the raw JSON keys (not Go field names) must match the
	// camelCase keys the Python endpoint emits. Snake_case here would break
	// any frontend that already binds to the Python contract.
	r, _ := newPluginRouter(true)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugin/tools", nil)
	r.ServeHTTP(resp, req)

	var envelope struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(envelope.Data) == 0 {
		t.Fatalf("data empty")
	}
	tool := envelope.Data[0]
	for _, key := range []string{"name", "displayName", "description", "displayDescription", "parameters"} {
		if _, ok := tool[key]; !ok {
			t.Errorf("missing key %q in tool metadata, got keys=%v", key, mapKeys(tool))
		}
	}
	params, ok := tool["parameters"].(map[string]interface{})
	if !ok || len(params) == 0 {
		t.Fatalf("parameters is not a non-empty object: %v", tool["parameters"])
	}
	for paramName, raw := range params {
		paramObj, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("parameter %q is not an object", paramName)
		}
		for _, key := range []string{"type", "description", "displayDescription", "required"} {
			if _, ok := paramObj[key]; !ok {
				t.Errorf("parameter %q missing key %q (keys=%v)", paramName, key, mapKeys(paramObj))
			}
		}
	}
}

func TestPluginHandlerListLLMToolsRejectsUnauthenticated(t *testing.T) {
	r, _ := newPluginRouter(false)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugin/tools", nil)
	r.ServeHTTP(resp, req)

	// jsonError encodes the error code into the JSON body; HTTP status is
	// still 200 in this codebase's response style, so check the body code.
	var body map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, resp.Body.String())
	}
	if code, _ := body["code"].(float64); int(code) == int(common.CodeSuccess) {
		t.Errorf("expected non-success code for unauthenticated request, got body=%v", body)
	}
}

func mapKeys(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
