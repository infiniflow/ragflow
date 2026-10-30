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
	"testing"

	"ragflow/internal/dao"
)

// TestListAllModelsPagesFromTheFirstModel pins the pagination contract of
// GET /api/v1/models/all-models (and its admin twin): `page` is 1-based, a
// partially filled last page is returned in full, and a page past the end is
// empty instead of panicking on an out-of-range slice.
func TestListAllModelsPagesFromTheFirstModel(t *testing.T) {
	all, err := dao.GetModelProviderManager().ListAllModels()
	if err != nil {
		t.Fatalf("list provider models: %v", err)
	}
	const pageSize = 10
	if len(all) <= 2*pageSize {
		t.Fatalf("provider catalog returned %d models, need more than %d to exercise paging", len(all), 2*pageSize)
	}
	svc := &ModelProviderService{}

	first, err := svc.ListAllModels(1, pageSize)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(first) != pageSize || first[0]["name"] != all[0]["name"] || first[pageSize-1]["name"] != all[pageSize-1]["name"] {
		t.Fatalf("page 1 returned %d models starting at %v, want %v..%v", len(first), first[0]["name"], all[0]["name"], all[pageSize-1]["name"])
	}

	lastPage := (len(all) + pageSize - 1) / pageSize
	last, err := svc.ListAllModels(lastPage, pageSize)
	if err != nil {
		t.Fatalf("last page: %v", err)
	}
	wantLast := len(all) - (lastPage-1)*pageSize
	if len(last) != wantLast || last[len(last)-1]["name"] != all[len(all)-1]["name"] {
		t.Fatalf("last page returned %d models, want %d ending at %v", len(last), wantLast, all[len(all)-1]["name"])
	}

	beyond, err := svc.ListAllModels(lastPage+1, pageSize)
	if err != nil {
		t.Fatalf("page past the end: %v", err)
	}
	if beyond == nil || len(beyond) != 0 {
		t.Fatalf("page past the end = %v, want empty non-nil so JSON encodes []", beyond)
	}

	// An omitted `page` reaches the service as 0; with a page size it must still
	// start at the first page instead of returning the whole catalog.
	omitted, err := svc.ListAllModels(0, pageSize)
	if err != nil {
		t.Fatalf("omitted page: %v", err)
	}
	if len(omitted) != pageSize || omitted[0]["name"] != all[0]["name"] {
		t.Fatalf("omitted page returned %d models starting at %v, want the first page of %d", len(omitted), omitted[0]["name"], pageSize)
	}
}
