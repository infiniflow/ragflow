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

package permission

import (
	"context"
	"errors"
	"testing"
)

type batchingChecker struct {
	filterCalls int
	checkCalls  int
	filterErr   error
	accessible  []ResourceRef
	denials     map[string]error
	checkedIDs  []string
}

func (c *batchingChecker) FilterResources(_ context.Context, refs []ResourceRef, _ Operation) ([]ResourceRef, error) {
	c.filterCalls++
	if c.filterErr != nil {
		return nil, c.filterErr
	}
	if c.accessible != nil {
		return append([]ResourceRef(nil), c.accessible...), nil
	}
	return append([]ResourceRef(nil), refs...), nil
}

func (c *batchingChecker) CheckResource(_ context.Context, ref ResourceRef, _ Operation) error {
	c.checkCalls++
	c.checkedIDs = append(c.checkedIDs, ref.ID)
	return c.denials[ref.ID]
}

var errSharedDenied = errors.New("resource denied")

func sharedTestRefs(ids ...string) []ResourceRef {
	refs := make([]ResourceRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, ResourceRef{Kind: ResourceKindDataset, ID: id})
	}
	return refs
}

// One batch load must answer every ref; no single-resource round-trips.
func TestCheckResourcesBatchesAllRefs(t *testing.T) {
	fake := &batchingChecker{}
	refs := sharedTestRefs("ds-a", "ds-b", "ds-c")
	if err := CheckResources(context.Background(), fake, refs, OperationUse); err != nil {
		t.Fatalf("CheckResources: %v", err)
	}
	if fake.filterCalls != 1 {
		t.Errorf("FilterResources calls = %d, want 1", fake.filterCalls)
	}
	if fake.checkCalls != 0 {
		t.Errorf("CheckResource calls = %d, want 0 when the batch answered every ref", fake.checkCalls)
	}
}

// A batch backend error must fall back to ordered single checks so the first
// error in request order survives.
func TestCheckResourcesBatchErrorKeepsFirstErrorOrder(t *testing.T) {
	fake := &batchingChecker{
		filterErr: errors.New("batch backend down"),
		denials:   map[string]error{"ds-b": errSharedDenied},
	}
	err := CheckResources(context.Background(), fake, sharedTestRefs("ds-a", "ds-b", "ds-c"), OperationUse)
	if !errors.Is(err, errSharedDenied) {
		t.Fatalf("error = %v, want %v", err, errSharedDenied)
	}
	if fake.filterCalls != 1 {
		t.Errorf("FilterResources calls = %d, want 1", fake.filterCalls)
	}
	if len(fake.checkedIDs) != 2 || fake.checkedIDs[0] != "ds-a" || fake.checkedIDs[1] != "ds-b" {
		t.Errorf("checked ids = %v, want [ds-a ds-b] in request order", fake.checkedIDs)
	}
}

// Omitted is not denied: a ref the batch skipped still gets its own check.
func TestCheckResourcesRechecksOmittedRef(t *testing.T) {
	fake := &batchingChecker{
		accessible: sharedTestRefs("ds-a"),
	}
	if err := CheckResources(context.Background(), fake, sharedTestRefs("ds-a", "ds-b"), OperationUse); err != nil {
		t.Fatalf("omitted ref should be re-checked, got error: %v", err)
	}
	if fake.checkCalls != 1 || len(fake.checkedIDs) != 1 || fake.checkedIDs[0] != "ds-b" {
		t.Errorf("re-checked ids = %v, want exactly [ds-b]", fake.checkedIDs)
	}
}

// An omitted ref that is still denied must surface its denial, and later
// omitted refs must still be examined if it turns out to be readable.
func TestCheckResourcesOmittedRefDenialWinsInOrder(t *testing.T) {
	fake := &batchingChecker{
		accessible: sharedTestRefs("ds-a", "ds-c"),
		denials:    map[string]error{"ds-b": errSharedDenied},
	}
	err := CheckResources(context.Background(), fake, sharedTestRefs("ds-a", "ds-b", "ds-c"), OperationUse)
	if !errors.Is(err, errSharedDenied) {
		t.Fatalf("error = %v, want %v", err, errSharedDenied)
	}
	if len(fake.checkedIDs) != 1 || fake.checkedIDs[0] != "ds-b" {
		t.Errorf("checked ids = %v, want exactly [ds-b]", fake.checkedIDs)
	}
}

func TestCheckResourcesEmptyRefs(t *testing.T) {
	fake := &batchingChecker{}
	if err := CheckResources(context.Background(), fake, nil, OperationUse); err != nil {
		t.Fatalf("CheckResources(nil): %v", err)
	}
	if fake.filterCalls != 0 || fake.checkCalls != 0 {
		t.Errorf("calls: filter=%d check=%d, want 0/0", fake.filterCalls, fake.checkCalls)
	}
}
