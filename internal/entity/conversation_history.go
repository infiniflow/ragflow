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

// ConversationMessageFields contains searchable fields and variable message metadata.
type ConversationMessageFields struct {
	MessageID   *string  `gorm:"column:message_id;size:255;index" json:"id,omitempty"`
	Role        *string  `gorm:"column:role;size:32;index" json:"role,omitempty"`
	Content     *string  `gorm:"column:content;type:longtext" json:"content,omitempty"`
	ContentType string   `gorm:"column:content_type;size:8" json:"-"`
	Status      *string  `gorm:"column:status;size:32" json:"status,omitempty"`
	ThumbUp     *bool    `gorm:"column:thumb_up" json:"thumbup,omitempty"`
	Feedback    *string  `gorm:"column:feedback;type:longtext" json:"feedback,omitempty"`
	CreatedAt   *float64 `gorm:"column:created_at" json:"created_at,omitempty"`
	Metadata    string   `gorm:"column:metadata;type:longtext" json:"-"`
}

// ConversationMessage stores one ordered message for a conversation.
type ConversationMessage struct {
	ConversationID string `gorm:"column:conversation_id;primaryKey;size:32" json:"conversation_id"`
	Position       int    `gorm:"column:position;primaryKey;autoIncrement:false" json:"position"`
	ConversationMessageFields
	Conversation ChatSession `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ConversationMessage) TableName() string {
	return "conversation_message"
}

// ConversationReference stores one ordered reference for a conversation.
type ConversationReference struct {
	ConversationID string          `gorm:"column:conversation_id;primaryKey;size:32" json:"conversation_id"`
	Position       int             `gorm:"column:position;primaryKey;autoIncrement:false" json:"position"`
	Reference      json.RawMessage `gorm:"column:reference;type:longtext;not null" json:"reference"`
	Conversation   ChatSession     `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ConversationReference) TableName() string {
	return "conversation_reference"
}

// API4ConversationMessage stores one ordered message for an API conversation.
type API4ConversationMessage struct {
	ConversationID string `gorm:"column:conversation_id;primaryKey;size:32" json:"conversation_id"`
	Position       int    `gorm:"column:position;primaryKey;autoIncrement:false" json:"position"`
	ConversationMessageFields
	Conversation API4Conversation `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
}

func (API4ConversationMessage) TableName() string {
	return "api_4_conversation_message"
}

// API4ConversationReference stores one ordered reference for an API conversation.
type API4ConversationReference struct {
	ConversationID string           `gorm:"column:conversation_id;primaryKey;size:32" json:"conversation_id"`
	Position       int              `gorm:"column:position;primaryKey;autoIncrement:false" json:"position"`
	Reference      json.RawMessage  `gorm:"column:reference;type:longtext;not null" json:"reference"`
	Conversation   API4Conversation `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
}

func (API4ConversationReference) TableName() string {
	return "api_4_conversation_reference"
}
