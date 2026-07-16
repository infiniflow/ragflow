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

// InvitationCode represents the invitation code model
type InvitationCode struct {
	ID        string     `gorm:"column:id;primaryKey;size:32" json:"id"`
	Code      string     `gorm:"column:code;size:32;not null;index" json:"code"`
	VisitTime *time.Time `gorm:"column:visit_time;index" json:"visit_time,omitempty"`
	UserID    *string    `gorm:"column:user_id;size:32;index" json:"user_id,omitempty"`
	TenantID  *string    `gorm:"column:tenant_id;size:32;index" json:"tenant_id,omitempty"`
	Status    *string    `gorm:"column:status;size:1;index" json:"status,omitempty"`
	BaseModel
}

// TableName returns the table name for InvitationCode model
func (InvitationCode) TableName() string {
	return "invitation_code"
}
