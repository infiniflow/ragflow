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

package dao

import (
	"context"
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func newModelRefTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&entity.Knowledgebase{}, &entity.DatasetModelRef{}); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}
	return db
}

// A template inside a group is reachable two ways: a dataset may name the
// template directly, or name the group that contains it. Both are live
// bindings, and resolving only the group is what makes a model in use answer
// "no dataset is affected". A dataset that names both must also be counted once.
func TestDatasetsByModelRefsUnionsBothReferences(t *testing.T) {
	db := newModelRefTestDB(t)
	ctx := context.Background()

	status := func(value entity.Status) *string {
		text := string(value)
		return &text
	}
	datasets := []*entity.Knowledgebase{
		{ID: "kb-group", TenantID: "t1", Name: "By group", Status: status(entity.StatusValid), DocNum: 2},
		{ID: "kb-template", TenantID: "t1", Name: "By template", Status: status(entity.StatusValid), DocNum: 1},
		{ID: "kb-both", TenantID: "t1", Name: "By both", Status: status(entity.StatusValid), DocNum: 3},
		{ID: "kb-elsewhere", TenantID: "t1", Name: "Other model", Status: status(entity.StatusValid), DocNum: 1},
		{ID: "kb-gone", TenantID: "t1", Name: "Deleted", Status: status(entity.StatusInvalid), DocNum: 1},
		{ID: "kb-other-tenant", TenantID: "t2", Name: "Other tenant", Status: status(entity.StatusValid), DocNum: 1},
	}
	if err := db.Create(&datasets).Error; err != nil {
		t.Fatalf("seed datasets: %v", err)
	}

	refRows := []*entity.DatasetModelRef{
		{DatasetID: "kb-group", TenantID: "t1", RefType: common.ModelRefGroup, ModelRef: "grp1"},
		{DatasetID: "kb-template", TenantID: "t1", RefType: common.ModelRefTemplate, ModelRef: "tpl1"},
		{DatasetID: "kb-both", TenantID: "t1", RefType: common.ModelRefGroup, ModelRef: "grp1"},
		{DatasetID: "kb-both", TenantID: "t1", RefType: common.ModelRefTemplate, ModelRef: "tpl1"},
		{DatasetID: "kb-elsewhere", TenantID: "t1", RefType: common.ModelRefTemplate, ModelRef: "tpl-other"},
		{DatasetID: "kb-gone", TenantID: "t1", RefType: common.ModelRefTemplate, ModelRef: "tpl1"},
		{DatasetID: "kb-other-tenant", TenantID: "t2", RefType: common.ModelRefTemplate, ModelRef: "tpl1"},
	}
	for i := range refRows {
		refRows[i].ID = fmt.Sprintf("ref-%02d", i)
	}
	if err := db.Create(&refRows).Error; err != nil {
		t.Fatalf("seed references: %v", err)
	}

	refDAO := NewDatasetModelRefDAO()
	refs := []common.ModelRef{
		{Kind: common.ModelRefGroup, ID: "grp1"},
		{Kind: common.ModelRefTemplate, ID: "tpl1"},
	}

	got, err := refDAO.DatasetsByModelRefs(ctx, db, "t1", refs)
	if err != nil {
		t.Fatalf("DatasetsByModelRefs: %v", err)
	}
	wantIDs := []string{"kb-both", "kb-group", "kb-template"}
	if len(got) != len(wantIDs) {
		t.Fatalf("datasets = %+v, want %v", got, wantIDs)
	}
	for i, want := range wantIDs {
		if got[i].ID != want {
			t.Fatalf("dataset %d = %q, want %q (whole result: %+v)", i, got[i].ID, want, got)
		}
	}
	if got[0].Name != "By both" || got[0].DocNum != 3 {
		t.Errorf("row = %+v, want the dataset's own name and doc count", got[0])
	}

	count, err := refDAO.CountByModelRefs(ctx, db, "t1", refs)
	if err != nil {
		t.Fatalf("CountByModelRefs: %v", err)
	}
	if count != int64(len(wantIDs)) {
		t.Fatalf("count = %d, want %d (a dataset naming both references counts once)", count, len(wantIDs))
	}

	// One reference still answers on its own — the single-reference path the
	// impact drill-down used before must keep working.
	direct, err := refDAO.DatasetsByModel(ctx, db, "t1", common.ModelRef{Kind: common.ModelRefTemplate, ID: "tpl1"})
	if err != nil {
		t.Fatalf("DatasetsByModel: %v", err)
	}
	if len(direct) != 2 || direct[0].ID != "kb-both" || direct[1].ID != "kb-template" {
		t.Fatalf("single-reference datasets = %+v, want [kb-both kb-template]", direct)
	}

	// No references means nothing to look up, not everything.
	empty, err := refDAO.DatasetsByModelRefs(ctx, db, "t1", nil)
	if err != nil {
		t.Fatalf("DatasetsByModelRefs(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("nil references returned %+v", empty)
	}
	if zero, err := refDAO.CountByModelRefs(ctx, db, "t1", nil); err != nil || zero != 0 {
		t.Fatalf("CountByModelRefs(nil) = %d, %v; want 0, nil", zero, err)
	}
}
