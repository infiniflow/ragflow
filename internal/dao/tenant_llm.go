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
	"ragflow/internal/model"
)

// TenantLLMDAO tenant LLM data access object
type TenantLLMDAO struct{}

// NewTenantLLMDAO create tenant LLM DAO
func NewTenantLLMDAO() *TenantLLMDAO {
	return &TenantLLMDAO{}
}

// GetByTenantAndModelName get tenant LLM by tenant ID and model name
func (dao *TenantLLMDAO) GetByTenantAndModelName(tenantID, providerName string, modelName string) (*model.TenantLLM, error) {
	var tenantLLM model.TenantLLM
	err := DB.Where("tenant_id = ? AND llm_factory = ? AND llm_name = ?", tenantID, providerName, modelName).First(&tenantLLM).Error
	if err != nil {
		return nil, err
	}
	return &tenantLLM, nil
}

// GetByTenantAndType get tenant LLM by tenant ID and model type
func (dao *TenantLLMDAO) GetByTenantAndType(tenantID string, modelType model.ModelType) (*model.TenantLLM, error) {
	var tenantLLM model.TenantLLM
	err := DB.Where("tenant_id = ? AND model_type = ?", tenantID, modelType).First(&tenantLLM).Error
	if err != nil {
		return nil, err
	}
	return &tenantLLM, nil
}

// GetByTenantAndFactory get tenant LLM by tenant ID, model type and factory
func (dao *TenantLLMDAO) GetByTenantAndFactory(tenantID string, modelType model.ModelType, factory string) (*model.TenantLLM, error) {
	var tenantLLM model.TenantLLM
	err := DB.Where("tenant_id = ? AND model_type = ? AND llm_factory = ?", tenantID, modelType, factory).First(&tenantLLM).Error
	if err != nil {
		return nil, err
	}
	return &tenantLLM, nil
}

// ListByTenant list all tenant LLMs for a tenant
func (dao *TenantLLMDAO) ListByTenant(tenantID string) ([]model.TenantLLM, error) {
	var tenantLLMs []model.TenantLLM
	err := DB.Where("tenant_id = ?", tenantID).Find(&tenantLLMs).Error
	if err != nil {
		return nil, err
	}
	return tenantLLMs, nil
}

// GetByTenantFactoryAndModelName get tenant LLM by tenant ID, factory and model name
func (dao *TenantLLMDAO) GetByTenantFactoryAndModelName(tenantID, factory, modelName string) (*model.TenantLLM, error) {
	var tenantLLM model.TenantLLM
	err := DB.Where("tenant_id = ? AND llm_factory = ? AND llm_name = ?", tenantID, factory, modelName).First(&tenantLLM).Error
	if err != nil {
		return nil, err
	}
	return &tenantLLM, nil
}

// Create create a new tenant LLM record
func (dao *TenantLLMDAO) Create(tenantLLM *model.TenantLLM) error {
	return DB.Create(tenantLLM).Error
}

// Update update an existing tenant LLM record
func (dao *TenantLLMDAO) Update(tenantLLM *model.TenantLLM) error {
	return DB.Save(tenantLLM).Error
}

// Delete delete a tenant LLM record by tenant ID, factory and model name
func (dao *TenantLLMDAO) Delete(tenantID, factory, modelName string) error {
	return DB.Where("tenant_id = ? AND llm_factory = ? AND llm_name = ?", tenantID, factory, modelName).Delete(&model.TenantLLM{}).Error
}

// GetMyLLMs get tenant LLMs with factory details
func (dao *TenantLLMDAO) GetMyLLMs(tenantID string, includeDetails bool) ([]model.MyLLM, error) {
	var myLLMs []model.MyLLM

	// Base query
	query := DB.Table("tenant_llm tl").
		Select("tl.llm_factory, lf.logo, lf.tags, tl.model_type, tl.llm_name, tl.used_tokens, tl.status").
		Joins("JOIN llm_factories lf ON tl.llm_factory = lf.name").
		Where("tl.tenant_id = ? AND tl.api_key IS NOT NULL", tenantID)

	// Add detailed fields if requested
	if includeDetails {
		query = query.Select("tl.llm_factory, lf.logo, lf.tags, tl.model_type, tl.llm_name, tl.used_tokens, tl.status, tl.api_base, tl.max_tokens")
	}

	err := query.Find(&myLLMs).Error
	if err != nil {
		return nil, err
	}
	return myLLMs, nil
}
