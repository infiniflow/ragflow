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
	"errors"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// APITokenDAO API token data access object
type APITokenDAO struct{}

// NewAPITokenDAO create API token DAO
func NewAPITokenDAO() *APITokenDAO {
	return &APITokenDAO{}
}

// Create creates a new API token
func (dao *APITokenDAO) Create(ctx context.Context, db *gorm.DB, apiToken *entity.APIToken) error {
	return db.WithContext(ctx).Create(apiToken).Error
}

// GetByTenantID gets API tokens by tenant ID
func (dao *APITokenDAO) GetByTenantID(ctx context.Context, db *gorm.DB, tenantID string) ([]*entity.APIToken, error) {
	var tokens []*entity.APIToken
	err := db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&tokens).Error
	return tokens, err
}

// DeleteByTenantID deletes all API tokens by tenant ID (hard delete)
func (dao *APITokenDAO) DeleteByTenantID(ctx context.Context, db *gorm.DB, tenantID string) (int64, error) {
	result := db.WithContext(ctx).Unscoped().Where("tenant_id = ?", tenantID).Delete(&entity.APIToken{})
	return result.RowsAffected, result.Error
}

// GetUserByAPIToken gets user by API token
func (dao *APITokenDAO) GetUserByAPIToken(ctx context.Context, db *gorm.DB, token string) (*entity.APIToken, error) {
	var apiToken entity.APIToken
	err := db.WithContext(ctx).Where("token = ?", token).First(&apiToken).Error
	if err != nil {
		return nil, err
	}
	return &apiToken, nil
}

// GetByBeta gets API tokens by beta key (SDK/bot authorization token).
// Mirrors Python's APIToken.query(beta=token), which returns a list.
func (dao *APITokenDAO) GetByBeta(ctx context.Context, db *gorm.DB, beta string) ([]*entity.APIToken, error) {
	var tokens []*entity.APIToken
	err := db.WithContext(ctx).Where("beta = ?", beta).Find(&tokens).Error
	return tokens, err
}

// DeleteByDialogIDs deletes API tokens by dialog IDs (hard delete)
func (dao *APITokenDAO) DeleteByDialogIDs(ctx context.Context, db *gorm.DB, dialogIDs []string) (int64, error) {
	if len(dialogIDs) == 0 {
		return 0, nil
	}
	result := db.WithContext(ctx).Unscoped().Where("dialog_id IN ?", dialogIDs).Delete(&entity.APIToken{})
	return result.RowsAffected, result.Error
}

// DeleteByTenantIDAndToken deletes a specific API token by tenant ID and token value
func (dao *APITokenDAO) DeleteByTenantIDAndToken(ctx context.Context, db *gorm.DB, tenantID, token string) (int64, error) {
	result := db.WithContext(ctx).Unscoped().Where("tenant_id = ? AND token = ?", tenantID, token).Delete(&entity.APIToken{})
	return result.RowsAffected, result.Error
}

// API4ConversationDAO API for conversation data access object
type API4ConversationDAO struct{}

// NewAPI4ConversationDAO create API4Conversation DAO
func NewAPI4ConversationDAO() *API4ConversationDAO {
	return &API4ConversationDAO{}
}

// ConversationStatsRow is one daily aggregate row for api_4_conversation.
type ConversationStatsRow struct {
	Dt       string  `gorm:"column:dt"`
	PV       int64   `gorm:"column:pv"`
	UV       int64   `gorm:"column:uv"`
	Tokens   float64 `gorm:"column:tokens"`
	Duration float64 `gorm:"column:duration"`
	Round    float64 `gorm:"column:round"`
	ThumbUp  int64   `gorm:"column:thumb_up"`
}

// Create inserts a new api_4_conversation row. The caller is responsible
// for setting ID, DialogID, UserID and the BaseModel time fields; the
// DAO does not assign defaults because session creation paths in the
// Python agent API generate an uuid + tenant timestamp and rely on the
// round-trip shape being byte-identical.
func (dao *API4ConversationDAO) Create(ctx context.Context, db *gorm.DB, conv *entity.API4Conversation) error {
	if conv == nil {
		return errors.New("api4 conversation: nil row")
	}
	return db.WithContext(ctx).Create(conv).Error
}

// Update writes back an existing api_4_conversation row. The bot
// completion path calls this with the updated Message JSON after each
// turn so multi-turn chatbot sessions carry prior history into the next
// LLM call. Matches the Python conversation_service.update pattern at
// api/db/services/conversation_service.py:236 (async_iframe_completion).
func (dao *API4ConversationDAO) Update(ctx context.Context, db *gorm.DB, conv *entity.API4Conversation) error {
	if conv == nil {
		return errors.New("api4 conversation: nil row")
	}
	if conv.ID == "" {
		return errors.New("api4 conversation: empty id")
	}
	return db.WithContext(ctx).Save(conv).Error
}

// Stats returns daily conversation aggregates for a tenant.
func (dao *API4ConversationDAO) Stats(ctx context.Context, db *gorm.DB, tenantID, fromDate, toDate string, source *string) ([]ConversationStatsRow, error) {
	var rows []ConversationStatsRow
	dateExpr := "DATE_FORMAT(a.create_date, '%Y-%m-%d 00:00:00')"
	query := db.WithContext(ctx).Table("api_4_conversation AS a").
		Select(`
			DATE_FORMAT(a.create_date, '%Y-%m-%d 00:00:00') AS dt,
			COUNT(a.id) AS pv,
			COUNT(DISTINCT a.user_id) AS uv,
			COALESCE(SUM(a.tokens), 0) AS tokens,
			COALESCE(SUM(a.duration), 0) AS duration,
			COALESCE(AVG(a.round), 0) AS round,
			COALESCE(SUM(a.thumb_up), 0) AS thumb_up
		`).
		Joins("JOIN dialog AS d ON a.dialog_id = d.id AND d.tenant_id = ?", tenantID).
		Where("a.create_date >= ? AND a.create_date <= ?", fromDate, toDate)

	if source == nil {
		query = query.Where("a.source IS NULL")
	} else {
		query = query.Where("a.source = ?", *source)
	}

	err := query.Group(dateExpr).
		Order(dateExpr).
		Scan(&rows).Error
	return rows, err
}

func (dao *API4ConversationDAO) GetBySessionID(ctx context.Context, db *gorm.DB, sessionID, agentID string) (*entity.API4Conversation, error) {
	var result entity.API4Conversation
	tx := db.WithContext(ctx).Where("id = ? AND dialog_id = ?", sessionID, agentID).Find(&result)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, nil
	}
	return &result, nil
}

// ListIDsByAgentID lists conversation IDs for one agent.
func (dao *API4ConversationDAO) ListIDsByAgentID(ctx context.Context, db *gorm.DB, agentID string) ([]string, error) {
	var ids []string
	err := db.WithContext(ctx).Model(&entity.API4Conversation{}).Where("dialog_id = ?", agentID).Pluck("id", &ids).Error
	return ids, err
}

// DeleteBySessionIDAndAgentID deletes API4Conversations by sessionID and agentID
func (dao *API4ConversationDAO) DeleteBySessionIDAndAgentID(ctx context.Context, db *gorm.DB, sessionID, agentID string) (int64, error) {
	result := db.WithContext(ctx).Where("id = ? AND dialog_id = ?", sessionID, agentID).Delete(&entity.API4Conversation{})
	return result.RowsAffected, result.Error
}

// DeleteByDialogIDs deletes API4Conversations by dialog IDs (hard delete)
func (dao *API4ConversationDAO) DeleteByDialogIDs(ctx context.Context, db *gorm.DB, dialogIDs []string) (int64, error) {
	if len(dialogIDs) == 0 {
		return 0, nil
	}
	result := db.WithContext(ctx).Unscoped().Where("dialog_id IN ?", dialogIDs).Delete(&entity.API4Conversation{})
	return result.RowsAffected, result.Error
}
