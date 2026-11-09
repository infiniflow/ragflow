package nlp

import (
	"slices"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestIsDatasetCompilationChunk(t *testing.T) {
	for _, tt := range []struct {
		name  string
		chunk map[string]interface{}
		want  bool
	}{
		{
			name:  "complete dataset compilation identity",
			chunk: map[string]interface{}{"scope_kwd": "dataset", "compile_kwd": "graph", "doc_id": "kb-1", "kb_id": "kb-1"},
			want:  true,
		},
		{name: "ordinary chunk without doc_id", chunk: map[string]interface{}{"kb_id": "kb-1"}},
		{name: "ordinary chunk with empty doc_id", chunk: map[string]interface{}{"doc_id": "", "kb_id": "kb-1"}},
		{name: "compilation without doc_id", chunk: map[string]interface{}{"scope_kwd": "dataset", "compile_kwd": "graph", "kb_id": "kb-1"}},
		{name: "sentinel without compilation marker", chunk: map[string]interface{}{"scope_kwd": "dataset", "doc_id": "kb-1", "kb_id": "kb-1"}},
		{name: "sentinel without dataset scope", chunk: map[string]interface{}{"compile_kwd": "graph", "doc_id": "kb-1", "kb_id": "kb-1"}},
		{name: "list-wrapped sentinel", chunk: map[string]interface{}{"scope_kwd": "dataset", "compile_kwd": "graph", "doc_id": []string{"kb-1"}, "kb_id": []string{"kb-1"}}},
		{name: "document compilation", chunk: map[string]interface{}{"scope_kwd": "doc", "compile_kwd": "graph", "doc_id": "doc-1", "kb_id": "kb-1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDatasetCompilationChunk(tt.chunk); got != tt.want {
				t.Fatalf("isDatasetCompilationChunk(%v) = %v, want %v", tt.chunk, got, tt.want)
			}
		})
	}
}

func TestPruneDeletedChunksFiltersOrphans(t *testing.T) {
	newRetrievalTestDB(t, "doc-live")
	liveA := map[string]interface{}{"id": "live-a", "doc_id": "doc-live", "kb_id": "kb-1", "_index": "ragflow_tenant-1"}
	liveB := map[string]interface{}{"id": "live-b", "doc_id": "doc-live", "kb_id": "kb-1", "_index": "ragflow_tenant-1"}
	missing := map[string]interface{}{"id": "missing", "kb_id": "kb-1"}
	empty := map[string]interface{}{"id": "empty", "doc_id": "", "kb_id": "kb-1"}
	compiledWithoutSource := map[string]interface{}{"id": "compiled", "kb_id": "kb-1", "compile_kwd": "graph", "scope_kwd": "dataset"}
	docEngine := &retrievalCountEngine{}
	service := NewRetrievalService(docEngine, &dao.DocumentDAO{})

	for _, tt := range []struct {
		name   string
		ids    []string
		chunks []map[string]interface{}
		want   []string
	}{
		{
			name:   "live sources mixed with missing and empty doc_id",
			ids:    []string{"live-b", "missing", "live-a", "empty"},
			chunks: []map[string]interface{}{liveA, missing, empty, liveB},
			want:   []string{"live-b", "live-a"},
		},
		{
			name:   "all ordinary chunks lack a source",
			ids:    []string{"missing", "empty"},
			chunks: []map[string]interface{}{missing, empty},
		},
		{
			name:   "compilation marker cannot preserve chunks without a source",
			ids:    []string{"missing", "compiled", "empty"},
			chunks: []map[string]interface{}{missing, compiledWithoutSource, empty},
		},
		{
			name:   "ghost is removed even when every source exists",
			ids:    []string{"live-a", "ghost"},
			chunks: []map[string]interface{}{liveA},
			want:   []string{"live-a"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			highlight := make(map[string]string, len(tt.ids))
			for _, id := range tt.ids {
				highlight[id] = "highlight " + id
			}
			result := &RetrievalSearchResult{
				IDs:       tt.ids,
				Chunks:    tt.chunks,
				Field:     docEngine.GetFields(tt.chunks, []string{"doc_id", "kb_id", "compile_kwd", "scope_kwd"}),
				Highlight: highlight,
				Total:     int64(len(tt.ids)),
			}
			pruned, err := service.PruneDeletedChunks(t.Context(), result)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(pruned.IDs, tt.want) || pruned.Total != int64(len(tt.want)) {
				t.Fatalf("IDs = %v, total = %d; want %v, %d", pruned.IDs, pruned.Total, tt.want, len(tt.want))
			}
			if len(pruned.Field) != len(tt.want) || len(pruned.Chunks) != len(tt.want) || len(pruned.Highlight) != len(tt.want) {
				t.Fatalf("field/chunks/highlight sizes = %d/%d/%d, want %d", len(pruned.Field), len(pruned.Chunks), len(pruned.Highlight), len(tt.want))
			}
			for i, id := range tt.want {
				if _, ok := pruned.Field[id]; !ok {
					t.Fatalf("retained chunk %q is missing from Field", id)
				}
				if chunk := pruned.Chunks[i]; chunk["id"] != id || chunk["_index"] != "ragflow_tenant-1" {
					t.Fatalf("raw chunk at %d = %v, want %q with its original index", i, chunk, id)
				}
				if got := pruned.Highlight[id]; got != "highlight "+id {
					t.Fatalf("highlight for %q = %q", id, got)
				}
			}
		})
	}
}

func TestPruneDeletedChunksSkipsLookupForDatasetCompilation(t *testing.T) {
	for _, tt := range []struct {
		name string
		ids  []string
	}{
		{name: "dataset compilation only", ids: []string{"compiled"}},
		{name: "dataset compilation with ghost", ids: []string{"compiled", "ghost"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := newRetrievalTestDB(t)
			queries := 0
			if err := db.Callback().Query().Before("gorm:query").Register("count_prune_document_queries", func(*gorm.DB) {
				queries++
			}); err != nil {
				t.Fatal(err)
			}
			compiled := map[string]interface{}{"id": "compiled", "doc_id": "kb-1", "kb_id": "kb-1", "compile_kwd": "graph", "scope_kwd": "dataset", "_index": "ragflow_tenant-1"}
			docEngine := &retrievalCountEngine{}
			chunks := []map[string]interface{}{compiled}
			result := &RetrievalSearchResult{
				IDs:       tt.ids,
				Chunks:    chunks,
				Field:     docEngine.GetFields(chunks, []string{"doc_id", "kb_id", "compile_kwd", "scope_kwd"}),
				Highlight: map[string]string{"compiled": "compiled highlight"},
				Total:     int64(len(tt.ids)),
			}
			if len(tt.ids) > 1 {
				result.Highlight["ghost"] = "ghost highlight"
			}
			pruned, err := NewRetrievalService(docEngine, &dao.DocumentDAO{}).PruneDeletedChunks(t.Context(), result)
			if err != nil {
				t.Fatal(err)
			}
			if queries != 0 {
				t.Fatalf("document queries = %d, want 0", queries)
			}
			if !slices.Equal(pruned.IDs, []string{"compiled"}) || pruned.Total != 1 {
				t.Fatalf("IDs = %v, total = %d; want [compiled], 1", pruned.IDs, pruned.Total)
			}
			if len(pruned.Field) != 1 || len(pruned.Chunks) != 1 || len(pruned.Highlight) != 1 {
				t.Fatalf("field/chunks/highlight sizes = %d/%d/%d, want 1", len(pruned.Field), len(pruned.Chunks), len(pruned.Highlight))
			}
			if _, ok := pruned.Field["compiled"]; !ok {
				t.Fatal("compiled chunk is missing from Field")
			}
			if pruned.Chunks[0]["id"] != "compiled" || pruned.Chunks[0]["_index"] != "ragflow_tenant-1" || pruned.Highlight["compiled"] != "compiled highlight" {
				t.Fatalf("compiled raw chunk or highlight was not preserved: %v, %v", pruned.Chunks, pruned.Highlight)
			}
		})
	}
}

func newRetrievalTestDB(t *testing.T, docIDs ...string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.Document{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range docIDs {
		if err := db.Create(&entity.Document{ID: id, KbID: "kb-1", ParserConfig: entity.JSONMap{}}).Error; err != nil {
			t.Fatal(err)
		}
	}
	oldDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = oldDB })
	return db
}
