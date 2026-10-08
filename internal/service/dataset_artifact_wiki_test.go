package service

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
)

type wikiContentEngine struct {
	fakeChatDocEngine
	row     map[string]interface{}
	queries []*types.SearchRequest
	update  map[string]interface{}
}

func (e *wikiContentEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.queries = append(e.queries, req)
	return &types.SearchResult{Chunks: []map[string]interface{}{e.row}, Total: 1}, nil
}

func (e *wikiContentEngine) UpdateChunks(_ context.Context, _ map[string]interface{}, update map[string]interface{}, _, _ string) error {
	e.update = update
	for key, value := range update {
		e.row[key] = value
	}
	return nil
}

func TestWikiPageReadAndEditUseContentWithWeight(t *testing.T) {
	const body = "# Alpha\n\n**Body** links to [Beta](artifact/kb1/entity/beta)."
	for _, duplicate := range []string{"", "stale duplicate"} {
		t.Run("duplicate="+duplicate, func(t *testing.T) {
			eng := &wikiContentEngine{row: map[string]interface{}{
				"id": "page1", "slug_kwd": "entity/alpha", "compile_kwd": "wiki",
				"type_kwd": "wiki_page", "entity_type_kwd": "entity",
				"content_with_weight": body,
			}}
			if duplicate != "" {
				eng.row["md_with_weight"] = duplicate
			}
			svc := &DatasetArtifactService{docEngine: func() engine.DocEngine { return eng }}
			page, err := svc.GetWikiPage(t.Context(), "t1", "kb1", "entity", "alpha")
			if err != nil || page == nil || page.ContentMd != body || page.PageType != "entity" || page.Slug != "alpha" {
				t.Fatalf("page = %+v, err = %v", page, err)
			}
			encoded, err := json.Marshal(page)
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["content_md_rendered"] != body {
				t.Fatalf("frontend Markdown contract changed: %s", encoded)
			}
			edited := body + "\n\nNew paragraph."
			page, err = svc.UpdateWikiPage(t.Context(), "t1", "kb1", "entity", "alpha", edited, "", nil)
			if err != nil || page == nil || page.ContentMd != edited {
				t.Fatalf("edited page = %+v, err = %v", page, err)
			}
			if !reflect.DeepEqual(eng.update, map[string]interface{}{"content_with_weight": edited}) {
				t.Fatalf("unexpected edit fields: %v", eng.update)
			}
			for _, req := range eng.queries {
				if slices.Contains(req.SelectFields, "md_with_weight") {
					t.Fatal("Wiki detail must not query md_with_weight")
				}
				if slices.Contains(req.SelectFields, "content_with_weight") {
					for _, field := range []string{"compile_kwd", "type_kwd", "entity_type_kwd"} {
						if !slices.Contains(req.SelectFields, field) {
							t.Fatalf("Wiki detail query is missing %s", field)
						}
					}
				}
			}
		})
	}
}
