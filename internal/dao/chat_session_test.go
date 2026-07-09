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
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func setupChatSessionDAOTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.API4Conversation{}, &entity.ChatSession{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func createAgentSessionForDAOTest(t *testing.T, db *gorm.DB, id, agentID, userID string, updateTime int64) {
	t.Helper()

	updateDate := time.UnixMilli(updateTime).Local()
	session := &entity.API4Conversation{
		ID:        id,
		DialogID:  agentID,
		UserID:    userID,
		Message:   json.RawMessage(`[{"role":"assistant","content":"hello"}]`),
		Reference: json.RawMessage(`[]`),
		BaseModel: entity.BaseModel{
			CreateTime: &updateTime,
			CreateDate: &updateDate,
			UpdateTime: &updateTime,
			UpdateDate: &updateDate,
		},
	}
	if err := db.Create(session).Error; err != nil {
		t.Fatalf("failed to create session %s: %v", id, err)
	}
}

func createChatSessionForDAOTest(t *testing.T, db *gorm.DB, id, chatID, name string, updateTime int64) {
	t.Helper()

	updateDate := time.UnixMilli(updateTime).Local()
	session := &entity.ChatSession{
		ID:        id,
		DialogID:  chatID,
		Name:      &name,
		Message:   json.RawMessage(`[{"role":"assistant","content":"hello"}]`),
		Reference: json.RawMessage(`[]`),
		BaseModel: entity.BaseModel{
			CreateTime: &updateTime,
			CreateDate: &updateDate,
			UpdateTime: &updateTime,
			UpdateDate: &updateDate,
		},
	}
	if err := db.Create(session).Error; err != nil {
		t.Fatalf("failed to create chat session %s: %v", id, err)
	}
}

func TestChatSessionDAOUpdateByIDRefreshesTimestampsOnEmptyUpdate(t *testing.T) {
	db := setupChatSessionDAOTestDB(t)
	pushDB(t, db)

	oldUpdateTime := int64(1000)
	createChatSessionForDAOTest(t, db, "session-1", "chat-1", "same", oldUpdateTime)

	if err := NewChatSessionDAO().UpdateByID("session-1", map[string]interface{}{}); err != nil {
		t.Fatalf("UpdateByID failed: %v", err)
	}

	session, err := NewChatSessionDAO().GetByID("session-1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if session.UpdateTime == nil || *session.UpdateTime <= oldUpdateTime {
		t.Fatalf("expected update_time to be refreshed, got %v", session.UpdateTime)
	}
	if session.UpdateDate == nil || !session.UpdateDate.After(time.UnixMilli(oldUpdateTime)) {
		t.Fatalf("expected update_date to be refreshed, got %v", session.UpdateDate)
	}
}

func TestChatSessionDAOUpdateByIDSameValueSucceeds(t *testing.T) {
	db := setupChatSessionDAOTestDB(t)
	pushDB(t, db)

	createChatSessionForDAOTest(t, db, "session-1", "chat-1", "same", 1000)

	if err := NewChatSessionDAO().UpdateByID("session-1", map[string]interface{}{"name": "same"}); err != nil {
		t.Fatalf("UpdateByID failed: %v", err)
	}
}

func TestChatSessionDAOUpdateByIDMissingSession(t *testing.T) {
	db := setupChatSessionDAOTestDB(t)
	pushDB(t, db)

	err := NewChatSessionDAO().UpdateByID("missing", nil)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestChatSessionDAOListAgentSessionsOrdersByUpdateTimeDesc(t *testing.T) {
	db := setupChatSessionDAOTestDB(t)
	pushDB(t, db)

	createAgentSessionForDAOTest(t, db, "session-old", "agent-1", "user-1", 1000)
	createAgentSessionForDAOTest(t, db, "session-new", "agent-1", "user-1", 3000)
	createAgentSessionForDAOTest(t, db, "session-middle", "agent-1", "user-1", 2000)
	createAgentSessionForDAOTest(t, db, "session-other-agent", "agent-2", "user-1", 9999)

	total, sessions, err := NewChatSessionDAO().ListAgentSessions(ListAgentSessionsParams{
		AgentID:  "agent-1",
		Page:     1,
		PageSize: 10,
		OrderBy:  "update_time",
		Desc:     true,
	})
	if err != nil {
		t.Fatalf("ListAgentSessions failed: %v", err)
	}

	if total != 3 {
		t.Fatalf("expected total 3, got %d", total)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}

	wantIDs := []string{"session-new", "session-middle", "session-old"}
	for i, wantID := range wantIDs {
		if sessions[i].ID != wantID {
			t.Fatalf("session[%d]: expected %s, got %s", i, wantID, sessions[i].ID)
		}
		if sessions[i].DialogID != "agent-1" {
			t.Fatalf("session[%d]: expected agent-1, got %s", i, sessions[i].DialogID)
		}
	}
}

func TestChatSessionDAOListAgentSessionsFiltersAndPaginates(t *testing.T) {
	db := setupChatSessionDAOTestDB(t)
	pushDB(t, db)

	createAgentSessionForDAOTest(t, db, "session-1", "agent-1", "user-1", 1000)
	createAgentSessionForDAOTest(t, db, "session-2", "agent-1", "user-1", 2000)
	createAgentSessionForDAOTest(t, db, "session-3", "agent-1", "user-1", 3000)
	createAgentSessionForDAOTest(t, db, "session-other-user", "agent-1", "user-2", 4000)

	total, sessions, err := NewChatSessionDAO().ListAgentSessions(ListAgentSessionsParams{
		AgentID:  "agent-1",
		UserID:   "user-1",
		Page:     2,
		PageSize: 1,
		OrderBy:  "update_time",
		Desc:     false,
	})
	if err != nil {
		t.Fatalf("ListAgentSessions failed: %v", err)
	}

	if total != 3 {
		t.Fatalf("expected total 3 after user filter, got %d", total)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected one paginated session, got %d", len(sessions))
	}
	if sessions[0].ID != "session-2" {
		t.Fatalf("expected second ascending session session-2, got %s", sessions[0].ID)
	}
	if sessions[0].UserID != "user-1" {
		t.Fatalf("expected user-1, got %s", sessions[0].UserID)
	}
}
