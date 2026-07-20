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

package service

import (
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// ---------------------------------------------------------------------------
// CreateSession contract — mirrors the assertions in
// test_session_create_validation_and_deleted_chat_contract that exercise the
// Go service layer (name validation -> 102, auth -> 109, truncation, success).
// ---------------------------------------------------------------------------

func TestCreateSession_Success(t *testing.T) {
	store := newFakeSessionStore()
	store.dialogExists["user-1|chat-1"] = true
	store.dialogs["chat-1"] = &entity.Chat{ID: "chat-1", PromptConfig: entity.JSONMap{"prologue": "hi"}}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	resp, code, err := svc.CreateSession("user-1", "chat-1", map[string]interface{}{"name": "valid"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%v", code)
	}
	if resp.Name == nil || *resp.Name != "valid" {
		t.Fatalf("name=%v", resp.Name)
	}
	if resp.ChatID != "chat-1" {
		t.Fatalf("chat_id=%q", resp.ChatID)
	}
	if len(store.sessions) != 1 {
		t.Fatalf("expected 1 session created, got %d", len(store.sessions))
	}
}

func TestCreateSession_RejectsEmptyOrNonStringName(t *testing.T) {
	store := newFakeSessionStore()
	store.dialogExists["user-1|chat-1"] = true
	store.dialogs["chat-1"] = &entity.Chat{ID: "chat-1"}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	for _, name := range []interface{}{"", "   ", 1} {
		_, code, err := svc.CreateSession("user-1", "chat-1", map[string]interface{}{"name": name})
		if err == nil || err.Error() != "`name` can not be empty." {
			t.Fatalf("name=%#v err=%v", name, err)
		}
		if code != common.CodeDataError {
			t.Fatalf("name=%#v code=%v", name, code)
		}
	}
}

func TestCreateSession_TruncatesLongName(t *testing.T) {
	store := newFakeSessionStore()
	store.dialogExists["user-1|chat-1"] = true
	store.dialogs["chat-1"] = &entity.Chat{ID: "chat-1"}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	longName := strings.Repeat("a", 300)
	resp, code, err := svc.CreateSession("user-1", "chat-1", map[string]interface{}{"name": longName})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%v", code)
	}
	if resp.Name == nil || len([]rune(*resp.Name)) != 255 {
		t.Fatalf("expected name truncated to 255 runes, got %v", resp.Name)
	}
}

func TestCreateSession_NotOwner(t *testing.T) {
	store := newFakeSessionStore()
	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	_, code, err := svc.CreateSession("user-1", "chat-1", map[string]interface{}{"name": "x"})
	if err == nil || err.Error() != "No authorization." {
		t.Fatalf("err=%v", err)
	}
	if code != common.CodeAuthenticationError {
		t.Fatalf("code=%v", code)
	}
}

// ---------------------------------------------------------------------------
// DeleteSessions contract — mirrors the service-layer assertions in
// test_session_delete_basic_scenarios.
// ---------------------------------------------------------------------------

func TestDeleteSessions_SuccessByIDs(t *testing.T) {
	store := newFakeSessionStore()
	store.dialogExists["user-1|chat-1"] = true
	store.sessions["s1"] = &entity.ChatSession{ID: "s1", DialogID: "chat-1"}
	store.sessions["s2"] = &entity.ChatSession{ID: "s2", DialogID: "chat-1"}

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	result, message, code, err := svc.DeleteSessions("user-1", "chat-1", map[string]interface{}{"ids": []interface{}{"s1"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%v", code)
	}
	if message != "success" {
		t.Fatalf("message=%q", message)
	}
	if result != true {
		t.Fatalf("result=%v", result)
	}
	if len(store.sessions) != 1 {
		t.Fatalf("expected 1 session remaining, got %d", len(store.sessions))
	}
}

func TestDeleteSessions_DeleteAllAndInvalidID(t *testing.T) {
	store := newFakeSessionStore()
	store.dialogExists["user-1|chat-1"] = true

	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	// Empty payload -> success with empty result map.
	if _, _, code, err := svc.DeleteSessions("user-1", "chat-1", map[string]interface{}{}); err != nil || code != common.CodeSuccess {
		t.Fatalf("empty payload: code=%v err=%v", code, err)
	}

	// delete_all removes every session for the chat.
	store.sessions["s1"] = &entity.ChatSession{ID: "s1", DialogID: "chat-1"}
	if _, _, code, err := svc.DeleteSessions("user-1", "chat-1", map[string]interface{}{"delete_all": true}); err != nil || code != common.CodeSuccess {
		t.Fatalf("delete_all: code=%v err=%v", code, err)
	}
	if len(store.sessions) != 0 {
		t.Fatalf("delete_all should remove all, got %d", len(store.sessions))
	}

	// Unknown id -> DataError reporting the unowned session.
	store.sessions["s1"] = &entity.ChatSession{ID: "s1", DialogID: "chat-1"}
	_, _, code, err := svc.DeleteSessions("user-1", "chat-1", map[string]interface{}{"ids": []interface{}{"missing"}})
	if err == nil || !strings.Contains(err.Error(), "The chat doesn't own the session missing") {
		t.Fatalf("invalid id: err=%v", err)
	}
	if code != common.CodeDataError {
		t.Fatalf("invalid id code=%v", code)
	}
}

func TestDeleteSessions_NotOwner(t *testing.T) {
	store := newFakeSessionStore()
	svc := &ChatSessionService{
		chatSessionDAO: store,
		userTenantDAO:  &fakeTenantStore{},
		pipeline:       &fakePipeline{},
	}

	_, _, code, err := svc.DeleteSessions("user-1", "chat-1", map[string]interface{}{"ids": []interface{}{"s1"}})
	if err == nil || err.Error() != "No authorization." {
		t.Fatalf("err=%v", err)
	}
	if code != common.CodeAuthenticationError {
		t.Fatalf("code=%v", code)
	}
}
