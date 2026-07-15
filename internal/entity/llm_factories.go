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

// LLMFactories LLM factory model
type LLMFactories struct {
	Name   string  `gorm:"column:name;primaryKey;size:128" json:"name"`
	Logo   *string `gorm:"column:logo;type:longtext" json:"logo,omitempty"`
	Tags   string  `gorm:"column:tags;size:255;not null;index" json:"tags"`
	Rank   int64   `gorm:"column:rank;default:0" json:"rank"`
	Status *string `gorm:"column:status;size:1;index;default:'1'" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (LLMFactories) TableName() string {
	return "llm_factories"
}
