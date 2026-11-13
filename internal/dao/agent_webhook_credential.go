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

package dao

import (
	"context"
	"time"

	"ragflow/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SaveAgentWebhookCredential creates or rotates the credential for one
// (canvas, user, tenant) identity. Reissuing replaces the old hash atomically.
func SaveAgentWebhookCredential(ctx context.Context, db *gorm.DB, credential *entity.AgentWebhookCredential) error {
	now := time.Now().Local()
	createTime := now.UnixMilli()
	createDate := now.Truncate(time.Second)
	updateTime := createTime
	updateDate := createDate
	credential.CreateTime = &createTime
	credential.CreateDate = &createDate
	credential.UpdateTime = &updateTime
	credential.UpdateDate = &updateDate

	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "canvas_id"}, {Name: "user_id"}, {Name: "tenant_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"token_hash", "revoked_at", "update_time", "update_date"}),
	}).Create(credential).Error
}

// GetActiveAgentWebhookCredential resolves a stored hash for the requested
// canvas. Explicit predicates keep the lookup scoped to both the URL canvas
// and the unguessable credential.
func GetActiveAgentWebhookCredential(ctx context.Context, db *gorm.DB, canvasID, tokenHash string) (*entity.AgentWebhookCredential, error) {
	var credential entity.AgentWebhookCredential
	err := db.WithContext(ctx).
		Where("canvas_id = ? AND token_hash = ? AND revoked_at IS NULL", canvasID, tokenHash).
		Take(&credential).Error
	if err != nil {
		return nil, err
	}
	return &credential, nil
}

// RevokeAgentWebhookCredential revokes only the specified user's credential
// for a canvas. It is safe to call repeatedly.
func RevokeAgentWebhookCredential(ctx context.Context, db *gorm.DB, canvasID, userID string) error {
	now := time.Now().Local()
	updateTime := now.UnixMilli()
	updateDate := now.Truncate(time.Second)
	return db.WithContext(ctx).Model(&entity.AgentWebhookCredential{}).
		Where("canvas_id = ? AND user_id = ? AND revoked_at IS NULL", canvasID, userID).
		Updates(map[string]any{"revoked_at": now, "update_time": updateTime, "update_date": updateDate}).Error
}
