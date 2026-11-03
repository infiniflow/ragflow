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

// TenantModelProviderDAO tenant model provider data access object
type TenantModelProviderDAO struct{}

// NewTenantModelProviderDAO create tenant model provider DAO
func NewTenantModelProviderDAO() *TenantModelProviderDAO {
	return &TenantModelProviderDAO{}
}

func (dao *TenantModelProviderDAO) Create(ctx context.Context, db *gorm.DB, provider *entity.TenantModelProvider) error {
	return db.WithContext(ctx).Create(provider).Error
}

// GetByID get tenant model provider by primary key (id)
func (dao *TenantModelProviderDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.TenantModelProvider, error) {
	var provider entity.TenantModelProvider
	err := db.WithContext(ctx).Take(&provider, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// GetByTenantIDAndUserIDAndProviderName gets a user's provider configuration in a tenant.
func (dao *TenantModelProviderDAO) GetByTenantIDAndUserIDAndProviderName(ctx context.Context, db *gorm.DB, tenantID, userID, providerName string) (*entity.TenantModelProvider, error) {
	var provider entity.TenantModelProvider
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND user_id = ? AND LOWER(provider_name) = LOWER(?)", tenantID, userID, providerName).
		First(&provider).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// ListByTenantIDAndUserID returns one user's provider configurations in a tenant.
func (dao *TenantModelProviderDAO) ListByTenantIDAndUserID(ctx context.Context, db *gorm.DB, tenantID, userID string) ([]*entity.TenantModelProvider, error) {
	var providers []*entity.TenantModelProvider
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND user_id = ?", tenantID, userID).
		Find(&providers).Error
	return providers, err
}

// DeleteByID hard-deletes one provider configuration.
func (dao *TenantModelProviderDAO) DeleteByID(ctx context.Context, db *gorm.DB, id string) (int64, error) {
	result := db.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&entity.TenantModelProvider{})
	return result.RowsAffected, result.Error
}
