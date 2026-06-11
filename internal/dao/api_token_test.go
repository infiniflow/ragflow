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
	"sort"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func setupAPI4ConversationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.API4Conversation{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func createAPI4ConversationForDAOTest(t *testing.T, id, agentID string) {
	t.Helper()
	if err := DB.Create(&entity.API4Conversation{
		ID:        id,
		DialogID:  agentID,
		UserID:    "user-1",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	}).Error; err != nil {
		t.Fatalf("failed to create api conversation %s: %v", id, err)
	}
}

func TestAPI4ConversationDAOGetBySessionID(t *testing.T) {
	db := setupAPI4ConversationTestDB(t)
	pushDB(t, db)

	createAPI4ConversationForDAOTest(t, "session-1", "agent-1")

	session, err := NewAPI4ConversationDAO().GetBySessionID("session-1", "agent-1")
	if err != nil {
		t.Fatalf("GetBySessionID failed: %v", err)
	}
	if session == nil {
		t.Fatal("expected session, got nil")
	}
	if session.ID != "session-1" {
		t.Fatalf("expected session-1, got %s", session.ID)
	}
	if session.DialogID != "agent-1" {
		t.Fatalf("expected agent-1, got %s", session.DialogID)
	}
}

func TestAPI4ConversationDAOGetBySessionIDWrongAgent(t *testing.T) {
	db := setupAPI4ConversationTestDB(t)
	pushDB(t, db)

	createAPI4ConversationForDAOTest(t, "session-1", "agent-1")

	session, err := NewAPI4ConversationDAO().GetBySessionID("session-1", "agent-2")
	if err != nil {
		t.Fatalf("GetBySessionID failed: %v", err)
	}
	if session != nil {
		t.Fatalf("expected nil for wrong agent, got %+v", session)
	}
}

func TestAPI4ConversationDAOGetBySessionIDNoRows(t *testing.T) {
	db := setupAPI4ConversationTestDB(t)
	pushDB(t, db)

	createAPI4ConversationForDAOTest(t, "session-1", "agent-1")

	session, err := NewAPI4ConversationDAO().GetBySessionID("missing-session", "agent-1")
	if err != nil {
		t.Fatalf("GetBySessionID failed: %v", err)
	}
	if session != nil {
		t.Fatalf("expected nil for missing session, got %+v", session)
	}
}

func TestAPI4ConversationDAOListIDsByAgentID(t *testing.T) {
	db := setupAPI4ConversationTestDB(t)
	pushDB(t, db)

	createAPI4ConversationForDAOTest(t, "session-1", "agent-1")
	createAPI4ConversationForDAOTest(t, "session-2", "agent-1")
	createAPI4ConversationForDAOTest(t, "session-other", "agent-2")

	ids, err := NewAPI4ConversationDAO().ListIDsByAgentID("agent-1")
	if err != nil {
		t.Fatalf("ListIDsByAgentID failed: %v", err)
	}

	sort.Strings(ids)
	want := []string{"session-1", "session-2"}
	if len(ids) != len(want) {
		t.Fatalf("expected %d ids, got %d: %v", len(want), len(ids), ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("expected ids %v, got %v", want, ids)
		}
	}
}

func TestAPI4ConversationDAOListIDsByAgentIDNoRows(t *testing.T) {
	db := setupAPI4ConversationTestDB(t)
	pushDB(t, db)

	createAPI4ConversationForDAOTest(t, "session-1", "agent-1")

	ids, err := NewAPI4ConversationDAO().ListIDsByAgentID("agent-2")
	if err != nil {
		t.Fatalf("ListIDsByAgentID failed: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected empty ids for missing agent, got %v", ids)
	}
}
