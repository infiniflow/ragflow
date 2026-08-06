package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupChatListTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&entity.Chat{},
		&entity.Tenant{},
		&entity.User{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate test schema: %v", err)
	}

	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = origDB })

	status := string(entity.StatusValid)
	if err := db.Create(&entity.Tenant{
		ID:        "user-1",
		LLMID:     "model-a",
		EmbdID:    "embd-a",
		ParserIDs: "naive",
		Status:    &status,
	}).Error; err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}

	if err := db.Create(&entity.User{
		ID:       "user-1",
		Nickname: "tester",
		Status:   sptr("1"),
	}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	return db
}

func createChatListTestChat(t *testing.T, db *gorm.DB, id, tenantID, name string) {
	t.Helper()

	status := string(entity.StatusValid)
	chat := &entity.Chat{
		ID:           id,
		TenantID:     tenantID,
		Name:         &name,
		LLMID:        "model-a",
		LLMSetting:   entity.JSONMap{},
		PromptType:   "simple",
		PromptConfig: entity.JSONMap{},
		KBIDs:        entity.JSONSlice{},
		Status:       &status,
	}
	if err := db.Create(chat).Error; err != nil {
		t.Fatalf("failed to create chat: %v", err)
	}
}

func TestChatServiceListChatsDefaultReturnsAllWithCorrectTotal(t *testing.T) {
	db := setupChatListTestDB(t)
	createChatListTestChat(t, db, "chat-1", "user-1", "list_test_0")
	createChatListTestChat(t, db, "chat-2", "user-1", "list_test_1")
	createChatListTestChat(t, db, "chat-3", "user-1", "list_test_2")

	svc := NewChatService()
	ctx := t.Context()
	result, err := svc.ListChats(ctx, "user-1", "1", "", 0, 0, "create_time", true, nil)
	if err != nil {
		t.Fatalf("ListChats failed: %v", err)
	}
	if result.Total != 3 {
		t.Fatalf("expected total=3, got %d", result.Total)
	}
	if len(result.Chats) != 3 {
		t.Fatalf("expected 3 chats, got %d", len(result.Chats))
	}
	if result.Chats[0].Nickname != "tester" {
		t.Fatalf("expected nickname tester, got %q", result.Chats[0].Nickname)
	}
}

func TestChatServiceListChatsFiltersByOwnerIDs(t *testing.T) {
	db := setupChatListTestDB(t)

	if err := db.Create(&entity.User{
		ID:       "tenant-2",
		Nickname: "team owner",
		Email:    "tenant-2@test.com",
		Status:   sptr("1"),
	}).Error; err != nil {
		t.Fatalf("failed to create tenant user: %v", err)
	}
	if err := db.Create(&entity.UserTenant{
		ID:        "rel-1",
		UserID:    "user-1",
		TenantID:  "tenant-2",
		Role:      "normal",
		InvitedBy: "tenant-2",
		Status:    sptr("1"),
	}).Error; err != nil {
		t.Fatalf("failed to create user tenant relation: %v", err)
	}

	createChatListTestChat(t, db, "chat-own", "user-1", "own_chat")
	createChatListTestChat(t, db, "chat-team", "tenant-2", "team_chat")

	svc := NewChatService()
	ctx := t.Context()
	result, err := svc.ListChats(ctx, "user-1", "1", "", 0, 0, "create_time", true, []string{"tenant-2"})
	if err != nil {
		t.Fatalf("ListChats failed: %v", err)
	}
	if result.Total != 1 || len(result.Chats) != 1 {
		t.Fatalf("expected one filtered chat, got total=%d len=%d", result.Total, len(result.Chats))
	}
	if result.Chats[0].TenantID != "tenant-2" {
		t.Fatalf("expected tenant-2 chat, got tenant %q", result.Chats[0].TenantID)
	}
	if result.Chats[0].Nickname != "team owner" {
		t.Fatalf("expected nickname team owner, got %q", result.Chats[0].Nickname)
	}
}

func TestChatServiceListChatsKeywordFiltersCorrectly(t *testing.T) {
	db := setupChatListTestDB(t)
	createChatListTestChat(t, db, "chat-1", "user-1", "list_keyword_0")
	createChatListTestChat(t, db, "chat-2", "user-1", "list_keyword_1")
	createChatListTestChat(t, db, "chat-3", "user-1", "list_other_2")

	svc := NewChatService()

	ctx := t.Context()
	exactResult, err := svc.ListChats(ctx, "user-1", "1", "list_keyword_1", 0, 0, "create_time", true, nil)
	if err != nil {
		t.Fatalf("ListChats keyword exact failed: %v", err)
	}
	if len(exactResult.Chats) != 1 {
		t.Fatalf("expected 1 chat for keyword 'list_keyword_1', got %d", len(exactResult.Chats))
	}
	if exactResult.Chats[0].Name == nil || *exactResult.Chats[0].Name != "list_keyword_1" {
		t.Fatalf("expected chat name 'list_keyword_1', got %+v", exactResult.Chats[0].Name)
	}

	unknownResult, err := svc.ListChats(ctx, "user-1", "1", "unknown_keyword", 0, 0, "create_time", true, nil)
	if err != nil {
		t.Fatalf("ListChats unknown keyword failed: %v", err)
	}
	if len(unknownResult.Chats) != 0 {
		t.Fatalf("expected 0 chats for unknown keyword, got %d", len(unknownResult.Chats))
	}

	partialResult, err := svc.ListChats(ctx, "user-1", "1", "list_keyword", 0, 0, "create_time", true, nil)
	if err != nil {
		t.Fatalf("ListChats partial keyword failed: %v", err)
	}
	if len(partialResult.Chats) != 2 {
		t.Fatalf("expected 2 chats for keyword 'list_keyword', got %d", len(partialResult.Chats))
	}
}

func TestChatServiceListChatsPagination(t *testing.T) {
	db := setupChatListTestDB(t)
	for i := 0; i < 5; i++ {
		createChatListTestChat(t, db, "chat-"+string(rune('a'+i)), "user-1", "page_test")
	}

	svc := NewChatService()

	ctx := t.Context()
	page1, err := svc.ListChats(ctx, "user-1", "1", "", 1, 2, "create_time", true, nil)
	if err != nil {
		t.Fatalf("ListChats page 1 failed: %v", err)
	}
	if len(page1.Chats) != 2 {
		t.Fatalf("expected 2 chats on page 1, got %d", len(page1.Chats))
	}
	if page1.Total != 5 {
		t.Fatalf("expected total=5, got %d", page1.Total)
	}

	page3, err := svc.ListChats(ctx, "user-1", "1", "", 3, 2, "create_time", true, nil)
	if err != nil {
		t.Fatalf("ListChats page 3 failed: %v", err)
	}
	if len(page3.Chats) != 1 {
		t.Fatalf("expected 1 chat on page 3, got %d", len(page3.Chats))
	}
}

func TestChatServiceListChatsExcludesDeletedChats(t *testing.T) {
	db := setupChatListTestDB(t)
	createChatListTestChat(t, db, "chat-1", "user-1", "active_chat")
	createChatListTestChat(t, db, "chat-2", "user-1", "deleted_chat")

	invalidStatus := string(entity.StatusInvalid)
	db.Model(&entity.Chat{}).Where("id = ?", "chat-2").Update("status", invalidStatus)

	svc := NewChatService()
	ctx := t.Context()
	result, err := svc.ListChats(ctx, "user-1", "1", "", 0, 0, "create_time", true, nil)
	if err != nil {
		t.Fatalf("ListChats failed: %v", err)
	}
	if len(result.Chats) != 1 {
		t.Fatalf("expected 1 active chat, got %d", len(result.Chats))
	}
	if result.Chats[0].Name == nil || *result.Chats[0].Name != "active_chat" {
		t.Fatalf("expected active_chat, got %+v", result.Chats[0].Name)
	}
}
