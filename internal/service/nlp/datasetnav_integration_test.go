//go:build integration
// +build integration

package nlp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/server"
	"ragflow/internal/service/nav"
)

// repoRootOf walks up from the package directory to the repository root (the
// dir containing go.mod). Kept local to this test package so it needs no shared
// helper from another package.
func repoRootOf(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root (go.mod) not found above cwd")
		}
		dir = parent
	}
}

// findNavRow reads the nav row for a doc directly from the document engine.
func findNavRow(t *testing.T, tenantID, kbID, docID string) map[string]interface{} {
	t.Helper()
	de := engine.Get()
	if de == nil {
		t.Skip("no live document engine")
	}
	idx := "ragflow_" + tenantID
	req := &types.SearchRequest{
		IndexNames:   []string{idx},
		Filter:       map[string]interface{}{"doc_id": []string{docID}, "compile_kwd": []string{"dataset_nav"}},
		SelectFields: []string{"available_int", "compile_kwd", "type_kwd"},
		Limit:        10,
	}
	res, err := de.Search(t.Context(), req)
	if err != nil {
		t.Fatalf("search nav row: %v", err)
	}
	for _, row := range res.Chunks {
		return row
	}
	t.Fatalf("no nav row found for doc %s", docID)
	return nil
}

// TestDatasetNav_AvailableIntZero_Isolation is an integration test against a real
// ES/Infinity backend (requires conf/service_conf.yaml + a live document store).
// It verifies acceptance criterion #6: a nav row written with available_int=0 is
// invisible to the default retriever (which filters available_int=1) but IS
// reachable through NavService.Search.
//
// Run with: bash build.sh --test-integration ./internal/service/nlp/...
func TestDatasetNav_AvailableIntZero_Isolation(t *testing.T) {
	if err := common.InitLogger("info", common.FileOutput{}, ""); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	configPath := filepath.Join(repoRootOf(t), "conf", "service_conf.yaml")
	if err := server.Init(configPath); err != nil {
		t.Fatalf("init service config: %v", err)
	}
	if err := engine.InitDocEngine(context.Background()); err != nil {
		t.Fatalf("init document engine: %v", err)
	}
	if engine.Get() == nil {
		t.Skip("no live document engine configured")
	}

	tenantID := "navint_t1"
	kbID := "navint_kb1"
	docID := "navint_doc1"

	ns := NewNavService(stubNavEmbedder{})
	if err := ns.UpsertDoc(t.Context(), nav.UpsertDocInput{
		TenantID: tenantID, KbID: kbID, DocID: docID, Summary: "rocket propulsion integration evidence",
	}); err != nil {
		t.Fatalf("upsert nav doc: %v", err)
	}
	t.Cleanup(func() { _ = ns.RemoveDoc(context.Background(), tenantID, kbID, docID) })

	// NavService.Search must find the nav row (reads nav rows directly).
	hits, err := ns.Search(t.Context(), tenantID, kbID, "rocket propulsion", nil, 5)
	if err != nil {
		t.Fatalf("nav search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("NavService.Search returned no hits; nav row is not reachable")
	}

	// The written nav row must carry compile_kwd=dataset_nav and available_int=0,
	// so the default retriever (available_int=1 filter) will not surface it.
	row := findNavRow(t, tenantID, kbID, docID)
	// compile_kwd may come back list-wrapped by the engine, so use firstStrOrSlice.
	if ck := firstStrOrSlice(row["compile_kwd"]); !strings.Contains(ck, "dataset_nav") {
		t.Errorf("nav row compile_kwd = %q, want dataset_nav", ck)
	}
	avail := intValue(row["available_int"])
	if avail != 0 {
		t.Errorf("nav row available_int = %d, want 0 (so it is hidden from the default retriever)", avail)
	}
}

// firstStrOrSlice returns the first string of a value that may be a plain string
// or a list-wrapped string (engine fields are often returned as []interface{}).
func firstStrOrSlice(v interface{}) string {
	switch tv := v.(type) {
	case string:
		return tv
	case []string:
		if len(tv) > 0 {
			return tv[0]
		}
	case []interface{}:
		if len(tv) > 0 {
			if s, ok := tv[0].(string); ok {
				return s
			}
		}
	}
	return ""
}
