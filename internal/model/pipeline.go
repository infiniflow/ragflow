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

// PipelineOperationLog pipeline operation log model
type PipelineOperationLog struct {
	ID              string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	DocumentID      string     `gorm:"column:document_id;size:32;index" json:"document_id"`
	TenantID        string     `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	KbID            string     `gorm:"column:kb_id;size:32;not null;index" json:"kb_id"`
	PipelineID      *string    `gorm:"column:pipeline_id;size:32;index" json:"pipeline_id,omitempty"`
	PipelineTitle   *string    `gorm:"column:pipeline_title;size:32;index" json:"pipeline_title,omitempty"`
	ParserID        string     `gorm:"column:parser_id;size:32;not null;index" json:"parser_id"`
	DocumentName    string     `gorm:"column:document_name;size:255;not null" json:"document_name"`
	DocumentSuffix  string     `gorm:"column:document_suffix;size:255;not null" json:"document_suffix"`
	DocumentType    string     `gorm:"column:document_type;size:255;not null" json:"document_type"`
	SourceFrom      string     `gorm:"column:source_from;size:255;not null" json:"source_from"`
	Progress        float64    `gorm:"column:progress;default:0;index" json:"progress"`
	ProgressMsg     *string    `gorm:"column:progress_msg;type:longtext" json:"progress_msg,omitempty"`
	ProcessBeginAt  *time.Time `gorm:"column:process_begin_at;index" json:"process_begin_at,omitempty"`
	ProcessDuration float64    `gorm:"column:process_duration;default:0" json:"process_duration"`
	DSL             JSONMap    `gorm:"column:dsl;type:json" json:"dsl,omitempty"`
	TaskType        string     `gorm:"column:task_type;size:32;not null;default:''" json:"task_type"`
	OperationStatus string     `gorm:"column:operation_status;size:32;not null" json:"operation_status"`
	Avatar          *string    `gorm:"column:avatar;type:longtext" json:"avatar,omitempty"`
	Status          *string    `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (PipelineOperationLog) TableName() string {
	return "pipeline_operation_log"
}
