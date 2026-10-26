package service

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func TestChatAgenticConfigPersistsAndResets(t *testing.T) {
	db := setupChatRESTUpdateServiceTestDB(t)
	createChatRESTUpdateServiceTestChat(t, db, "chat-1", "user-1")
	t.Setenv("AGENTIC_RAG_CONFIG", filepath.Join("..", "..", "conf", "agentic_rag.yaml"))
	svc := NewChatService()
	ctx := serviceTestContext(t, "user-1", "user-1")

	for _, override := range []any{
		map[string]any{"content": "Use this saved instruction", "tools": []any{"grep_chunks", "list_chunks"}},
		map[string]any{"content": "Only content overrides defaults"},
		map[string]any{"tools": []any{"grep_chunks", "list_chunks"}},
		nil,
	} {
		_, err := svc.PatchChat(ctx, "user-1", "chat-1", map[string]any{
			"prompt_config": map[string]any{"agentic_rag": override},
		})
		if err != nil {
			t.Fatalf("PatchChat(%#v): %v", override, err)
		}
		// A new service and DAO read prove this is stored JSON, not process state.
		reloaded := NewChatService()
		chat, err := reloaded.chatDAO.GetByID(ctx, dao.DB, "chat-1")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(chat.PromptConfig["agentic_rag"], override) {
			t.Fatalf("persisted override = %#v, want %#v", chat.PromptConfig["agentic_rag"], override)
		}
		if chat.PromptConfig["system"] != "old system" || chat.PromptConfig["quote"] != true {
			t.Fatalf("unrelated settings changed: %#v", chat.PromptConfig)
		}
		settings, err := reloaded.GetAgenticRAGConfig(ctx, "user-1", "chat-1")
		if err != nil || settings == nil {
			t.Fatalf("GetAgenticRAGConfig() = (%#v, %v)", settings, err)
		}
		if override == nil {
			if !settings.UsingDefaults || !reflect.DeepEqual(settings.Effective, settings.Defaults) || settings.Override != nil {
				t.Fatalf("reset did not restore defaults: %#v", settings)
			}
			continue
		}
		object := override.(map[string]any)
		if content, ok := object["content"]; ok {
			if settings.Effective.Content != content {
				t.Fatalf("effective content = %q, want %q", settings.Effective.Content, content)
			}
		} else if settings.Effective.Content != settings.Defaults.Content {
			t.Fatal("tools-only override did not inherit default content")
		}
		if _, hasTools := object["tools"]; !hasTools && !slices.Equal(settings.Effective.Tools, settings.Defaults.Tools) {
			t.Fatal("content-only override did not inherit default tools")
		}
	}
}

func TestChatAgenticConfigCreateAndPut(t *testing.T) {
	setupChatRESTUpdateServiceTestDB(t)
	svc := NewChatService()
	ctx := serviceTestContext(t, "user-1", "user-1")
	override := map[string]any{"content": "Created instruction", "tools": []any{"grep_chunks", "list_chunks"}}
	result, code, err := svc.Create(ctx, "user-1", map[string]any{
		"name":          "Agentic configuration",
		"prompt_config": map[string]any{"agentic_rag": override},
	})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("Create() = (%d, %v)", code, err)
	}
	chatID := result["id"].(string)
	chat, err := svc.chatDAO.GetByID(ctx, dao.DB, chatID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(chat.PromptConfig["agentic_rag"], override) {
		t.Fatalf("create did not persist override: %#v", chat.PromptConfig)
	}
	_, err = svc.UpdateChat(ctx, "user-1", chatID, map[string]any{
		"prompt_config": map[string]any{"agentic_rag": nil, "system": "replacement"},
	})
	if err != nil {
		t.Fatal(err)
	}
	chat, err = svc.chatDAO.GetByID(ctx, dao.DB, chatID)
	if err != nil {
		t.Fatal(err)
	}
	if chat.PromptConfig["agentic_rag"] != nil || chat.PromptConfig["system"] != "replacement" || len(chat.PromptConfig) != 2 {
		t.Fatalf("PUT did not replace prompt config: %#v", chat.PromptConfig)
	}
}

func TestChatAgenticConfigRejectsInvalidWrites(t *testing.T) {
	for name, override := range map[string]any{
		"string":        "invalid",
		"array":         []any{},
		"prompt type":   map[string]any{"content": 42},
		"empty prompt":  map[string]any{"content": ""},
		"null prompt":   map[string]any{"content": nil},
		"large prompt":  map[string]any{"content": strings.Repeat("x", 32769)},
		"tools type":    map[string]any{"tools": "grep_chunks"},
		"null tools":    map[string]any{"tools": nil},
		"tool type":     map[string]any{"tools": []any{42}},
		"empty tools":   map[string]any{"tools": []any{}},
		"unknown tool":  map[string]any{"tools": []any{"unknown_tool"}},
		"duplicate":     map[string]any{"tools": []any{"grep_chunks", "grep_chunks"}},
		"web privilege": map[string]any{"tools": []any{"web_search"}},
		"unknown field": map[string]any{"temperature": 0.4},
	} {
		t.Run(name, func(t *testing.T) {
			db := setupChatRESTUpdateServiceTestDB(t)
			createChatRESTUpdateServiceTestChat(t, db, "chat-1", "user-1")
			svc := NewChatService()
			for _, method := range []string{"create", "put", "patch"} {
				payload := map[string]any{"prompt_config": map[string]any{"agentic_rag": override}}
				var err error
				switch method {
				case "create":
					payload["name"] = "Invalid configuration"
					_, _, err = svc.Create(t.Context(), "user-1", payload)
				case "put":
					_, err = svc.UpdateChat(t.Context(), "user-1", "chat-1", payload)
				case "patch":
					_, err = svc.PatchChat(t.Context(), "user-1", "chat-1", payload)
				}
				if err == nil {
					t.Fatalf("%s accepted invalid override %#v", method, override)
				}
			}
			chat, err := svc.chatDAO.GetByID(t.Context(), dao.DB, "chat-1")
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := chat.PromptConfig["agentic_rag"]; exists {
				t.Fatalf("invalid override was persisted: %#v", chat.PromptConfig)
			}
			var count int64
			if err := db.Model(&entity.Chat{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("invalid create persisted a row: count=%d error=%v", count, err)
			}
		})
	}
}

func TestChatAgenticConfigEnforcesPermissions(t *testing.T) {
	db := setupChatRESTUpdateServiceTestDB(t)
	createChatRESTUpdateServiceTestChat(t, db, "other-chat", "other-tenant")
	svc := NewChatService()
	ctx := serviceTestContext(t, "user-1", "user-1")
	if _, err := svc.GetAgenticRAGConfig(ctx, "user-1", "other-chat"); !errors.Is(err, permission.ErrResourceNotFound) && !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("cross-tenant read error = %v", err)
	}
	_, err := svc.PatchChat(ctx, "user-1", "other-chat", map[string]any{
		"prompt_config": map[string]any{"agentic_rag": map[string]any{"content": "Forbidden"}},
	})
	if !errors.Is(err, permission.ErrResourceNotFound) && !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("cross-tenant update error = %v", err)
	}
}
