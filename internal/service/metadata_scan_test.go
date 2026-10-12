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
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
)

// metadataPagesDocEngine serves, for each tenant, a metadata index of n rows
// sorted on id, and records every request, so the scan's paging is observable.
type metadataPagesDocEngine struct {
	fakeChatDocEngine
	n int
	// noTotal makes the engine report no total, as a store that cannot count.
	noTotal bool
	reqs    []types.SearchMetadataRequest
}

func (e *metadataPagesDocEngine) SearchMetadata(_ context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	e.reqs = append(e.reqs, *req)
	var rows []map[string]interface{}
	for i := req.Offset; i < e.n && i < req.Offset+req.Limit; i++ {
		rows = append(rows, map[string]interface{}{"id": fmt.Sprintf("%s-doc-%05d", req.TenantID, i)})
	}
	total := int64(e.n)
	if e.noTotal {
		total = 0
	}
	return &types.SearchMetadataResult{MetadataRecords: rows, Total: total}, nil
}

// TestSearchMetadataByKBsReadsPastTheResultWindow pins that the scan returns
// every document of the datasets, in each tenant's store: a single capped read
// stopped at 10,000, so a larger dataset lost documents from metadata filters
// and counts.
func TestSearchMetadataByKBsReadsPastTheResultWindow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, kb := range []entity.Knowledgebase{
		{ID: "kb1", TenantID: "tenant1", Name: "kb1", Status: new(string(entity.StatusValid))},
		{ID: "kb2", TenantID: "tenant2", Name: "kb2", Status: new(string(entity.StatusValid))},
	} {
		if err := db.Create(&kb).Error; err != nil {
			t.Fatalf("create kb: %v", err)
		}
	}
	prev := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = prev })

	const docs = 12345
	eng := &metadataPagesDocEngine{n: docs}
	res, err := NewMetadataServiceForTest(nil, eng).SearchMetadataByKBs(t.Context(), []string{"kb1", "kb2"})
	if err != nil {
		t.Fatalf("SearchMetadataByKBs: %v", err)
	}
	seen := make(map[string]bool, 2*docs)
	for _, r := range res.MetadataRecords {
		seen[r["id"].(string)] = true
	}
	if len(res.MetadataRecords) != 2*docs || len(seen) != 2*docs {
		t.Fatalf("read %d rows (%d distinct), want all %d of both tenants", len(res.MetadataRecords), len(seen), 2*docs)
	}
	// Per tenant: the first page, then the rest in one request sized by the
	// total, so an engine that walks to the offset does it once.
	want := []string{"tenant1:0:5000", "tenant1:5000:7345", "tenant2:0:5000", "tenant2:5000:7345"}
	var got []string
	for _, req := range eng.reqs {
		got = append(got, fmt.Sprintf("%s:%d:%d", req.TenantID, req.Offset, req.Limit))
		if req.OrderBy == nil || len(req.OrderBy.Fields) != 1 || req.OrderBy.Fields[0].Field != "id" {
			t.Fatalf("request is not sorted on id: %+v", req.OrderBy)
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

// TestScanMetadataWithoutTotal pins the fallback for a store that reports no
// total: the scan reads page by page until a short page.
func TestScanMetadataWithoutTotal(t *testing.T) {
	eng := &metadataPagesDocEngine{n: 12345, noTotal: true}
	records, err := NewMetadataServiceForTest(nil, eng).scanMetadata(t.Context(), "tenant1", map[string]interface{}{"kb_id": []string{"kb1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 12345 || len(eng.reqs) != 3 {
		t.Fatalf("read %d rows in %d requests, want 12345 in 3 pages", len(records), len(eng.reqs))
	}
}
