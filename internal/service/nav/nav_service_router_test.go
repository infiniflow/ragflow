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

package nav

import (
	"context"
	"testing"
)

// scopeRecordingNav records the doc scope Search was called with and returns a
// controlled hit list, so a test can assert the router forwards the ceiling and
// routes only document leaves.
type scopeRecordingNav struct {
	scope []string
	hits  []NavHit
}

func (f *scopeRecordingNav) UpsertDoc(context.Context, UpsertDocInput) error { return nil }
func (f *scopeRecordingNav) RemoveDoc(context.Context, string, string, string) error {
	return nil
}
func (f *scopeRecordingNav) Search(_ context.Context, _, _ string, _ string, _ []float32, docScope []string, _ int) ([]NavHit, error) {
	f.scope = append([]string(nil), docScope...)
	return f.hits, nil
}
func (f *scopeRecordingNav) ListClusters(context.Context, string, string, int, int) ([]NavNode, int64, error) {
	return nil, 0, nil
}
func (f *scopeRecordingNav) ListChildren(context.Context, string, string, string, int, int) ([]NavNode, int64, error) {
	return nil, 0, nil
}
func (f *scopeRecordingNav) SummariesByDocIDs(context.Context, string, string, []string) map[string]string {
	return nil
}

// TestNavServiceRouter_ForwardsDocScopeAndRoutesLeavesOnly pins the two halves of
// the nav-row routing contract:
//
//  1. The doc scope must reach the service (Python search_dataset_layers threads
//     doc_scope into the "navigation_tree" mode, dataset_api_service.py:4107-4119,
//     and _nav_search_titled ceilings it with the session scope on the way in).
//  2. Only document leaves route: that strategy pins type_kwd="nav_doc", so a
//     nav_cluster hit — whose doc_id is the kb_id, not a document — must never be
//     emitted as a routed document.
func TestNavServiceRouter_ForwardsDocScopeAndRoutesLeavesOnly(t *testing.T) {
	fake := &scopeRecordingNav{hits: []NavHit{
		{Type: TypeNavCluster, DocID: "kb1", DocIDs: []string{"d1"}, Name: "cluster covering d1"},
		{Type: TypeNavDoc, DocID: "d1", Name: "rocket"},
	}}
	prev := GetNavService()
	SetNavService(fake)
	defer SetNavService(prev)

	routed, err := NewNavServiceRouter().Route(t.Context(), "t1", "kb1", "rocket propulsion", []string{"d1"}, 5)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(fake.scope) != 1 || fake.scope[0] != "d1" {
		t.Errorf("Search doc scope = %v, want [d1] (the router must not drop the ceiling)", fake.scope)
	}
	if len(routed) != 1 || routed[0][0] != "d1" || routed[0][1] != "rocket" {
		t.Fatalf("routed = %v, want exactly [[d1 rocket]] (cluster rows must not route)", routed)
	}
}

// TestNavServiceRouter_NilServiceMeansNoTree keeps the "no compiled tree" signal
// distinct from "routed to nothing": a missing service returns nil, not an empty
// non-nil slice (which the caller reads as "structure exists, query missed").
func TestNavServiceRouter_NilServiceMeansNoTree(t *testing.T) {
	prev := GetNavService()
	SetNavService(nil)
	defer SetNavService(prev)

	routed, err := NewNavServiceRouter().Route(t.Context(), "t1", "kb1", "q", nil, 5)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if routed != nil {
		t.Fatalf("routed = %v, want nil (no compiled tree)", routed)
	}
}
