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
func (dao *TenantLLMDAO) GetByTenantAndModelName(tenantID, modelName string) (*model.TenantLLM, error) {
	var tenantLLM model.TenantLLM
	err := DB.Where("tenant_id = ? AND llm_name = ?", tenantID, modelName).First(&tenantLLM).Error
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