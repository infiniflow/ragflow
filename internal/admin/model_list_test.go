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

package admin

import (
	"testing"

	"ragflow/internal/dao"
)

// TestListAllModelsTreatsOmittedPageAsFirst keeps the admin twin of
// GET /api/v1/models/all-models on the same paging contract: an omitted page
// (0) with a page size returns the first page, and a page past the end is an
// empty non-nil slice so JSON encodes [].
func TestListAllModelsTreatsOmittedPageAsFirst(t *testing.T) {
	all, err := dao.GetModelProviderManager().ListAllModels()
	if err != nil {
		t.Fatalf("list provider models: %v", err)
	}
	const pageSize = 10
	if len(all) <= pageSize {
		t.Fatalf("provider catalog returned %d models, need more than %d", len(all), pageSize)
	}
	svc := &Service{}

	first, err := svc.ListAllModels(0, pageSize)
	if err != nil {
		t.Fatalf("omitted page: %v", err)
	}
	if len(first) != pageSize || first[0]["name"] != all[0]["name"] {
		t.Fatalf("omitted page returned %d models starting at %v, want the first page of %d", len(first), first[0]["name"], pageSize)
	}

	lastPage := (len(all) + pageSize - 1) / pageSize
	beyond, err := svc.ListAllModels(lastPage+1, pageSize)
	if err != nil {
		t.Fatalf("page past the end: %v", err)
	}
	if beyond == nil || len(beyond) != 0 {
		t.Fatalf("page past the end = %v, want empty non-nil so JSON encodes []", beyond)
	}
}
