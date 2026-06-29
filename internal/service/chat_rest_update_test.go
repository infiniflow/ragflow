package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupChatRESTUpdateServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.Chat{}, &entity.Tenant{}, &entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
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
		ASRID:     "asr-a",
		Img2TxtID: "img2txt-a",
		RerankID:  "rerank-a",
		ParserIDs: "naive",
		Status:    &status,
	}).Error; err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}

	return db
}

func createChatRESTUpdateServiceTestChat(t *testing.T, db *gorm.DB, id, tenantID string) {
	t.Helper()

	name := "chat-" + id
	status := string(entity.StatusValid)
	chat := &entity.Chat{
		ID:           id,
		TenantID:     tenantID,
		Name:         &name,
		LLMID:        "model-a",
		LLMSetting:   entity.JSONMap{"temperature": float64(0.1), "top_p": float64(0.9)},
		PromptType:   "simple",
		PromptConfig: entity.JSONMap{"system": "old system", "quote": true},
		KBIDs:        entity.JSONSlice{},
		Status:       &status,
	}
	if err := db.Create(chat).Error; err != nil {
		t.Fatalf("failed to create chat: %v", err)
	}
}

func assertEmptyMetaDataFilter(t *testing.T, value interface{}) {
	t.Helper()

	switch typed := value.(type) {
	case entity.JSONMap:
		if len(typed) != 0 {
			t.Fatalf("expected empty meta_data_filter, got %+v", typed)
		}
	case map[string]interface{}:
		if len(typed) != 0 {
			t.Fatalf("expected empty meta_data_filter, got %+v", typed)
		}
	default:
		t.Fatalf("expected meta_data_filter object, got %T: %+v", value, value)
	}
}

func TestChatServiceCreateDefaultsMetaDataFilter(t *testing.T) {
	setupChatRESTUpdateServiceTestDB(t)

	svc := NewChatService()
	resp, code, err := svc.Create("user-1", map[string]interface{}{
		"name": "created chat",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("unexpected code: %v", code)
	}
	assertEmptyMetaDataFilter(t, resp["meta_data_filter"])

	chatID, ok := resp["id"].(string)
	if !ok || chatID == "" {
		t.Fatalf("expected created chat id, got %+v", resp["id"])
	}
	chat, err := svc.chatDAO.GetByID(chatID)
	if err != nil {
		t.Fatalf("failed to fetch created chat: %v", err)
	}
	if chat.MetaDataFilter == nil {
		t.Fatal("expected persisted meta_data_filter to be non-nil")
	}
	assertEmptyMetaDataFilter(t, *chat.MetaDataFilter)
}

func TestChatServiceCreateAcceptsNilMetaDataFilter(t *testing.T) {
	setupChatRESTUpdateServiceTestDB(t)

	svc := NewChatService()
	resp, code, err := svc.Create("user-1", map[string]interface{}{
		"name":             "created chat",
		"meta_data_filter": nil,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("unexpected code: %v", code)
	}
	assertEmptyMetaDataFilter(t, resp["meta_data_filter"])
}

func TestChatServiceCreateRejectsInvalidMetaDataFilter(t *testing.T) {
	setupChatRESTUpdateServiceTestDB(t)

	svc := NewChatService()
	_, code, err := svc.Create("user-1", map[string]interface{}{
		"name":             "created chat",
		"meta_data_filter": []interface{}{"invalid"},
	})
	if err == nil || err.Error() != "`meta_data_filter` should be an object." {
		t.Fatalf("expected meta_data_filter error, got %v", err)
	}
	if code != common.CodeDataError {
		t.Fatalf("unexpected code: %v", code)
	}
}

func TestChatServicePatchChatMergesPromptConfigAndLLMSetting(t *testing.T) {
	db := setupChatRESTUpdateServiceTestDB(t)
	createChatRESTUpdateServiceTestChat(t, db, "chat-1", "user-1")

	svc := NewChatService()
	resp, err := svc.PatchChat("user-1", "chat-1", map[string]interface{}{
		"prompt_config": map[string]interface{}{"quote": false},
		"llm_setting":   map[string]interface{}{"temperature": float64(0.2)},
	})
	if err != nil {
		t.Fatalf("PatchChat failed: %v", err)
	}
	if _, ok := resp["kb_ids"]; ok {
		t.Fatalf("response must not expose kb_ids: %+v", resp)
	}
	if _, ok := resp["dataset_ids"]; !ok {
		t.Fatalf("response should expose dataset_ids: %+v", resp)
	}

	chat, err := svc.chatDAO.GetByID("chat-1")
	if err != nil {
		t.Fatalf("failed to fetch updated chat: %v", err)
	}
	if chat.PromptConfig["system"] != "old system" {
		t.Fatalf("expected prompt_config.system to be preserved, got %+v", chat.PromptConfig)
	}
	if chat.PromptConfig["quote"] != false {
		t.Fatalf("expected prompt_config.quote to be patched, got %+v", chat.PromptConfig)
	}
	if chat.LLMSetting["top_p"] != float64(0.9) {
		t.Fatalf("expected llm_setting.top_p to be preserved, got %+v", chat.LLMSetting)
	}
	if chat.LLMSetting["temperature"] != float64(0.2) {
		t.Fatalf("expected llm_setting.temperature to be patched, got %+v", chat.LLMSetting)
	}
}

func TestChatServiceUpdateChatRejectsTenantID(t *testing.T) {
	db := setupChatRESTUpdateServiceTestDB(t)
	createChatRESTUpdateServiceTestChat(t, db, "chat-1", "user-1")

	svc := NewChatService()
	_, err := svc.UpdateChat("user-1", "chat-1", map[string]interface{}{
		"tenant_id": "tenant-2",
	})
	if err == nil || err.Error() != "`tenant_id` must not be provided." {
		t.Fatalf("expected tenant_id error, got %v", err)
	}
}

func TestChatServiceUpdateChatRejectsInvalidLLMSetting(t *testing.T) {
	db := setupChatRESTUpdateServiceTestDB(t)
	createChatRESTUpdateServiceTestChat(t, db, "chat-1", "user-1")

	svc := NewChatService()
	_, err := svc.UpdateChat("user-1", "chat-1", map[string]interface{}{
		"llm_setting": "invalid",
	})
	if err == nil || err.Error() != "`llm_setting` should be an object." {
		t.Fatalf("expected llm_setting error, got %v", err)
	}

	chat, err := svc.chatDAO.GetByID("chat-1")
	if err != nil {
		t.Fatalf("failed to fetch chat: %v", err)
	}
	if chat.LLMSetting["temperature"] != float64(0.1) {
		t.Fatalf("expected llm_setting to remain unchanged, got %+v", chat.LLMSetting)
	}
}

func TestChatServiceUpdateChatAcceptsMetaDataFilterObject(t *testing.T) {
	db := setupChatRESTUpdateServiceTestDB(t)
	createChatRESTUpdateServiceTestChat(t, db, "chat-1", "user-1")

	svc := NewChatService()
	_, err := svc.UpdateChat("user-1", "chat-1", map[string]interface{}{
		"name": "chat-chat-1",
		"meta_data_filter": map[string]interface{}{
			"method": "disabled",
			"manual": []interface{}{},
		},
	})
	if err != nil {
		t.Fatalf("UpdateChat failed: %v", err)
	}

	chat, err := svc.chatDAO.GetByID("chat-1")
	if err != nil {
		t.Fatalf("failed to fetch chat: %v", err)
	}
	if chat.MetaDataFilter == nil || (*chat.MetaDataFilter)["method"] != "disabled" {
		t.Fatalf("expected meta_data_filter to be persisted, got %+v", chat.MetaDataFilter)
	}
}

func TestChatServiceUpdateChatBackfillsNilMetaDataFilter(t *testing.T) {
	db := setupChatRESTUpdateServiceTestDB(t)
	createChatRESTUpdateServiceTestChat(t, db, "chat-1", "user-1")

	svc := NewChatService()
	resp, err := svc.UpdateChat("user-1", "chat-1", map[string]interface{}{
		"name": "chat-chat-1",
	})
	if err != nil {
		t.Fatalf("UpdateChat failed: %v", err)
	}
	assertEmptyMetaDataFilter(t, resp["meta_data_filter"])

	chat, err := svc.chatDAO.GetByID("chat-1")
	if err != nil {
		t.Fatalf("failed to fetch chat: %v", err)
	}
	if chat.MetaDataFilter == nil {
		t.Fatal("expected meta_data_filter to be backfilled")
	}
	assertEmptyMetaDataFilter(t, *chat.MetaDataFilter)
}

func TestChatServicePatchChatIgnoresTenantIDAndUpdatesName(t *testing.T) {
	db := setupChatRESTUpdateServiceTestDB(t)
	createChatRESTUpdateServiceTestChat(t, db, "chat-1", "user-1")

	svc := NewChatService()
	_, err := svc.PatchChat("user-1", "chat-1", map[string]interface{}{
		"tenant_id": "tenant-2",
		"name":      "  renamed chat  ",
	})
	if err != nil {
		t.Fatalf("PatchChat failed: %v", err)
	}

	chat, err := svc.chatDAO.GetByID("chat-1")
	if err != nil {
		t.Fatalf("failed to fetch updated chat: %v", err)
	}
	if chat.TenantID != "user-1" {
		t.Fatalf("expected tenant_id to remain user-1, got %s", chat.TenantID)
	}
	if chat.Name == nil || *chat.Name != "renamed chat" {
		t.Fatalf("expected trimmed name, got %+v", chat.Name)
	}
}
