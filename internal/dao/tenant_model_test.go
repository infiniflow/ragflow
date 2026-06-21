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

func setupTenantModelDAOTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.TenantModel{}); err != nil {
		t.Fatalf("failed to migrate tenant_model: %v", err)
	}
	return db
}

func useTenantModelDAOTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })
}

func seedTenantModel(t *testing.T, db *gorm.DB, model *entity.TenantModel) {
	t.Helper()
	if err := db.Create(model).Error; err != nil {
		t.Fatalf("failed to seed tenant model: %v", err)
	}
}

func TestTenantModelDAODeleteByModelIDAndScopeDeletesOnlyMatchingModel(t *testing.T) {
	db := setupTenantModelDAOTestDB(t)
	useTenantModelDAOTestDB(t, db)

	seedTenantModel(t, db, &entity.TenantModel{ID: "model-delete", ModelName: "m", ModelType: "chat", ProviderID: "provider-1", InstanceID: "instance-1", Status: "active"})
	seedTenantModel(t, db, &entity.TenantModel{ID: "model-keep", ModelName: "m", ModelType: "chat", ProviderID: "provider-1", InstanceID: "instance-2", Status: "active"})

	rows, err := NewTenantModelDAO().DeleteByModelIDAndProviderIDAndInstanceID("model-delete", "provider-1", "instance-1")
	if err != nil {
		t.Fatalf("DeleteByModelIDAndProviderIDAndInstanceID() error = %v", err)
	}
	if rows != 1 {
		t.Fatalf("rows = %d, want 1", rows)
	}

	var count int64
	if err := db.Model(&entity.TenantModel{}).Where("id = ?", "model-delete").Count(&count).Error; err != nil {
		t.Fatalf("count deleted model: %v", err)
	}
	if count != 0 {
		t.Fatalf("deleted model count = %d, want 0", count)
	}
	if err := db.Model(&entity.TenantModel{}).Where("id = ?", "model-keep").Count(&count).Error; err != nil {
		t.Fatalf("count kept model: %v", err)
	}
	if count != 1 {
		t.Fatalf("kept model count = %d, want 1", count)
	}
}

func TestTenantModelDAOUpdateStatusByIDAndScope(t *testing.T) {
	db := setupTenantModelDAOTestDB(t)
	useTenantModelDAOTestDB(t, db)

	seedTenantModel(t, db, &entity.TenantModel{ID: "model-status", ModelName: "m", ModelType: "chat", ProviderID: "provider-1", InstanceID: "instance-1", Status: "active"})

	rows, err := NewTenantModelDAO().UpdateStatusByIDAndScope("model-status", "provider-1", "instance-1", "inactive")
	if err != nil {
		t.Fatalf("UpdateStatusByIDAndScope() error = %v", err)
	}
	if rows != 1 {
		t.Fatalf("rows = %d, want 1", rows)
	}

	var got entity.TenantModel
	if err := db.Where("id = ?", "model-status").First(&got).Error; err != nil {
		t.Fatalf("failed to reload model: %v", err)
	}
	if got.Status != "inactive" {
		t.Fatalf("status = %q, want inactive", got.Status)
	}

	rows, err = NewTenantModelDAO().UpdateStatusByIDAndScope("model-status", "provider-1", "wrong-instance", "active")
	if err != nil {
		t.Fatalf("UpdateStatusByIDAndScope() wrong scope error = %v", err)
	}
	if rows != 0 {
		t.Fatalf("wrong-scope rows = %d, want 0", rows)
	}
}
