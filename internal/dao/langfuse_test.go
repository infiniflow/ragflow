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

// setupLangfuseTestDB initializes an in-memory SQLite database for Langfuse DAO tests.
func setupLangfuseTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.TenantLangfuse{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestLangfuseDAO_GetByTenantID_NotFound(t *testing.T) {
	db := setupLangfuseTestDB(t)
	pushDB(t, db)
	dao := NewLangfuse()

	row, err := dao.GetByTenantID("missing")
	if err != nil {
		t.Fatalf("expected nil error for missing row, got %v", err)
	}
	if row != nil {
		t.Fatalf("expected nil row for missing tenant, got %+v", row)
	}
}

func TestLangfuseDAO_CRUD(t *testing.T) {
	db := setupLangfuseTestDB(t)
	pushDB(t, db)
	dao := NewLangfuse()

	// 1. Create
	row := &entity.TenantLangfuse{
		TenantID:  "tenant-1",
		SecretKey: "sk-1",
		PublicKey: "pk-1",
		Host:      "https://cloud.langfuse.com",
	}
	if err := dao.Create(row); err != nil {
		t.Fatalf("failed to create: %v", err)
	}

	// 2. GetByTenantID
	got, err := dao.GetByTenantID("tenant-1")
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}
	if got == nil {
		t.Fatalf("expected a row, got nil")
	}
	if got.SecretKey != "sk-1" || got.PublicKey != "pk-1" || got.Host != "https://cloud.langfuse.com" {
		t.Fatalf("unexpected row: %+v", got)
	}
	// BeforeCreate hook should have populated timestamps.
	if got.CreateTime == nil || got.UpdateTime == nil {
		t.Fatalf("expected timestamps to be populated, got %+v", got)
	}

	// 3. UpdateByTenantID
	updates := map[string]any{
		"secret_key": "sk-2",
		"public_key": "pk-2",
		"host":       "https://eu.langfuse.com",
	}
	if err := dao.UpdateByTenantID("tenant-1", updates); err != nil {
		t.Fatalf("failed to update: %v", err)
	}
	got, err = dao.GetByTenantID("tenant-1")
	if err != nil {
		t.Fatalf("failed to get after update: %v", err)
	}
	if got.SecretKey != "sk-2" || got.PublicKey != "pk-2" || got.Host != "https://eu.langfuse.com" {
		t.Fatalf("update not applied: %+v", got)
	}

	// 4. DeleteByTenantID
	if err := dao.DeleteByTenantID("tenant-1"); err != nil {
		t.Fatalf("failed to delete: %v", err)
	}
	got, err = dao.GetByTenantID("tenant-1")
	if err != nil {
		t.Fatalf("expected nil error after delete, got %v", err)
	}
	if got != nil {
		t.Fatalf("expected row to be deleted, got %+v", got)
	}
}
