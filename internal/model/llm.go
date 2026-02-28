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

// LLMFactories LLM factory model
type LLMFactories struct {
	Name   string  `gorm:"column:name;primaryKey;size:128" json:"name"`
	Logo   *string `gorm:"column:logo;type:longtext" json:"logo,omitempty"`
	Tags   string  `gorm:"column:tags;size:255;not null;index" json:"tags"`
	Rank   int64   `gorm:"column:rank;default:0" json:"rank"`
	Status *string `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (LLMFactories) TableName() string {
	return "llm_factories"
}

// LLM LLM model
type LLM struct {
	LLMName   string  `gorm:"column:llm_name;size:128;not null;primaryKey" json:"llm_name"`
	ModelType string  `gorm:"column:model_type;size:128;not null;index" json:"model_type"`
	FID       string  `gorm:"column:fid;size:128;not null;primaryKey" json:"fid"`
	MaxTokens int64   `gorm:"column:max_tokens;default:0" json:"max_tokens"`
	Tags      string  `gorm:"column:tags;size:255;not null;index" json:"tags"`
	IsTools   bool    `gorm:"column:is_tools;default:false" json:"is_tools"`
	Status    *string `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (LLM) TableName() string {
	return "llm"
}

// TenantLangfuse tenant langfuse model
type TenantLangfuse struct {
	TenantID  string `gorm:"column:tenant_id;primaryKey;size:32" json:"tenant_id"`
	SecretKey string `gorm:"column:secret_key;size:2048;not null;index" json:"secret_key"`
	PublicKey string `gorm:"column:public_key;size:2048;not null;index" json:"public_key"`
	Host      string `gorm:"column:host;size:128;not null;index" json:"host"`
	BaseModel
}

// TableName specify table name
func (TenantLangfuse) TableName() string {
	return "tenant_langfuse"
}

// MyLLM represents LLM information for a tenant with factory details
type MyLLM struct {
	LLMFactory string  `gorm:"column:llm_factory" json:"llm_factory"`
	Logo       *string `gorm:"column:logo" json:"logo,omitempty"`
	Tags       string  `gorm:"column:tags" json:"tags"`
	ModelType  string  `gorm:"column:model_type" json:"model_type"`
	LLMName    string  `gorm:"column:llm_name" json:"llm_name"`
	UsedTokens int64   `gorm:"column:used_tokens" json:"used_tokens"`
	Status     string  `gorm:"column:status" json:"status"`
	APIBase    string  `gorm:"column:api_base" json:"api_base,omitempty"`
	MaxTokens  int64   `gorm:"column:max_tokens" json:"max_tokens,omitempty"`
}
