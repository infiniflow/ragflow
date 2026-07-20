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
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
)

// fakePipelineRegistry is a test double for builtinPipelineRegistry.
type fakePipelineRegistry struct {
	items []*pipelinepkg.BuiltinPipelineMeta
}

func (f fakePipelineRegistry) List() *pipelinepkg.BuiltinPipelineListResponse {
	return &pipelinepkg.BuiltinPipelineListResponse{
		BuiltinPipelines: f.items,
		Total:            int64(len(f.items)),
	}
}

func (f fakePipelineRegistry) Get(ref string) (*pipelinepkg.BuiltinPipeline, bool) {
	for _, item := range f.items {
		if item.ParserID == ref {
			return &pipelinepkg.BuiltinPipeline{
				BuiltinPipelineMeta: *item,
				DSL:                 map[string]any{"components": map[string]any{}},
			}, true
		}
	}
	return nil, false
}

func pipelineCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/pipelines?type=builtin", nil)
	return c, w
}

func TestPipelineHandler_ListPipelines_TypeBuiltin(t *testing.T) {
	items := []*pipelinepkg.BuiltinPipelineMeta{
		{
			ParserID:    "general",
			Title:       "General",
			Description: "general desc",
			Filename:    "ingestion_pipeline_general.json",
		},
		{
			ParserID: "book",
			Title:    "Book",
			Filename: "ingestion_pipeline_book.json",
		},
	}
	h := &PipelineHandler{registry: fakePipelineRegistry{items: items}}

	c, w := pipelineCtx()
	h.ListPipelines(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Canvas []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"canvas"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Code != int(common.CodeSuccess) {
		t.Fatalf("code = %d, want %d", resp.Code, int(common.CodeSuccess))
	}
	if resp.Data.Total != 2 {
		t.Fatalf("total = %d, want 2", resp.Data.Total)
	}
	if len(resp.Data.Canvas) != 2 {
		t.Fatalf("len(canvas) = %d, want 2", len(resp.Data.Canvas))
	}
	if resp.Data.Canvas[0].ID != "general" || resp.Data.Canvas[0].Title != "General" {
		t.Errorf("canvas[0] = %+v", resp.Data.Canvas[0])
	}
}

func TestPipelineHandler_ListPipelines_NoTypeParam(t *testing.T) {
	items := []*pipelinepkg.BuiltinPipelineMeta{
		{ParserID: "general", Title: "General"},
	}
	h := &PipelineHandler{registry: fakePipelineRegistry{items: items}}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/pipelines", nil)

	h.ListPipelines(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Data struct {
			Canvas []struct {
				ID string `json:"id"`
			} `json:"canvas"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Total != 1 {
		t.Fatalf("total = %d, want 1", resp.Data.Total)
	}
	if len(resp.Data.Canvas) != 1 {
		t.Fatalf("len(canvas) = %d, want 1 (no type param defaults to builtin)", len(resp.Data.Canvas))
	}
}

// TestPipelineHandler_ListPipelines_RealRegistry verifies the production
// DefaultRegistry wires up correctly: the handler returns the actual
// embedded builtin templates (general, book, ...) and the legacy alias
// naive is hidden from the listing.
func TestPipelineHandler_ListPipelines_RealRegistry(t *testing.T) {
	h := NewPipelineHandler()
	if h == nil || h.registry == nil {
		t.Fatal("NewPipelineHandler did not wire a registry")
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/pipelines?type=builtin", nil)

	h.ListPipelines(c)

	var resp struct {
		Data struct {
			Canvas []struct {
				ID string `json:"id"`
			} `json:"canvas"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data.Canvas) == 0 {
		t.Fatal("expected non-empty builtin pipeline list from real registry")
	}
	seen := map[string]bool{}
	for _, item := range resp.Data.Canvas {
		seen[item.ID] = true
	}
	if !seen["general"] {
		t.Error("real registry listing missing general")
	}
	if seen["naive"] {
		t.Error("alias naive must be hidden from listing")
	}
}

func TestPipelineHandler_GetPipeline(t *testing.T) {
	items := []*pipelinepkg.BuiltinPipelineMeta{
		{ParserID: "general", Title: "General"},
	}
	h := &PipelineHandler{registry: fakePipelineRegistry{items: items}}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/pipelines/general", nil)
	c.Params = gin.Params{{Key: "id", Value: "general"}}

	h.GetPipeline(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			DSL map[string]any `json:"dsl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Code != int(common.CodeSuccess) {
		t.Fatalf("code = %d, want %d", resp.Code, int(common.CodeSuccess))
	}
	if resp.Data.DSL == nil {
		t.Fatal("dsl should not be nil")
	}
}

func TestPipelineHandler_GetPipeline_NotFound(t *testing.T) {
	items := []*pipelinepkg.BuiltinPipelineMeta{
		{ParserID: "general", Title: "General"},
	}
	h := &PipelineHandler{registry: fakePipelineRegistry{items: items}}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/pipelines/nonexistent", nil)
	c.Params = gin.Params{{Key: "id", Value: "nonexistent"}}

	h.GetPipeline(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200 (error is in body code)", w.Code)
	}
	var resp struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Code == int(common.CodeSuccess) {
		t.Fatal("expected error code for nonexistent pipeline")
	}
}
