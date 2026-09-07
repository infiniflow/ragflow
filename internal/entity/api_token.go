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

// APIToken API token model
type APIToken struct {
	TenantID string  `gorm:"column:tenant_id;size:32;not null;primaryKey" json:"tenant_id"`
	Token    string  `gorm:"column:token;size:255;not null;primaryKey" json:"token"`
	DialogID *string `gorm:"column:dialog_id;size:32;index" json:"dialog_id,omitempty"`
	Source   *string `gorm:"column:source;size:16;index" json:"source,omitempty"`
	Beta     *string `gorm:"column:beta;size:255;index" json:"beta,omitempty"`
	BaseModel
}

// TableName specify table name
func (APIToken) TableName() string {
	return "api_token"
}

// API4Conversation API for conversation model
type API4Conversation struct {
	ID           string          `gorm:"column:id;primaryKey;size:32" json:"id"`
	Name         *string         `gorm:"column:name;size:255" json:"name,omitempty"`
	DialogID     string          `gorm:"column:dialog_id;size:32;not null;index" json:"dialog_id"`
	UserID       string          `gorm:"column:user_id;size:255;not null;index" json:"user_id"`
	ExpUserID    *string         `gorm:"column:exp_user_id;size:255;index" json:"exp_user_id,omitempty"`
	Message      json.RawMessage `gorm:"column:message;type:longtext" json:"message,omitempty"`
	Reference    json.RawMessage `gorm:"column:reference;type:longtext" json:"reference,omitempty"`
	Tokens       int             `gorm:"column:tokens" json:"tokens"`
	Source       *string         `gorm:"column:source;size:16" json:"source,omitempty"`
	DSL          JSONMap         `gorm:"column:dsl;type:longtext" json:"dsl,omitempty"`
	Duration     float64         `gorm:"column:duration" json:"duration"`
	Round        int             `gorm:"column:round" json:"round"`
	ThumbUp      int             `gorm:"column:thumb_up" json:"thumb_up"`
	Errors       *string         `gorm:"column:errors;type:text" json:"errors,omitempty"`
	VersionTitle *string         `gorm:"column:version_title;size:255" json:"version_title,omitempty"`
	BaseModel
}

// TableName specify table name
func (API4Conversation) TableName() string {
	return "api_4_conversation"
}
