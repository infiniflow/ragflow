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
	MessagePos     *int   `gorm:"column:message_position"`
}

// ConversationHistoryUpdate changes only the addressed turn, never synchronizing
// historical payloads. QuestionID places an answer in its question's reserved slot.
type ConversationHistoryUpdate struct {
	Message           map[string]interface{}
	QuestionID        string
	Reference         map[string]interface{}
	AppendReference   bool
	DeleteMessageID   string
	FeedbackMessageID string
	Feedback          map[string]interface{}
}

func historyReferenceQuery(ctx context.Context, db *gorm.DB, messageTable, referenceTable, conversationID string, messagePosition int) (*gorm.DB, error) {
	query := db.WithContext(ctx).Table(referenceTable).Where("conversation_id = ?", conversationID)
	var row conversationHistoryRow
	err := query.Session(&gorm.Session{}).Select("position").Where("position = ? AND message_position = ?", messagePosition, messagePosition).Take(&row).Error
	if err == nil {
		return query.Where("position = ?", row.Position), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// Histories explicitly supplied as arrays have no position association.
	// Resolve their remaining references by assistant order, not array pairs.
	var ordinal int64
	err = db.WithContext(ctx).Table(messageTable).
		Where("conversation_id = ? AND role = ? AND position < ? AND position >= (SELECT MIN(position) FROM "+messageTable+" WHERE conversation_id = ? AND role = ?)", conversationID, "assistant", messagePosition, conversationID, "user").Count(&ordinal).Error
	if err != nil {
		return nil, err
	}
	err = query.Session(&gorm.Session{}).Select("position").Where("message_position IS NULL").Order("position").Offset(int(ordinal)).Take(&row).Error
	if err != nil {
		return nil, err
	}
	return query.Where("position = ?", row.Position), nil
}

func updateConversationHistory(ctx context.Context, db *gorm.DB, messageTable, referenceTable, conversationID string, update ConversationHistoryUpdate) error {
	if update.DeleteMessageID != "" {
		var question entity.ConversationMessage
		query := db.WithContext(ctx).Table(messageTable).Where("conversation_id = ? AND message_id = ?", conversationID, update.DeleteMessageID)
		if err := query.Session(&gorm.Session{}).Select("position").Where("role = ?", "user").Order("position").Take(&question).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		positions := []int{question.Position}
		var answer entity.ConversationMessage
		err := query.Session(&gorm.Session{}).Select("position").Where("role = ? AND position = ?", "assistant", question.Position+1).Take(&answer).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			positions = append(positions, answer.Position)
			refQuery, err := historyReferenceQuery(ctx, db, messageTable, referenceTable, conversationID, answer.Position)
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				if err := refQuery.Delete(map[string]interface{}{}).Error; err != nil {
					return err
				}
			}
		}
		return query.Where("position IN ?", positions).Delete(map[string]interface{}{}).Error
	}
	if update.FeedbackMessageID != "" {
		query := db.WithContext(ctx).Table(messageTable).Where("conversation_id = ? AND message_id = ? AND role = ?", conversationID, update.FeedbackMessageID, "assistant")
		var message entity.ConversationMessage
		if err := query.Session(&gorm.Session{}).Select("position", "metadata").Order("position").Take(&message).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		metadata := make(map[string]json.RawMessage)
		if message.Metadata != "" {
			if err := json.Unmarshal([]byte(message.Metadata), &metadata); err != nil {
				return err
			}
		}
		if metadata == nil {
			metadata = make(map[string]json.RawMessage)
		}
		delete(metadata, "thumbup")
		if feedback, ok := update.Feedback["feedback"]; ok {
			delete(metadata, "feedback")
			if _, isString := feedback.(string); feedback != nil && !isString {
				raw, err := json.Marshal(feedback)
				if err != nil {
					return err
				}
				metadata["feedback"] = raw
				update.Feedback["feedback"] = nil
			}
		}
		raw, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		update.Feedback["metadata"] = string(raw)
		return query.Where("position = ?", message.Position).Updates(update.Feedback).Error
	}
	if update.Message == nil && !update.AppendReference {
		return nil
	}
	var fields entity.ConversationMessageFields
	if update.Message != nil {
		raw, err := json.Marshal(update.Message)
		if err != nil {
			return err
		}
		fields, err = flattenMessage(raw)
		if err != nil {
			return err
		}
	}
	position := 0
	if update.QuestionID != "" {
		var question entity.ConversationMessage
		if err := db.WithContext(ctx).Table(messageTable).Select("position").Where("conversation_id = ? AND message_id = ? AND role = ?", conversationID, update.QuestionID, "user").Order("position DESC").Take(&question).Error; err != nil {
			return err
		}
		position = question.Position + 1
	} else {
		var lastMessage, lastReference int
		if err := db.WithContext(ctx).Table(messageTable).Select("COALESCE(MAX(position), -2)").Where("conversation_id = ?", conversationID).Scan(&lastMessage).Error; err != nil {
			return err
		}
		if err := db.WithContext(ctx).Table(referenceTable).Select("COALESCE(MAX(position), -2)").Where("conversation_id = ?", conversationID).Scan(&lastReference).Error; err != nil {
			return err
		}
		position = max(lastMessage, lastReference) + 2
	}
	if update.Message != nil {
		values := messageValues(fields)
		values["conversation_id"], values["position"] = conversationID, position
		// A reserved answer slot cannot overwrite a later question.
		if err := db.WithContext(ctx).Table(messageTable).Create(values).Error; err != nil {
			return err
		}
	}
	if update.AppendReference {
		raw, err := json.Marshal(update.Reference)
		if err != nil {
			return err
		}
		row := map[string]interface{}{"conversation_id": conversationID, "position": position, "reference": string(raw)}
		if update.Message != nil {
			row["message_position"] = position
		}
		return db.WithContext(ctx).Table(referenceTable).Create(row).Error
	}
	return nil
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

func createHistory(ctx context.Context, db *gorm.DB, table, kind, conversationID string, raw json.RawMessage) error {
	items, err := splitHistory(raw, kind)
	if err != nil {
		return fmt.Errorf("split %s history: %w", kind, err)
	}
	rows := make([]map[string]interface{}, 0, len(items))
	for position, item := range items {
		row := map[string]interface{}{"conversation_id": conversationID, "position": position, "reference": string(item)}
		if kind == "message" {
			fields, err := flattenMessage(item)
			if err != nil {
				return fmt.Errorf("flatten message %d: %w", position, err)
			}
			row = messageValues(fields)
			row["conversation_id"], row["position"] = conversationID, position
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}
	return db.WithContext(ctx).Table(table).CreateInBatches(rows, 100).Error
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
