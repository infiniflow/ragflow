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

// TenantModelProvider tenant model provider table
type TenantModelProvider struct {
	ID           string `gorm:"column:id;primaryKey;size:32" json:"id"`
	ProviderName string `gorm:"column:provider_name;size:128;not null;index:idx_tenant_provider_unique,unique" json:"provider_name"`
	TenantID     string `gorm:"column:tenant_id;size:32;not null;index;index:idx_tenant_provider_unique,unique" json:"tenant_id"`
	BaseModel
}

// TableName specify table name
func (TenantModelProvider) TableName() string {
	return "tenant_model_provider"
}
