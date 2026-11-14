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
	"errors"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	kc "ragflow/internal/ingestion/component/knowledge_compiler/common"
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
	row      map[string]interface{}
	getErr   error
	getCalls int
	getIndex string
	getID    string
	getKbIDs []string
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

func (e *wikiPageStoreEngine) GetChunk(_ context.Context, index, id string, kbIDs []string) (interface{}, error) {
	e.getCalls++
	e.getIndex = index
	e.getID = id
	e.getKbIDs = append([]string(nil), kbIDs...)
	return e.row, e.getErr
}

func TestWikiPageStoreReadsBody(t *testing.T) {
	const body = "# Alpha\n\n**Body** links to [Beta](artifact/kb1/entity/beta)."
	getErr := errors.New("get chunk failed")
	paths := []struct {
		name string
		read func(context.Context, *kcWikiPageStore) ([]kc.WikiPageCandidate, error)
	}{
		{name: "similar", read: func(ctx context.Context, store *kcWikiPageStore) ([]kc.WikiPageCandidate, error) {
			return store.FindSimilarPages(ctx, "t1", "kb1", []float32{1, 2}, 3)
		}},
		{name: "slug", read: func(ctx context.Context, store *kcWikiPageStore) ([]kc.WikiPageCandidate, error) {
			page, err := store.GetPageBySlug(ctx, "t1", "kb1", "entity/alpha")
			if page == nil {
				return nil, err
			}
			return []kc.WikiPageCandidate{*page}, err
		}},
		{name: "source chunks", read: func(ctx context.Context, store *kcWikiPageStore) ([]kc.WikiPageCandidate, error) {
			return store.FindPagesBySourceChunks(ctx, "t1", "kb1", []string{"source1"}, 3)
		}},
	}
	cases := []struct {
		name         string
		canonical    string
		hasCanonical bool
		getErr       error
		wantBody     string
		wantGetCalls int
	}{
		{name: "stored markdown", wantBody: body, wantGetCalls: 1},
		{name: "empty canonical", hasCanonical: true, wantBody: body, wantGetCalls: 1},
		{name: "canonical wins", canonical: "# Updated body", hasCanonical: true, wantBody: "# Updated body"},
		{name: "whitespace canonical", canonical: "\n\t ", hasCanonical: true},
		{name: "load failure", getErr: getErr, wantGetCalls: 1},
	}
	for _, path := range paths {
		t.Run(path.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					row := map[string]interface{}{
						"id": "page1", "slug_kwd": "entity/alpha", "md_with_weight": body,
						"compile_kwd": "wiki_page", "page_type_kwd": "entity",
					}
					if tc.hasCanonical {
						row["content_with_weight"] = tc.canonical
					}
					eng := &wikiPageStoreEngine{row: row, getErr: tc.getErr}
					pages, err := path.read(t.Context(), &kcWikiPageStore{docEngine: eng})
					if !errors.Is(err, tc.getErr) {
						t.Fatalf("read error = %v, want %v", err, tc.getErr)
					}
					if eng.getCalls != tc.wantGetCalls {
						t.Fatalf("GetChunk calls = %d, want %d", eng.getCalls, tc.wantGetCalls)
					}
					if eng.getCalls > 0 && (eng.getIndex != "ragflow_t1" || eng.getID != "page1" || len(eng.getKbIDs) != 1 || eng.getKbIDs[0] != "kb1") {
						t.Fatalf("GetChunk scope = (%q, %q, %v), want (ragflow_t1, page1, [kb1])", eng.getIndex, eng.getID, eng.getKbIDs)
					}
					if err != nil {
						return
					}
					if len(pages) != 1 || pages[0].ContentMD != tc.wantBody || pages[0].ContentMDRaw != tc.wantBody {
						t.Fatalf("pages = %+v, want one page with body %q", pages, tc.wantBody)
					}
				})
			}
		})
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
