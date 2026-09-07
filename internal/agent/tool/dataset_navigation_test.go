package tool

import (
	"context"
	"sync"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/service/nav"
)

// navRoutingFake is a nav.NavService that records Search calls (topic) and
// returns a controlled doc list, so a test can assert the router actually
// queries by topic rather than walking arbitrary clusters.
type navRoutingFake struct {
	mu       sync.Mutex
	searched []string // topics passed to Search
	hits     []nav.NavHit
	clusters []nav.NavNode
	children map[string][]nav.NavNode
}

func (f *navRoutingFake) UpsertDoc(context.Context, nav.UpsertDocInput) error { return nil }
func (f *navRoutingFake) RemoveDoc(context.Context, string, string, string) error {
	return nil
}
func (f *navRoutingFake) Search(_ context.Context, _, _ string, query string, _ []float32, _ int) ([]nav.NavHit, error) {
	f.mu.Lock()
	f.searched = append(f.searched, query)
	f.mu.Unlock()
	return f.hits, nil
}
func (f *navRoutingFake) ListClusters(context.Context, string, string, int, int) ([]nav.NavNode, int64, error) {
	return f.clusters, int64(len(f.clusters)), nil
}
func (f *navRoutingFake) ListChildren(_ context.Context, _, _, name string, _, _ int) ([]nav.NavNode, int64, error) {
	return f.children[name], int64(len(f.children[name])), nil
}
func (f *navRoutingFake) searchedTopics() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.searched...)
}

// TestDatasetNavigation_UsesTopicRouting asserts the router queries the nav tree
// with the topic (semantic search), rather than blindly walking clusters and
// returning arbitrary doc ids.
func TestDatasetNavigation_UsesTopicRouting(t *testing.T) {
	fake := &navRoutingFake{
		hits: []nav.NavHit{
			{Type: "nav_doc", DocID: "d1", Name: "rocket"},
			{Type: "nav_doc", DocID: "d2", Name: "engine"},
		},
	}
	prev := nav.GetNavService()
	nav.SetNavService(fake)
	defer func() { nav.SetNavService(prev) }()

	state := runtime.NewCanvasState("run-1", "task-1")
	state.Sys["tenant_id"] = "tenant-1"
	ctx := runtime.WithState(t.Context(), state)

	tool := NewDatasetNavigationByTree()
	out, err := tool.InvokableRun(ctx, `{"topic":"rocket propulsion","keywords":"engine","dataset_ids":["kb1"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	// The topic (plus keywords) must have been used as the Search query.
	topics := fake.searchedTopics()
	if len(topics) == 0 {
		t.Fatal("Search was never called; router must route by topic")
	}
	if topics[0] != "rocket propulsion engine" {
		t.Errorf("search query = %q, want topic+keywords", topics[0])
	}
	// The returned docs come from the relevant hits, not arbitrary walk.
	if !containsStr(out, "d1") || !containsStr(out, "d2") {
		t.Errorf("routed docs missing hits: %s", out)
	}
}

// TestCanvasDatasetIDs_MultiKB asserts all explicit dataset ids are preserved
// (a multi-KB session must not collapse to the first KB).
func TestCanvasDatasetIDs_MultiKB(t *testing.T) {
	ids := canvasDatasetIDs(t.Context(), []string{"kb1", "kb2", "kb3"})
	if len(ids) != 3 || ids[0] != "kb1" || ids[1] != "kb2" || ids[2] != "kb3" {
		t.Errorf("canvasDatasetIDs = %v, want all three KBs", ids)
	}
}

// TestDatasetNavigation_MultiKB asserts the router searches EVERY bound dataset
// (not just the first), so docs in other KBs stay reachable.
func TestDatasetNavigation_MultiKB(t *testing.T) {
	fake := &navRoutingFake{hits: []nav.NavHit{{Type: "nav_doc", DocID: "d1", Name: "topic"}}}
	prev := nav.GetNavService()
	nav.SetNavService(fake)
	defer func() { nav.SetNavService(prev) }()

	state := runtime.NewCanvasState("run-1", "task-1")
	state.Sys["tenant_id"] = "tenant-1"
	ctx := runtime.WithState(t.Context(), state)

	tool := NewDatasetNavigationByTree()
	_, err := tool.InvokableRun(ctx, `{"topic":"X","dataset_ids":["kb1","kb2","kb3"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	// Search must have been called once per dataset (3 calls), not collapsed to
	// the first KB.
	if got := len(fake.searchedTopics()); got != 3 {
		t.Errorf("Search called %d times, want 3 (once per dataset)", got)
	}
}

// TestCanvasDatasetIDs_DedupEmpty asserts empty ids are dropped.
func TestCanvasDatasetIDs_DedupEmpty(t *testing.T) {
	ids := canvasDatasetIDs(t.Context(), []string{"kb1", "", "kb2"})
	if len(ids) != 2 || ids[0] != "kb1" || ids[1] != "kb2" {
		t.Errorf("canvasDatasetIDs = %v, want [kb1 kb2]", ids)
	}
}

func containsStr(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && (s == sub || containsSub(s, sub))
}

func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
