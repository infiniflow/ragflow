package harness

import (
	"context"
	"testing"

	"ragflow/internal/service/nav"
)

// TestAskNavSelect_IndexBased asserts the model's index-based selection maps
// back to the item subset.
func TestAskNavSelect_IndexBased(t *testing.T) {
	installChat(t, `{"relevant":[0,2]}`)
	items := []navSelectItem{
		{Name: "Alpha", Description: "aaa"},
		{Name: "Beta", Description: "bbb"},
		{Name: "Gamma", Description: "ggg"},
	}
	out := askNavSelect(context.Background(), nil, "query", "clusters", items, 10)
	if len(out) != 2 {
		t.Fatalf("selected = %d, want 2", len(out))
	}
	if out[0].Name != "Alpha" || out[1].Name != "Gamma" {
		t.Errorf("selected names = %q, %q; want Alpha, Gamma", out[0].Name, out[1].Name)
	}
}

// TestAskNavSelect_Empty asserts an empty "relevant" list yields nothing.
func TestAskNavSelect_Empty(t *testing.T) {
	installChat(t, `{"relevant":[]}`)
	if out := askNavSelect(context.Background(), nil, "q", "clusters", []navSelectItem{{Name: "A"}}, 10); len(out) != 0 {
		t.Errorf("expected no selection, got %d", len(out))
	}
}

// TestAskNavSelect_OutOfRange asserts invalid indices are skipped.
func TestAskNavSelect_OutOfRange(t *testing.T) {
	installChat(t, `{"relevant":[0,99,-1]}`)
	out := askNavSelect(context.Background(), nil, "q", "clusters", []navSelectItem{{Name: "A"}, {Name: "B"}}, 10)
	if len(out) != 1 || out[0].Name != "A" {
		t.Errorf("out-of-range selection = %+v, want [A]", out)
	}
}

// TestNavigateDatasetByTree_NoQuery asserts empty query returns nil without
// calling anything.
func TestNavigateDatasetByTree_NoQuery(t *testing.T) {
	if out := NavigateDatasetByTree(context.Background(), nil, nil, "t1", "kb1", "  "); out != nil {
		t.Errorf("expected nil for empty query, got %v", out)
	}
}

// fakeNavSvcHarness is an in-memory nav.NavService for the BFS test.
type fakeNavSvcHarness struct {
	clusters []nav.NavNode
	children map[string][]nav.NavNode
}

func (f *fakeNavSvcHarness) UpsertDoc(context.Context, nav.UpsertDocInput) error { return nil }
func (f *fakeNavSvcHarness) RemoveDoc(context.Context, string, string, string) error {
	return nil
}
func (f *fakeNavSvcHarness) Search(context.Context, string, string, string, []float32, int) ([]nav.NavHit, error) {
	return nil, nil
}
func (f *fakeNavSvcHarness) ListClusters(context.Context, string, string, int, int) ([]nav.NavNode, int64, error) {
	return f.clusters, int64(len(f.clusters)), nil
}
func (f *fakeNavSvcHarness) ListChildren(_ context.Context, _, _, name string, _, _ int) ([]nav.NavNode, int64, error) {
	return f.children[name], int64(len(f.children[name])), nil
}

// TestCollectNavLeaves_BFS asserts document leaves are collected, sub-clusters
// descended, and leaves deduped by doc_id.
func TestCollectNavLeaves_BFS(t *testing.T) {
	ns := &fakeNavSvcHarness{
		clusters: []nav.NavNode{{Name: "C1", Description: "cluster 1"}},
		children: map[string][]nav.NavNode{
			"C1": {
				{Name: "Sub", Type: "cluster"},
				{Name: "DocA", Type: "doc", DocID: "d1"},
				{Name: "DocB", Type: "doc", DocID: "d2"},
			},
			"Sub": {{Name: "DocC", Type: "doc", DocID: "d3"}},
		},
	}
	selected := []navSelectItem{{Name: "C1", Description: "cluster 1"}}
	leaves := collectNavLeaves(context.Background(), ns, "t1", "kb1", selected)
	if len(leaves) != 3 {
		t.Fatalf("leaves = %d, want 3 (d1,d2 from C1 + d3 from Sub)", len(leaves))
	}
	seen := map[string]bool{}
	for _, l := range leaves {
		if l.DocID == "" {
			t.Errorf("leaf %q has empty doc_id", l.Name)
		}
		seen[l.DocID] = true
	}
	if !seen["d1"] || !seen["d2"] || !seen["d3"] {
		t.Errorf("collected doc_ids = %v, want d1,d2,d3", seen)
	}
}
