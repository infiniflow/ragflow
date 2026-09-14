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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

const (
	conversationMessageTable       = "conversation_message"
	conversationReferenceTable     = "conversation_reference"
	apiConversationMessageTable    = "api_4_conversation_message"
	apiConversationReferenceTable  = "api_4_conversation_reference"
	conversationHistoryIDColumn    = "conversation_id"
	conversationHistoryOrderColumn = "position"
)

type conversationHistoryRow struct {
	ConversationID string `gorm:"column:conversation_id"`
	Position       int    `gorm:"column:position"`
	Payload        string `gorm:"column:payload"`
}

func historyRaw(value interface{}) (json.RawMessage, error) {
	switch typed := value.(type) {
	case nil:
		return json.RawMessage(`[]`), nil
	case json.RawMessage:
		return typed, nil
	case []byte:
		return json.RawMessage(typed), nil
	case string:
		return json.RawMessage(typed), nil
	default:
		return json.Marshal(value)
	}
}

func splitHistory(raw json.RawMessage, kind string) ([]json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return []json.RawMessage{}, nil
	}
	if raw[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		return items, nil
	}
	if raw[0] != '{' {
		return nil, errors.New("conversation history must be a JSON array or object")
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	if len(object) == 0 {
		return []json.RawMessage{}, nil
	}
	if kind == "message" {
		if wrapped, ok := object["messages"]; ok {
			return splitHistory(wrapped, kind)
		}
		return nil, errors.New("conversation message object has no messages array")
	}
	if _, ok := object["chunks"]; ok {
		return []json.RawMessage{raw}, nil
	}
	if _, ok := object["doc_aggs"]; ok {
		return []json.RawMessage{raw}, nil
	}

	positions := make([]int, 0, len(object))
	byPosition := make(map[int]json.RawMessage, len(object))
	for key, value := range object {
		position, err := strconv.Atoi(key)
		if err != nil {
			return nil, fmt.Errorf("conversation reference key %q is not numeric", key)
		}
		positions = append(positions, position)
		byPosition[position] = value
	}
	sort.Ints(positions)
	items := make([]json.RawMessage, 0, len(positions))
	for _, position := range positions {
		items = append(items, byPosition[position])
	}
	return items, nil
}

func compactHistoryItem(item json.RawMessage) (json.RawMessage, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, item); err != nil {
		return nil, err
	}
	return json.RawMessage(compact.Bytes()), nil
}

func flattenMessage(item json.RawMessage) (entity.ConversationMessageFields, error) {
	var fields entity.ConversationMessageFields
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(item, &metadata); err != nil {
		return fields, err
	}
	if metadata == nil {
		return fields, errors.New("conversation message must be a JSON object")
	}
	for key, target := range map[string]interface{}{
		"id": &fields.MessageID, "role": &fields.Role, "status": &fields.Status,
		"thumbup": &fields.ThumbUp, "feedback": &fields.Feedback, "created_at": &fields.CreatedAt,
	} {
		if value, ok := metadata[key]; ok && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			if err := json.Unmarshal(value, target); err == nil {
				delete(metadata, key)
			} else {
				// Keep nonstandard values in metadata without populating the typed column.
				if err := json.Unmarshal([]byte("null"), target); err != nil {
					return fields, err
				}
			}
		}
	}
	fields.ContentType = "text"
	if value, ok := metadata["content"]; ok {
		var content string
		if err := json.Unmarshal(value, &content); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			content = string(value)
			fields.ContentType = "json"
		}
		fields.Content = &content
		delete(metadata, "content")
	}
	raw, err := json.Marshal(metadata)
	fields.Metadata = string(raw)
	return fields, err
}

func marshalMessage(fields entity.ConversationMessageFields) (json.RawMessage, error) {
	metadata := make(map[string]json.RawMessage)
	if len(fields.Metadata) > 0 {
		if err := json.Unmarshal([]byte(fields.Metadata), &metadata); err != nil {
			return nil, err
		}
		if metadata == nil {
			metadata = make(map[string]json.RawMessage)
		}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	for key, value := range values {
		metadata[key] = value
	}
	if fields.Content != nil && fields.ContentType == "json" {
		metadata["content"] = json.RawMessage(*fields.Content)
	}
	return json.Marshal(metadata)
}

func messageValues(fields entity.ConversationMessageFields) map[string]interface{} {
	return map[string]interface{}{
		"message_id": fields.MessageID, "role": fields.Role, "content": fields.Content, "content_type": fields.ContentType,
		"status": fields.Status, "thumb_up": fields.ThumbUp, "feedback": fields.Feedback, "created_at": fields.CreatedAt, "metadata": fields.Metadata,
	}
}

func syncMessages(ctx context.Context, db *gorm.DB, table, conversationID string, items []json.RawMessage) error {
	var existing []entity.ConversationMessage
	if err := db.WithContext(ctx).Table(table).Where("conversation_id = ?", conversationID).Find(&existing).Error; err != nil {
		return err
	}
	existingByPosition := make(map[int]entity.ConversationMessageFields, len(existing))
	for _, row := range existing {
		existingByPosition[row.Position] = row.ConversationMessageFields
	}
	for position, item := range items {
		fields, err := flattenMessage(item)
		if err != nil {
			return fmt.Errorf("flatten message %d: %w", position, err)
		}
		query := db.WithContext(ctx).Table(table).Where("conversation_id = ? AND position = ?", conversationID, position)
		values := messageValues(fields)
		if old, ok := existingByPosition[position]; ok {
			oldRaw, err := marshalMessage(old)
			if err != nil {
				return err
			}
			newRaw, err := marshalMessage(fields)
			if err != nil {
				return err
			}
			if bytes.Equal(oldRaw, newRaw) {
				continue
			}
			if err := query.Updates(values).Error; err != nil {
				return err
			}
			continue
		}
		values["conversation_id"], values["position"] = conversationID, position
		if err := db.WithContext(ctx).Table(table).Create(values).Error; err != nil {
			if !errors.Is(err, gorm.ErrDuplicatedKey) {
				return err
			}
			delete(values, "conversation_id")
			delete(values, "position")
			if err := query.Updates(values).Error; err != nil {
				return err
			}
		}
	}
	return db.WithContext(ctx).Table(table).Where("conversation_id = ? AND position >= ?", conversationID, len(items)).Delete(map[string]interface{}{}).Error
}

func syncHistory(ctx context.Context, db *gorm.DB, table, payloadColumn, kind, conversationID string, raw json.RawMessage) error {
	items, err := splitHistory(raw, kind)
	if err != nil {
		return fmt.Errorf("split %s history: %w", kind, err)
	}
	if kind == "message" {
		return syncMessages(ctx, db, table, conversationID, items)
	}

	var existing []conversationHistoryRow
	if err = db.WithContext(ctx).Table(table).
		Select(conversationHistoryOrderColumn+", "+payloadColumn+" AS payload").
		Where(conversationHistoryIDColumn+" = ?", conversationID).
		Find(&existing).Error; err != nil {
		return err
	}
	existingByPosition := make(map[int]string, len(existing))
	for _, row := range existing {
		existingByPosition[row.Position] = row.Payload
	}

	for position, item := range items {
		item, err = compactHistoryItem(item)
		if err != nil {
			return fmt.Errorf("compact %s history item %d: %w", kind, position, err)
		}
		if old, ok := existingByPosition[position]; ok {
			oldCompact, compactErr := compactHistoryItem(json.RawMessage(old))
			if compactErr == nil && bytes.Equal(oldCompact, item) {
				continue
			}
			if err = db.WithContext(ctx).Table(table).
				Where(conversationHistoryIDColumn+" = ? AND "+conversationHistoryOrderColumn+" = ?", conversationID, position).
				Update(payloadColumn, string(item)).Error; err != nil {
				return err
			}
			continue
		}
		row := map[string]interface{}{conversationHistoryIDColumn: conversationID, conversationHistoryOrderColumn: position, payloadColumn: string(item)}
		if err = db.WithContext(ctx).Table(table).Create(row).Error; err != nil {
			if !errors.Is(err, gorm.ErrDuplicatedKey) {
				return err
			}
			if err = db.WithContext(ctx).Table(table).
				Where(conversationHistoryIDColumn+" = ? AND "+conversationHistoryOrderColumn+" = ?", conversationID, position).
				Update(payloadColumn, string(item)).Error; err != nil {
				return err
			}
		}
	}

	return db.WithContext(ctx).Table(table).
		Where(conversationHistoryIDColumn+" = ? AND "+conversationHistoryOrderColumn+" >= ?", conversationID, len(items)).
		Delete(map[string]interface{}{}).Error
}

func loadHistory(ctx context.Context, db *gorm.DB, table, payloadColumn string, conversationIDs []string) (map[string]json.RawMessage, error) {
	result := make(map[string]json.RawMessage, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return result, nil
	}
	itemsByConversation := make(map[string][]json.RawMessage, len(conversationIDs))
	query := db.WithContext(ctx).Table(table).Where(conversationHistoryIDColumn+" IN ?", conversationIDs).Order(conversationHistoryIDColumn + ", " + conversationHistoryOrderColumn)
	if payloadColumn == "message" {
		var rows []entity.ConversationMessage
		if err := query.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			raw, err := marshalMessage(row.ConversationMessageFields)
			if err != nil {
				return nil, fmt.Errorf("marshal message for conversation %s: %w", row.ConversationID, err)
			}
			itemsByConversation[row.ConversationID] = append(itemsByConversation[row.ConversationID], raw)
		}
	} else {
		var rows []conversationHistoryRow
		if err := query.Select(conversationHistoryIDColumn + ", " + conversationHistoryOrderColumn + ", " + payloadColumn + " AS payload").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			itemsByConversation[row.ConversationID] = append(itemsByConversation[row.ConversationID], json.RawMessage(row.Payload))
		}
	}
	for _, conversationID := range conversationIDs {
		items := itemsByConversation[conversationID]
		if items == nil {
			items = []json.RawMessage{}
		}
		raw, err := json.Marshal(items)
		if err != nil {
			return nil, fmt.Errorf("marshal history for conversation %s: %w", conversationID, err)
		}
		result[conversationID] = raw
	}
	return result, nil
}

func hydrateChatSessions(ctx context.Context, db *gorm.DB, sessions []*entity.ChatSession) error {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}
	messages, err := loadHistory(ctx, db, conversationMessageTable, "message", ids)
	if err != nil {
		return err
	}
	references, err := loadHistory(ctx, db, conversationReferenceTable, "reference", ids)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		session.Message = messages[session.ID]
		session.Reference = references[session.ID]
	}
	return nil
}

func hydrateAPIConversations(ctx context.Context, db *gorm.DB, sessions []*entity.API4Conversation) error {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}
	messages, err := loadHistory(ctx, db, apiConversationMessageTable, "message", ids)
	if err != nil {
		return err
	}
	references, err := loadHistory(ctx, db, apiConversationReferenceTable, "reference", ids)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		session.Message = messages[session.ID]
		session.Reference = references[session.ID]
	}
	return nil
}

func deleteHistory(ctx context.Context, db *gorm.DB, tables []string, conversationIDs []string) error {
	if len(conversationIDs) == 0 {
		return nil
	}
	for _, table := range tables {
		if err := db.WithContext(ctx).Table(table).Where(conversationHistoryIDColumn+" IN ?", conversationIDs).Delete(map[string]interface{}{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func popHistoryUpdate(updates map[string]interface{}, key string) (json.RawMessage, bool, error) {
	value, ok := updates[key]
	if !ok {
		value, ok = updates[strings.ToUpper(key[:1])+key[1:]]
		if !ok {
			return nil, false, nil
		}
		delete(updates, strings.ToUpper(key[:1])+key[1:])
	} else {
		delete(updates, key)
	}
	raw, err := historyRaw(value)
	return raw, true, err
}
