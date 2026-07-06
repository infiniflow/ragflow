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
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	_ "ragflow/internal/agent/component" // registers the production factory
	agentruntime "ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// componentCtx builds a Gin context for one of the
// /agents/:canvas_id/components/:component_id/* endpoints. The
// supplied `body` is bound as the request body (empty for GET).
func componentCtx(t *testing.T, method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	c.Set("user", &entity.User{ID: "u-1"})
	return c, w
}

// beginCanvas returns a UserCanvas whose DSL has a single Begin
// component with both an input_form and a params block. Used for
// happy-path tests.
func beginCanvas() *entity.UserCanvas {
	return &entity.UserCanvas{
		ID: "c1",
		DSL: map[string]any{
			"components": map[string]any{
				"begin": map[string]any{
					"obj": map[string]any{
						"component_name": "Begin",
						"params": map[string]any{
							"mode": "Manual",
						},
						"input_form": map[string]any{
							"query": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}
}

// ---------- GetComponentInputForm ----------

func TestGetComponentInputForm_HappyPath(t *testing.T) {
	cv := beginCanvas()
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}

	c, w := componentCtx(t, "GET", "/api/v1/agents/c1/components/begin/input-form", "")
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "begin"},
	}
	h.GetComponentInputForm(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	q, ok := env.Data["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("data.query = %v, want a map", env.Data["query"])
	}
	if q["type"] != "string" {
		t.Errorf("data.query.type = %v, want string", q["type"])
	}
}

func TestGetComponentInputForm_ExeSQLDynamicInputForm(t *testing.T) {
	cv := &entity.UserCanvas{
		ID: "c1",
		DSL: map[string]any{
			"components": map[string]any{
				"exesql:0": map[string]any{
					"obj": map[string]any{
						"component_name": "ExeSQL",
						"params": map[string]any{
							"database": "demo",
							"username": "root",
							"host":     "127.0.0.1",
							"port":     float64(3306),
							"password": "secret",
							"top_n":    float64(50),
						},
					},
				},
			},
		},
	}
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}

	c, w := componentCtx(t, "GET", "/api/v1/agents/c1/components/exesql:0/input-form", "")
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "exesql:0"},
	}
	h.GetComponentInputForm(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if env.Code != 0 {
		t.Fatalf("code = %d, want 0; body=%s", env.Code, w.Body.String())
	}
	sql, ok := env.Data["sql"].(map[string]interface{})
	if !ok {
		t.Fatalf("data.sql = %v, want a map", env.Data["sql"])
	}
	if sql["name"] != "SQL" {
		t.Errorf("data.sql.name = %v, want SQL", sql["name"])
	}
	if sql["type"] != "line" {
		t.Errorf("data.sql.type = %v, want line", sql["type"])
	}
}

func TestGetComponentInputForm_BrowserDynamicInputForm(t *testing.T) {
	cv := &entity.UserCanvas{
		ID: "c1",
		DSL: map[string]any{
			"components": map[string]any{
				"Browser:FlatWolvesTry": map[string]any{
					"obj": map[string]any{
						"component_name": "Browser",
						"params": map[string]any{
							"llm_id":  "gpt-4o@OpenAI",
							"prompts": "{sys.query}",
						},
					},
				},
			},
		},
	}
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}

	c, w := componentCtx(t, "GET", "/api/v1/agents/c1/components/Browser:FlatWolvesTry/input-form", "")
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "Browser:FlatWolvesTry"},
	}
	h.GetComponentInputForm(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if env.Code != 0 {
		t.Fatalf("code = %d, want 0; body=%s", env.Code, w.Body.String())
	}
	prompts, ok := env.Data["prompts"].(map[string]interface{})
	if !ok {
		t.Fatalf("data.prompts = %v, want a map", env.Data["prompts"])
	}
	if prompts["name"] != "Prompts" {
		t.Errorf("data.prompts.name = %v, want Prompts", prompts["name"])
	}
	if prompts["type"] != "text" {
		t.Errorf("data.prompts.type = %v, want text", prompts["type"])
	}
	uploadSources, ok := env.Data["upload_sources"].(map[string]interface{})
	if !ok {
		t.Fatalf("data.upload_sources = %v, want a map", env.Data["upload_sources"])
	}
	if uploadSources["name"] != "Upload sources" {
		t.Errorf("data.upload_sources.name = %v, want Upload sources", uploadSources["name"])
	}
	if uploadSources["type"] != "line" {
		t.Errorf("data.upload_sources.type = %v, want line", uploadSources["type"])
	}
}

func TestGetComponentInputForm_CanvasNotFound(t *testing.T) {
	h := &AgentHandler{loader: &fakeCanvasLoader{err: dao.ErrUserCanvasNotFound}}

	c, w := componentCtx(t, "GET", "/api/v1/agents/c1/components/begin/input-form", "")
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "begin"},
	}
	h.GetComponentInputForm(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != 103 { // CodeOperatingError — python @_require_canvas_access parity
		t.Errorf("code = %d, want 103; msg=%q", code, msg)
	}
	want := "Make sure you have permission to access the agent."
	if msg != want {
		t.Errorf("msg = %q, want %q", msg, want)
	}
}

func TestGetComponentInputForm_ComponentNotFound(t *testing.T) {
	cv := beginCanvas()
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}

	c, w := componentCtx(t, "GET", "/api/v1/agents/c1/components/missing/input-form", "")
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "missing"},
	}
	h.GetComponentInputForm(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != 102 { // CodeDataError
		t.Errorf("code = %d, want 102; msg=%q", code, msg)
	}
	if !strings.Contains(msg, "component not found") {
		t.Errorf("msg = %q, want 'component not found'", msg)
	}
}

func TestGetComponentInputForm_NoInputForm(t *testing.T) {
	cv := &entity.UserCanvas{
		ID: "c1",
		DSL: map[string]any{
			"components": map[string]any{
				"answer": map[string]any{
					"obj": map[string]any{
						"component_name": "Answer",
						// no input_form
					},
				},
			},
		},
	}
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}

	c, w := componentCtx(t, "GET", "/api/v1/agents/c1/components/answer/input-form", "")
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "answer"},
	}
	h.GetComponentInputForm(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != 102 {
		t.Errorf("code = %d, want 102; msg=%q", code, msg)
	}
	if !strings.Contains(msg, "no input_form") {
		t.Errorf("msg = %q, want 'no input_form'", msg)
	}
}

// ---------- DebugComponent ----------

func TestDebugComponent_HappyPath_Begin(t *testing.T) {
	cv := beginCanvas()
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}

	body := `{"params":{"query":{"value":"hello world"}}}`
	c, w := componentCtx(t, "POST", "/api/v1/agents/c1/components/begin/debug", body)
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "begin"},
	}
	h.DebugComponent(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	// Begin's Invoke is a passthrough — it returns the input map as
	// outputs (see internal/agent/component/begin.go:96-98). Pin the
	// exact passthrough so a regression that drops the `query` field
	// would be caught (architect review A2).
	var env struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if env.Code != 0 {
		t.Errorf("code = %d, want 0; body=%s", env.Code, w.Body.String())
	}
	if env.Data["query"] != "hello world" {
		t.Errorf("data.query = %v, want 'hello world'", env.Data["query"])
	}
}

type sysEchoComponent struct{}

func (s *sysEchoComponent) Invoke(ctx context.Context, _ map[string]any) (map[string]any, error) {
	state, _, err := agentruntime.GetStateFromContext[*agentruntime.CanvasState](ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"query":     state.Sys["query"],
		"tenant_id": state.Sys["tenant_id"],
	}, nil
}

func TestDebugComponent_SeedsSysInputsIntoCanvasState(t *testing.T) {
	origFactory := agentruntime.DefaultFactory()
	agentruntime.SetDefaultFactory(func(name string, _ map[string]any) (agentruntime.Component, error) {
		if name != "Probe" {
			return nil, errors.New("unexpected component name: " + name)
		}
		return &sysEchoComponent{}, nil
	})
	t.Cleanup(func() { agentruntime.SetDefaultFactory(origFactory) })

	cv := &entity.UserCanvas{
		ID: "c1",
		DSL: map[string]any{
			"components": map[string]any{
				"probe": map[string]any{
					"obj": map[string]any{
						"component_name": "Probe",
						"params":         map[string]any{},
					},
				},
			},
		},
	}
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}

	body := `{"params":{"sys.query":{"value":"hello sys"}}}`
	c, w := componentCtx(t, "POST", "/api/v1/agents/c1/components/probe/debug", body)
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "probe"},
	}
	h.DebugComponent(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if env.Data["query"] != "hello sys" {
		t.Fatalf("data.query = %v, want 'hello sys'", env.Data["query"])
	}
}

func TestDebugComponent_RejectsSysTenantIDOverride(t *testing.T) {
	origFactory := agentruntime.DefaultFactory()
	agentruntime.SetDefaultFactory(func(name string, _ map[string]any) (agentruntime.Component, error) {
		if name != "Probe" {
			return nil, errors.New("unexpected component name: " + name)
		}
		return &sysEchoComponent{}, nil
	})
	t.Cleanup(func() { agentruntime.SetDefaultFactory(origFactory) })

	cv := &entity.UserCanvas{
		ID: "c1",
		DSL: map[string]any{
			"components": map[string]any{
				"probe": map[string]any{
					"obj": map[string]any{
						"component_name": "Probe",
						"params":         map[string]any{},
					},
				},
			},
		},
	}
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}

	body := `{"params":{"sys.query":{"value":"hello sys"},"sys.tenant_id":{"value":"attacker-tenant"}}}`
	c, w := componentCtx(t, "POST", "/api/v1/agents/c1/components/probe/debug", body)
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "probe"},
	}
	h.DebugComponent(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if env.Data["tenant_id"] != "u-1" {
		t.Fatalf("data.tenant_id = %v, want u-1", env.Data["tenant_id"])
	}
}

func TestDebugComponent_InvalidParams_MissingField(t *testing.T) {
	cv := beginCanvas()
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}

	// No `params` field.
	c, w := componentCtx(t, "POST", "/api/v1/agents/c1/components/begin/debug", `{}`)
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "begin"},
	}
	h.DebugComponent(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != 101 { // CodeArgumentError
		t.Errorf("code = %d, want 101; msg=%q", code, msg)
	}
	if !strings.Contains(msg, "params") {
		t.Errorf("msg = %q, want 'params'", msg)
	}
}

// TestDebugComponent_InvalidParams_MissingValue covers CodeRabbit
// PR review #2: `{"params":{"q":{}}}` (no `value` key) should
// fail-fast with 101 rather than silently invoking the component
// with inputs["q"]=nil.
func TestDebugComponent_InvalidParams_MissingValue(t *testing.T) {
	cv := beginCanvas()
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}

	c, w := componentCtx(t, "POST", "/api/v1/agents/c1/components/begin/debug", `{"params":{"q":{}}}`)
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "begin"},
	}
	h.DebugComponent(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != 101 {
		t.Errorf("code = %d, want 101; msg=%q", code, msg)
	}
	if !strings.Contains(msg, "value") {
		t.Errorf("msg = %q, want mention of 'value'", msg)
	}
}

func TestDebugComponent_UnknownComponent(t *testing.T) {
	cv := beginCanvas()
	h := &AgentHandler{loader: &fakeCanvasLoader{canvas: cv}}

	body := `{"params":{"x":{"value":1}}}`
	c, w := componentCtx(t, "POST", "/api/v1/agents/c1/components/missing/debug", body)
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "missing"},
	}
	h.DebugComponent(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != 102 {
		t.Errorf("code = %d, want 102; msg=%q", code, msg)
	}
	if !strings.Contains(msg, "component not found") {
		t.Errorf("msg = %q, want 'component not found'", msg)
	}
}

func TestDebugComponent_CannotAccessCanvas(t *testing.T) {
	h := &AgentHandler{loader: &fakeCanvasLoader{err: dao.ErrUserCanvasNotFound}}

	body := `{"params":{"x":{"value":1}}}`
	c, w := componentCtx(t, "POST", "/api/v1/agents/c1/components/begin/debug", body)
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "begin"},
	}
	h.DebugComponent(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != 103 { // CodeOperatingError
		t.Errorf("code = %d, want 103; msg=%q", code, msg)
	}
	want := "Make sure you have permission to access the agent."
	if msg != want {
		t.Errorf("msg = %q, want %q", msg, want)
	}
}

func TestDebugComponent_LoaderError_NonNotFound(t *testing.T) {
	h := &AgentHandler{loader: &fakeCanvasLoader{err: errors.New("db conn lost")}}

	body := `{"params":{"x":{"value":1}}}`
	c, w := componentCtx(t, "POST", "/api/v1/agents/c1/components/begin/debug", body)
	c.Params = gin.Params{
		{Key: "canvas_id", Value: "c1"},
		{Key: "component_id", Value: "begin"},
	}
	h.DebugComponent(c)

	code, _ := errBody(t, w.Body.Bytes())
	if code != 500 { // CodeServerError
		t.Errorf("code = %d, want 500", code)
	}
}
