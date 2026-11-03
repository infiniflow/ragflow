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
	"encoding/json"
	"errors"
	"fmt"
	"ragflow/internal/entity"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
// DAO does not assign defaults because callers own session IDs and timestamps.
func (dao *API4ConversationDAO) Create(ctx context.Context, db *gorm.DB, conv *entity.API4Conversation) error {
	if conv == nil {
		return errors.New("api4 conversation: nil row")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(conv).Error; err != nil {
			return err
		}
		return createHistory(ctx, tx, apiConversationMessageTable, "message", conv.ID, conv.Message)
	})
}

// UpdateHistory locks the parent before writing only the addressed child rows.
func (dao *API4ConversationDAO) UpdateHistory(ctx context.Context, db *gorm.DB, sessionID, agentID, userID string, updates map[string]interface{}, history ConversationHistoryUpdate) error {
	if updates == nil {
		updates = make(map[string]interface{})
	}
	for key := range updates {
		switch key {
		case "message", "Message", "messages", "Messages", "reference", "Reference":
			return errors.New("does not support")
		}
	}
	now := time.Now().Local()
	updates["update_time"], updates["update_date"] = now.UnixMilli(), now.Truncate(time.Second)
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Session(&gorm.Session{SkipHooks: true}).Model(&entity.API4Conversation{}).Where("id = ? AND dialog_id = ? AND user_id = ?", sessionID, agentID, userID)
		var parent entity.API4Conversation
		if err := query.Session(&gorm.Session{}).Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Take(&parent).Error; err != nil {
			return err
		}
		if err := query.Updates(updates).Error; err != nil {
			return err
		}
		return updateConversationHistory(ctx, tx, apiConversationMessageTable, sessionID, history)
	})
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

func (dao *API4ConversationDAO) GetBySessionID(ctx context.Context, db *gorm.DB, sessionID, agentID string, options ...ConversationHistoryOptions) (*entity.API4Conversation, error) {
	historyOptions := ConversationHistoryOptions{}
	if len(options) > 0 {
		historyOptions = options[0]
	}
	result, err := dao.GetMetadataBySessionID(ctx, db, sessionID, agentID)
	if err != nil || result == nil {
		return result, err
	}
	if err := hydrateAPIConversations(ctx, db, []*entity.API4Conversation{result}, historyOptions); err != nil {
		return nil, err
	}
	return result, nil
}

// GetMetadataBySessionID avoids history hydration for authorization and DSL reads.
func (dao *API4ConversationDAO) GetMetadataBySessionID(ctx context.Context, db *gorm.DB, sessionID, agentID string) (*entity.API4Conversation, error) {
	var result entity.API4Conversation
	tx := db.WithContext(ctx).Session(&gorm.Session{QueryFields: true}).Where("id = ? AND dialog_id = ?", sessionID, agentID).Find(&result)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, nil
	}
	return &result, nil
}

// GetByID returns a conversation without requiring the caller to know its
// agent. It is used when the session itself is the authorization resource.
func (dao *API4ConversationDAO) GetByID(ctx context.Context, db *gorm.DB, id string, options ...ConversationHistoryOptions) (*entity.API4Conversation, error) {
	historyOptions := ConversationHistoryOptions{}
	if len(options) > 0 {
		historyOptions = options[0]
	}
	var result entity.API4Conversation
	tx := db.WithContext(ctx).Session(&gorm.Session{QueryFields: true}).Take(&result, "id = ?", id)
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, tx.Error
	}
	if err := hydrateAPIConversations(ctx, db, []*entity.API4Conversation{&result}, historyOptions); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListIDsByAgentID lists conversation IDs for one agent.
func (dao *API4ConversationDAO) ListIDsByAgentID(ctx context.Context, db *gorm.DB, agentID string) ([]string, error) {
	var ids []string
	err := db.WithContext(ctx).Model(&entity.API4Conversation{}).Where("dialog_id = ?", agentID).Pluck("id", &ids).Error
	return ids, err
}

// DeleteBySessionIDAndAgentID deletes API4Conversations by sessionID and agentID.
func (dao *API4ConversationDAO) DeleteBySessionIDAndAgentID(ctx context.Context, db *gorm.DB, sessionID, agentID string) (int64, error) {
	var rowsAffected int64
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []string
		if err := tx.Model(&entity.API4Conversation{}).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND dialog_id = ?", sessionID, agentID).Order("id").Pluck("id", &ids).Error; err != nil {
			return err
		}
		if err := deleteHistory(ctx, tx, []string{apiConversationMessageTable}, ids); err != nil {
			return err
		}
		result := tx.Where("id = ? AND dialog_id = ?", sessionID, agentID).Delete(&entity.API4Conversation{})
		rowsAffected = result.RowsAffected
		return result.Error
	})
	return rowsAffected, err
}

// ArchiveAndDeleteBySessionIDAndAgentID moves a session's historical log out
// of chat history before deleting its conversation and message rows.
func (dao *API4ConversationDAO) ArchiveAndDeleteBySessionIDAndAgentID(ctx context.Context, db *gorm.DB, sessionID, agentID string) (int64, error) {
	var rowsAffected int64
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation entity.API4Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND dialog_id = ?", sessionID, agentID).Take(&conversation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if err := hydrateAPIConversations(ctx, tx, []*entity.API4Conversation{&conversation}, ConversationHistoryOptions{
			IncludeReference: true,
			IncludePrompt:    true,
			IncludeUsage:     true,
			IncludeElapsedMs: true,
		}); err != nil {
			return err
		}
		message, err := json.Marshal(conversation.Message)
		if err != nil {
			return fmt.Errorf("marshal conversation log: %w", err)
		}
		log := entity.API4ConversationLog{
			ID: conversation.ID, Name: conversation.Name, DialogID: conversation.DialogID,
			UserID: conversation.UserID, ExpUserID: conversation.ExpUserID, Message: string(message),
			Tokens: conversation.Tokens, Source: conversation.Source, DSL: conversation.DSL,
			Duration: conversation.Duration, Round: conversation.Round, ThumbUp: conversation.ThumbUp,
			Errors: conversation.Errors, VersionTitle: conversation.VersionTitle,
			CreateTime: conversation.CreateTime, CreateDate: conversation.CreateDate,
			UpdateTime: conversation.UpdateTime, UpdateDate: conversation.UpdateDate,
		}
		if err := tx.Create(&log).Error; err != nil {
			return fmt.Errorf("archive conversation log: %w", err)
		}
		if err := deleteHistory(ctx, tx, []string{apiConversationMessageTable}, []string{sessionID}); err != nil {
			return err
		}
		result := tx.Where("id = ? AND dialog_id = ?", sessionID, agentID).Delete(&entity.API4Conversation{})
		rowsAffected = result.RowsAffected
		return result.Error
	})
	return rowsAffected, err
}

// Move rows hidden by the previous deletion flag into independent logs.
func archiveDeletedAgentSessions(ctx context.Context, db *gorm.DB) error {
	if !db.Migrator().HasColumn(&entity.API4Conversation{}, "is_deleted") {
		return nil
	}
	var rows []entity.API4Conversation
	if err := db.WithContext(ctx).Select("id", "dialog_id").Where("is_deleted = ?", true).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := NewAPI4ConversationDAO().ArchiveAndDeleteBySessionIDAndAgentID(ctx, db, row.ID, row.DialogID); err != nil {
			return err
		}
	}
	return nil
}

type agentLogListRow struct {
	ID           string         `gorm:"column:id"`
	Name         *string        `gorm:"column:name"`
	DialogID     string         `gorm:"column:dialog_id"`
	UserID       string         `gorm:"column:user_id"`
	ExpUserID    *string        `gorm:"column:exp_user_id"`
	Message      string         `gorm:"column:message"`
	Tokens       int            `gorm:"column:tokens"`
	Source       *string        `gorm:"column:source"`
	DSL          entity.JSONMap `gorm:"column:dsl"`
	Duration     float64        `gorm:"column:duration"`
	Round        int            `gorm:"column:round"`
	ThumbUp      int            `gorm:"column:thumb_up"`
	Errors       *string        `gorm:"column:errors"`
	VersionTitle *string        `gorm:"column:version_title"`
	CreateTime   *int64         `gorm:"column:create_time"`
	CreateDate   *time.Time     `gorm:"column:create_date"`
	UpdateTime   *int64         `gorm:"column:update_time"`
	UpdateDate   *time.Time     `gorm:"column:update_date"`
	IsArchived   bool           `gorm:"column:is_archived"`
}

// ListAgentLogs returns active conversations and archived logs as one paginated list.
func (dao *API4ConversationDAO) ListAgentLogs(ctx context.Context, db *gorm.DB, params ListAgentSessionsParams, historyOptions ConversationHistoryOptions) (int64, []*entity.API4Conversation, error) {
	union := `(SELECT id, name, dialog_id, user_id, exp_user_id, '' AS message, tokens, source, dsl, duration, round, thumb_up, errors, version_title, create_time, create_date, update_time, update_date, 0 AS is_archived FROM api_4_conversation
UNION ALL
SELECT id, name, dialog_id, user_id, exp_user_id, message, tokens, source, dsl, duration, round, thumb_up, errors, version_title, create_time, create_date, update_time, update_date, 1 AS is_archived FROM api_4_conversation_log) AS agent_logs`
	query := db.WithContext(ctx).Table(union).Where("dialog_id = ?", params.AgentID)
	if params.SessionID != "" {
		query = query.Where("id = ?", params.SessionID)
	}
	if params.UserID != "" {
		query = query.Where("user_id = ?", params.UserID)
	}
	if params.ExpUserID != "" {
		query = query.Where("exp_user_id = ?", params.ExpUserID)
	}
	dateColumn := "create_date"
	if params.OrderBy == "update_time" || params.OrderBy == "update_date" {
		dateColumn = "update_date"
	}
	if params.FromDate != nil {
		query = query.Where(dateColumn+" >= ?", *params.FromDate)
	}
	if params.ToDate != nil {
		query = query.Where(dateColumn+" <= ?", *params.ToDate)
	}
	if params.Keywords != "" {
		pattern := "%" + strings.ToLower(params.Keywords) + "%"
		query = query.Where(`(LOWER(id) LIKE ? OR LOWER(name) LIKE ? OR
(is_archived = 1 AND LOWER(message) LIKE ?) OR
(is_archived = 0 AND EXISTS (SELECT 1 FROM api_4_conversation_message AS cm WHERE cm.conversation_id = agent_logs.id AND LOWER(cm.content) LIKE ?)))`, pattern, pattern, pattern, pattern)
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
	page, pageSize := params.Page, params.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 30
	}
	var rows []agentLogListRow
	if err := query.Order(orderBy).Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return 0, nil, err
	}
	sessions := make([]*entity.API4Conversation, 0, len(rows))
	active := make([]*entity.API4Conversation, 0, len(rows))
	for _, row := range rows {
		session := &entity.API4Conversation{
			ID: row.ID, Name: row.Name, DialogID: row.DialogID, UserID: row.UserID,
			ExpUserID: row.ExpUserID, Tokens: row.Tokens, Source: row.Source, DSL: row.DSL,
			Duration: row.Duration, Round: row.Round, ThumbUp: row.ThumbUp, Errors: row.Errors,
			VersionTitle: row.VersionTitle,
			BaseModel:    entity.BaseModel{CreateTime: row.CreateTime, CreateDate: row.CreateDate, UpdateTime: row.UpdateTime, UpdateDate: row.UpdateDate},
		}
		if row.IsArchived {
			session.Message = json.RawMessage(row.Message)
			if err := filterArchivedAgentLogMessage(&session.Message, historyOptions); err != nil {
				return 0, nil, err
			}
		} else {
			active = append(active, session)
		}
		sessions = append(sessions, session)
	}
	if !params.NoHistory && len(active) > 0 {
		if err := hydrateAPIConversations(ctx, db, active, historyOptions); err != nil {
			return 0, nil, err
		}
	}
	return total, sessions, nil
}

func filterArchivedAgentLogMessage(raw *json.RawMessage, options ConversationHistoryOptions) error {
	if options.IncludeReference && options.IncludePrompt && options.IncludeUsage && options.IncludeElapsedMs {
		return nil
	}
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(*raw, &messages); err != nil {
		return err
	}
	for _, message := range messages {
		for field, include := range map[string]bool{
			"reference": options.IncludeReference, "prompt": options.IncludePrompt,
			"usage": options.IncludeUsage, "elapsed_ms": options.IncludeElapsedMs,
		} {
			if !include {
				delete(message, field)
			}
		}
	}
	filtered, err := json.Marshal(messages)
	if err != nil {
		return err
	}
	*raw = filtered
	return nil
}

// DeleteByDialogIDs deletes API4Conversations by dialog IDs (hard delete)
func (dao *API4ConversationDAO) DeleteByDialogIDs(ctx context.Context, db *gorm.DB, dialogIDs []string) (int64, error) {
	if len(dialogIDs) == 0 {
		return 0, nil
	}
	var rowsAffected int64
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []string
		if err := tx.Model(&entity.API4Conversation{}).Clauses(clause.Locking{Strength: "UPDATE"}).Where("dialog_id IN ?", dialogIDs).Order("id").Pluck("id", &ids).Error; err != nil {
			return err
		}
		if err := deleteHistory(ctx, tx, []string{apiConversationMessageTable}, ids); err != nil {
			return err
		}
		result := tx.Where("dialog_id IN ?", dialogIDs).Delete(&entity.API4Conversation{})
		rowsAffected = result.RowsAffected
		return result.Error
	})
	return rowsAffected, err
}
