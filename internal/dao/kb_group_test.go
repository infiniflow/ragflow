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
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func TestGroupKBIDsByTenantSkipsMissingDatasets(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatalf("migrate datasets: %v", err)
	}
	valid := string(entity.StatusValid)
	invalid := string(entity.StatusInvalid)
	rows := []*entity.Knowledgebase{
		{ID: "kb-a", UserID: "u1", TenantID: "tenant-a", Name: "a", EmbdID: "e", Permission: "team", CreatedBy: "u1", Status: &valid},
		{ID: "kb-b", UserID: "u2", TenantID: "tenant-b", Name: "b", EmbdID: "e", Permission: "team", CreatedBy: "u2", Status: &valid},
		{ID: "kb-gone", UserID: "u1", TenantID: "tenant-a", Name: "gone", EmbdID: "e", Permission: "team", CreatedBy: "u1", Status: &invalid},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed datasets: %v", err)
	}

	groups, err := GroupKBIDsByTenant(t.Context(), db, []string{"kb-a", "kb-b", "kb-missing", "kb-gone"})
	if err != nil {
		t.Fatalf("a missing dataset must not fail the whole scope: %v", err)
	}
	if len(groups) != 2 || groups[0].TenantID != "tenant-a" || groups[1].TenantID != "tenant-b" {
		t.Fatalf("groups = %+v, want tenant-a then tenant-b", groups)
	}
	if len(groups[0].KBIDs) != 1 || groups[0].KBIDs[0] != "kb-a" {
		t.Fatalf("tenant-a datasets = %v, want [kb-a]", groups[0].KBIDs)
	}

	groups, err = GroupKBIDsByTenant(t.Context(), db, []string{"kb-missing"})
	if err != nil || len(groups) != 0 {
		t.Fatalf("an all-missing scope = %+v, %v; want no groups and no error", groups, err)
	}
}
