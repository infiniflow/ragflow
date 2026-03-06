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

// Knowledgebase knowledge base model
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
	ParserConfig           JSONMap    `gorm:"column:parser_config;type:json;not null;default:'{\"pages\":[[1,1000000]],\"table_context_size\":0,\"image_context_size\":0}'" json:"parser_config"`
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

// TableName specify table name
func (Knowledgebase) TableName() string {
	return "knowledgebase"
}

// InvitationCode invitation code model
type InvitationCode struct {
	ID        string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	Code      string     `gorm:"column:code;size:32;not null;index" json:"code"`
	VisitTime *time.Time `gorm:"column:visit_time;index" json:"visit_time,omitempty"`
	UserID    *string    `gorm:"column:user_id;size:32;index" json:"user_id,omitempty"`
	TenantID  *string    `gorm:"column:tenant_id;size:32;index" json:"tenant_id,omitempty"`
	Status    *string    `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (InvitationCode) TableName() string {
	return "invitation_code"
}
