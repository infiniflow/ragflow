//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package dao

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ragflow/internal/entity"
)

func TestMigrateTenantModelProviderUserUniqueIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err = db.Exec(`CREATE TABLE tenant_model_provider (
		id varchar(32) PRIMARY KEY,
		user_id varchar(32),
		provider_name varchar(128) NOT NULL,
		tenant_id varchar(32) NOT NULL
	)`).Error; err != nil {
		t.Fatalf("create legacy provider table: %v", err)
	}
	if err = db.Exec("CREATE UNIQUE INDEX idx_tenant_provider_unique ON tenant_model_provider (provider_name, tenant_id)").Error; err != nil {
		t.Fatalf("create legacy provider index: %v", err)
	}
	if err = db.Exec(`INSERT INTO tenant_model_provider (id, user_id, provider_name, tenant_id)
		VALUES ('provider-1', 'user-1', 'OpenAI', 'tenant-1')`).Error; err != nil {
		t.Fatalf("create existing provider: %v", err)
	}
	if err = db.AutoMigrate(&entity.TenantModelProvider{}); err != nil {
		t.Fatalf("auto-migrate provider table: %v", err)
	}

	ctx := context.Background()
	if err = migrateTenantModelProviderUserUniqueIndex(ctx, db); err != nil {
		t.Fatalf("migrate provider index: %v", err)
	}
	if err = migrateTenantModelProviderUserUniqueIndex(ctx, db); err != nil {
		t.Fatalf("rerun provider index migration: %v", err)
	}

	indexes, err := db.Migrator().GetIndexes(&entity.TenantModelProvider{})
	if err != nil {
		t.Fatalf("inspect provider indexes: %v", err)
	}
	var hasUserScopedIndex, hasLegacyIndex bool
	for _, index := range indexes {
		if index.Name() == tenantModelProviderUserUniqueIndex {
			unique, known := index.Unique()
			hasUserScopedIndex = known && unique && sameIndexColumns(index.Columns(), []string{"provider_name", "tenant_id", "user_id"})
		}
		if sameIndexColumns(index.Columns(), []string{"provider_name", "tenant_id"}) {
			unique, known := index.Unique()
			hasLegacyIndex = known && unique
		}
	}
	if !hasUserScopedIndex {
		t.Fatal("user-scoped unique provider index was not created")
	}
	if hasLegacyIndex {
		t.Fatal("legacy unique provider index was not removed")
	}

	if err = db.Create(&entity.TenantModelProvider{ID: "provider-2", UserID: "user-2", ProviderName: "OpenAI", TenantID: "tenant-1"}).Error; err != nil {
		t.Fatalf("same provider name for another user in the tenant was rejected: %v", err)
	}
	if err = db.Create(&entity.TenantModelProvider{ID: "provider-3", UserID: "user-1", ProviderName: "OpenAI", TenantID: "tenant-1"}).Error; err == nil {
		t.Fatal("duplicate provider name for the same user and tenant was accepted")
	}
}
