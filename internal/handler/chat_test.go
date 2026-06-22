package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

func setupChatHandlerTestDB(t *testing.T) *gorm.DB {
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
	t.Cleanup(func() { dao.DB = origDB })

	return db
}

func createChatHandlerTestChat(t *testing.T, db *gorm.DB, id, tenantID string) {
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

func TestDeleteChatHandlerSuccess(t *testing.T) {
	db := setupChatHandlerTestDB(t)
	createChatHandlerTestChat(t, db, "chat-1", "user-1")

	h := NewChatHandler(service.NewChatService(), service.NewUserService())
	c, w := setupGinContextWithUser("DELETE", "/api/v1/chats/chat-1", "")
	c.Params = []gin.Param{{Key: "chat_id", Value: "chat-1"}}

	h.DeleteChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected success code, got %v", resp["code"])
	}
	if resp["data"] != true {
		t.Fatalf("expected data=true, got %v", resp["data"])
	}
}

func TestBulkDeleteChatsHandlerPartialSuccess(t *testing.T) {
	db := setupChatHandlerTestDB(t)
	createChatHandlerTestChat(t, db, "chat-1", "user-1")
	createChatHandlerTestChat(t, db, "chat-2", "tenant-2")

	h := NewChatHandler(service.NewChatService(), service.NewUserService())
	c, w := setupGinContextWithUser("DELETE", "/api/v1/chats", `{"ids":["chat-1","chat-2","chat-1"]}`)

	h.BulkDeleteChats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected success code, got %v", resp["code"])
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected object data, got %+v", resp["data"])
	}
	if data["success_count"] != float64(1) {
		t.Fatalf("expected success_count=1, got %v", data["success_count"])
	}
	errorsList, ok := data["errors"].([]interface{})
	if !ok || len(errorsList) != 2 {
		t.Fatalf("expected 2 errors, got %+v", data["errors"])
	}
	if resp["message"] != "Partially deleted 1 chats with 2 errors" {
		t.Fatalf("unexpected message: %v", resp["message"])
	}
}
