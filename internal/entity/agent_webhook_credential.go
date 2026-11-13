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

import "time"

// AgentWebhookCredential binds an external webhook capability to the user and
// tenant whose permissions and model connections must be used for execution.
// Only a one-way hash of the credential is persisted.
type AgentWebhookCredential struct {
	ID        string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	CanvasID  string     `gorm:"column:canvas_id;size:32;not null;uniqueIndex:idx_agent_webhook_credential_identity,priority:1" json:"canvas_id"`
	UserID    string     `gorm:"column:user_id;size:32;not null;uniqueIndex:idx_agent_webhook_credential_identity,priority:2" json:"user_id"`
	TenantID  string     `gorm:"column:tenant_id;size:32;not null;uniqueIndex:idx_agent_webhook_credential_identity,priority:3" json:"tenant_id"`
	TokenHash string     `gorm:"column:token_hash;size:64;not null;uniqueIndex:idx_agent_webhook_credential_hash" json:"-"`
	RevokedAt *time.Time `gorm:"column:revoked_at" json:"revoked_at,omitempty"`
	BaseModel
}

// TableName returns the persistence table for AgentWebhookCredential.
func (AgentWebhookCredential) TableName() string {
	return "agent_webhook_credential"
}
