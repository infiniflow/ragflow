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

import (
	"encoding/json"
	"time"
)

// Connector connector model
type Connector struct {
	ID            string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID      string     `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	Name          string     `gorm:"column:name;size:128;not null" json:"name"`
	Source        string     `gorm:"column:source;size:128;not null;index" json:"source"`
	InputType     string     `gorm:"column:input_type;size:128;not null;index" json:"input_type"`
	Config        JSONMap    `gorm:"column:config;type:longtext;not null" json:"config"`
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

// MarshalJSON formats connector timestamps to match the Python API contract.
func (c Connector) MarshalJSON() ([]byte, error) {
	type connectorJSON struct {
		ID            string  `json:"id"`
		TenantID      string  `json:"tenant_id"`
		Name          string  `json:"name"`
		Source        string  `json:"source"`
		InputType     string  `json:"input_type"`
		Config        JSONMap `json:"config"`
		RefreshFreq   int64   `json:"refresh_freq"`
		PruneFreq     int64   `json:"prune_freq"`
		TimeoutSecs   int64   `json:"timeout_secs"`
		IndexingStart *string `json:"indexing_start"`
		Status        string  `json:"status"`
		CreateTime    *int64  `json:"create_time,omitempty"`
		CreateDate    *string `json:"create_date,omitempty"`
		UpdateTime    *int64  `json:"update_time,omitempty"`
		UpdateDate    *string `json:"update_date,omitempty"`
	}

	return json.Marshal(connectorJSON{
		ID:            c.ID,
		TenantID:      c.TenantID,
		Name:          c.Name,
		Source:        c.Source,
		InputType:     c.InputType,
		Config:        c.Config,
		RefreshFreq:   c.RefreshFreq,
		PruneFreq:     c.PruneFreq,
		TimeoutSecs:   c.TimeoutSecs,
		IndexingStart: formatConnectorTime(c.IndexingStart),
		Status:        c.Status,
		CreateTime:    c.CreateTime,
		CreateDate:    formatConnectorTime(c.CreateDate),
		UpdateTime:    c.UpdateTime,
		UpdateDate:    formatConnectorTime(c.UpdateDate),
	})
}

func formatConnectorTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format("2006-01-02T15:04:05")
	return &formatted
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
	TaskType             string     `gorm:"column:task_type;size:32;not null;default:sync;index" json:"task_type"`
	Status               string     `gorm:"column:status;size:128;not null;index" json:"status"`
	FromBeginning        *string    `gorm:"column:from_beginning;size:1" json:"from_beginning,omitempty"`
	NewDocsIndexed       int64      `gorm:"column:new_docs_indexed;default:0" json:"new_docs_indexed"`
	TotalDocsIndexed     int64      `gorm:"column:total_docs_indexed;default:0" json:"total_docs_indexed"`
	DocsRemovedFromIndex int64      `gorm:"column:docs_removed_from_index;default:0" json:"docs_removed_from_index"`
	ErrorMsg             string     `gorm:"column:error_msg;type:longtext;not null" json:"error_msg"`
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

// ConnectorSyncLog is the API projection used by the connector logs endpoint.
type ConnectorSyncLog struct {
	ID                   string     `gorm:"column:id" json:"id"`
	ConnectorID          string     `gorm:"column:connector_id" json:"connector_id"`
	TaskType             string     `gorm:"column:task_type" json:"task_type"`
	KbID                 string     `gorm:"column:kb_id" json:"kb_id"`
	UpdateDate           *time.Time `gorm:"column:update_date" json:"update_date,omitempty"`
	NewDocsIndexed       int64      `gorm:"column:new_docs_indexed" json:"new_docs_indexed"`
	TotalDocsIndexed     int64      `gorm:"column:total_docs_indexed" json:"total_docs_indexed"`
	DocsRemovedFromIndex int64      `gorm:"column:docs_removed_from_index" json:"docs_removed_from_index"`
	ErrorMsg             string     `gorm:"column:error_msg" json:"error_msg"`
	ErrorCount           int64      `gorm:"column:error_count" json:"error_count"`
	TimeStarted          *time.Time `gorm:"column:time_started" json:"time_started,omitempty"`
	RefreshFreq          int64      `gorm:"column:refresh_freq" json:"refresh_freq"`
	PruneFreq            int64      `gorm:"column:prune_freq" json:"prune_freq"`
	KbName               string     `gorm:"column:kb_name" json:"kb_name"`
	Status               string     `gorm:"column:status" json:"status"`
}

// MarshalJSON formats datetime fields to match the Python API encoder.
func (c ConnectorSyncLog) MarshalJSON() ([]byte, error) {
	type connectorSyncLogJSON struct {
		ID                   string  `json:"id"`
		ConnectorID          string  `json:"connector_id"`
		TaskType             string  `json:"task_type"`
		KbID                 string  `json:"kb_id"`
		UpdateDate           *string `json:"update_date,omitempty"`
		NewDocsIndexed       int64   `json:"new_docs_indexed"`
		TotalDocsIndexed     int64   `json:"total_docs_indexed"`
		DocsRemovedFromIndex int64   `json:"docs_removed_from_index"`
		ErrorMsg             string  `json:"error_msg"`
		ErrorCount           int64   `json:"error_count"`
		TimeStarted          *string `json:"time_started,omitempty"`
		RefreshFreq          int64   `json:"refresh_freq"`
		PruneFreq            int64   `json:"prune_freq"`
		KbName               string  `json:"kb_name"`
		Status               string  `json:"status"`
	}

	return json.Marshal(connectorSyncLogJSON{
		ID:                   c.ID,
		ConnectorID:          c.ConnectorID,
		TaskType:             c.TaskType,
		KbID:                 c.KbID,
		UpdateDate:           formatConnectorLogTime(c.UpdateDate),
		NewDocsIndexed:       c.NewDocsIndexed,
		TotalDocsIndexed:     c.TotalDocsIndexed,
		DocsRemovedFromIndex: c.DocsRemovedFromIndex,
		ErrorMsg:             c.ErrorMsg,
		ErrorCount:           c.ErrorCount,
		TimeStarted:          formatConnectorLogTime(c.TimeStarted),
		RefreshFreq:          c.RefreshFreq,
		PruneFreq:            c.PruneFreq,
		KbName:               c.KbName,
		Status:               c.Status,
	})
}

func formatConnectorLogTime(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.Format("2006-01-02 15:04:05")
	return &formatted
}
