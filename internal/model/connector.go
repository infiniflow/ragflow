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

// Connector connector model
type Connector struct {
	ID            string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID      string     `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	Name          string     `gorm:"column:name;size:128;not null" json:"name"`
	Source        string     `gorm:"column:source;size:128;not null;index" json:"source"`
	InputType     string     `gorm:"column:input_type;size:128;not null;index" json:"input_type"`
	Config        JSONMap    `gorm:"column:config;type:json;not null;default:'{}'" json:"config"`
	RefreshFreq   int64      `gorm:"column:refresh_freq;default:0" json:"refresh_freq"`
	PruneFreq     int64      `gorm:"column:prune_freq;default:0" json:"prune_freq"`
	TimeoutSecs   int64      `gorm:"column:timeout_secs;default:3600" json:"timeout_secs"`
	IndexingStart *time.Time `gorm:"column:indexing_start;index" json:"indexing_start,omitempty"`
	Status        string     `gorm:"column:status;size:16;not null;default:schedule;index" json:"status"`
	BaseModel
}

// TableName specify table name
func (Connector) TableName() string {
	return "connector"
}

// Connector2Kb connector to knowledge base mapping model
type Connector2Kb struct {
	ID          string `gorm:"column:id;primaryKey;size:32" json:"id"`
	ConnectorID string `gorm:"column:connector_id;size:32;not null;index" json:"connector_id"`
	KbID        string `gorm:"column:kb_id;size:32;not null;index" json:"kb_id"`
	AutoParse   string `gorm:"column:auto_parse;size:1;not null;default:1" json:"auto_parse"`
	BaseModel
}

// TableName specify table name
func (Connector2Kb) TableName() string {
	return "connector2kb"
}

// SyncLogs sync logs model
type SyncLogs struct {
	ID                   string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	ConnectorID          string     `gorm:"column:connector_id;size:32;index" json:"connector_id"`
	Status               string     `gorm:"column:status;size:128;not null;index" json:"status"`
	FromBeginning        *string    `gorm:"column:from_beginning;size:1" json:"from_beginning,omitempty"`
	NewDocsIndexed       int64      `gorm:"column:new_docs_indexed;default:0" json:"new_docs_indexed"`
	TotalDocsIndexed     int64      `gorm:"column:total_docs_indexed;default:0" json:"total_docs_indexed"`
	DocsRemovedFromIndex int64      `gorm:"column:docs_removed_from_index;default:0" json:"docs_removed_from_index"`
	ErrorMsg             string     `gorm:"column:error_msg;type:longtext;not null;default:''" json:"error_msg"`
	ErrorCount           int64      `gorm:"column:error_count;default:0" json:"error_count"`
	FullExceptionTrace   *string    `gorm:"column:full_exception_trace;type:longtext" json:"full_exception_trace,omitempty"`
	TimeStarted          *time.Time `gorm:"column:time_started;index" json:"time_started,omitempty"`
	PollRangeStart       *string    `gorm:"column:poll_range_start;size:255;index" json:"poll_range_start,omitempty"`
	PollRangeEnd         *string    `gorm:"column:poll_range_end;size:255;index" json:"poll_range_end,omitempty"`
	KbID                 string     `gorm:"column:kb_id;size:32;not null;index" json:"kb_id"`
	BaseModel
}

// TableName specify table name
func (SyncLogs) TableName() string {
	return "sync_logs"
}
