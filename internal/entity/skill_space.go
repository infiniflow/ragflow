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

// Space status constants
const (
	SpaceStatusActive   = "1" // Normal active space
	SpaceStatusDeleted  = "0" // Soft-deleted space
	SpaceStatusDeleting = "2" // Space is being asynchronously deleted
)

// SkillSpace represents a skills space (library) that contains skills
type SkillSpace struct {
	ID          string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID    string     `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	Name        string     `gorm:"column:name;size:128;not null" json:"name"`
	FolderID    string     `gorm:"column:folder_id;size:32;not null" json:"folder_id"`
	Description string     `gorm:"column:description;type:text" json:"description"`
	EmbdID      string     `gorm:"column:embd_id;size:128" json:"embd_id"`
	RerankID    string     `gorm:"column:rerank_id;size:128" json:"rerank_id"`
	TopK        int        `gorm:"column:top_k;default:10" json:"top_k"`
	Status      string     `gorm:"column:status;size:1;default:1" json:"status"`
	CreateTime  *int64     `gorm:"column:create_time" json:"create_time,omitempty"`
	UpdateTime  *time.Time `gorm:"column:update_time" json:"update_time,omitempty"`
}

// TableName returns the table name for SkillSpace model
func (SkillSpace) TableName() string {
	return "skill_spaces"
}

// StatusDescription returns a human-readable status string
func (s *SkillSpace) StatusDescription() string {
	switch s.Status {
	case SpaceStatusActive:
		return "active"
	case SpaceStatusDeleted:
		return "deleted"
	case SpaceStatusDeleting:
		return "deleting"
	default:
		return "unknown"
	}
}

// ToMap converts SkillSpace to a map for JSON response
func (s *SkillSpace) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"id":          s.ID,
		"tenant_id":   s.TenantID,
		"name":        s.Name,
		"folder_id":   s.FolderID,
		"top_k":       s.TopK,
		"status":      s.StatusDescription(),
	}

	if s.Description != "" {
		result["description"] = s.Description
	}
	if s.EmbdID != "" {
		result["embd_id"] = s.EmbdID
	}
	if s.RerankID != "" {
		result["rerank_id"] = s.RerankID
	}
	if s.CreateTime != nil {
		result["create_time"] = s.CreateTime
	}
	if s.UpdateTime != nil {
		result["update_time"] = s.UpdateTime.Format("2006-01-02 15:04:05")
	}

	return result
}
