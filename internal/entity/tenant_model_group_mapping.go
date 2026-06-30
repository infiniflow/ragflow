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

// TenantModelGroupMapping tenant model group mapping table
type TenantModelGroupMapping struct {
	GroupID    string `gorm:"column:group_id;primaryKey;size:32;index" json:"group_id"`
	ProviderID string `gorm:"column:provider_id;primaryKey;size:32" json:"provider_id"`
	InstanceID string `gorm:"column:instance_id;primaryKey;size:32" json:"instance_id"`
	ModelID    string `gorm:"column:model_id;primaryKey;size:32;index" json:"model_id"`
	Weight     int    `gorm:"column:weight;default:100" json:"weight"`
	Status     string `gorm:"column:status;size:32;default:'active'" json:"status"`
	BaseModel
}

// TableName specify table name
func (TenantModelGroupMapping) TableName() string {
	return "tenant_model_group_mapping"
}
