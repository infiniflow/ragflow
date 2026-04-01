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
	"ragflow/internal/entity"
)

// TenantModelProviderDAO tenant model provider data access object
type TenantModelProviderDAO struct{}

// NewTenantModelProviderDAO create tenant model provider DAO
func NewTenantModelProviderDAO() *TenantModelProviderDAO {
	return &TenantModelProviderDAO{}
}

func (dao *TenantModelProviderDAO) Create(provider *entity.TenantModelProvider) error {
	return DB.Create(provider).Error
}

// GetByID get tenant model provider by primary key (id)
func (dao *TenantModelProviderDAO) GetByID(id string) (*entity.TenantModelProvider, error) {
	var provider entity.TenantModelProvider
	err := DB.Where("id = ?", id).First(&provider).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// DeleteByTenantID deletes all model providers by tenant ID (hard delete)
func (dao *TenantModelProviderDAO) DeleteByTenantID(tenantID string) (int64, error) {
	result := DB.Unscoped().Where("tenant_id = ?", tenantID).Delete(&entity.TenantModelProvider{})
	return result.RowsAffected, result.Error
}

// DeleteByTenantID deletes all providers by tenant ID (hard delete)
func (dao *TenantModelProviderDAO) DeleteByTenantIDAndProviderName(tenantID, providerName string) (int64, error) {
	result := DB.Unscoped().Where("tenant_id = ? AND provider_name = ?", tenantID, providerName).Delete(&entity.TenantModelProvider{})
	return result.RowsAffected, result.Error
}

// ListByID list tenant model providers by ID
func (dao *TenantModelProviderDAO) ListByID(id string) ([]string, error) {
	var providerNames []string
	err := DB.Model(&entity.TenantModelProvider{}).
		Where("tenant_id = ?", id).
		Pluck("provider_name", &providerNames).Error
	return providerNames, err
}
