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
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// ChatSessionDAO chat session data access object
type ChatSessionDAO struct{}

type ListAgentSessionsParams struct {
	AgentID    string
	Page       int
	PageSize   int
	OrderBy    string
	Desc       bool
	SessionID  string
	UserID     string
	IncludeDSL bool
	Keywords   string
	FromDate   *time.Time
	ToDate     *time.Time
	ExpUserID  string
}

// NewChatSessionDAO create chat session DAO
func NewChatSessionDAO() *ChatSessionDAO {
	return &ChatSessionDAO{}
}

// GetByID gets chat session by ID
func (dao *ChatSessionDAO) GetByID(id string) (*entity.ChatSession, error) {
	var conv entity.ChatSession
	err := DB.Where("id = ?", id).First(&conv).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// GetBySessionIDAndChatID gets a chat session by session ID and chat ID.
func (dao *ChatSessionDAO) GetBySessionIDAndChatID(sessionID, chatID string) (*entity.ChatSession, error) {
	var conv entity.ChatSession
	err := DB.Where("id = ? AND dialog_id = ?", sessionID, chatID).First(&conv).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// Create creates a new chat session
func (dao *ChatSessionDAO) Create(conv *entity.ChatSession) error {
	return DB.Create(conv).Error
}

// UpdateByID updates a chat session by ID
func (dao *ChatSessionDAO) UpdateByID(id string, updates map[string]interface{}) error {
	if updates == nil {
		updates = make(map[string]interface{})
	}

	now := time.Now().Local()
	updates["update_time"] = now.UnixMilli()
	updates["update_date"] = now.Truncate(time.Second)

	result := DB.Session(&gorm.Session{SkipHooks: true}).Model(&entity.ChatSession{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var count int64
		if err := DB.Model(&entity.ChatSession{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
	}
	return nil
}

// DeleteByID deletes a chat session by ID (hard delete)
func (dao *ChatSessionDAO) DeleteByID(id string) error {
	return DB.Where("id = ?", id).Delete(&entity.ChatSession{}).Error
}

// ListByChatID lists chat sessions by chat ID
func (dao *ChatSessionDAO) ListByChatID(chatID string) ([]*entity.ChatSession, error) {
	var convs []*entity.ChatSession
	err := DB.Where("dialog_id = ?", chatID).
		Order("create_time DESC").
		Find(&convs).Error
	return convs, err
}

// CheckDialogExists checks if a dialog exists with given tenant_id and dialog_id
func (dao *ChatSessionDAO) CheckDialogExists(tenantID, chatID string) (bool, error) {
	var count int64
	err := DB.Model(&entity.Chat{}).
		Where("tenant_id = ? AND id = ? AND status = ?", tenantID, chatID, common.StatusDialogValid).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetDialogByID gets dialog by ID
func (dao *ChatSessionDAO) GetDialogByID(chatID string) (*entity.Chat, error) {
	var dialog entity.Chat
	err := DB.Where("id = ? AND status = ?", chatID, common.StatusDialogValid).First(&dialog).Error
	if err != nil {
		return nil, err
	}
	return &dialog, nil
}

// DeleteByDialogIDs deletes chat sessions by dialog IDs (hard delete)
func (dao *ChatSessionDAO) DeleteByDialogIDs(dialogIDs []string) (int64, error) {
	if len(dialogIDs) == 0 {
		return 0, nil
	}
	result := DB.Unscoped().Where("dialog_id IN ?", dialogIDs).Delete(&entity.ChatSession{})
	return result.RowsAffected, result.Error
}

func (dao *ChatSessionDAO) ListAgentSessionNames(agentID, expUserID string) ([]map[string]interface{}, error) {
	var rows []map[string]interface{}
	err := DB.Model(&entity.API4Conversation{}).
		Select("id", "name").
		Where("dialog_id = ? AND exp_user_id = ?", agentID, expUserID).
		Order("create_date DESC").
		Find(&rows).Error
	return rows, err
}

func normalizeAgentSessionOrderBy(orderBy string) string {
	switch orderBy {
	case "id":
		return "id"
	case "name":
		return "name"
	case "create_time":
		return "create_time"
	case "create_date":
		return "create_date"
	case "update_time":
		return "update_time"
	case "update_date":
		return "update_date"
	case "tokens":
		return "tokens"
	case "duration":
		return "duration"
	case "round":
		return "round"
	case "thumb_up":
		return "thumb_up"
	default:
		return "update_time"
	}
}

func (dao *ChatSessionDAO) ListAgentSessions(params ListAgentSessionsParams) (int64, []*entity.API4Conversation, error) {
	query := DB.Model(&entity.API4Conversation{}).Where("dialog_id = ?", params.AgentID)
	if !params.IncludeDSL {
		query = query.Omit("dsl")
	}

	if params.SessionID != "" {
		query = query.Where("id = ?", params.SessionID)
	}

	if params.UserID != "" {
		query = query.Where("user_id = ?", params.UserID)
	}

	if params.Keywords != "" {
		keywords := strings.ToLower(params.Keywords)
		escapedKeywords := strings.Trim(strconv.QuoteToASCII(keywords), `"`)
		if escapedKeywords == keywords {
			query = query.Where("LOWER(message) LIKE ?", "%"+keywords+"%")
		} else {
			query = query.Where("(LOWER(message) LIKE ? OR LOWER(message) LIKE ?)", "%"+keywords+"%", "%"+escapedKeywords+"%")
		}
	}

	dateColumn := "create_date"
	if strings.HasPrefix(params.OrderBy, "update_") {
		dateColumn = "update_date"
	}

	if params.FromDate != nil {
		query = query.Where(dateColumn+" >= ?", *params.FromDate)
	}

	if params.ToDate != nil {
		query = query.Where(dateColumn+" <= ?", *params.ToDate)
	}

	if params.ExpUserID != "" {
		query = query.Where("exp_user_id = ?", params.ExpUserID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return 0, nil, err
	}

	orderBy := normalizeAgentSessionOrderBy(params.OrderBy)
	if params.Desc {
		orderBy += " DESC"
	} else {
		orderBy += " ASC"
	}

	page := params.Page
	if page <= 0 {
		page = 1
	}

	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = 30
	}

	var sessions []*entity.API4Conversation
	err := query.
		Order(orderBy).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&sessions).Error

	return total, sessions, err
}
