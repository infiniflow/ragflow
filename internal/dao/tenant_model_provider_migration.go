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
	"fmt"
	"strings"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

const tenantModelProviderUserUniqueIndex = "idx_tenant_provider_user_unique"

// migrateTenantModelProviderUserUniqueIndex scopes provider names to the
// owning user and tenant. It removes the former (provider_name, tenant_id)
// constraint after the replacement index exists.
func migrateTenantModelProviderUserUniqueIndex(ctx context.Context, db *gorm.DB) error {
	scoped := db.WithContext(ctx)
	model := &entity.TenantModelProvider{}
	migrator := scoped.Migrator()
	if !migrator.HasTable(model) {
		return nil
	}
	if !migrator.HasColumn(model, "UserID") {
		return fmt.Errorf("tenant_model_provider.user_id is missing")
	}

	indexes, err := migrator.GetIndexes(model)
	if err != nil {
		return fmt.Errorf("inspect tenant_model_provider indexes: %w", err)
	}
	var hasUserScopedIndex bool
	for _, index := range indexes {
		if index.Name() == tenantModelProviderUserUniqueIndex && !sameIndexColumns(index.Columns(), []string{"provider_name", "tenant_id", "user_id"}) {
			if err := migrator.DropIndex(model, index.Name()); err != nil {
				return fmt.Errorf("drop malformed index %s: %w", index.Name(), err)
			}
			continue
		}
		unique, known := index.Unique()
		if known && unique && sameIndexColumns(index.Columns(), []string{"provider_name", "tenant_id", "user_id"}) {
			hasUserScopedIndex = true
		}
	}
	if !hasUserScopedIndex {
		if err := migrator.CreateIndex(model, tenantModelProviderUserUniqueIndex); err != nil {
			return fmt.Errorf("create user-scoped provider index: %w", err)
		}
	}

	indexes, err = migrator.GetIndexes(model)
	if err != nil {
		return fmt.Errorf("inspect tenant_model_provider indexes after creating replacement: %w", err)
	}
	for _, index := range indexes {
		unique, known := index.Unique()
		if !known || !unique || index.Name() == tenantModelProviderUserUniqueIndex {
			continue
		}
		if !sameIndexColumns(index.Columns(), []string{"provider_name", "tenant_id"}) {
			continue
		}
		if err := migrator.DropIndex(model, index.Name()); err != nil {
			return fmt.Errorf("drop legacy provider index %s: %w", index.Name(), err)
		}
	}
	return nil
}

func sameIndexColumns(columns, expected []string) bool {
	if len(columns) != len(expected) {
		return false
	}
	columnSet := make(map[string]struct{}, len(expected))
	for _, column := range expected {
		columnSet[strings.ToLower(column)] = struct{}{}
	}
	for _, column := range columns {
		delete(columnSet, strings.ToLower(column))
	}
	return len(columnSet) == 0
}
