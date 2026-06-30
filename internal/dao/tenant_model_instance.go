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
	"errors"
	"fmt"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// TenantModelInstanceDAO tenant model instance data access object
type TenantModelInstanceDAO struct{}

// NewTenantModelInstanceDAO create tenant model instance DAO
func NewTenantModelInstanceDAO() *TenantModelInstanceDAO {
	return &TenantModelInstanceDAO{}
}

func (dao *TenantModelInstanceDAO) Create(instance *entity.TenantModelInstance) error {
	// begin tx and check if the same provider instance exists
	tx := DB.Begin()
	defer tx.Rollback()
	var existingInstance entity.TenantModelInstance
	err := tx.Where("provider_id = ? AND instance_name = ?", instance.ProviderID, instance.InstanceName).First(&existingInstance).Error
	if err == nil {
		return fmt.Errorf("instance %s already exists", instance.InstanceName)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	err = tx.Create(instance).Error
	if err != nil {
		return err
	}
	tx.Commit()
	return nil
}

func (dao *TenantModelInstanceDAO) GetAllInstancesByProviderID(providerID string) ([]*entity.TenantModelInstance, error) {
	var instances []*entity.TenantModelInstance
	err := DB.Where("provider_id = ?", providerID).Find(&instances).Error
	if err != nil {
		return nil, err
	}
	return instances, nil
}

// GetByProviderIDs returns all TenantModelInstance rows whose provider_id
// is in providerIDs. Mirrors Python's
// TenantModelInstanceService.get_by_provider_ids used by
// models_api_service.list_tenant_added_models. An empty input slice
// returns an empty (non-nil) slice with no error.
func (dao *TenantModelInstanceDAO) GetByProviderIDs(providerIDs []string) ([]*entity.TenantModelInstance, error) {
	instances := make([]*entity.TenantModelInstance, 0)
	if len(providerIDs) == 0 {
		return instances, nil
	}
	err := DB.Where("provider_id IN ?", providerIDs).Find(&instances).Error
	if err != nil {
		return nil, err
	}
	return instances, nil
}

func (dao *TenantModelInstanceDAO) GetInstanceByApiKey(apiKey, providerID string) (*entity.TenantModelInstance, error) {
	var instance entity.TenantModelInstance
	err := DB.Where("api_key = ? && provider_id = ?", apiKey, providerID).First(&instance).Error
	if err != nil {
		return nil, err
	}
	return &instance, nil
}

func (dao *TenantModelInstanceDAO) GetByProviderIDAndInstanceName(providerID, instanceName string) (*entity.TenantModelInstance, error) {
	var instance entity.TenantModelInstance
	err := DB.Where("provider_id = ? AND instance_name = ?", providerID, instanceName).First(&instance).Error
	if err != nil {
		return nil, err
	}
	return &instance, nil
}

// GetByID get tenant model instance by primary key (id)
func (dao *TenantModelInstanceDAO) GetByID(id string) (*entity.TenantModelInstance, error) {
	var instance entity.TenantModelInstance
	err := DB.Where("id = ?", id).First(&instance).Error
	if err != nil {
		return nil, err
	}
	return &instance, nil
}

func (dao *TenantModelInstanceDAO) DeleteByProviderIDAndInstanceName(providerID, instanceName string) (int64, error) {
	result := DB.Unscoped().Where("provider_id = ? and instance_name = ?", providerID, instanceName).Delete(&entity.TenantModelInstance{})
	return result.RowsAffected, result.Error
}
