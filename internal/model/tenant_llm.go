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

package model

// TenantLLM tenant LLM model
// Python uses PrimaryKeyField (auto-increment ID) with unique index on (tenant_id, llm_factory, llm_name)
type TenantLLM struct {
	ID         int64   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TenantID   string  `gorm:"column:tenant_id;size:32;not null;index:idx_tenant_llm_unique,unique" json:"tenant_id"`
	LLMFactory string  `gorm:"column:llm_factory;size:128;not null;index:idx_tenant_llm_unique,unique" json:"llm_factory"`
	ModelType  *string `gorm:"column:model_type;size:128;index" json:"model_type,omitempty"`
	LLMName    *string `gorm:"column:llm_name;size:128;index:idx_tenant_llm_unique,unique;default:\"\"" json:"llm_name,omitempty"`
	APIKey     *string `gorm:"column:api_key;type:longtext" json:"api_key,omitempty"`
	APIBase    *string `gorm:"column:api_base;size:255" json:"api_base,omitempty"`
	MaxTokens  int64   `gorm:"column:max_tokens;default:8192;index" json:"max_tokens"`
	UsedTokens int64   `gorm:"column:used_tokens;default:0;index" json:"used_tokens"`
	Status     string  `gorm:"column:status;size:1;not null;default:1;index" json:"status"`
	BaseModel
}

// TableName specify table name
func (TenantLLM) TableName() string {
	return "tenant_llm"
}
