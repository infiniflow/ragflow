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

// Document document model
type Document struct {
	ID              string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	Thumbnail       *string    `gorm:"column:thumbnail;type:longtext" json:"thumbnail,omitempty"`
	KbID            string     `gorm:"column:kb_id;size:256;not null;index" json:"kb_id"`
	ParserID        string     `gorm:"column:parser_id;size:32;not null;index" json:"parser_id"`
	PipelineID      *string    `gorm:"column:pipeline_id;size:32;index" json:"pipeline_id,omitempty"`
	ParserConfig    JSONMap    `gorm:"column:parser_config;type:json;not null;default:'{\"pages\":[[1,1000000]],\"table_context_size\":0,\"image_context_size\":0}'" json:"parser_config"`
	SourceType      string     `gorm:"column:source_type;size:128;not null;default:local;index" json:"source_type"`
	Type            string     `gorm:"column:type;size:32;not null;index" json:"type"`
	CreatedBy       string     `gorm:"column:created_by;size:32;not null;index" json:"created_by"`
	Name            *string    `gorm:"column:name;size:255;index" json:"name,omitempty"`
	Location        *string    `gorm:"column:location;size:255;index" json:"location,omitempty"`
	Size            int64      `gorm:"column:size;default:0;index" json:"size"`
	TokenNum        int64      `gorm:"column:token_num;default:0;index" json:"token_num"`
	ChunkNum        int64      `gorm:"column:chunk_num;default:0;index" json:"chunk_num"`
	Progress        float64    `gorm:"column:progress;default:0;index" json:"progress"`
	ProgressMsg     *string    `gorm:"column:progress_msg;type:longtext" json:"progress_msg,omitempty"`
	ProcessBeginAt  *time.Time `gorm:"column:process_begin_at;index" json:"process_begin_at,omitempty"`
	ProcessDuration float64    `gorm:"column:process_duration;default:0" json:"process_duration"`
	MetaFields      *JSONMap   `gorm:"column:meta_fields;type:json" json:"meta_fields,omitempty"`
	Suffix          string     `gorm:"column:suffix;size:32;not null;index" json:"suffix"`
	Run             *string    `gorm:"column:run;size:1;index" json:"run,omitempty"`
	Status          *string    `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (Document) TableName() string {
	return "document"
}
