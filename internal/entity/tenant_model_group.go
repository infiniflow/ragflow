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

// TenantModelGroup tenant model group table
type TenantModelGroup struct {
	ID        string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	GroupType string  `gorm:"column:group_type;size:32;not null" json:"group_type"`
	ModelName *string `gorm:"column:model_name;size:128" json:"model_name,omitempty"`
	Strategy  string  `gorm:"column:strategy;size:32;default:'weighted'" json:"strategy"`
	BaseModel
}

// TableName specify table name
func (TenantModelGroup) TableName() string {
	return "tenant_model_group"
}
