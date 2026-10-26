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

func setupTenantDAOTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Tenant{}); err != nil {
		t.Fatalf("failed to migrate tenant: %v", err)
	}
	return db
}

func useTenantDAOTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })
}

// TestTenantDAOGetByIDSkipsInactiveTenant pins the behaviour GetByID relies on:
// it filters status = "1", so a tenant row that is still present but inactive is
// reported as not found. This is the part of the old soft-delete test that
// covered live code; it no longer needs a delete helper to set up.
func TestTenantDAOGetByIDSkipsInactiveTenant(t *testing.T) {
	db := setupTenantDAOTestDB(t)
	useTenantDAOTestDB(t, db)
	ctx := t.Context()

	seed := func(id, status string) {
		t.Helper()
		tenant := &entity.Tenant{
			ID:        id,
			ParserIDs: "naive",
			Status:    &status,
		}
		if err := NewTenantDAO().Create(ctx, db, tenant); err != nil {
			t.Fatalf("Create(%s) error = %v", id, err)
		}
	}
	seed("tenant-active", "1")
	seed("tenant-inactive", "0")

	got, err := NewTenantDAO().GetByID(ctx, db, "tenant-active")
	if err != nil {
		t.Fatalf("GetByID(active) error = %v", err)
	}
	if got.ID != "tenant-active" {
		t.Fatalf("GetByID(active) = %+v", got)
	}

	if _, err := NewTenantDAO().GetByID(ctx, db, "tenant-inactive"); err == nil {
		t.Fatal("GetByID(inactive) error = nil, want not found")
	}
}

func TestTenantDAOUpdateStatus(t *testing.T) {
	db := setupTenantDAOTestDB(t)
	useTenantDAOTestDB(t, db)
	ctx := t.Context()

	active := "1"
	tenant := &entity.Tenant{
		ID:        "tenant-update",
		ParserIDs: "naive",
		Status:    &active,
	}
	if err := NewTenantDAO().Create(ctx, db, tenant); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := NewTenantDAO().Update(ctx, db, tenant.ID, map[string]interface{}{"status": "0"}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	var got entity.Tenant
	if err := db.WithContext(ctx).Where("id = ?", tenant.ID).First(&got).Error; err != nil {
		t.Fatalf("failed to reload tenant: %v", err)
	}
	if got.Status == nil || *got.Status != "0" {
		t.Fatalf("status = %v, want 0", got.Status)
	}
}
