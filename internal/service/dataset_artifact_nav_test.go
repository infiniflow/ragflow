package service

import (
	"context"
	"testing"

	"ragflow/internal/service/nav"
)

// fakeArtifactNav is an in-memory nav.NavService used to lock the limited
// DeleteNav/DeleteNavNode semantics (direct doc children only; no sub-cluster
// or cascade deletion).
type fakeArtifactNav struct {
	clusters    []nav.NavNode
	children    map[string][]nav.NavNode
	removedDocs []string
}

func (f *fakeArtifactNav) UpsertDoc(context.Context, nav.UpsertDocInput) error { return nil }
func (f *fakeArtifactNav) RemoveDoc(_ context.Context, _, _, docID string) error {
	f.removedDocs = append(f.removedDocs, docID)
	return nil
}
func (f *fakeArtifactNav) Search(context.Context, string, string, string, []float32, int) ([]nav.NavHit, error) {
	return nil, nil
}
func (f *fakeArtifactNav) ListClusters(context.Context, string, string, int, int) ([]nav.NavNode, int64, error) {
	return f.clusters, int64(len(f.clusters)), nil
}
func (f *fakeArtifactNav) ListChildren(_ context.Context, _, _, name string, _, _ int) ([]nav.NavNode, int64, error) {
	return f.children[name], int64(len(f.children[name])), nil
}

// TestDeleteNav_RemovesOnlyDirectDocChildren documents the limited semantic of
// the deprecated DeleteNav: it removes the nav_doc rows directly under root
// clusters, and does NOT touch sub-clusters (Python's full cascade is not
// implemented in the minimal loop).
func TestDeleteNav_RemovesOnlyDirectDocChildren(t *testing.T) {
	fake := &fakeArtifactNav{
		clusters: []nav.NavNode{{Name: "C1", Description: "root"}},
		children: map[string][]nav.NavNode{
			"C1":  {{Name: "Sub", Type: "cluster"}, {Name: "DocA", Type: "doc", DocID: "d1"}},
			"Sub": {{Name: "DocB", Type: "doc", DocID: "d2"}},
		},
	}
	prev := nav.GetNavService()
	nav.SetNavService(fake)
	defer func() { nav.SetNavService(prev) }()

	svc := NewDatasetArtifactService()
	n, err := svc.DeleteNav(t.Context(), "t1", "kb1")
	if err != nil {
		t.Fatalf("DeleteNav: %v", err)
	}
	// Only the direct doc child (d1) under the root cluster is removed; the
	// sub-cluster's doc (d2) is NOT reached (no subtree traversal).
	if n != 1 {
		t.Errorf("deleted = %d, want 1 (direct doc child only)", n)
	}
	if len(fake.removedDocs) != 1 || fake.removedDocs[0] != "d1" {
		t.Errorf("removed docs = %v, want [d1] only (no cascade into Sub)", fake.removedDocs)
	}
}

// TestDeleteNavNode_RemovesDirectChildren documents that DeleteNavNode drains
// only the immediate doc children of a named cluster.
func TestDeleteNavNode_RemovesDirectChildren(t *testing.T) {
	fake := &fakeArtifactNav{
		children: map[string][]nav.NavNode{
			"C1": {
				{Name: "Sub2", Type: "cluster"},
				{Name: "DocA", Type: "doc", DocID: "d1"},
				{Name: "DocB", Type: "doc", DocID: "d2"},
			},
		},
	}
	prev := nav.GetNavService()
	nav.SetNavService(fake)
	defer func() { nav.SetNavService(prev) }()

	svc := NewDatasetArtifactService()
	n, err := svc.DeleteNavNode(t.Context(), "t1", "kb1", "C1")
	if err != nil {
		t.Fatalf("DeleteNavNode: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted = %d, want 2 (direct doc children d1,d2)", n)
	}
	if len(fake.removedDocs) != 2 {
		t.Errorf("removed docs = %v, want [d1 d2]", fake.removedDocs)
	}
}
