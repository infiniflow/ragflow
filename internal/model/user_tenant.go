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

// UserTenant user tenant relationship model
type UserTenant struct {
	ID        string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	UserID    string  `gorm:"column:user_id;size:32;not null;index" json:"user_id"`
	TenantID  string  `gorm:"column:tenant_id;size:32;not null;index" json:"tenant_id"`
	Role      string  `gorm:"column:role;size:32;not null;index" json:"role"`
	InvitedBy string  `gorm:"column:invited_by;size:32;not null;index" json:"invited_by"`
	Status    *string `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (UserTenant) TableName() string {
	return "user_tenant"
}
