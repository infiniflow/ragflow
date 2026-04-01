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

// TenantModelInstanceDAO tenant model instance data access object
type TenantModelInstanceDAO struct{}

// NewTenantModelInstanceDAO create tenant model instance DAO
func NewTenantModelInstanceDAO() *TenantModelInstanceDAO {
	return &TenantModelInstanceDAO{}
}

func (dao *TenantModelInstanceDAO) Create(instance *entity.TenantModelInstance) error {
	return DB.Create(instance).Error
}

func (dao *TenantModelInstanceDAO) GetByProviderIDAndTenantID(providerID string) ([]*entity.TenantModelInstance, error) {
	var instances []*entity.TenantModelInstance
	err := DB.Where("provider_id = ?", providerID).Find(&instances).Error
	if err != nil {
		return nil, err
	}
	return instances, nil
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
