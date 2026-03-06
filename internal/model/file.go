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

// File file model
type File struct {
	ID         string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	ParentID   string  `gorm:"column:parent_id;size:32;not null;index" json:"parent_id"`
	TenantID   string  `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	CreatedBy  string  `gorm:"column:created_by;size:32;not null;index" json:"created_by"`
	Name       string  `gorm:"column:name;size:255;not null;index" json:"name"`
	Location   *string `gorm:"column:location;size:255;index" json:"location,omitempty"`
	Size       int64   `gorm:"column:size;default:0;index" json:"size"`
	Type       string  `gorm:"column:type;size:32;not null;index" json:"type"`
	SourceType string  `gorm:"column:source_type;size:128;not null;default:'';index" json:"source_type"`
	BaseModel
}

// TableName specify table name
func (File) TableName() string {
	return "file"
}

// File2Document file to document mapping model
type File2Document struct {
	ID         string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	FileID     *string `gorm:"column:file_id;size:32;index" json:"file_id,omitempty"`
	DocumentID *string `gorm:"column:document_id;size:32;index" json:"document_id,omitempty"`
	BaseModel
}

// TableName specify table name
func (File2Document) TableName() string {
	return "file2document"
}
