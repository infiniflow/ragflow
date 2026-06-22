package service

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupChatChannelServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.ChatChannel{}, &entity.Chat{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("failed to migrate test schema: %v", err)
	}

	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() {
		dao.DB = origDB
	})

	return db
}

func createServiceTestDialog(t *testing.T, db *gorm.DB, id, tenantID, name string) *entity.Chat {
	t.Helper()

	dialogName := name
	dialog := &entity.Chat{
		ID:           id,
		TenantID:     tenantID,
		Name:         &dialogName,
		LLMID:        "model-a",
		LLMSetting:   entity.JSONMap{"temperature": 0.1},
		PromptType:   "simple",
		PromptConfig: entity.JSONMap{"system": "sys"},
		KBIDs:        entity.JSONSlice{"kb-1"},
	}
	if err := db.Create(dialog).Error; err != nil {
		t.Fatalf("failed to create dialog: %v", err)
	}
	return dialog
}

func createServiceTestChannel(t *testing.T, db *gorm.DB, channel *entity.ChatChannel) *entity.ChatChannel {
	t.Helper()

	if err := db.Create(channel).Error; err != nil {
		t.Fatalf("failed to create chat channel: %v", err)
	}
	return channel
}

func createServiceTestMembership(t *testing.T, db *gorm.DB, userID, tenantID string) {
	t.Helper()

	status := "1"
	member := &entity.UserTenant{
		ID:       userID + "_" + tenantID,
		UserID:   userID,
		TenantID: tenantID,
		Role:     "normal",
		Status:   &status,
	}
	if err := db.Create(member).Error; err != nil {
		t.Fatalf("failed to create tenant membership: %v", err)
	}
}

func TestChatChannelServiceCreateAndList(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createServiceTestDialog(t, db, "dialog-1", "tenant-1", "Assistant A")

	svc := NewChatChannelService()
	chatID := "dialog-1"

	channel, err := svc.CreateChatChannel(
		"tenant-1",
		"bot-a",
		"dingtalk",
		entity.JSONMap{"token": "abc"},
		&chatID,
	)
	if err != nil {
		t.Fatalf("CreateChatChannel failed: %v", err)
	}
	if channel.ID == "" {
		t.Fatal("expected generated id")
	}
	if channel.TenantID != "tenant-1" || channel.Name != "bot-a" || channel.Channel != "dingtalk" {
		t.Fatalf("unexpected created channel: %+v", channel)
	}

	list, err := svc.List("tenant-1")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(list))
	}
	if list[0].DialogName == nil || *list[0].DialogName != "Assistant A" {
		t.Fatalf("expected joined dialog name, got %+v", list[0])
	}
}

func TestChatChannelServiceGetChatChannelAllowsJoinedTenant(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID:       "cc-1",
		TenantID: "tenant-1",
		Name:     "bot-a",
		Channel:  "wecom",
		Config:   entity.JSONMap{"token": "abc"},
		Status:   1,
	})
	createServiceTestMembership(t, db, "user-2", "tenant-1")

	channel, code, err := NewChatChannelService().GetChatChannel("user-2", "cc-1")
	if err != nil {
		t.Fatalf("GetChatChannel failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %v", code)
	}
	if channel == nil || channel.ID != "cc-1" {
		t.Fatalf("unexpected channel: %+v", channel)
	}
}

func TestChatChannelServiceUpdateChatChannelSuccess(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createServiceTestDialog(t, db, "dialog-1", "tenant-1", "Assistant A")
	createServiceTestDialog(t, db, "dialog-2", "tenant-1", "Assistant B")
	chatID := "dialog-1"
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID:       "cc-1",
		TenantID: "tenant-1",
		Name:     "bot-a",
		Channel:  "wecom",
		Config:   entity.JSONMap{"token": "old"},
		ChatID:   &chatID,
		Status:   1,
	})

	updated, code, err := NewChatChannelService().UpdateChatChannel("tenant-1", "cc-1", map[string]interface{}{
		"name":    "bot-b",
		"config":  map[string]interface{}{"token": "new"},
		"chat_id": "dialog-2",
	})
	if err != nil {
		t.Fatalf("UpdateChatChannel failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("expected success code, got %v", code)
	}
	if updated.Name != "bot-b" {
		t.Fatalf("expected updated name, got %+v", updated)
	}
	if updated.ChatID == nil || *updated.ChatID != "dialog-2" {
		t.Fatalf("expected updated chat_id, got %+v", updated.ChatID)
	}
	if updated.Config["token"] != "new" {
		t.Fatalf("expected updated config, got %+v", updated.Config)
	}
}

func TestChatChannelServiceUpdateChatChannelRejectsCrossTenantDialog(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createServiceTestDialog(t, db, "dialog-2", "tenant-2", "Assistant B")
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID:       "cc-1",
		TenantID: "tenant-1",
		Name:     "bot-a",
		Channel:  "wecom",
		Config:   entity.JSONMap{"token": "old"},
		Status:   1,
	})

	_, code, err := NewChatChannelService().UpdateChatChannel("tenant-1", "cc-1", map[string]interface{}{
		"chat_id": "dialog-2",
	})
	if code != common.CodeAuthenticationError {
		t.Fatalf("expected authentication error, got %v", code)
	}
	if err == nil || !strings.Contains(err.Error(), "No authorization.") {
		t.Fatalf("expected authorization error, got %v", err)
	}
}

func TestChatChannelServiceDeleteChatChannel(t *testing.T) {
	db := setupChatChannelServiceTestDB(t)
	createServiceTestChannel(t, db, &entity.ChatChannel{
		ID:       "cc-1",
		TenantID: "tenant-1",
		Name:     "bot-a",
		Channel:  "wecom",
		Config:   entity.JSONMap{"token": "old"},
		Status:   1,
	})

	svc := NewChatChannelService()

	deleted, code, err := svc.DeleteChatChannel("user-2", "cc-1")
	if code != common.CodeAuthenticationError {
		t.Fatalf("expected authentication error, got %v", code)
	}
	if err == nil || !strings.Contains(err.Error(), "No authorization.") {
		t.Fatalf("expected authorization error, got %v", err)
	}
	if deleted {
		t.Fatal("expected delete to be rejected")
	}

	deleted, code, err = svc.DeleteChatChannel("tenant-1", "cc-1")
	if err != nil {
		t.Fatalf("DeleteChatChannel failed: %v", err)
	}
	if code != common.CodeSuccess || !deleted {
		t.Fatalf("unexpected delete result: deleted=%v code=%v err=%v", deleted, code, err)
	}

	if _, err := svc.GetByID("cc-1"); err == nil {
		t.Fatal("expected deleted record to be gone")
	}
}
