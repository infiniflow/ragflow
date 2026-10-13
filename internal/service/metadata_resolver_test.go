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

package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
)

type tenantMetadataEngine struct {
	fakeChatDocEngine
	requests []*types.SearchMetadataRequest
	fail     bool
}

func (e *tenantMetadataEngine) SearchMetadata(_ context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	e.requests = append(e.requests, req)
	if e.fail && req.TenantID == "tenant-b" {
		return nil, errors.New("metadata unavailable")
	}
	return &types.SearchMetadataResult{MetadataRecords: []map[string]interface{}{
		{"id": "doc-" + req.TenantID, "meta_fields": map[string]interface{}{"author": "Alice"}},
	}}, nil
}

func TestMetadataFallbackSearchesAllTenants(t *testing.T) {
	pushServiceDB(t, setupServiceTestDB(t))
	insertTestKB(t, "kb-a", "tenant-a", 0, 0, 0)
	insertTestKB(t, "kb-b", "tenant-a", 0, 0, 0)
	insertTestKB(t, "kb-c", "tenant-b", 0, 0, 0)
	eng := &tenantMetadataEngine{}
	svc := NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), eng)
	meta, err := svc.GetFlattedMetaByKBs(t.Context(), []string{"kb-a", "kb-c", "kb-b", "kb-a"})
	if err != nil {
		t.Fatal(err)
	}
	if got := meta["author"]["Alice"]; !reflect.DeepEqual(got, []string{"doc-tenant-a", "doc-tenant-b"}) {
		t.Fatalf("matching documents = %v, want both tenants", got)
	}
	if len(eng.requests) != 2 || eng.requests[0].TenantID != "tenant-a" || eng.requests[1].TenantID != "tenant-b" {
		t.Fatalf("metadata requests = %#v, want one per tenant", eng.requests)
	}
	if !reflect.DeepEqual(eng.requests[0].Filter["kb_id"], []string{"kb-a", "kb-b"}) ||
		!reflect.DeepEqual(eng.requests[1].Filter["kb_id"], []string{"kb-c"}) {
		t.Fatal("metadata queries were not scoped to each tenant's datasets")
	}
	eng.fail = true
	if got, err := svc.GetFlattedMetaByKBs(t.Context(), []string{"kb-a", "kb-c"}); err == nil || got != nil {
		t.Fatalf("partial metadata = %v, err = %v, want error without partial result", got, err)
	}
}

// pushdownDocEngine is the shared fake with a scripted push-down answer: the agentic
// metadata_search adapter is only interesting for what it does with the engine's
// nil / non-nil contract.
type pushdownDocEngine struct {
	fakeChatDocEngine
	ids      []string
	calls    int
	gotKbIDs []string
	gotConds []map[string]interface{}
	gotLogic string
}

func (e *pushdownDocEngine) FilterDocIdsByMetaPushdown(_ context.Context, _ *gorm.DB, kbIDs []string, conditions []map[string]interface{}, logic string) []string {
	e.calls++
	e.gotKbIDs = kbIDs
	e.gotConds = conditions
	e.gotLogic = logic
	return e.ids
}

// TestFilterDocIDsByMetaPushdownSurfacesTheEngineContract pins the nil / non-nil split:
// nil means "push-down not viable" and must reach the caller as ok=false (so it falls
// back to the in-memory filter), while a non-nil slice — empty included — is definitive.
func TestFilterDocIDsByMetaPushdownSurfacesTheEngineContract(t *testing.T) {
	filters := []map[string]any{{"key": "title", "op": "contains", "value": "New York"}}

	eng := &pushdownDocEngine{}
	svc := NewMetadataServiceForTest(nil, eng)
	if ids, ok := svc.FilterDocIDsByMetaPushdown(context.Background(), []string{"kb1"}, filters, "and"); ok || ids != nil {
		t.Errorf("ids=%v ok=%v, want the not-viable signal", ids, ok)
	}
	if eng.calls != 1 {
		t.Errorf("engine calls = %d, want 1", eng.calls)
	}
	if len(eng.gotKbIDs) != 1 || eng.gotKbIDs[0] != "kb1" {
		t.Errorf("engine kbIDs = %v", eng.gotKbIDs)
	}
	if len(eng.gotConds) != 1 || eng.gotConds[0]["key"] != "title" {
		t.Errorf("engine conditions = %v", eng.gotConds)
	}

	// An empty-but-definitive result must stay distinguishable from "not viable".
	emptySvc := NewMetadataServiceForTest(nil, &pushdownDocEngine{ids: []string{}})
	if ids, ok := emptySvc.FilterDocIDsByMetaPushdown(context.Background(), []string{"kb1"}, filters, "and"); !ok || len(ids) != 0 {
		t.Errorf("ids=%v ok=%v, want a definitive empty result", ids, ok)
	}

	// Hits are forwarded verbatim, with logic intact.
	hitEng := &pushdownDocEngine{ids: []string{"d1", "d2"}}
	hitSvc := NewMetadataServiceForTest(nil, hitEng)
	ids, ok := hitSvc.FilterDocIDsByMetaPushdown(context.Background(), []string{"kb1"}, filters, "or")
	if !ok || len(ids) != 2 || ids[0] != "d1" || ids[1] != "d2" {
		t.Errorf("ids=%v ok=%v, want the engine's hits", ids, ok)
	}
	if hitEng.gotLogic != "or" {
		t.Errorf("engine logic = %q, want or", hitEng.gotLogic)
	}
}

// TestFilterDocIDsByMetaPushdownGuards pins the guards: a missing engine, no datasets or
// no conditions answer "not viable" WITHOUT reaching the engine (the caller then falls
// back, or reports a miss when the fallback has nothing either).
func TestFilterDocIDsByMetaPushdownGuards(t *testing.T) {
	filters := []map[string]any{{"key": "title", "op": "contains", "value": "New York"}}
	ctx := context.Background()

	if ids, ok := NewMetadataServiceForTest(nil, nil).FilterDocIDsByMetaPushdown(ctx, []string{"kb1"}, filters, "and"); ok || ids != nil {
		t.Errorf("ids=%v ok=%v, want not-viable without an engine", ids, ok)
	}

	eng := &pushdownDocEngine{ids: []string{"d1"}}
	svc := NewMetadataServiceForTest(nil, eng)
	if _, ok := svc.FilterDocIDsByMetaPushdown(ctx, nil, filters, "and"); ok {
		t.Error("no datasets must report not-viable")
	}
	if _, ok := svc.FilterDocIDsByMetaPushdown(ctx, []string{"kb1"}, nil, "and"); ok {
		t.Error("no conditions must report not-viable")
	}
	if eng.calls != 0 {
		t.Errorf("engine calls = %d, want 0 for every guard", eng.calls)
	}
}
