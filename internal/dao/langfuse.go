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
	"gorm.io/gorm/clause"

	"ragflow/internal/entity"
)

// LangfuseDAO data access for tenant_langfuse table.
type LangfuseDAO struct{}

// NewLangfuseDAO creates a LangfuseDAO.
func NewLangfuseDAO() *LangfuseDAO {
	return &LangfuseDAO{}
}

// GetByTenantID returns the Langfuse credential record for a tenant, or nil
// when none exists.
func (d *LangfuseDAO) GetByTenantID(tenantID string) (*entity.TenantLangfuse, error) {
	var entry entity.TenantLangfuse
	err := DB.Where("tenant_id = ?", tenantID).First(&entry).Error
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

// Create inserts a new TenantLangfuse record.
func (d *LangfuseDAO) Create(entry *entity.TenantLangfuse) error {
	return DB.Create(entry).Error
}

// UpdateByTenantID applies updates to the record for a tenant.
func (d *LangfuseDAO) UpdateByTenantID(tenantID string, updates map[string]interface{}) error {
	return DB.Model(&entity.TenantLangfuse{}).
		Where("tenant_id = ?", tenantID).
		Updates(updates).Error
}

// UpsertByTenantID atomically inserts the record or, when a row already exists
// for the same tenant_id (unique index), updates its credentials in a single
// statement. This removes the read-then-write race between concurrent callers.
func (d *LangfuseDAO) UpsertByTenantID(entry *entity.TenantLangfuse) error {
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"secret_key", "public_key", "host", "update_time", "update_date",
		}),
	}).Create(entry).Error
}

// DeleteByTenantID hard-deletes the record for a tenant.
func (d *LangfuseDAO) DeleteByTenantID(tenantID string) error {
	return DB.Unscoped().
		Where("tenant_id = ?", tenantID).
		Delete(&entity.TenantLangfuse{}).Error
}
