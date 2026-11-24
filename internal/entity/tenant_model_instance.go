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
	ID           string `gorm:"column:id;primaryKey;size:32" json:"id"`
	InstanceName string `gorm:"column:instance_name;size:128;not null" json:"instance_name"`
	ProviderID   string `gorm:"column:provider_id;size:32;not null" json:"provider_id"`
	// varchar(4096), bounded on purpose (review of #22424): provider keys are
	// not uniformly short (IONOS model-hub keys exceed 1,700 chars) and MySQL
	// rejects writes past a sized varchar with Error 1406, but the column
	// keeps a declared maximum so the schema cannot grow it without a
	// deliberate change.
	APIKey string `gorm:"column:api_key;type:varchar(4096);not null" json:"api_key"`
	Status string `gorm:"column:status;size:32;default:'active'" json:"status"`
	Extra  string `gorm:"column:extra;size:512;default:'{}'" json:"extra"`
	BaseModel
}

// TableName specify table name
func (TenantModelInstance) TableName() string {
	return "tenant_model_instance"
}
