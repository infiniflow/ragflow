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

// DocumentCleanupClaim fences cross-instance document cleanup at batch
// boundaries. A replacement token prevents the prior holder from starting a
// later batch.
type DocumentCleanupClaim struct {
	DocumentID string `gorm:"column:document_id;primaryKey;size:32" json:"document_id"`
	Token      string `gorm:"column:token;size:64;not null" json:"token"`
	Owner      string `gorm:"column:owner;size:128;not null" json:"owner"`
	ExpiresAt  int64  `gorm:"column:expires_at;not null" json:"expires_at"`
	UpdateTime int64  `gorm:"column:update_time;not null" json:"update_time"`
}

// TableName specifies the table name.
func (DocumentCleanupClaim) TableName() string {
	return "document_cleanup_claim"
}
