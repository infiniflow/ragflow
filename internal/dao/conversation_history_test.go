// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package dao

import (
	"encoding/json"
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

func TestFlattenMessageRoundTrip(t *testing.T) {
	for _, item := range []string{
		`{"id":"turn-1","role":"assistant","content":"你好\nanswer","status":"completed","thumbup":false,"feedback":"","created_at":123.25,"attachments":[{"id":"file-1"}]}`,
		`{"role":"user","content":[{"type":"text","text":"question"},{"type":"image_url","image_url":{"url":"test"}}],"tool_calls":[]}`,
		`{"id":null,"role":null,"content":null,"thumbup":null,"created_at":null,"feedback":null}`,
		`{"id":1,"role":false,"content":{"text":"question"},"thumbup":"false","created_at":"123","feedback":{"reason":"bad"},"status":[]}`,
		`{}`,
	} {
		t.Run(item, func(t *testing.T) {
			fields, err := flattenMessage(json.RawMessage(item))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := marshalMessage(fields)
			if err != nil {
				t.Fatal(err)
			}
			var want, got interface{}
			if err := json.Unmarshal([]byte(item), &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("round trip = %s, want %s", raw, item)
			}
		})
	}
	for _, item := range []string{`null`, `[]`, `"text"`} {
		if _, err := flattenMessage(json.RawMessage(item)); err == nil {
			t.Fatalf("accepted non-object message %s", item)
		}
	}
}

func TestFlattenedMessagePersistence(t *testing.T) {
	for _, table := range []string{conversationMessageTable, apiConversationMessageTable} {
		t.Run(table, func(t *testing.T) {
			db := setupChatSessionDAOTestDB(t)
			parent := "conversation"
			if table == apiConversationMessageTable {
				parent = "api_4_conversation"
			}
			values := map[string]interface{}{"id": "session-1", "dialog_id": "dialog-1"}
			if parent == "api_4_conversation" {
				values["user_id"] = "user-1"
			}
			if err := db.Table(parent).Create(values).Error; err != nil {
				t.Fatal(err)
			}
			raw := json.RawMessage(`[{"id":"turn-1","role":"assistant","content":"searchable content","thumbup":false,"feedback":"bad","created_at":123.25,"attachment":"file-1"}]`)
			if err := createHistory(t.Context(), db, table, "message", "session-1", raw); err != nil {
				t.Fatal(err)
			}
			var row entity.ConversationMessage
			if err := db.Table(table).Where("conversation_id = ? AND role = ? AND content LIKE ?", "session-1", "assistant", "%searchable%").First(&row).Error; err != nil {
				t.Fatal(err)
			}
			if row.MessageID == nil || *row.MessageID != "turn-1" || row.ThumbUp == nil || *row.ThumbUp || row.CreatedAt == nil || *row.CreatedAt != 123.25 {
				t.Fatalf("unexpected flattened fields: %+v", row.ConversationMessageFields)
			}
			if string(row.Metadata) != `{"attachment":"file-1"}` {
				t.Fatalf("metadata contains flattened fields: %s", row.Metadata)
			}
			histories, err := loadHistory(t.Context(), db, table, "message", []string{"session-1"})
			if err != nil {
				t.Fatal(err)
			}
			var want, got interface{}
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(histories["session-1"], &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("loaded history = %s, want %s", histories["session-1"], raw)
			}
		})
	}
}
