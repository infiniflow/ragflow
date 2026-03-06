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

// EvaluationDataset evaluation dataset model
type EvaluationDataset struct {
	ID          string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID    string  `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	Name        string  `gorm:"column:name;size:255;not null;index" json:"name"`
	Description *string `gorm:"column:description;type:longtext" json:"description,omitempty"`
	KbIDs       JSONMap `gorm:"column:kb_ids;type:json;not null" json:"kb_ids"`
	CreatedBy   string  `gorm:"column:created_by;size:32;not null;index" json:"created_by"`
	Status      int64   `gorm:"column:status;default:1;index" json:"status"`
	BaseModel
}

// TableName specify table name
func (EvaluationDataset) TableName() string {
	return "evaluation_datasets"
}

// EvaluationCase evaluation case model
type EvaluationCase struct {
	ID               string   `gorm:"column:id;primaryKey;size:32" json:"id"`
	DatasetID        string   `gorm:"column:dataset_id;size:32;not null;index" json:"dataset_id"`
	Question         string   `gorm:"column:question;type:longtext;not null" json:"question"`
	ReferenceAnswer  *string  `gorm:"column:reference_answer;type:longtext" json:"reference_answer,omitempty"`
	RelevantDocIDs   *JSONMap `gorm:"column:relevant_doc_ids;type:json" json:"relevant_doc_ids,omitempty"`
	RelevantChunkIDs *JSONMap `gorm:"column:relevant_chunk_ids;type:json" json:"relevant_chunk_ids,omitempty"`
	Metadata         *JSONMap `gorm:"column:metadata;type:json" json:"metadata,omitempty"`
	BaseModel
}

// TableName specify table name
func (EvaluationCase) TableName() string {
	return "evaluation_cases"
}

// EvaluationRun evaluation run model
type EvaluationRun struct {
	ID             string   `gorm:"column:id;primaryKey;size:32" json:"id"`
	DatasetID      string   `gorm:"column:dataset_id;size:32;not null;index" json:"dataset_id"`
	DialogID       string   `gorm:"column:dialog_id;size:32;not null;index" json:"dialog_id"`
	Name           string   `gorm:"column:name;size:255;not null" json:"name"`
	ConfigSnapshot JSONMap  `gorm:"column:config_snapshot;type:json;not null" json:"config_snapshot"`
	MetricsSummary *JSONMap `gorm:"column:metrics_summary;type:json" json:"metrics_summary,omitempty"`
	Status         string   `gorm:"column:status;size:32;not null;default:PENDING" json:"status"`
	CreatedBy      string   `gorm:"column:created_by;size:32;not null;index" json:"created_by"`
	BaseModel
}

// TableName specify table name
func (EvaluationRun) TableName() string {
	return "evaluation_runs"
}

// EvaluationResult evaluation result model
type EvaluationResult struct {
	ID              string   `gorm:"column:id;primaryKey;size:32" json:"id"`
	RunID           string   `gorm:"column:run_id;size:32;not null;index" json:"run_id"`
	CaseID          string   `gorm:"column:case_id;size:32;not null;index" json:"case_id"`
	GeneratedAnswer string   `gorm:"column:generated_answer;type:longtext;not null" json:"generated_answer"`
	RetrievedChunks JSONMap  `gorm:"column:retrieved_chunks;type:json;not null" json:"retrieved_chunks"`
	Metrics         JSONMap  `gorm:"column:metrics;type:json;not null" json:"metrics"`
	ExecutionTime   float64  `gorm:"column:execution_time;not null" json:"execution_time"`
	TokenUsage      *JSONMap `gorm:"column:token_usage;type:json" json:"token_usage,omitempty"`
	BaseModel
}

// TableName specify table name
func (EvaluationResult) TableName() string {
	return "evaluation_results"
}
