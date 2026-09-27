// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package dao

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/entity"
)

type historyBenchmarkLogger struct {
	logger.Interface
	enabled      bool
	statements   int
	selectedRows int64
}

func (l *historyBenchmarkLogger) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	if !l.enabled {
		return
	}
	sql, rows := query()
	l.statements++
	if strings.HasPrefix(sql, "SELECT") && rows > 0 {
		l.selectedRows += rows
	}
}

// BenchmarkConversationHistoryOperations excludes history loading for an LLM
// prompt or a full-history API response. Search misses use 20 conversations.
func BenchmarkConversationHistoryOperations(b *testing.B) {
	for _, size := range []int{100, 1000, 5000} {
		for _, operation := range []string{"question", "delayed_answer", "search_miss"} {
			answer := operation == "delayed_answer"
			b.Run(fmt.Sprintf("%s/%d", operation, size), func(b *testing.B) {
				stats := &historyBenchmarkLogger{Interface: logger.Default.LogMode(logger.Silent)}
				db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: stats, TranslateError: true})
				if err != nil {
					b.Fatal(err)
				}
				sqlDB, err := db.DB()
				if err != nil {
					b.Fatal(err)
				}
				sqlDB.SetMaxOpenConns(1)
				b.Cleanup(func() { _ = sqlDB.Close() })
				if err := db.AutoMigrate(&entity.API4Conversation{}, &entity.API4ConversationMessage{}, &entity.API4ConversationReference{}); err != nil {
					b.Fatal(err)
				}
				if err := db.Exec("CREATE INDEX idx_api_4_conversation_dialog_updated ON api_4_conversation (dialog_id, update_time, id)").Error; err != nil {
					b.Fatal(err)
				}
				conversation := &entity.API4Conversation{ID: "history-benchmark", DialogID: "dialog", UserID: "user"}
				if err := db.Create(conversation).Error; err != nil {
					b.Fatal(err)
				}
				prologue, userRole, assistantRole, pendingID, content := "Welcome", "user", "assistant", "pending", "message content"
				messages := []entity.API4ConversationMessage{
					{ConversationID: conversation.ID, Position: 0, ConversationMessageFields: entity.ConversationMessageFields{Role: &assistantRole, Content: &prologue, ContentType: "text", Metadata: "{}"}},
					{ConversationID: conversation.ID, Position: 2, ConversationMessageFields: entity.ConversationMessageFields{Role: &userRole, MessageID: &pendingID, Content: &content, ContentType: "text", Metadata: "{}"}},
				}
				var references []entity.API4ConversationReference
				for i := 0; i < (size-2)/2; i++ {
					id, position := fmt.Sprintf("turn-%d", i), 4+i*4
					messages = append(messages,
						entity.API4ConversationMessage{ConversationID: conversation.ID, Position: position, ConversationMessageFields: entity.ConversationMessageFields{Role: &userRole, MessageID: &id, Content: &content, ContentType: "text", Metadata: "{}"}},
						entity.API4ConversationMessage{ConversationID: conversation.ID, Position: position + 1, ConversationMessageFields: entity.ConversationMessageFields{Role: &assistantRole, MessageID: &id, Content: &content, ContentType: "text", Metadata: "{}"}})
					answerPosition := position + 1
					references = append(references, entity.API4ConversationReference{ConversationID: conversation.ID, Position: answerPosition, MessagePos: &answerPosition, Reference: []byte(`{"chunks":[]}`)})
				}
				if err := db.CreateInBatches(messages, 100).Error; err != nil {
					b.Fatal(err)
				}
				if err := db.CreateInBatches(references, 100).Error; err != nil {
					b.Fatal(err)
				}
				if operation == "search_miss" {
					for i := 1; i < 20; i++ {
						id := fmt.Sprintf("history-benchmark-%d", i)
						if err := db.Create(&entity.API4Conversation{ID: id, DialogID: conversation.DialogID, UserID: conversation.UserID}).Error; err != nil {
							b.Fatal(err)
						}
						for j := range messages {
							messages[j].ConversationID = id
						}
						if err := db.CreateInBatches(messages, 100).Error; err != nil {
							b.Fatal(err)
						}
					}
				}
				store := NewAPI4ConversationDAO()
				update := ConversationHistoryUpdate{Message: map[string]interface{}{"role": "user", "id": "new-question", "content": content, "created_at": 1.0}}
				if answer {
					update = ConversationHistoryUpdate{Message: map[string]interface{}{"role": "assistant", "id": pendingID, "content": content, "created_at": 2.0}, QuestionID: pendingID, Reference: map[string]interface{}{"chunks": []interface{}{}}, AppendReference: true}
				}
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if answer && i > 0 {
						b.StopTimer()
						stats.enabled = false
						if err := db.Where("conversation_id = ? AND position = ?", conversation.ID, 3).Delete(&entity.API4ConversationReference{}).Error; err != nil {
							b.Fatal(err)
						}
						if err := db.Where("conversation_id = ? AND position = ?", conversation.ID, 3).Delete(&entity.API4ConversationMessage{}).Error; err != nil {
							b.Fatal(err)
						}
						b.StartTimer()
					}
					stats.enabled = true
					if operation == "search_miss" {
						total, sessions, err := NewChatSessionDAO().ListAgentSessions(b.Context(), db, ListAgentSessionsParams{AgentID: conversation.DialogID, Keywords: "absent-keyword", NoHistory: true})
						if err != nil || total != 0 || len(sessions) != 0 {
							b.Fatalf("search miss: total=%d, sessions=%d, err=%v", total, len(sessions), err)
						}
						continue
					}
					if err := store.UpdateHistory(b.Context(), db, conversation.ID, conversation.DialogID, conversation.UserID, nil, update); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				stats.enabled = false
				b.ReportMetric(float64(stats.statements)/float64(b.N), "sql/op")
				b.ReportMetric(float64(stats.selectedRows)/float64(b.N), "selected_rows/op")
				if operation != "search_miss" {
					messageID, remaining := "new-question", size+b.N-1
					if answer {
						messageID, remaining = pendingID, size-1
						if err := store.UpdateHistory(b.Context(), db, conversation.ID, conversation.DialogID, conversation.UserID, nil, ConversationHistoryUpdate{FeedbackMessageID: pendingID, Feedback: map[string]interface{}{"thumb_up": true, "feedback": nil}}); err != nil {
							b.Fatal(err)
						}
					}
					if err := store.UpdateHistory(b.Context(), db, conversation.ID, conversation.DialogID, conversation.UserID, nil, ConversationHistoryUpdate{DeleteMessageID: messageID}); err != nil {
						b.Fatal(err)
					}
					var messageCount, referenceCount int64
					if err := db.Model(&entity.API4ConversationMessage{}).Where("conversation_id = ?", conversation.ID).Count(&messageCount).Error; err != nil {
						b.Fatal(err)
					}
					if err := db.Model(&entity.API4ConversationReference{}).Where("conversation_id = ?", conversation.ID).Count(&referenceCount).Error; err != nil {
						b.Fatal(err)
					}
					if messageCount != int64(remaining) || referenceCount != int64((size-2)/2) {
						b.Fatalf("delete must preserve later turns: messages=%d, references=%d", messageCount, referenceCount)
					}
				}
			})
		}
	}
}
