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

// TenantLangfuse tenant langfuse model
type TenantLangfuse struct {
	TenantID  string `gorm:"column:tenant_id;primaryKey;size:32" json:"tenant_id"`
	SecretKey string `gorm:"column:secret_key;size:2048;not null" json:"secret_key"`
	PublicKey string `gorm:"column:public_key;size:2048;not null" json:"public_key"`
	Host      string `gorm:"column:host;size:128;not null" json:"host"`
	BaseModel
}

// TableName specify table name
func (TenantLangfuse) TableName() string {
	return "tenant_langfuse"
}
