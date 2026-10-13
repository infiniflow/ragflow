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
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// ChatDAO chat data access object
type ChatDAO struct{}

// NewChatDAO create chat DAO
func NewChatDAO() *ChatDAO {
	return &ChatDAO{}
}

// ListByResourceIDs lists chats that have already been authorized by the
// permission layer, with the same filters and pagination as the chat list API.
func (dao *ChatDAO) ListByResourceIDs(ctx context.Context, db *gorm.DB, resourceIDs, ownerIDs []string, page, pageSize int, terms []OrderTerm, keywords, id, name string) ([]*entity.ChatListItem, int64, error) {
	if len(resourceIDs) == 0 || (ownerIDs != nil && len(ownerIDs) == 0) {
		return []*entity.ChatListItem{}, 0, nil
	}

	var chats []*entity.ChatListItem
	var total int64
	query := db.WithContext(ctx).Model(&entity.Chat{}).
		Select(`
			dialog.*,
			user.nickname,
			user.avatar as tenant_avatar
		`).
		Joins("LEFT JOIN user ON dialog.tenant_id = user.id").
		Where("dialog.id IN ? AND dialog.status = ?", resourceIDs, "1")
	if ownerIDs != nil {
		query = query.Where("dialog.tenant_id IN ?", ownerIDs)
	}
	if keywords != "" {
		query = query.Where("LOWER(dialog.name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}
	if id != "" {
		query = query.Where("dialog.id = ?", id)
	}
	if name != "" {
		query = query.Where("dialog.name = ?", name)
	}
	query = query.Order(chatOrderClause(terms))

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}
	if err := query.Scan(&chats).Error; err != nil {
		return nil, 0, err
	}
	return chats, total, nil
}

// GetByID gets chat by ID
func (dao *ChatDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.Chat, error) {
	var chat entity.Chat
	err := db.WithContext(ctx).Take(&chat, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &chat, nil
}

// GetByIDAndStatus gets chat by ID and status
func (dao *ChatDAO) GetByIDAndStatus(ctx context.Context, db *gorm.DB, id string, status string) (*entity.Chat, error) {
	var chat entity.Chat
	err := db.WithContext(ctx).Where("id = ? AND status = ?", id, status).First(&chat).Error
	if err != nil {
		return nil, err
	}
	return &chat, nil
}

// ExistsByNameTenantStatus checks whether a chat with the given name exists.
func (dao *ChatDAO) ExistsByNameTenantStatus(ctx context.Context, db *gorm.DB, name, tenantID, status string) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&entity.Chat{}).
		Where("LOWER(name) = LOWER(?) AND tenant_id = ? AND status = ?", name, tenantID, status).
		Count(&count).Error
	return count > 0, err
}

// Create creates a new chat/dialog
func (dao *ChatDAO) Create(ctx context.Context, db *gorm.DB, chat *entity.Chat) error {
	// Select("*") forces GORM to persist explicit zero values (e.g. similarity_threshold=0,
	// vector_similarity_weight=0, top_n=0) instead of substituting the column defaults.
	return db.WithContext(ctx).Select("*").Create(chat).Error
}

// UpdateByID updates a chat by ID
func (dao *ChatDAO) UpdateByID(ctx context.Context, db *gorm.DB, id string, updates map[string]interface{}) error {
	if updates == nil {
		updates = make(map[string]interface{})
	}

	now := time.Now().Local()
	updates["update_time"] = now.UnixMilli()
	updates["update_date"] = now.Truncate(time.Second)

	result := db.WithContext(ctx).Session(&gorm.Session{SkipHooks: true}).Model(&entity.Chat{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := db.WithContext(ctx).Model(&entity.Chat{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
	}
	return nil
}

// UpdateManyByID updates multiple chats by ID (batch update)
func (dao *ChatDAO) UpdateManyByID(ctx context.Context, db *gorm.DB, updates []map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	// Use transaction for batch update
	tx := db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return tx.Error
	}

	for _, update := range updates {
		id, ok := update["id"].(string)
		if !ok {
			tx.Rollback()
			return fmt.Errorf("invalid id in update")
		}

		// Remove id from updates map
		updatesWithoutID := make(map[string]interface{})
		for k, v := range update {
			if k != "id" {
				updatesWithoutID[k] = v
			}
		}

		if err := tx.Model(&entity.Chat{}).Where("id = ?", id).Updates(updatesWithoutID).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

// DeleteByTenantID deletes all chats by tenant ID (hard delete)
func (dao *ChatDAO) DeleteByTenantID(ctx context.Context, db *gorm.DB, tenantID string) (int64, error) {
	result := db.WithContext(ctx).Unscoped().Where("tenant_id = ?", tenantID).Delete(&entity.Chat{})
	return result.RowsAffected, result.Error
}

// GetAllDialogIDsByTenantID gets all dialog IDs by tenant ID
func (dao *ChatDAO) GetAllDialogIDsByTenantID(ctx context.Context, db *gorm.DB, tenantID string) ([]string, error) {
	var dialogIDs []string
	err := db.WithContext(ctx).Model(&entity.Chat{}).
		Where("tenant_id = ?", tenantID).
		Pluck("id", &dialogIDs).Error
	return dialogIDs, err
}
