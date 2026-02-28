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

import "time"

// Task task model
type Task struct {
	ID              string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	DocID           string     `gorm:"column:doc_id;size:32;not null;index" json:"doc_id"`
	FromPage        int64      `gorm:"column:from_page;default:0" json:"from_page"`
	ToPage          int64      `gorm:"column:to_page;default:100000000" json:"to_page"`
	TaskType        string     `gorm:"column:task_type;size:32;not null;default:''" json:"task_type"`
	Priority        int64      `gorm:"column:priority;default:0" json:"priority"`
	BeginAt         *time.Time `gorm:"column:begin_at;index" json:"begin_at,omitempty"`
	ProcessDuration float64    `gorm:"column:process_duration;default:0" json:"process_duration"`
	Progress        float64    `gorm:"column:progress;default:0;index" json:"progress"`
	ProgressMsg     *string    `gorm:"column:progress_msg;type:longtext" json:"progress_msg,omitempty"`
	RetryCount      int64      `gorm:"column:retry_count;default:0" json:"retry_count"`
	Digest          *string    `gorm:"column:digest;type:longtext" json:"digest,omitempty"`
	ChunkIDs        *string    `gorm:"column:chunk_ids;type:longtext" json:"chunk_ids,omitempty"`
	BaseModel
}

// TableName specify table name
func (Task) TableName() string {
	return "task"
}
