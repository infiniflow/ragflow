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

// SystemSettings system settings model
type SystemSettings struct {
	Name     string `gorm:"column:name;primaryKey;size:128" json:"name"`
	Source   string `gorm:"column:source;size:32;not null" json:"source"`
	DataType string `gorm:"column:data_type;size:32;not null" json:"data_type"`
	Value    string `gorm:"column:value;size:1024;not null" json:"value"`
}

// TableName specify table name
func (SystemSettings) TableName() string {
	return "system_settings"
}
