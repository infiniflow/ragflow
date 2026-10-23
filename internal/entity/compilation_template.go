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

// CompilationTemplate represents a compilation template
type CompilationTemplate struct {
	ID          string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID    *string `gorm:"column:tenant_id;size:32;index" json:"tenant_id,omitempty"`
	GroupID     *string `gorm:"column:group_id;size:32;index" json:"group_id,omitempty"`
	Name        string  `gorm:"column:name;size:128;not null;index" json:"name"`
	Description *string `gorm:"column:description;type:text" json:"description,omitempty"`
	Kind        string  `gorm:"column:kind;size:64;not null;index" json:"kind"`
	Config      JSONMap `gorm:"column:config;type:text;not null" json:"config"`
	IsBuiltin   bool    `gorm:"column:is_builtin;not null;default:false;index" json:"is_builtin"`
	Status      *string `gorm:"column:status;size:1;default:1;index" json:"status,omitempty"`

	// Version is the published version number. 0 means "never published": the
	// template is still a plain editable config and compilation reads the working
	// Config. From 1 on, compilation reads the immutable snapshot with that
	// number instead, which is what makes a model change take effect on publish
	// rather than on save.
	Version int `gorm:"column:version;not null;default:0;index" json:"version"`
	// PublishStatus is the model lifecycle state. It is deliberately a separate
	// column from Status above: Status is the valid/invalid soft-delete flag
	// every table in the project carries, and overloading it would make "this
	// row is gone" indistinguishable from "this model is a draft".
	PublishStatus string `gorm:"column:publish_status;size:16;not null;default:draft;index" json:"publish_status"`
	// PublishedAt and PublishedBy describe the CURRENT version. Earlier ones are
	// described by their own snapshot rows.
	PublishedAt *int64  `gorm:"column:published_at" json:"published_at,omitempty"`
	PublishedBy *string `gorm:"column:published_by;size:64" json:"published_by,omitempty"`
	// ReviewNote is the last review decision's explanation, kept where the next
	// person to open the model will see it rather than in a log they are not
	// reading.
	ReviewNote *string `gorm:"column:review_note;type:text" json:"review_note,omitempty"`
	ReviewedBy *string `gorm:"column:reviewed_by;size:64" json:"reviewed_by,omitempty"`
	ReviewedAt *int64  `gorm:"column:reviewed_at" json:"reviewed_at,omitempty"`
	BaseModel
}

// TableName returns the table name for CompilationTemplate model
func (CompilationTemplate) TableName() string {
	return "compilation_template"
}
