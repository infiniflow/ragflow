package service

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupChatDeleteServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.Chat{}); err != nil {
		t.Fatalf("failed to migrate test schema: %v", err)
	}

	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() {
		dao.DB = origDB
	})

	return db
}

func createChatDeleteServiceTestChat(t *testing.T, db *gorm.DB, id, tenantID string) {
	t.Helper()

	name := "chat-" + id
	status := string(entity.StatusValid)
	chat := &entity.Chat{
		ID:           id,
		TenantID:     tenantID,
		Name:         &name,
		LLMID:        "model-a",
		LLMSetting:   entity.JSONMap{},
		PromptType:   "simple",
		PromptConfig: entity.JSONMap{"system": "sys"},
		KBIDs:        entity.JSONSlice{},
		Status:       &status,
	}
	if err := db.Create(chat).Error; err != nil {
		t.Fatalf("failed to create chat: %v", err)
	}
}

func TestChatServiceDeleteChatRejectsNonOwner(t *testing.T) {
	db := setupChatDeleteServiceTestDB(t)
	createChatDeleteServiceTestChat(t, db, "chat-1", "tenant-1")

	svc := NewChatService()
	err := svc.DeleteChat("user-1", "chat-1")
	if err == nil || err.Error() != "no authorization" {
		t.Fatalf("expected no authorization, got %v", err)
	}

	chat, getErr := svc.chatDAO.GetByID("chat-1")
	if getErr != nil {
		t.Fatalf("failed to fetch chat: %v", getErr)
	}
	if chat.Status == nil || *chat.Status != string(entity.StatusValid) {
		t.Fatalf("expected chat status to remain valid, got %+v", chat.Status)
	}
}

func TestChatServiceBulkDeleteChatsDeleteAllOnlyDeletesOwnedChats(t *testing.T) {
	db := setupChatDeleteServiceTestDB(t)
	createChatDeleteServiceTestChat(t, db, "chat-1", "user-1")
	createChatDeleteServiceTestChat(t, db, "chat-2", "user-1")
	createChatDeleteServiceTestChat(t, db, "chat-3", "tenant-2")

	svc := NewChatService()
	result, err := svc.BulkDeleteChats("user-1", &BulkDeleteChatsRequest{DeleteAll: true})
	if err != nil {
		t.Fatalf("BulkDeleteChats failed: %v", err)
	}

	if got, ok := result["success_count"].(int); !ok || got != 2 {
		t.Fatalf("expected success_count 2, got %+v", result["success_count"])
	}

	owned1, err := svc.chatDAO.GetByID("chat-1")
	if err != nil {
		t.Fatalf("failed to fetch chat-1: %v", err)
	}
	if owned1.Status == nil || *owned1.Status != string(entity.StatusInvalid) {
		t.Fatalf("expected chat-1 invalid, got %+v", owned1.Status)
	}

	owned2, err := svc.chatDAO.GetByID("chat-2")
	if err != nil {
		t.Fatalf("failed to fetch chat-2: %v", err)
	}
	if owned2.Status == nil || *owned2.Status != string(entity.StatusInvalid) {
		t.Fatalf("expected chat-2 invalid, got %+v", owned2.Status)
	}

	other, err := svc.chatDAO.GetByID("chat-3")
	if err != nil {
		t.Fatalf("failed to fetch chat-3: %v", err)
	}
	if other.Status == nil || *other.Status != string(entity.StatusValid) {
		t.Fatalf("expected chat-3 to remain valid, got %+v", other.Status)
	}
}

func TestChatServiceBulkDeleteChatsReturnsPartialSuccessErrors(t *testing.T) {
	db := setupChatDeleteServiceTestDB(t)
	createChatDeleteServiceTestChat(t, db, "chat-1", "user-1")
	createChatDeleteServiceTestChat(t, db, "chat-2", "tenant-2")

	svc := NewChatService()
	result, err := svc.BulkDeleteChats("user-1", &BulkDeleteChatsRequest{
		IDs: []string{"chat-1", "chat-2", "chat-1"},
	})
	if err != nil {
		t.Fatalf("BulkDeleteChats failed: %v", err)
	}

	if got, ok := result["success_count"].(int); !ok || got != 1 {
		t.Fatalf("expected success_count 1, got %+v", result["success_count"])
	}

	errorsList, ok := result["errors"].([]string)
	if !ok {
		t.Fatalf("expected errors list, got %+v", result["errors"])
	}
	joined := strings.Join(errorsList, " | ")
	if !strings.Contains(joined, "Duplicate chat ids: chat-1") {
		t.Fatalf("expected duplicate id error, got %v", errorsList)
	}
	if !strings.Contains(joined, "Chat(chat-2) not found.") {
		t.Fatalf("expected not found error, got %v", errorsList)
	}
}
