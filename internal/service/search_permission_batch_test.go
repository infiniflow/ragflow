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
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

type countingDatasetChecker struct {
	filterCalls int
	checkCalls  int
	filterErr   error
	filterRefs  []permission.ResourceRef
	accessible  []permission.ResourceRef
	denials     map[string]error
	checkedIDs  []string
}

func (c *countingDatasetChecker) FilterResources(_ context.Context, refs []permission.ResourceRef, _ permission.Operation) ([]permission.ResourceRef, error) {
	c.filterCalls++
	c.filterRefs = append([]permission.ResourceRef(nil), refs...)
	if c.filterErr != nil {
		return nil, c.filterErr
	}
	if c.accessible != nil {
		return append([]permission.ResourceRef(nil), c.accessible...), nil
	}
	return append([]permission.ResourceRef(nil), refs...), nil
}

func (c *countingDatasetChecker) CheckResource(_ context.Context, ref permission.ResourceRef, _ permission.Operation) error {
	c.checkCalls++
	c.checkedIDs = append(c.checkedIDs, ref.ID)
	return c.denials[ref.ID]
}

var errTestDatasetDenied = errors.New("dataset denied")

// N datasets must cost one batch load, not N single-resource round-trips.
func TestCheckDatasetsUsableBatchesAllChecks(t *testing.T) {
	fake := &countingDatasetChecker{}
	ids := []string{"ds-1", "ds-2", "ds-3"}
	if err := checkDatasetsUsable(context.Background(), fake, ids); err != nil {
		t.Fatalf("checkDatasetsUsable: %v", err)
	}
	if fake.filterCalls != 1 {
		t.Errorf("FilterResources calls = %d, want 1", fake.filterCalls)
	}
	if fake.checkCalls != 0 {
		t.Errorf("CheckResource calls = %d, want 0 when the batch answered every dataset", fake.checkCalls)
	}
}

// An omitted dataset must be re-checked individually and its denial returned
// in the original request order, never masked by a later dataset.
func TestCheckDatasetsUsableDeniedDatasetReturnsFirstInOrder(t *testing.T) {
	fake := &countingDatasetChecker{
		accessible: []permission.ResourceRef{
			{Kind: permission.ResourceKindDataset, ID: "ds-1"},
			{Kind: permission.ResourceKindDataset, ID: "ds-3"},
		},
		denials: map[string]error{"ds-2": errTestDatasetDenied},
	}
	ids := []string{"ds-1", "ds-2", "ds-3"}
	if err := checkDatasetsUsable(context.Background(), fake, ids); !errors.Is(err, errTestDatasetDenied) {
		t.Fatalf("error = %v, want %v", err, errTestDatasetDenied)
	}
	if fake.checkCalls != 1 {
		t.Errorf("CheckResource calls = %d, want 1 (only the omitted ds-2)", fake.checkCalls)
	}
	if len(fake.checkedIDs) != 1 || fake.checkedIDs[0] != "ds-2" {
		t.Errorf("re-checked ids = %v, want [ds-2]", fake.checkedIDs)
	}
}

// Omission is not denial: a batch that legitimately omits a dataset still has
// to give that dataset a real single check before concluding it is usable.
func TestCheckDatasetsUsableRechecksOmittedDataset(t *testing.T) {
	fake := &countingDatasetChecker{
		accessible: []permission.ResourceRef{{Kind: permission.ResourceKindDataset, ID: "ds-1"}},
	}
	if err := checkDatasetsUsable(context.Background(), fake, []string{"ds-1", "ds-2"}); err != nil {
		t.Fatalf("omitted dataset should be re-checked, got error: %v", err)
	}
	if fake.checkCalls != 1 || len(fake.checkedIDs) != 1 || fake.checkedIDs[0] != "ds-2" {
		t.Errorf("re-checked ids = %v, want exactly [ds-2]", fake.checkedIDs)
	}
}

// A batch backend error must fall back to ordered single checks so the first
// error in request order survives (the pre-change error-code contract).
func TestCheckDatasetsUsableFallsBackToOrderedChecksOnBatchError(t *testing.T) {
	fake := &countingDatasetChecker{
		filterErr: errors.New("batch backend down"),
		denials:   map[string]error{"ds-2": errTestDatasetDenied},
	}
	ids := []string{"ds-1", "ds-2", "ds-3"}
	if err := checkDatasetsUsable(context.Background(), fake, ids); !errors.Is(err, errTestDatasetDenied) {
		t.Fatalf("error = %v, want %v", err, errTestDatasetDenied)
	}
	if fake.filterCalls != 1 {
		t.Errorf("FilterResources calls = %d, want 1", fake.filterCalls)
	}
	if fake.checkCalls != 2 {
		t.Errorf("CheckResource calls = %d, want 2 (stop at the first denial)", fake.checkCalls)
	}
	if len(fake.checkedIDs) != 2 || fake.checkedIDs[0] != "ds-1" || fake.checkedIDs[1] != "ds-2" {
		t.Errorf("checked ids = %v, want [ds-1 ds-2] in request order", fake.checkedIDs)
	}
}

// Duplicate ids collapse to one ref, but an empty id stays: the old loop
// checked it and let the permission layer reject it, which must not change.
func TestCheckDatasetsUsableDedupsRefsWithoutDroppingEmptyID(t *testing.T) {
	fake := &countingDatasetChecker{}
	ids := []string{"", "ds-1", "ds-1", "ds-2"}
	if err := checkDatasetsUsable(context.Background(), fake, ids); err != nil {
		t.Fatalf("checkDatasetsUsable: %v", err)
	}
	if fake.filterCalls != 1 || fake.checkCalls != 0 {
		t.Fatalf("calls: filter=%d check=%d, want 1/0", fake.filterCalls, fake.checkCalls)
	}
	if len(fake.filterRefs) != 3 {
		t.Fatalf("batch refs = %d, want 3 (empty, ds-1, ds-2)", len(fake.filterRefs))
	}
	if fake.filterRefs[0].ID != "" || fake.filterRefs[1].ID != "ds-1" || fake.filterRefs[2].ID != "ds-2" {
		t.Errorf("batch ref ids = %q, want [\"\" ds-1 ds-2] in first-seen order", []string{
			fake.filterRefs[0].ID, fake.filterRefs[1].ID, fake.filterRefs[2].ID,
		})
	}
}

// The wiring matters as much as the helper: a future edit must not slip back
// to a per-dataset CheckResource loop inside PrepareCompletion unnoticed.
func TestPrepareCompletionUsesBatchDatasetAuthorization(t *testing.T) {
	setupSearchServiceTestDB(t)
	ctx := serviceTestContext(t, "user-1", "team-1")
	status := string(entity.StatusValid)
	if err := dao.DB.Create(&entity.UserTenant{
		ID: "team-owner", UserID: "team-1", TenantID: "team-1", Role: string(permission.RoleOwner), Status: &status,
	}).Error; err != nil {
		t.Fatalf("create tenant owner membership: %v", err)
	}
	for _, id := range []string{"kb-a", "kb-b", "kb-c"} {
		if err := dao.DB.Create(&entity.Knowledgebase{
			ID: id, TenantID: "team-1", CreatedBy: "user-1", Name: id,
			Permission: "team", Status: &status,
		}).Error; err != nil {
			t.Fatalf("create dataset %s: %v", id, err)
		}
	}
	if err := dao.DB.Create(&entity.Search{
		ID: "search-batch", TenantID: "team-1", CreatedBy: "user-1", Name: "search",
		SearchConfig: entity.JSONMap{"kb_ids": []interface{}{"kb-a", "kb-b", "kb-c"}},
		Status:       &status,
	}).Error; err != nil {
		t.Fatalf("create search: %v", err)
	}

	fake := &countingDatasetChecker{}
	original := newDatasetUseChecker
	newDatasetUseChecker = func(*gorm.DB) permission.ResourceChecker { return fake }
	t.Cleanup(func() { newDatasetUseChecker = original })

	plan, code, err := NewSearchService().PrepareCompletion(ctx, "search-batch", &SearchCompletionsRequest{Question: "question"})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("PrepareCompletion() code = %v, err = %v", code, err)
	}
	if len(plan.DatasetIDs) != 3 {
		t.Fatalf("DatasetIDs = %v, want 3 datasets", plan.DatasetIDs)
	}
	if fake.filterCalls != 1 {
		t.Errorf("FilterResources calls = %d, want 1", fake.filterCalls)
	}
	if fake.checkCalls != 0 {
		t.Errorf("CheckResource calls = %d, want 0 when the batch answered all datasets", fake.checkCalls)
	}
}

func BenchmarkCheckDatasetsUsableBatch(b *testing.B) {
	ids := []string{"ds-1", "ds-2", "ds-3", "ds-4", "ds-5"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := checkDatasetsUsable(context.Background(), &countingDatasetChecker{}, ids); err != nil {
			b.Fatal(err)
		}
	}
}
