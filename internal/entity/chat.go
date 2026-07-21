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

import "encoding/json"

// Chat chat model (mapped to dialog table)
type Chat struct {
	ID             string   `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID       string   `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	Name           *string  `gorm:"column:name;size:255;index" json:"name,omitempty"`
	Description    *string  `gorm:"column:description;type:longtext" json:"description,omitempty"`
	Icon           *string  `gorm:"column:icon;type:longtext" json:"icon,omitempty"`
	Language       *string  `gorm:"column:language;size:32;index" json:"language,omitempty"`
	LLMID          string   `gorm:"column:llm_id;size:128;not null" json:"llm_id"`
	TenantLLMID    *string  `gorm:"column:tenant_llm_id;size:32;index" json:"tenant_llm_id,omitempty"`
	LLMSetting     JSONMap  `gorm:"column:llm_setting;type:longtext;not null" json:"llm_setting"`
	PromptType     string   `gorm:"column:prompt_type;size:16;not null;default:'simple';index" json:"prompt_type"`
	PromptConfig   JSONMap  `gorm:"column:prompt_config;type:longtext;not null" json:"prompt_config"`
	MetaDataFilter *JSONMap `gorm:"column:meta_data_filter;type:longtext" json:"meta_data_filter,omitempty"`
	// NOTE: No `default:` GORM tags here. The service layer (chat.go Create)
	// supplies sensible defaults (0.1 / 0.3 / 6 / 1024 / "1") when a field is
	// omitted, and honors an explicitly provided zero value. A GORM `default:`
	// tag would force GORM to overwrite an explicit zero (e.g.
	// similarity_threshold=0) with the column default during Create, breaking
	// the API contract that permits 0.
	SimilarityThreshold    float64   `gorm:"column:similarity_threshold" json:"similarity_threshold"`
	VectorSimilarityWeight float64   `gorm:"column:vector_similarity_weight" json:"vector_similarity_weight"`
	TopN                   int64     `gorm:"column:top_n" json:"top_n"`
	TopK                   int64     `gorm:"column:top_k" json:"top_k"`
	DoRefer                string    `gorm:"column:do_refer;size:1;not null" json:"do_refer"`
	RerankID               string    `gorm:"column:rerank_id;size:128;not null;default:''" json:"rerank_id"`
	TenantRerankID         *string   `gorm:"column:tenant_rerank_id;size:32;index" json:"tenant_rerank_id,omitempty"`
	KBIDs                  JSONSlice `gorm:"column:kb_ids;type:longtext;not null" json:"kb_ids"`
	Status                 *string   `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (Chat) TableName() string {
	return "dialog"
}

// Conversation conversation model
type ChatSession struct {
	ID        string          `gorm:"column:id;primaryKey;size:32" json:"id"`
	DialogID  string          `gorm:"column:dialog_id;size:32;not null;index" json:"dialog_id"`
	Name      *string         `gorm:"column:name;size:255;index" json:"name,omitempty"`
	Message   json.RawMessage `gorm:"column:message;type:longtext" json:"message,omitempty"`
	Reference json.RawMessage `gorm:"column:reference;type:longtext" json:"reference"`
	UserID    *string         `gorm:"column:user_id;size:255;index" json:"user_id,omitempty"`
	BaseModel
}

// TableName specify table name
func (ChatSession) TableName() string {
	return "conversation"
}
