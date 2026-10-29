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
	conversationMessageTable      = "conversation_message"
	conversationReferenceTable    = "conversation_reference"
	apiConversationMessageTable   = "api_4_conversation_message"
	apiConversationReferenceTable = "api_4_conversation_reference"
	conversationHistoryIDColumn   = "conversation_id"
)

// ConversationHistoryOptions selects message payloads needed by the caller.
// The zero value loads history without answer details.
type ConversationHistoryOptions struct {
	NoHistory        bool
	IncludeReference bool
	IncludePrompt    bool
	IncludeUsage     bool
	IncludeElapsedMs bool
}

// ConversationHistoryUpdate changes only the addressed turn, never synchronizing
// historical payloads. QuestionID places an answer in its question's reserved slot.
type ConversationHistoryUpdate struct {
	Message           map[string]interface{}
	QuestionID        string
	DeleteMessageID   string
	FeedbackMessageID string
	Feedback          map[string]interface{}
}

func updateConversationHistory(ctx context.Context, db *gorm.DB, messageTable, conversationID string, update ConversationHistoryUpdate) error {
	if update.DeleteMessageID != "" {
		var question entity.ConversationMessage
		query := db.WithContext(ctx).Table(messageTable).Where("conversation_id = ? AND message_id = ?", conversationID, update.DeleteMessageID)
		if err := query.Session(&gorm.Session{}).Select("position").Where("role = ?", "user").Order("position").Take(&question).Error; err != nil {
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
	if update.Message == nil {
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
		var lastMessage int
		if err := db.WithContext(ctx).Table(messageTable).Select("COALESCE(MAX(position), -2)").Where("conversation_id = ?", conversationID).Scan(&lastMessage).Error; err != nil {
			return err
		}
		position = lastMessage + 2
	}
	if update.Message != nil {
		values := messageValues(fields)
		values["conversation_id"], values["position"] = conversationID, position
		// A reserved answer slot cannot overwrite a later question.
		if err := db.WithContext(ctx).Table(messageTable).Create(values).Error; err != nil {
			return err
		}
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
		"prompt": &fields.Prompt, "usage": &fields.Usage, "elapsed_ms": &fields.ElapsedMs, "reference": &fields.Reference,
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

func marshalMessage(fields entity.ConversationMessageFields, options ConversationHistoryOptions) (json.RawMessage, error) {
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
	for key, include := range map[string]bool{
		"reference": options.IncludeReference, "prompt": options.IncludePrompt,
		"usage": options.IncludeUsage, "elapsed_ms": options.IncludeElapsedMs,
	} {
		if !include {
			delete(metadata, key)
		}
	}
	return json.Marshal(metadata)
}

func messageValues(fields entity.ConversationMessageFields) map[string]interface{} {
	var reference interface{}
	if len(fields.Reference) > 0 {
		reference = string(fields.Reference)
	}
	return map[string]interface{}{
		"message_id": fields.MessageID, "role": fields.Role, "content": fields.Content, "content_type": fields.ContentType,
		"status": fields.Status, "thumb_up": fields.ThumbUp, "feedback": fields.Feedback, "created_at": fields.CreatedAt, "metadata": fields.Metadata,
		"prompt": fields.Prompt, "usage": fields.Usage, "elapsed_ms": fields.ElapsedMs, "reference": reference,
	}
}

func createHistory(ctx context.Context, db *gorm.DB, table, kind, conversationID string, raw json.RawMessage) error {
	rows, err := historyRows(kind, conversationID, raw)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return db.WithContext(ctx).Table(table).CreateInBatches(rows, 100).Error
}

// historyRows flattens a message or reference payload into child rows. Its
// positions are the payload indexes, starting from zero.
func historyRows(kind, conversationID string, raw json.RawMessage) ([]map[string]interface{}, error) {
	items, err := splitHistory(raw, kind)
	if err != nil {
		return nil, fmt.Errorf("split %s history: %w", kind, err)
	}
	rows := make([]map[string]interface{}, 0, len(items))
	for position, item := range items {
		row := map[string]interface{}{"conversation_id": conversationID, "position": position, "reference": string(item)}
		if kind == "message" {
			fields, err := flattenMessage(item)
			if err != nil {
				return nil, fmt.Errorf("flatten message %d: %w", position, err)
			}
			row = messageValues(fields)
			row["conversation_id"], row["position"] = conversationID, position
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// loadHistory excludes answer details unless the caller needs them for display.
func loadHistory(ctx context.Context, db *gorm.DB, table string, conversationIDs []string, options ConversationHistoryOptions) (map[string]json.RawMessage, error) {
	result := make(map[string]json.RawMessage, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return result, nil
	}
	query := db.WithContext(ctx).Session(&gorm.Session{QueryFields: true}).Table(table).Where("conversation_id IN ?", conversationIDs).Order("conversation_id, position")
	var omitted []string
	for column, include := range map[string]bool{
		"reference": options.IncludeReference, "prompt": options.IncludePrompt,
		"usage": options.IncludeUsage, "elapsed_ms": options.IncludeElapsedMs,
	} {
		if !include {
			omitted = append(omitted, column)
		}
	}
	if len(omitted) > 0 {
		query = query.Omit(omitted...)
	}
	var rows []entity.ConversationMessage
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	itemsByConversation := make(map[string][]json.RawMessage, len(conversationIDs))
	for _, row := range rows {
		fields := row.ConversationMessageFields
		raw, err := marshalMessage(fields, options)
		if err != nil {
			return nil, fmt.Errorf("marshal message for conversation %s: %w", row.ConversationID, err)
		}
		itemsByConversation[row.ConversationID] = append(itemsByConversation[row.ConversationID], raw)
	}
	for _, id := range conversationIDs {
		items := itemsByConversation[id]
		if items == nil {
			items = []json.RawMessage{}
		}
		raw, err := json.Marshal(items)
		if err != nil {
			return nil, fmt.Errorf("marshal history for conversation %s: %w", id, err)
		}
		result[id] = raw
	}
	return result, nil
}

func hydrateChatSessions(ctx context.Context, db *gorm.DB, sessions []*entity.ChatSession, options ConversationHistoryOptions) error {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}
	messages, err := loadHistory(ctx, db, conversationMessageTable, ids, options)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		session.Message = messages[session.ID]
	}
	return nil
}

func hydrateAPIConversations(ctx context.Context, db *gorm.DB, sessions []*entity.API4Conversation, options ConversationHistoryOptions) error {
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
	}
	messages, err := loadHistory(ctx, db, apiConversationMessageTable, ids, options)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		session.Message = messages[session.ID]
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
