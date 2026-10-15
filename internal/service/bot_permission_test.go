//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func TestBotServiceChatbotEndpointsUseChatPermissions(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.Chat{}); err != nil {
		t.Fatalf("migrate chat: %v", err)
	}
	pushServiceDB(t, db)

	status := string(entity.StatusValid)
	name := "shared bot"
	if err := db.Create(&entity.Chat{
		ID:           "chat-1",
		TenantID:     "owner-1",
		Name:         &name,
		LLMID:        "model-1",
		LLMSetting:   entity.JSONMap{},
		PromptType:   "simple",
		PromptConfig: entity.JSONMap{},
		KBIDs:        entity.JSONSlice{},
		Status:       &status,
	}).Error; err != nil {
		t.Fatalf("seed chat: %v", err)
	}

	svc := NewBotService(nil)
	_, _, _, _, _, code, err := svc.ChatbotInfo(t.Context(), "owner-1", "chat-1")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ChatbotInfo(owner) = (%d, %v), want success", code, err)
	}

	_, _, _, _, _, code, err = svc.ChatbotInfo(t.Context(), "outsider", "chat-1")
	if !errors.Is(err, permission.ErrPermissionDenied) || code != common.CodeNotFound {
		t.Fatalf("ChatbotInfo(outsider) = (%d, %v), want hidden permission denial", code, err)
	}

	_, code, err = svc.ChatbotCompletion(t.Context(), "outsider", "chat-1", ChatbotCompletionRequest{Question: "hello"})
	if !errors.Is(err, permission.ErrPermissionDenied) || code != common.CodeNotFound {
		t.Fatalf("ChatbotCompletion(outsider) = (%d, %v), want hidden permission denial", code, err)
	}
}
