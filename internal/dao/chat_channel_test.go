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
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// setupChatChannelTestDB initializes an in-memory SQLite database for ChatChannel DAO tests.
func setupChatChannelTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	// Migrate chat_channel and dialog (entity.Chat) tables
	if err := db.AutoMigrate(&entity.ChatChannel{}, &entity.Chat{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func TestChatChannelDAO_CRUD(t *testing.T) {
	db := setupChatChannelTestDB(t)
	pushDB(t, db)
	dao := NewChatChannel()

	// 1. Test Create
	channelConfig := entity.JSONMap{
		"credential": map[string]interface{}{
			"corp_id":  "ww123456",
			"agent_id": float64(1000001),
			"secret":   "sec-key",
		},
	}
	statusActive := 1
	cc := &entity.ChatChannel{
		ID:       "chan-1",
		TenantID: "tenant-1",
		Name:     "Test WeCom Bot",
		Channel:  "wecom",
		Config:   channelConfig,
		Status:   statusActive,
	}

	err := dao.Create(cc)
	if err != nil {
		t.Fatalf("failed to create chat channel: %v", err)
	}

	// 2. Test GetByID
	res, err := dao.GetByID("chan-1", "tenant-1")
	if err != nil {
		t.Fatalf("failed to get chat channel: %v", err)
	}
	if res.Name != "Test WeCom Bot" {
		t.Fatalf("expected Name %q, got %q", "Test WeCom Bot", res.Name)
	}
	cred, ok := res.Config["credential"].(map[string]interface{})
	if !ok {
		t.Fatalf("failed to parse config credentials")
	}
	if cred["corp_id"] != "ww123456" {
		t.Fatalf("expected corp_id %q, got %v", "ww123456", cred["corp_id"])
	}

	// 2b. Test tenant isolation for GetByID
	_, err = dao.GetByID("chan-1", "tenant-2")
	if err == nil {
		t.Fatalf("expected error (not found) when getting with wrong tenant, got nil")
	}

	// 3. Test UpdateByID
	updates := map[string]interface{}{
		"name": "Updated WeCom Bot",
	}
	// Try updating with wrong tenant
	err = dao.UpdateByID("chan-1", "tenant-2", updates)
	if err != nil {
		t.Fatalf("failed to run UpdateByID with wrong tenant: %v", err)
	}
	// Verify it was NOT updated (should still be "Test WeCom Bot" since wrong tenant was used)
	res, err = dao.GetByID("chan-1", "tenant-1")
	if err != nil {
		t.Fatalf("failed to get chat channel: %v", err)
	}
	if res.Name != "Test WeCom Bot" {
		t.Fatalf("expected Name to remain %q, but got %q", "Test WeCom Bot", res.Name)
	}

	// Update with correct tenant
	err = dao.UpdateByID("chan-1", "tenant-1", updates)
	if err != nil {
		t.Fatalf("failed to update chat channel: %v", err)
	}

	res, err = dao.GetByID("chan-1", "tenant-1")
	if err != nil {
		t.Fatalf("failed to get updated chat channel: %v", err)
	}
	if res.Name != "Updated WeCom Bot" {
		t.Fatalf("expected updated Name %q, got %q", "Updated WeCom Bot", res.Name)
	}

	// 3b. Test DeleteByID with wrong tenant (should not delete)
	err = dao.DeleteByID("chan-1", "tenant-2")
	if err != nil {
		t.Fatalf("failed to delete with wrong tenant: %v", err)
	}
	// Verify it still exists for tenant-1
	_, err = dao.GetByID("chan-1", "tenant-1")
	if err != nil {
		t.Fatalf("expected chat channel to still exist for tenant-1, got error: %v", err)
	}

	// 4. Test DeleteByID
	err = dao.DeleteByID("chan-1", "tenant-1")
	if err != nil {
		t.Fatalf("failed to delete chat channel: %v", err)
	}

	_, err = dao.GetByID("chan-1", "tenant-1")
	if err == nil {
		t.Fatalf("expected record not found error, got nil")
	}
}

func TestChatChannelDAO_ListByTenantID(t *testing.T) {
	db := setupChatChannelTestDB(t)
	pushDB(t, db)
	dao := NewChatChannel()

	// Create a test dialog (chat assistant)
	dialogName := "Dialog Assistant A"
	d := &entity.Chat{
		ID:           "diag-1",
		TenantID:     "tenant-1",
		Name:         &dialogName,
		LLMID:        "model-a",
		LLMSetting:   entity.JSONMap{"temp": 0.1},
		PromptConfig: entity.JSONMap{"system": "sys"},
		KBIDs:        entity.JSONSlice{"kb-1"},
	}
	if err := db.Create(d).Error; err != nil {
		t.Fatalf("failed to create test dialog: %v", err)
	}

	statusActive := 1
	now := time.Now().UnixMilli()
	t1 := now - 1000
	t2 := now

	// Create chat channels (order: oldest first, so list should return youngest first)
	cc1 := &entity.ChatChannel{
		ID:       "chan-1",
		TenantID: "tenant-1",
		Name:     "WeCom Channel 1",
		Channel:  "wecom",
		Config:   entity.JSONMap{"cred": "A"},
		ChatID:   &d.ID,
		Status:   statusActive,
		BaseModel: entity.BaseModel{
			CreateTime: &t1,
		},
	}
	cc2 := &entity.ChatChannel{
		ID:       "chan-2",
		TenantID: "tenant-1",
		Name:     "WeCom Channel 2",
		Channel:  "wecom",
		Config:   entity.JSONMap{"cred": "B"},
		ChatID:   &d.ID,
		Status:   statusActive,
		BaseModel: entity.BaseModel{
			CreateTime: &t2,
		},
	}

	if err := db.Create(cc1).Error; err != nil {
		t.Fatalf("failed to create cc1: %v", err)
	}
	if err := db.Create(cc2).Error; err != nil {
		t.Fatalf("failed to create cc2: %v", err)
	}

	// Perform query
	list, err := dao.ListByTenantID("tenant-1")
	if err != nil {
		t.Fatalf("ListByTenantID failed: %v", err)
	}

	// Verify count and order (create_time DESC, so cc2 should be index 0)
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}

	// cc2 is first because t2 > t1
	if list[0].ID != "chan-2" {
		t.Fatalf("expected first item ID %q, got %q (ordering failed)", "chan-2", list[0].ID)
	}
	if list[1].ID != "chan-1" {
		t.Fatalf("expected second item ID %q, got %q", "chan-1", list[1].ID)
	}
	if list[0].ChatID == nil || *list[0].ChatID != d.ID {
		t.Fatalf("expected chat_id %q, got %v", d.ID, list[0].ChatID)
	}

	// Verify Left Join mapping
	if list[0].DialogName == nil || *list[0].DialogName != "Dialog Assistant A" {
		t.Fatalf("expected dialog name %q, got %v (left join failed)", "Dialog Assistant A", list[0].DialogName)
	}
}
