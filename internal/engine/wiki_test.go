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

package engine

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	infinity "github.com/infiniflow/infinity-go-sdk"

	"ragflow/internal/engine/types"
)

type wikiSearchEngine struct {
	DocEngine
	search func(*types.SearchRequest) (*types.SearchResult, error)
}

func (e wikiSearchEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	return e.search(req)
}

func TestSearchWithWikiContent(t *testing.T) {
	const body = "# Page\n\n[Link](entity/example)"
	for _, test := range []struct {
		name, content, markdown string
		fallbackErr             error
		calls                   int
	}{
		{"canonical", body, "stale duplicate", nil, 1},
		{"empty content", "", body, nil, 2},
		{"blank content", " \n", body, nil, 2},
		{"empty Markdown", "", "", nil, 2},
		{"missing Markdown column", "", "", infinity.NewInfinityException(3024, "Failed to execute query: Column: md_with_weight doesn't exist"), 2},
		{"missing Markdown binder", "", "", infinity.NewInfinityException(3013, "Failed to execute query: Fail to bind the expression: md_with_weight"), 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			eng := wikiSearchEngine{search: func(req *types.SearchRequest) (*types.SearchResult, error) {
				calls++
				if calls == 1 {
					if slices.Contains(req.SelectFields, "md_with_weight") {
						t.Fatal("primary query must not select Markdown")
					}
					return &types.SearchResult{Chunks: []map[string]any{{
						"id": "p1", "compile_kwd": "wiki", "type_kwd": "wiki_page",
						"content_with_weight": test.content, "_score": 0.9,
					}, {
						"id": "p2", "compile_kwd": "wiki", "type_kwd": "wiki_page",
						"content_with_weight": "# Another page", "_score": 0.8,
					}, {
						"id": "g1", "compile_kwd": "graph", "content_with_weight": "",
					}}, Total: 7}, nil
				}
				if !reflect.DeepEqual(req.SelectFields, []string{"id", "md_with_weight"}) ||
					!reflect.DeepEqual(req.Filter["id"], []string{"p1"}) || req.Filter["doc_id"] != "d1" ||
					!reflect.DeepEqual(req.IndexNames, []string{"ragflow_t1"}) ||
					!reflect.DeepEqual(req.KbIDs, []string{"kb1"}) || req.Offset != 0 || req.Limit != 1 || len(req.MatchExprs) != 0 {
					t.Fatalf("unexpected fallback query: %+v", req)
				}
				if test.fallbackErr != nil {
					return nil, fmt.Errorf("query table: %w", test.fallbackErr)
				}
				return &types.SearchResult{Chunks: []map[string]any{{"id": "p1", "md_with_weight": test.markdown}}}, nil
			}}
			req := &types.SearchRequest{
				IndexNames: []string{"ragflow_t1"}, KbIDs: []string{"kb1"}, Offset: 10,
				SelectFields: []string{"id", "compile_kwd", "type_kwd", "content_with_weight", "_score"},
				Filter:       map[string]any{"doc_id": "d1"}, MatchExprs: []any{"query"},
			}
			result, err := SearchWithWikiContent(t.Context(), eng, req)
			if err != nil || result == nil || len(result.Chunks) != 3 || result.Total != 7 || calls != test.calls {
				t.Fatalf("result = %+v, calls = %d, err = %v", result, calls, err)
			}
			want := body
			if test.markdown == "" && test.content == "" {
				want = ""
			}
			if got := types.WikiPageContent(result.Chunks[0]); got != want || result.Chunks[0]["_score"] != 0.9 {
				t.Fatalf("body or score changed: %+v", result.Chunks[0])
			}
			if _, ok := req.Filter["id"]; ok {
				t.Fatal("fallback mutated the primary filter")
			}
		})
	}
}

func TestSearchWithWikiContentMissingCanonicalField(t *testing.T) {
	for _, field := range []string{"content", "content_with_weight"} {
		t.Run(field, func(t *testing.T) {
			calls := 0
			eng := wikiSearchEngine{search: func(req *types.SearchRequest) (*types.SearchResult, error) {
				calls++
				if calls == 1 {
					var queryErr error = infinity.NewInfinityException(3024, "Column: "+field+" doesn't exist")
					return nil, fmt.Errorf("query table: %w", queryErr)
				}
				if calls == 2 {
					if slices.Contains(req.SelectFields, "content_with_weight") || slices.Contains(req.SelectFields, "md_with_weight") || req.Offset != 3 {
						t.Fatalf("unexpected metadata query: %+v", req)
					}
					return &types.SearchResult{Chunks: []map[string]any{{"id": "p1", "compile_kwd": "wiki", "_score": 0.9}}, Total: 5}, nil
				}
				return &types.SearchResult{Chunks: []map[string]any{{"id": "p1", "md_with_weight": "# Body"}}}, nil
			}}
			req := &types.SearchRequest{SelectFields: []string{"id", "compile_kwd", "content_with_weight", "_score"}, Offset: 3}
			result, err := SearchWithWikiContent(t.Context(), eng, req)
			if err != nil || result == nil || result.Total != 5 || calls != 3 || types.WikiPageContent(result.Chunks[0]) != "# Body" {
				t.Fatalf("result = %+v, calls = %d, err = %v", result, calls, err)
			}
			if !slices.Contains(req.SelectFields, "content_with_weight") {
				t.Fatal("fallback mutated the primary projection")
			}
		})
	}
}

func TestSearchWithWikiContentPropagatesErrors(t *testing.T) {
	for _, queryErr := range []error{
		errors.New("connection failed"),
		infinity.NewInfinityException(3024, "Column: doc_id doesn't exist"),
		infinity.NewInfinityException(3013, "Fail to bind the expression: q_"),
		infinity.NewInfinityException(3013, "Fail to bind the expression: md_with_weight_extra"),
	} {
		for _, failAt := range []int{1, 2} {
			calls := 0
			eng := wikiSearchEngine{search: func(*types.SearchRequest) (*types.SearchResult, error) {
				calls++
				if calls == failAt {
					return nil, fmt.Errorf("query table: %w", queryErr)
				}
				return &types.SearchResult{Chunks: []map[string]any{{"id": "p1", "compile_kwd": "wiki"}}}, nil
			}}
			_, err := SearchWithWikiContent(t.Context(), eng, &types.SearchRequest{SelectFields: []string{"id", "compile_kwd", "content_with_weight"}})
			if !errors.Is(err, queryErr) || calls != failAt {
				t.Fatalf("query %d: calls = %d, got %v, want %v", failAt, calls, err, queryErr)
			}
		}
	}
}
