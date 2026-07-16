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

// CompilationTemplateGroup represents a group for compilation templates
type CompilationTemplateGroup struct {
	ID          string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	TenantID    string  `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	Name        string  `gorm:"column:name;size:128;not null;index" json:"name"`
	Description *string `gorm:"column:description;type:text" json:"description,omitempty"`
	Scope       string  `gorm:"column:scope;size:16;not null;index" json:"scope"`
	Status      *string `gorm:"column:status;size:1;default:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName returns the table name for CompilationTemplateGroup model
func (CompilationTemplateGroup) TableName() string {
	return "compilation_template_group"
}
