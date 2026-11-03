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

const (
	TenantMembershipStatusRemoved   = "0"
	TenantMembershipStatusJoined    = "1"
	TenantMembershipStatusInvited   = "2"
	TenantMembershipStatusRejected  = "3"
	TenantMembershipStatusRequested = "4"
	TenantMembershipStatusLeft      = "5"
)

// UserTenant user tenant relationship model
type UserTenant struct {
	ID               string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	UserID           string  `gorm:"column:user_id;size:32;not null;index;uniqueIndex:idx_user_tenant_user_tenant,priority:1" json:"user_id"`
	TenantID         string  `gorm:"column:tenant_id;size:32;not null;index;uniqueIndex:idx_user_tenant_user_tenant,priority:2" json:"tenant_id"`
	Role             string  `gorm:"column:role;size:32;not null;index" json:"role"`
	InvitedBy        string  `gorm:"column:invited_by;size:32;index" json:"invited_by"`
	Status           *string `gorm:"column:status;size:16;index;default:'1'" json:"status,omitempty"`
	LLMID            string  `gorm:"column:llm_id;size:128;not null;default:''" json:"llm_id"`
	TenantLLMID      *string `gorm:"column:tenant_llm_id;size:32" json:"tenant_llm_id,omitempty"`
	EmbdID           string  `gorm:"column:embd_id;size:128;not null;default:''" json:"embd_id"`
	TenantEmbdID     *string `gorm:"column:tenant_embd_id;size:32" json:"tenant_embd_id,omitempty"`
	ASRID            string  `gorm:"column:asr_id;size:128;not null;default:''" json:"asr_id"`
	TenantASRID      *string `gorm:"column:tenant_asr_id;size:32" json:"tenant_asr_id,omitempty"`
	Img2TxtID        string  `gorm:"column:img2txt_id;size:128;not null;default:''" json:"img2txt_id"`
	TenantImg2TxtID  *string `gorm:"column:tenant_img2txt_id;size:32" json:"tenant_img2txt_id,omitempty"`
	RerankID         string  `gorm:"column:rerank_id;size:128;not null;default:''" json:"rerank_id"`
	TenantRerankID   *string `gorm:"column:tenant_rerank_id;size:32" json:"tenant_rerank_id,omitempty"`
	TTSID            *string `gorm:"column:tts_id;size:256" json:"tts_id,omitempty"`
	TenantTTSID      *string `gorm:"column:tenant_tts_id;size:32" json:"tenant_tts_id,omitempty"`
	OCRID            *string `gorm:"column:ocr_id;size:256" json:"ocr_id,omitempty"`
	TenantOCRID      *string `gorm:"column:tenant_ocr_id;size:32" json:"tenant_ocr_id,omitempty"`
	DecisionID       *string `gorm:"column:decision_id;size:256" json:"decision_id,omitempty"`
	TenantDecisionID *string `gorm:"column:tenant_decision_id;size:32" json:"tenant_decision_id,omitempty"`
	BaseModel
}

// TableName specify table name
func (UserTenant) TableName() string {
	return "user_tenant"
}
