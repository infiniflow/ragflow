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

// Tenant tenant model
type Tenant struct {
	ID            string  `gorm:"column:id;primaryKey;size:32" json:"id"`
	Name          *string `gorm:"column:name;size:100;index" json:"name,omitempty"`
	PublicKey     *string `gorm:"column:public_key;size:255;index" json:"public_key,omitempty"`
	ParserIDs     string  `gorm:"column:parser_ids;size:256;not null;index" json:"parser_ids"`
	Credit        int     `gorm:"column:credit;default:512;index" json:"credit"`
	InviteEnabled bool    `gorm:"column:invite_enabled;not null;default:false" json:"invite_enabled"`
	RoleID        *int32  `gorm:"column:role_id" json:"role_id,omitempty"`
	Status        *string `gorm:"column:status;size:1;index;default:'1'" json:"status,omitempty"`
	BaseModel
}

// TableName specify table name
func (Tenant) TableName() string {
	return "tenant"
}
