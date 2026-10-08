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

package task

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
)

// TestKnowledgeCompilerRegisteredByWiring locks the composition-root contract:
// the task package imports the knowledge_compiler root package (via blank
// import in knowledge_compiler_wiring.go) so its init() registers the
// knowledge-compilation component under the unified name "Compiler" in the
// production runtime registry. Without this blank import the component is never
// registered and an ingestor consuming a canvas with a Compiler node fails with
// "unknown component".
func TestKnowledgeCompilerRegisteredByWiring(t *testing.T) {
	factory, category, _, ok := runtime.DefaultRegistry.Lookup("Compiler")
	if !ok {
		t.Fatal("knowledge-compiler component \"Compiler\" is not registered; the task package blank-import must be present for its init() to run")
	}
	if category != runtime.CategoryIngestion {
		t.Fatalf("component \"Compiler\" category = %q, want %q", category, runtime.CategoryIngestion)
	}
	if factory == nil {
		t.Fatal("component \"Compiler\" registered with a nil factory")
	}
}

type wikiPageStoreEngine struct {
	engine.DocEngine
	row map[string]interface{}
}

func (e *wikiPageStoreEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	row := make(map[string]interface{})
	for _, field := range req.SelectFields {
		if value, ok := e.row[field]; ok {
			row[field] = value
		}
	}
	return &types.SearchResult{Chunks: []map[string]interface{}{row}}, nil
}

func TestGetPageBySlugReadsStoredMarkdownBody(t *testing.T) {
	const body = "# Alpha\n\n**Body** links to [Beta](artifact/kb1/entity/beta)."
	eng := &wikiPageStoreEngine{row: map[string]interface{}{
		"id": "page1", "slug_kwd": "entity/alpha", "md_with_weight": body,
		"compile_kwd": "wiki_page", "page_type_kwd": "entity",
	}}
	store := &kcWikiPageStore{docEngine: eng}
	page, err := store.GetPageBySlug(t.Context(), "t1", "kb1", "entity/alpha")
	if err != nil || page == nil {
		t.Fatalf("page = %+v, err = %v", page, err)
	}
	if page.ContentMD != body || page.ContentMDRaw != body {
		t.Fatalf("incremental page body changed: %+v", page)
	}
}

// TestDatasetEmbeddingIDBindsTheDatasetModel mirrors Python's compile-task
// binding: the tenant-level model wins over the dataset's own, a blank tenant id
// falls back to the dataset model, and an empty id defers to the tenant default.
func TestDatasetEmbeddingIDBindsTheDatasetModel(t *testing.T) {
	tenantModel := "tenant-model-1"
	blank := "  "

	tests := []struct {
		name string
		kb   *entity.Knowledgebase
		want string
	}{
		{name: "nil knowledgebase", kb: nil, want: ""},
		{name: "dataset model", kb: &entity.Knowledgebase{EmbdID: "bge-large"}, want: "bge-large"},
		{
			name: "tenant model wins",
			kb:   &entity.Knowledgebase{EmbdID: "bge-large", TenantEmbdID: &tenantModel},
			want: "tenant-model-1",
		},
		{
			name: "blank tenant model falls back",
			kb:   &entity.Knowledgebase{EmbdID: "bge-large", TenantEmbdID: &blank},
			want: "bge-large",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := datasetEmbeddingID(tc.kb); got != tc.want {
				t.Fatalf("datasetEmbeddingID() = %q, want %q", got, tc.want)
			}
		})
	}
}
