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

// Tenant tenant model
type Tenant struct {
	ID              string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	Name            *string `gorm:"column:name;size:100;index" json:"name,omitempty"`
	PublicKey       *string `gorm:"column:public_key;size:255;index" json:"public_key,omitempty"`
	LLMID           string  `gorm:"column:llm_id;size:128;not null;index" json:"llm_id"`
	TenantLLMID     *int64  `gorm:"column:tenant_llm_id;index" json:"tenant_llm_id,omitempty"`
	EmbdID          string  `gorm:"column:embd_id;size:128;not null;index" json:"embd_id"`
	TenantEmbdID    *int64  `gorm:"column:tenant_embd_id;index" json:"tenant_embd_id,omitempty"`
	ASRID           string  `gorm:"column:asr_id;size:128;not null;index" json:"asr_id"`
	TenantASRID     *int64  `gorm:"column:tenant_asr_id;index" json:"tenant_asr_id,omitempty"`
	Img2TxtID       string  `gorm:"column:img2txt_id;size:128;not null;index" json:"img2txt_id"`
	TenantImg2TxtID *int64  `gorm:"column:tenant_img2txt_id;index" json:"tenant_img2txt_id,omitempty"`
	RerankID        string  `gorm:"column:rerank_id;size:128;not null;index" json:"rerank_id"`
	TenantRerankID  *int64  `gorm:"column:tenant_rerank_id;index" json:"tenant_rerank_id,omitempty"`
	TTSID           *string `gorm:"column:tts_id;size:256;index" json:"tts_id,omitempty"`
	TenantTTSID     *int64  `gorm:"column:tenant_tts_id;index" json:"tenant_tts_id,omitempty"`
	ParserIDs       string  `gorm:"column:parser_ids;size:256;not null;index" json:"parser_ids"`
	OCRID           string  `gorm:"column:ocr_id;size:256;not null" json:"ocr_id"`
	TenantOCRID     *int64  `gorm:"column:tenant_ocr_id" json:"tenant_ocr_id,omitempty"`
	Credit          int64   `gorm:"column:credit;default:512;index" json:"credit"`
	Status          *string `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (Tenant) TableName() string {
	return "tenant"
}
