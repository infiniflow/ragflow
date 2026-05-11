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

// DatasetNameLimit is the maximum length for dataset name
const DatasetNameLimit = 128

// Status represents the status enum values
type Status string

const (
	// StatusValid indicates a valid/active record
	StatusValid Status = "1"
	// StatusInvalid indicates a deleted/inactive record
	StatusInvalid Status = "0"
)

// TenantPermission represents the permission level for tenant access
type TenantPermission string

const (
	// TenantPermissionMe indicates only the creator can access
	TenantPermissionMe TenantPermission = "me"
	// TenantPermissionTeam indicates all team members can access
	TenantPermissionTeam TenantPermission = "team"
)

// ParserType represents the document parser type
type ParserType string

const (
	ParserTypePresentation ParserType = "presentation"
	ParserTypeLaws         ParserType = "laws"
	ParserTypeManual       ParserType = "manual"
	ParserTypePaper        ParserType = "paper"
	ParserTypeResume       ParserType = "resume"
	ParserTypeBook         ParserType = "book"
	ParserTypeQA           ParserType = "qa"
	ParserTypeTable        ParserType = "table"
	ParserTypeNaive        ParserType = "naive"
	ParserTypePicture      ParserType = "picture"
	ParserTypeOne          ParserType = "one"
	ParserTypeAudio        ParserType = "audio"
	ParserTypeEmail        ParserType = "email"
	ParserTypeKG           ParserType = "knowledge_graph"
	ParserTypeTag          ParserType = "tag"
)

// TaskStatus represents the status of a processing task
type TaskStatus string

const (
	TaskStatusUnstart  TaskStatus = "0"
	TaskStatusRunning  TaskStatus = "1"
	TaskStatusCancel   TaskStatus = "2"
	TaskStatusDone     TaskStatus = "3"
	TaskStatusFail     TaskStatus = "4"
	TaskStatusSchedule TaskStatus = "5"
)

// PipelineTaskType represents the type of pipeline task
type PipelineTaskType string

const (
	PipelineTaskTypeParse    PipelineTaskType = "Parse"
	PipelineTaskTypeDownload PipelineTaskType = "Download"
	PipelineTaskTypeRAPTOR   PipelineTaskType = "RAPTOR"
	PipelineTaskTypeGraphRAG PipelineTaskType = "GraphRAG"
	PipelineTaskTypeMindmap  PipelineTaskType = "Mindmap"
	PipelineTaskTypeMemory   PipelineTaskType = "Memory"
)

// FileSource represents the source of a file
type FileSource string

const (
	FileSourceLocal         FileSource = ""
	FileSourceKnowledgebase FileSource = "knowledgebase"
	FileSourceS3            FileSource = "s3"
)

// Knowledgebase represents the knowledge base model
type Knowledgebase struct {
	ID                     string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	Avatar                 *string    `gorm:"column:avatar;type:longtext" json:"avatar,omitempty"`
	TenantID               string     `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	Name                   string     `gorm:"column:name;size:128;not null;index" json:"name"`
	Language               *string    `gorm:"column:language;size:32;index" json:"language,omitempty"`
	Description            *string    `gorm:"column:description;type:longtext" json:"description,omitempty"`
	EmbdID                 string     `gorm:"column:embd_id;size:128;not null;index" json:"embd_id"`
	Permission             string     `gorm:"column:permission;size:16;not null;default:me;index" json:"permission"`
	CreatedBy              string     `gorm:"column:created_by;size:32;not null;index" json:"created_by"`
	DocNum                 int64      `gorm:"column:doc_num;default:0;index" json:"doc_num"`
	TokenNum               int64      `gorm:"column:token_num;default:0;index" json:"token_num"`
	ChunkNum               int64      `gorm:"column:chunk_num;default:0;index" json:"chunk_num"`
	SimilarityThreshold    float64    `gorm:"column:similarity_threshold;default:0.2;index" json:"similarity_threshold"`
	VectorSimilarityWeight float64    `gorm:"column:vector_similarity_weight;default:0.3;index" json:"vector_similarity_weight"`
	ParserID               string     `gorm:"column:parser_id;size:32;not null;default:naive;index" json:"parser_id"`
	PipelineID             *string    `gorm:"column:pipeline_id;size:32;index" json:"pipeline_id,omitempty"`
	ParserConfig           JSONMap    `gorm:"column:parser_config;type:json" json:"parser_config"`
	Pagerank               int64      `gorm:"column:pagerank;default:0" json:"pagerank"`
	GraphragTaskID         *string    `gorm:"column:graphrag_task_id;size:32;index" json:"graphrag_task_id,omitempty"`
	GraphragTaskFinishAt   *time.Time `gorm:"column:graphrag_task_finish_at" json:"graphrag_task_finish_at,omitempty"`
	RaptorTaskID           *string    `gorm:"column:raptor_task_id;size:32;index" json:"raptor_task_id,omitempty"`
	RaptorTaskFinishAt     *time.Time `gorm:"column:raptor_task_finish_at" json:"raptor_task_finish_at,omitempty"`
	MindmapTaskID          *string    `gorm:"column:mindmap_task_id;size:32;index" json:"mindmap_task_id,omitempty"`
	MindmapTaskFinishAt    *time.Time `gorm:"column:mindmap_task_finish_at" json:"mindmap_task_finish_at,omitempty"`
	Status                 *string    `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName returns the table name for Knowledgebase model
func (Knowledgebase) TableName() string {
	return "knowledgebase"
}

// ToMap converts Knowledgebase to a map for JSON response
func (kb *Knowledgebase) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"id":                       kb.ID,
		"tenant_id":                kb.TenantID,
		"name":                     kb.Name,
		"embd_id":                  kb.EmbdID,
		"permission":               kb.Permission,
		"created_by":               kb.CreatedBy,
		"doc_num":                  kb.DocNum,
		"token_num":                kb.TokenNum,
		"chunk_num":                kb.ChunkNum,
		"similarity_threshold":     kb.SimilarityThreshold,
		"vector_similarity_weight": kb.VectorSimilarityWeight,
		"parser_id":                kb.ParserID,
		"parser_config":            kb.ParserConfig,
		"pagerank":                 kb.Pagerank,
		"create_time":              kb.CreateTime,
	}

	if kb.Avatar != nil {
		result["avatar"] = *kb.Avatar
	}
	if kb.Language != nil {
		result["language"] = *kb.Language
	}
	if kb.Description != nil {
		result["description"] = *kb.Description
	}
	if kb.PipelineID != nil {
		result["pipeline_id"] = *kb.PipelineID
	}
	if kb.GraphragTaskID != nil {
		result["graphrag_task_id"] = *kb.GraphragTaskID
	}
	if kb.GraphragTaskFinishAt != nil {
		result["graphrag_task_finish_at"] = kb.GraphragTaskFinishAt.Format("2006-01-02 15:04:05")
	}
	if kb.RaptorTaskID != nil {
		result["raptor_task_id"] = *kb.RaptorTaskID
	}
	if kb.RaptorTaskFinishAt != nil {
		result["raptor_task_finish_at"] = kb.RaptorTaskFinishAt.Format("2006-01-02 15:04:05")
	}
	if kb.MindmapTaskID != nil {
		result["mindmap_task_id"] = *kb.MindmapTaskID
	}
	if kb.MindmapTaskFinishAt != nil {
		result["mindmap_task_finish_at"] = kb.MindmapTaskFinishAt.Format("2006-01-02 15:04:05")
	}
	if kb.UpdateTime != nil {
		result["update_time"] = *kb.UpdateTime
	}

	return result
}

// KnowledgebaseDetail represents detailed knowledge base information with joined data
type KnowledgebaseDetail struct {
	ID                   string   `json:"id"`
	EmbdID               string   `json:"embd_id"`
	Avatar               *string  `json:"avatar,omitempty"`
	Name                 string   `json:"name"`
	Language             *string  `json:"language,omitempty"`
	Description          *string  `json:"description,omitempty"`
	Permission           string   `json:"permission"`
	DocNum               int64    `json:"doc_num"`
	TokenNum             int64    `json:"token_num"`
	ChunkNum             int64    `json:"chunk_num"`
	ParserID             string   `json:"parser_id"`
	PipelineID           *string  `json:"pipeline_id,omitempty"`
	PipelineName         *string  `json:"pipeline_name,omitempty"`
	PipelineAvatar       *string  `json:"pipeline_avatar,omitempty"`
	ParserConfig         JSONMap  `json:"parser_config"`
	Pagerank             int64    `json:"pagerank"`
	GraphragTaskID       *string  `json:"graphrag_task_id,omitempty"`
	GraphragTaskFinishAt *string  `json:"graphrag_task_finish_at,omitempty"`
	RaptorTaskID         *string  `json:"raptor_task_id,omitempty"`
	RaptorTaskFinishAt   *string  `json:"raptor_task_finish_at,omitempty"`
	MindmapTaskID        *string  `json:"mindmap_task_id,omitempty"`
	MindmapTaskFinishAt  *string  `json:"mindmap_task_finish_at,omitempty"`
	CreateTime           *int64   `json:"create_time,omitempty"`
	UpdateTime           *int64   `json:"update_time,omitempty"`
	Size                 int64    `json:"size"`
	Connectors           []string `json:"connectors"`
}

// KnowledgebaseListItem represents a knowledge base item in list responses
type KnowledgebaseListItem struct {
	ID           string  `json:"id"`
	Avatar       *string `json:"avatar,omitempty"`
	Name         string  `json:"name"`
	Language     *string `json:"language,omitempty"`
	Description  *string `json:"description,omitempty"`
	TenantID     string  `json:"tenant_id"`
	Permission   string  `json:"permission"`
	DocNum       int64   `json:"doc_num"`
	TokenNum     int64   `json:"token_num"`
	ChunkNum     int64   `json:"chunk_num"`
	ParserID     string  `json:"parser_id"`
	EmbdID       string  `json:"embd_id"`
	Nickname     string  `json:"nickname"`
	TenantAvatar *string `json:"tenant_avatar,omitempty"`
	UpdateTime   *int64  `json:"update_time,omitempty"`
}

// InvitationCode represents the invitation code model
type InvitationCode struct {
	ID        string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	Code      string     `gorm:"column:code;size:32;not null;index" json:"code"`
	VisitTime *time.Time `gorm:"column:visit_time;index" json:"visit_time,omitempty"`
	UserID    *string    `gorm:"column:user_id;size:32;index" json:"user_id,omitempty"`
	TenantID  *string    `gorm:"column:tenant_id;size:32;index" json:"tenant_id,omitempty"`
	Status    *string    `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName returns the table name for InvitationCode model
func (InvitationCode) TableName() string {
	return "invitation_code"
}
