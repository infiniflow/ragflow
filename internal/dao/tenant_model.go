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

// TenantModelDAO tenant model data access object
type TenantModelDAO struct{}

// NewTenantModelDAO create tenant model DAO
func NewTenantModelDAO() *TenantModelDAO {
	return &TenantModelDAO{}
}

// GetByID get tenant model by primary key (id)
func (dao *TenantModelDAO) GetByID(id string) (*entity.TenantModel, error) {
	var model entity.TenantModel
	err := DB.Where("id = ?", id).First(&model).Error
	if err != nil {
		return nil, err
	}
	return &model, nil
}
