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

// TenantLLMDAO tenant LLM data access object
type TenantLLMDAO struct{}

// NewTenantLLMDAO create tenant LLM DAO
func NewTenantLLMDAO() *TenantLLMDAO {
	return &TenantLLMDAO{}
}

// GetByID get tenant LLM by primary key ID
func (dao *TenantLLMDAO) GetByID(id int64) (*entity.TenantLLM, error) {
	var tenantLLM entity.TenantLLM
	err := DB.Where("id = ?", id).First(&tenantLLM).Error
	if err != nil {
		return nil, err
	}
	return &tenantLLM, nil
}

// GetByTenantAndModelName get tenant LLM by tenant ID and model name
func (dao *TenantLLMDAO) GetByTenantAndModelName(tenantID, providerName string, modelName string) (*entity.TenantLLM, error) {
	var tenantLLM entity.TenantLLM
	err := DB.Where("tenant_id = ? AND llm_factory = ? AND llm_name = ?", tenantID, providerName, modelName).First(&tenantLLM).Error
	if err != nil {
		return nil, err
	}
	return &tenantLLM, nil
}

// GetByTenantNameAndType get tenant LLM by tenant ID, model name, and model type (factory is optional)
func (dao *TenantLLMDAO) GetByTenantNameAndType(tenantID, modelName string, modelType entity.ModelType) (*entity.TenantLLM, error) {
	var tenantLLM entity.TenantLLM
	err := DB.Where("tenant_id = ? AND llm_name = ? AND model_type = ?", tenantID, modelName, modelType).First(&tenantLLM).Error
	if err != nil {
		return nil, err
	}
	return &tenantLLM, nil
}

// GetByTenantAndType get tenant LLM by tenant ID and model type
func (dao *TenantLLMDAO) GetByTenantAndType(tenantID string, modelType entity.ModelType) (*entity.TenantLLM, error) {
	var tenantLLM entity.TenantLLM
	err := DB.Where("tenant_id = ? AND model_type = ?", tenantID, modelType).First(&tenantLLM).Error
	if err != nil {
		return nil, err
	}
	return &tenantLLM, nil
}

// GetByTenantAndFactory get tenant LLM by tenant ID, model type and factory
func (dao *TenantLLMDAO) GetByTenantAndFactory(tenantID string, modelType entity.ModelType, factory string) (*entity.TenantLLM, error) {
	var tenantLLM entity.TenantLLM
	err := DB.Where("tenant_id = ? AND model_type = ? AND llm_factory = ?", tenantID, modelType, factory).First(&tenantLLM).Error
	if err != nil {
		return nil, err
	}
	return &tenantLLM, nil
}

// ListByTenant list all tenant LLMs for a tenant
func (dao *TenantLLMDAO) ListByTenant(tenantID string) ([]entity.TenantLLM, error) {
	var tenantLLMs []entity.TenantLLM
	err := DB.Where("tenant_id = ?", tenantID).Find(&tenantLLMs).Error
	if err != nil {
		return nil, err
	}
	return tenantLLMs, nil
}

// GetByTenantFactoryAndModelName get tenant LLM by tenant ID, factory and model name
func (dao *TenantLLMDAO) GetByTenantFactoryAndModelName(tenantID, factory, modelName string) (*entity.TenantLLM, error) {
	var tenantLLM entity.TenantLLM
	err := DB.Where("tenant_id = ? AND llm_factory = ? AND llm_name = ?", tenantID, factory, modelName).First(&tenantLLM).Error
	if err != nil {
		return nil, err
	}
	return &tenantLLM, nil
}

// Create create a new tenant LLM record
func (dao *TenantLLMDAO) Create(tenantLLM *entity.TenantLLM) error {
	return DB.Create(tenantLLM).Error
}

// Update update an existing tenant LLM record
func (dao *TenantLLMDAO) Update(tenantLLM *entity.TenantLLM) error {
	return DB.Save(tenantLLM).Error
}

// Delete delete a tenant LLM record by tenant ID, factory and model name
func (dao *TenantLLMDAO) Delete(tenantID, factory, modelName string) error {
	return DB.Where("tenant_id = ? AND llm_factory = ? AND llm_name = ?", tenantID, factory, modelName).Delete(&entity.TenantLLM{}).Error
}

// GetMyLLMs get tenant LLMs with factory details
func (dao *TenantLLMDAO) GetMyLLMs(tenantID string) ([]entity.MyLLM, error) {
	var myLLMs []entity.MyLLM

	err := DB.Table("tenant_llm tl").
		Select("tl.id, tl.llm_factory, lf.logo, lf.tags, tl.model_type, tl.llm_name, tl.used_tokens, tl.status").
		Joins("JOIN llm_factories lf ON tl.llm_factory = lf.name").
		Where("tl.tenant_id = ? AND tl.api_key IS NOT NULL", tenantID).
		Find(&myLLMs).Error
	if err != nil {
		return nil, err
	}
	return myLLMs, nil
}

// ListValidByTenant lists valid tenant LLMs for a tenant
func (dao *TenantLLMDAO) ListValidByTenant(tenantID string) ([]*entity.TenantLLM, error) {
	var tenantLLMs []*entity.TenantLLM
	err := DB.Where("tenant_id = ? AND api_key IS NOT NULL AND api_key != ? AND status = ?", tenantID, "", "1").Find(&tenantLLMs).Error
	if err != nil {
		return nil, err
	}
	return tenantLLMs, nil
}

// ListAllByTenant lists all tenant LLMs for a tenant
func (dao *TenantLLMDAO) ListAllByTenant(tenantID string) ([]*entity.TenantLLM, error) {
	var tenantLLMs []*entity.TenantLLM
	err := DB.Where("tenant_id = ?", tenantID).Find(&tenantLLMs).Error
	if err != nil {
		return nil, err
	}
	return tenantLLMs, nil
}

// InsertMany inserts multiple tenant LLM records
func (dao *TenantLLMDAO) InsertMany(tenantLLMs []*entity.TenantLLM) error {
	if len(tenantLLMs) == 0 {
		return nil
	}
	return DB.Create(&tenantLLMs).Error
}

// DeleteByTenantID deletes all tenant LLM records by tenant ID (hard delete)
func (dao *TenantLLMDAO) DeleteByTenantID(tenantID string) (int64, error) {
	result := DB.Unscoped().Where("tenant_id = ?", tenantID).Delete(&entity.TenantLLM{})
	return result.RowsAffected, result.Error
}

// splitModelNameAndFactory splits model name and factory from combined format
// This matches Python's split_model_name_and_factory logic
//
// Parameters:
//   - modelName: The model name which can be in format "ModelName" or "ModelName@Factory"
//
// Returns:
//   - string: The model name without factory prefix
//   - string: The factory name (empty string if not specified)
//
// Example:
//
//	modelName, factory := splitModelNameAndFactory("gpt-4")
//	// Returns: "gpt-4", ""
//
//	modelName, factory := splitModelNameAndFactory("gpt-4@OpenAI")
//	// Returns: "gpt-4", "OpenAI"
func splitModelNameAndFactory(modelName string) (string, string) {
	// Split by "@" separator
	// Handle cases like "model@factory" or "model@sub@factory"
	lastAtIndex := -1
	for i := len(modelName) - 1; i >= 0; i-- {
		if modelName[i] == '@' {
			lastAtIndex = i
			break
		}
	}

	// No "@" found, return original name
	if lastAtIndex == -1 {
		return modelName, ""
	}

	// Split into model name and potential factory
	modelNamePart := modelName[:lastAtIndex]
	factory := modelName[lastAtIndex+1:]

	// Validate if factory exists in llm_factories table
	// This matches Python's logic of checking against model providers
	var factoryCount int64
	DB.Model(&entity.LLMFactories{}).Where("name = ?", factory).Count(&factoryCount)

	// If factory doesn't exist in database, treat the whole string as model name
	if factoryCount == 0 {
		return modelName, ""
	}

	return modelNamePart, factory
}

// GetByTenantIDAndLLMName gets tenant LLM by tenant ID and LLM name
// This is used to resolve tenant_llm_id from llm_id
// It supports both simple model names and factory-prefixed names (e.g., "gpt-4@OpenAI")
//
// Parameters:
//   - tenantID: The tenant identifier
//   - llmName: The LLM model name (can include factory prefix like "OpenAI@gpt-4")
//
// Returns:
//   - *model.TenantLLM: The tenant LLM record
//   - error: Error if not found
//
// Example:
//
//	// Simple model name
//	tenantLLM, err := dao.GetByTenantIDAndLLMName("tenant123", "gpt-4")
//
//	// Model name with factory prefix
//	tenantLLM, err := dao.GetByTenantIDAndLLMName("tenant123", "gpt-4@OpenAI")
func (dao *TenantLLMDAO) GetByTenantIDAndLLMName(tenantID string, llmName string) (*entity.TenantLLM, error) {
	var tenantLLM entity.TenantLLM

	// Split model name and factory from the combined format
	modelName, factory := splitModelNameAndFactory(llmName)

	// First attempt: try to find with model name only
	err := DB.Where("tenant_id = ? AND llm_name = ?", tenantID, modelName).First(&tenantLLM).Error
	if err == nil {
		return &tenantLLM, nil
	}

	// Second attempt: if factory is specified, try with both model name and factory
	if factory != "" {
		err = DB.Where("tenant_id = ? AND llm_name = ? AND llm_factory = ?", tenantID, modelName, factory).First(&tenantLLM).Error
		if err == nil {
			return &tenantLLM, nil
		}

		// Special handling for LocalAI and HuggingFace (matching Python logic)
		// These factories append "___FactoryName" to the model name
		if factory == "LocalAI" || factory == "HuggingFace" || factory == "OpenAI-API-Compatible" {
			specialModelName := modelName + "___" + factory
			err = DB.Where("tenant_id = ? AND llm_name = ?", tenantID, specialModelName).First(&tenantLLM).Error
			if err == nil {
				return &tenantLLM, nil
			}
		}
	}

	// Return the last error (record not found)
	return nil, err
}

// GetByTenantIDLLMNameAndFactory gets tenant LLM by tenant ID, LLM name and factory
// This is used when model name includes factory suffix (e.g., "model@factory")
//
// Parameters:
//   - tenantID: The tenant identifier
//   - llmName: The LLM model name
//   - factory: The LLM factory name
//
// Returns:
//   - *model.TenantLLM: The tenant LLM record
//   - error: Error if not found
//
// Example:
//
//	tenantLLM, err := dao.GetByTenantIDLLMNameAndFactory("tenant123", "gpt-4", "OpenAI")
func (dao *TenantLLMDAO) GetByTenantIDLLMNameAndFactory(tenantID, llmName, factory string) (*entity.TenantLLM, error) {
	var tenantLLM entity.TenantLLM
	err := DB.Where("tenant_id = ? AND llm_name = ? AND llm_factory = ?", tenantID, llmName, factory).First(&tenantLLM).Error
	if err != nil {
		return nil, err
	}
	return &tenantLLM, nil
}
