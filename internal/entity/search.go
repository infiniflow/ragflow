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

// Search search model
type Search struct {
	ID           string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	Avatar       *string `gorm:"column:avatar;type:longtext" json:"avatar,omitempty"`
	TenantID     string  `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	Name         string  `gorm:"column:name;size:128;not null;index" json:"name"`
	Description  *string `gorm:"column:description;type:longtext" json:"description,omitempty"`
	CreatedBy    string  `gorm:"column:created_by;size:32;not null;index" json:"created_by"`
	SearchConfig JSONMap `gorm:"column:search_config;type:longtext;not null" json:"search_config"`
	Status       *string `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (Search) TableName() string {
	return "search"
}
