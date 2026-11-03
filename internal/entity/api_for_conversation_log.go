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

// API4ConversationLog preserves an execution record independently of its chat session.
type API4ConversationLog struct {
	ID           string     `gorm:"column:id;primaryKey;size:32"`
	Name         *string    `gorm:"column:name;size:255"`
	DialogID     string     `gorm:"column:dialog_id;size:32;not null;index"`
	UserID       string     `gorm:"column:user_id;size:255;not null;index"`
	ExpUserID    *string    `gorm:"column:exp_user_id;size:255;index"`
	Message      string     `gorm:"column:message;type:longtext"`
	Tokens       int        `gorm:"column:tokens"`
	Source       *string    `gorm:"column:source;size:16"`
	DSL          JSONMap    `gorm:"column:dsl;type:longtext"`
	Duration     float64    `gorm:"column:duration"`
	Round        int        `gorm:"column:round"`
	ThumbUp      int        `gorm:"column:thumb_up"`
	Errors       *string    `gorm:"column:errors;type:text"`
	VersionTitle *string    `gorm:"column:version_title;size:255"`
	CreateTime   *int64     `gorm:"column:create_time;index"`
	CreateDate   *time.Time `gorm:"column:create_date;index"`
	UpdateTime   *int64     `gorm:"column:update_time;index"`
	UpdateDate   *time.Time `gorm:"column:update_date;index"`
}

func (API4ConversationLog) TableName() string { return "api_4_conversation_log" }
