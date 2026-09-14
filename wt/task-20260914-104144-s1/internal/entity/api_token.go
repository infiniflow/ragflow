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

// APIToken API token model
type APIToken struct {
	TenantID string  `gorm:"column:tenant_id;size:32;not null;primaryKey" json:"tenant_id"`
	Token    string  `gorm:"column:token;size:255;not null;primaryKey" json:"token"`
	DialogID *string `gorm:"column:dialog_id;size:32;index" json:"dialog_id,omitempty"`
	Source   *string `gorm:"column:source;size:16;index" json:"source,omitempty"`
	Beta     *string `gorm:"column:beta;size:255;index" json:"beta,omitempty"`
	BaseModel
}

// TableName specify table name
func (APIToken) TableName() string {
	return "api_token"
}
