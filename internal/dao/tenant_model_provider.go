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
	err := db.WithContext(ctx).Where("id = ?", id).First(&provider).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// GetByTenantIDAndProviderName get the providers by tenant ID and provider name
func (dao *TenantModelProviderDAO) GetByTenantIDAndProviderName(ctx context.Context, db *gorm.DB, tenantID, providerName string) (*entity.TenantModelProvider, error) {
	var provider entity.TenantModelProvider
	err := db.WithContext(ctx).Where("tenant_id = ? AND provider_name = ?", tenantID, providerName).First(&provider).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// DeleteByTenantID deletes all model providers by tenant ID (hard delete)
func (dao *TenantModelProviderDAO) DeleteByTenantID(ctx context.Context, db *gorm.DB, tenantID string) (int64, error) {
	result := db.WithContext(ctx).Unscoped().Where("tenant_id = ?", tenantID).Delete(&entity.TenantModelProvider{})
	return result.RowsAffected, result.Error
}

// DeleteByTenantIDAndProviderName DeleteByTenantID deletes all providers by tenant ID (hard delete)
func (dao *TenantModelProviderDAO) DeleteByTenantIDAndProviderName(ctx context.Context, db *gorm.DB, tenantID, providerName string) (int64, error) {
	result := db.WithContext(ctx).Unscoped().Where("tenant_id = ? AND provider_name = ?", tenantID, providerName).Delete(&entity.TenantModelProvider{})
	return result.RowsAffected, result.Error
}

// ListByID list tenant model providers by ID
func (dao *TenantModelProviderDAO) ListByID(ctx context.Context, db *gorm.DB, id string) ([]string, error) {
	var providerNames []string
	err := db.WithContext(ctx).Model(&entity.TenantModelProvider{}).
		Where("tenant_id = ?", id).
		Pluck("provider_name", &providerNames).Error
	return providerNames, err
}

// GetByTenantID returns all TenantModelProvider rows for a tenant.
// Mirrors Python's TenantModelProviderService.get_by_tenant_id and is the
// entry point for /api/v1/models ("list all added models"). The Go port
// uses this to enumerate which providers a tenant has linked before
// fanning out to TenantModelInstanceDAO / TenantModelDAO for the joined
// result.
func (dao *TenantModelProviderDAO) GetByTenantID(ctx context.Context, db *gorm.DB, tenantID string) ([]*entity.TenantModelProvider, error) {
	var providers []*entity.TenantModelProvider
	err := db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&providers).Error
	if err != nil {
		return nil, err
	}
	return providers, nil
}
