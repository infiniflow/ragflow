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

// fakePipelineLister is a test double for builtinPipelineLister.
type fakePipelineLister struct {
	items []*pipelinepkg.BuiltinPipelineMeta
}

func (f fakePipelineLister) List() []*pipelinepkg.BuiltinPipelineMeta { return f.items }

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
			DSL:         map[string]any{"components": map[string]any{}},
		},
		{
			ParserID: "book",
			Title:    "Book",
			Filename: "ingestion_pipeline_book.json",
			DSL:      map[string]any{"components": map[string]any{}},
		},
	}
	h := &PipelineHandler{registry: fakePipelineLister{items: items}}

	c, w := pipelineCtx()
	h.ListPipelines(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Code int `json:"code"`
		Data []struct {
			ParserID string         `json:"parser_id"`
			Title    string         `json:"title"`
			DSL      map[string]any `json:"dsl"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Code != int(common.CodeSuccess) {
		t.Fatalf("code = %d, want %d", resp.Code, int(common.CodeSuccess))
	}
	if len(resp.Data) != 2 {
		t.Fatalf("len(data) = %d, want 2", len(resp.Data))
	}
	if resp.Data[0].ParserID != "general" || resp.Data[0].Title != "General" {
		t.Errorf("data[0] = %+v", resp.Data[0])
	}
}

func TestPipelineHandler_ListPipelines_NoTypeParam(t *testing.T) {
	items := []*pipelinepkg.BuiltinPipelineMeta{
		{ParserID: "general", Title: "General"},
	}
	h := &PipelineHandler{registry: fakePipelineLister{items: items}}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/pipelines", nil)

	h.ListPipelines(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Data []struct {
			ParserID string `json:"parser_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(data) = %d, want 1 (no type param defaults to builtin)", len(resp.Data))
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
		Data []struct {
			ParserID string `json:"parser_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) == 0 {
		t.Fatal("expected non-empty builtin pipeline list from real registry")
	}
	seen := map[string]bool{}
	for _, item := range resp.Data {
		seen[item.ParserID] = true
	}
	if !seen["general"] {
		t.Error("real registry listing missing general")
	}
	if seen["naive"] {
		t.Error("alias naive must be hidden from listing")
	}
}
