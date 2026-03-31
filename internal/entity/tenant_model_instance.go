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

// TenantModelInstance tenant model instance table
type TenantModelInstance struct {
	ID         string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	ProviderID string  `gorm:"column:provider_id;size:32;not null;index" json:"provider_id"`
	APIKey     string  `gorm:"column:api_key;size:512;not null;uniqueIndex" json:"api_key"`
	Endpoint   *string `gorm:"column:endpoint;size:512" json:"endpoint,omitempty"`
	Status     string  `gorm:"column:status;size:32;default:'active'" json:"status"`
	BaseModel
}

// TableName specify table name
func (TenantModelInstance) TableName() string {
	return "tenant_model_instance"
}
