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

// PipelineDSLVersion stores one immutable definition in a pipeline's ordered
// DSL history. DSLID identifies the pipeline definition family and Version is
// monotonic within that family.
type PipelineDSLVersion struct {
	DSLID   string  `gorm:"column:dsl_id;primaryKey;size:255" json:"dsl_id"`
	Version int64   `gorm:"column:version;primaryKey;autoIncrement:false" json:"version"`
	DSL     JSONMap `gorm:"column:dsl;type:longtext;not null" json:"dsl"`
	BaseModel
}

// TableName specifies the database table name.
func (PipelineDSLVersion) TableName() string {
	return "pipeline_dsl_version"
}
