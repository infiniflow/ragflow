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

package entity

// TenantModel tenant model table
type TenantModel struct {
	ID         string `gorm:"column:id;primaryKey;size:32" json:"id"`
	ModelName  string `gorm:"column:model_name;size:128" json:"model_name"`
	ProviderID string `gorm:"column:provider_id;size:32;not null" json:"provider_id"`
	InstanceID string `gorm:"column:instance_id;size:32;not null;index" json:"instance_id"`
	ModelType  string `gorm:"column:model_type;size:32;not null" json:"model_type"`
	Status     string `gorm:"column:status;size:32;default:'active'" json:"status"`
	Extra      string `gorm:"column:extra;size:1024;default:'{}'" json:"extra"`
	BaseModel
}

// TableName specify table name
func (TenantModel) TableName() string {
	return "tenant_model"
}
