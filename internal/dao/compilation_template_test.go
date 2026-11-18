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

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCompilationTemplateGroupDAO_TenantScope(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&entity.CompilationTemplateGroup{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	status := string(entity.StatusValid)
	if err = db.Create([]*entity.CompilationTemplateGroup{
		{ID: "tenant-group", TenantID: "tenant-1", Name: "Tenant", Scope: "file", Status: &status},
		{ID: "global-group", TenantID: "", Name: "Built-in templates", Scope: "file", Status: &status},
	}).Error; err != nil {
		t.Fatalf("insert groups: %v", err)
	}

	dao := NewCompilationTemplateGroupDAO()
	groups, err := dao.ListSaved(t.Context(), db, "tenant-1", "", "", nil)
	if err != nil {
		t.Fatalf("ListSaved: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != "tenant-group" {
		t.Fatalf("ListSaved returned %#v, want only tenant-group", groups)
	}

	group, err := dao.GetSaved(t.Context(), db, "tenant-1", "global-group")
	if err != nil {
		t.Fatalf("GetSaved: %v", err)
	}
	if group != nil {
		t.Fatalf("GetSaved returned global group %#v", group)
	}
}

func TestCompilationTemplateDAO_TenantScope(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&entity.CompilationTemplate{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	status := string(entity.StatusValid)
	tenantID := "tenant-1"
	if err = db.Create([]*entity.CompilationTemplate{
		{ID: "tenant-template", TenantID: &tenantID, Name: "Tenant", Kind: "tree", Config: entity.JSONMap{"kind": "tree"}, Status: &status},
		{ID: "global-template", TenantID: nil, Name: "Built-in", Kind: "tree", Config: entity.JSONMap{"kind": "tree"}, IsBuiltin: true, Status: &status},
	}).Error; err != nil {
		t.Fatalf("insert templates: %v", err)
	}

	dao := NewCompilationTemplateDAO()
	if _, err = dao.GetTemplate(t.Context(), db, "tenant-1", "tenant-template"); err != nil {
		t.Fatalf("GetTemplate tenant row: %v", err)
	}
	if _, err = dao.GetTemplate(t.Context(), db, "tenant-1", "global-template"); err == nil {
		t.Fatal("GetTemplate resolved a non-persisted global builtin row")
	}
}
