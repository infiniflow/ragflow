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

// FieldWeight represents the weight configuration for a field
type FieldWeight struct {
	Enabled bool    `json:"enabled"`
	Weight  float64 `json:"weight"`
}

// FieldConfig represents the field configuration for skill indexing
type FieldConfig struct {
	Name        FieldWeight `json:"name"`
	Tags        FieldWeight `json:"tags"`
	Description FieldWeight `json:"description"`
	Content     FieldWeight `json:"content"`
}

// DefaultFieldConfig returns the default field configuration
func DefaultFieldConfig() FieldConfig {
	return FieldConfig{
		Name:        FieldWeight{Enabled: true, Weight: 3.0},
		Tags:        FieldWeight{Enabled: true, Weight: 2.0},
		Description: FieldWeight{Enabled: true, Weight: 1.0},
		Content:     FieldWeight{Enabled: false, Weight: 0.5},
	}
}

// SkillSearchConfig represents the search configuration for skills
type SkillSearchConfig struct {
	ID                     string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID               string     `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	EmbdID                 string     `gorm:"column:embd_id;size:128;not null" json:"embd_id"`
	VectorSimilarityWeight float64    `gorm:"column:vector_similarity_weight;default:0.3" json:"vector_similarity_weight"`
	SimilarityThreshold    float64    `gorm:"column:similarity_threshold;default:0.2" json:"similarity_threshold"`
	FieldConfig            JSONMap    `gorm:"column:field_config;type:json" json:"field_config"`
	RerankID               *string    `gorm:"column:rerank_id;size:128" json:"rerank_id,omitempty"`
	TenantRerankID         *int64     `gorm:"column:tenant_rerank_id" json:"tenant_rerank_id,omitempty"`
	TopK                   int64      `gorm:"column:top_k;default:10" json:"top_k"`
	IndexVersion           string     `gorm:"column:index_version;size:32;default:'1.0.0'" json:"index_version"`
	Status                 string     `gorm:"column:status;size:1;default:1" json:"status"`
	CreateTime             *int64     `gorm:"column:create_time" json:"create_time,omitempty"`
	UpdateTime             *time.Time `gorm:"column:update_time" json:"update_time,omitempty"`
}

// TableName returns the table name for SkillSearchConfig model
func (SkillSearchConfig) TableName() string {
	return "skill_search_configs"
}

// ToMap converts SkillSearchConfig to a map for JSON response
func (s *SkillSearchConfig) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"id":                       s.ID,
		"tenant_id":                s.TenantID,
		"embd_id":                  s.EmbdID,
		"vector_similarity_weight": s.VectorSimilarityWeight,
		"similarity_threshold":     s.SimilarityThreshold,
		"field_config":             s.FieldConfig,
		"top_k":                    s.TopK,
		"index_version":            s.IndexVersion,
		"status":                   s.Status,
	}

	if s.RerankID != nil {
		result["rerank_id"] = *s.RerankID
	}
	if s.TenantRerankID != nil {
		result["tenant_rerank_id"] = *s.TenantRerankID
	}
	if s.CreateTime != nil {
		result["create_time"] = s.CreateTime
	}
	if s.UpdateTime != nil {
		result["update_time"] = s.UpdateTime.Format("2006-01-02 15:04:05")
	}

	return result
}

// SkillSearchResult represents a skill search result
type SkillSearchResult struct {
	SkillID      string   `json:"skill_id"`
	FolderID     string   `json:"folder_id"` // File system folder ID for retrieving files
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	Score        float64  `json:"score"`
	BM25Score    float64  `json:"bm25_score,omitempty"`
	VectorScore  float64  `json:"vector_score,omitempty"`
	IndexVersion string   `json:"index_version,omitempty"`
}
