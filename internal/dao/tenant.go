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
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// TenantDAO tenant data access object
type TenantDAO struct{}

// NewTenantDAO create tenant DAO
func NewTenantDAO() *TenantDAO {
	return &TenantDAO{}
}

// GetJoinedTenantsByUserID get joined tenants by user ID
func (dao *TenantDAO) GetJoinedTenantsByUserID(ctx context.Context, db *gorm.DB, userID string) ([]*TenantWithRole, error) {
	var results []*TenantWithRole

	err := db.WithContext(ctx).Model(&entity.Tenant{}).
		Select("tenant.id as tenant_id, tenant.name, user_tenant.role").
		Joins("INNER JOIN user_tenant ON user_tenant.tenant_id = tenant.id").
		Where("user_tenant.user_id = ? AND user_tenant.status = ? AND user_tenant.role = ? AND tenant.status = ?", userID, "1", "normal", "1").
		Scan(&results).Error

	return results, err
}

// TenantWithRole tenant with role information
type TenantWithRole struct {
	TenantID string `gorm:"column:tenant_id" json:"tenant_id"`
	Name     string `gorm:"column:name" json:"name"`
	Role     string `gorm:"column:role" json:"role"`
}

// GetByID gets tenant by ID
func (dao *TenantDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.Tenant, error) {
	var tenant entity.Tenant
	err := db.WithContext(ctx).Where("id = ? AND status = ?", id, "1").First(&tenant).Error
	if err != nil {
		return nil, err
	}
	return &tenant, nil
}

// Create creates a new tenant
func (dao *TenantDAO) Create(ctx context.Context, db *gorm.DB, tenant *entity.Tenant) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := initializeTenantDefaultRoleEE(ctx, tx, tenant); err != nil {
			return err
		}
		return tx.Create(tenant).Error
	})
}

// GetAllIDs returns the IDs of all active tenants.
func (dao *TenantDAO) GetAllIDs(ctx context.Context, db *gorm.DB) ([]string, error) {
	var ids []string
	err := db.WithContext(ctx).Model(&entity.Tenant{}).Where("status = ?", string(entity.StatusValid)).
		Pluck("id", &ids).Error
	return ids, err
}

// Update updates a tenant by ID
func (dao *TenantDAO) Update(ctx context.Context, db *gorm.DB, id string, updates map[string]interface{}) error {
	return db.WithContext(ctx).Model(&entity.Tenant{}).Where("id = ?", id).Updates(updates).Error
}

// HardDelete hard deletes a tenant by ID
func (dao *TenantDAO) HardDelete(ctx context.Context, db *gorm.DB, id string) error {
	return db.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&entity.Tenant{}).Error
}
