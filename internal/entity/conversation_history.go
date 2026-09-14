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

// ConversationMessage stores one ordered message for a conversation.
type ConversationMessage struct {
	ConversationID string          `gorm:"column:conversation_id;primaryKey;size:32" json:"conversation_id"`
	Position       int             `gorm:"column:position;primaryKey;autoIncrement:false" json:"position"`
	Message        json.RawMessage `gorm:"column:message;type:longtext;not null" json:"message"`
	Conversation   ChatSession     `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
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
	ConversationID string           `gorm:"column:conversation_id;primaryKey;size:32" json:"conversation_id"`
	Position       int              `gorm:"column:position;primaryKey;autoIncrement:false" json:"position"`
	Message        json.RawMessage  `gorm:"column:message;type:longtext;not null" json:"message"`
	Conversation   API4Conversation `gorm:"foreignKey:ConversationID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
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
