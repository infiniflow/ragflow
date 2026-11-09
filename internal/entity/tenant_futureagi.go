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

// TenantFutureAGI holds the Future AGI credentials for one tenant.
// Keys are stored as plain strings, the same way TenantLangfuse stores
// its secret. This table does not add encryption RAGFlow does not apply.
type TenantFutureAGI struct {
	TenantID    string `gorm:"column:tenant_id;primaryKey;size:32" json:"tenant_id"`
	APIKey      string `gorm:"column:api_key;size:2048;not null" json:"api_key"`
	SecretKey   string `gorm:"column:secret_key;size:2048;not null" json:"secret_key"`
	Endpoint    string `gorm:"column:endpoint;size:256;not null" json:"endpoint"`
	ProjectName string `gorm:"column:project_name;size:128;not null" json:"project_name"`
	Redact      bool   `gorm:"column:redact;not null;default:false" json:"redact"`
	BaseModel
}

func (TenantFutureAGI) TableName() string {
	return "tenant_futureagi"
}
