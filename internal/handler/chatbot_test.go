//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

package handler

import (
	"encoding/json"

	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// setupChatbotTestDB sets up SQLite in-memory DB for chatbot handler tests.
func setupChatbotTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&entity.User{},
		&entity.Chat{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func TestGetChatbotInfo_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupChatbotTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	db.Create(&entity.User{ID: "user-1", Nickname: "test", Email: "test@test.com"})
	db.Create(&entity.Chat{
		ID:         "dialog-1",
		TenantID:   "user-1",
		Name:       sp("Test Chatbot"),
		Icon:       sp("data:image/png;base64,abc"),
		Status:     sp("1"),
		LLMID:      "model-1",
		LLMSetting: entity.JSONMap{},
		KBIDs:      entity.JSONSlice{},
		PromptConfig: entity.JSONMap{
			"prologue":       "Hello! How can I help you?",
			"tavily_api_key": "tvly-secret-key",
		},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/chatbots/dialog-1/info", nil)
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "dialog_id", Value: "dialog-1"}}

	GetChatbotInfo(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code 0, got %v: %v", resp["code"], resp["message"])
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %T", resp["data"])
	}
	if data["title"] != "Test Chatbot" {
		t.Errorf("expected title 'Test Chatbot', got %v", data["title"])
	}
	if data["avatar"] != "data:image/png;base64,abc" {
		t.Errorf("expected avatar, got %v", data["avatar"])
	}
	if data["prologue"] != "Hello! How can I help you?" {
		t.Errorf("expected prologue, got %v", data["prologue"])
	}
	if data["has_tavily_key"] != true {
		t.Errorf("expected has_tavily_key=true, got %v", data["has_tavily_key"])
	}
}

func TestGetChatbotInfo_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupChatbotTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	db.Create(&entity.User{ID: "user-1", Nickname: "test", Email: "test@test.com"})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/chatbots/non-existent/info", nil)
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "dialog_id", Value: "non-existent"}}

	GetChatbotInfo(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == 0 {
		t.Errorf("expected error code, got 0")
	}
}

func TestGetChatbotInfo_WrongTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupChatbotTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	db.Create(&entity.User{ID: "user-a", Nickname: "a", Email: "a@test.com"})
	db.Create(&entity.Chat{
		ID:         "dialog-1",
		TenantID:   "user-b",
		Name:       sp("Not Yours"),
		Status:     sp("1"),
		LLMID:      "model-1",
		LLMSetting: entity.JSONMap{},
		KBIDs:      entity.JSONSlice{},
		PromptConfig: entity.JSONMap{},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/chatbots/dialog-1/info", nil)
	c.Set("user", &entity.User{ID: "user-a"})
	c.Set("user_id", "user-a")
	c.Params = gin.Params{{Key: "dialog_id", Value: "dialog-1"}}

	GetChatbotInfo(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == 0 {
		t.Errorf("expected error code, got 0")
	}
}

// TestGetChatbotInfo_TenantMember verifies that a user who is a member
// of the dialog's tenant (via user_tenant) can access the info.
func TestGetChatbotInfo_TenantMember(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupChatbotTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	// owner-user owns the tenant, member-user is a tenant member
	db.Create(&entity.User{ID: "owner-user", Nickname: "owner", Email: "owner@test.com"})
	db.Create(&entity.User{ID: "member-user", Nickname: "member", Email: "member@test.com"})
	db.Create(&entity.UserTenant{
		ID: "ut-1", UserID: "member-user", TenantID: "owner-user",
		Role: "member", Status: sp("1"),
	})
	db.Create(&entity.Chat{
		ID:           "dialog-1",
		TenantID:     "owner-user",
		Name:         sp("Team Chatbot"),
		Status:       sp("1"),
		LLMID:        "model-1",
		LLMSetting:   entity.JSONMap{},
		KBIDs:        entity.JSONSlice{},
		PromptConfig: entity.JSONMap{},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/chatbots/dialog-1/info", nil)
	c.Set("user", &entity.User{ID: "member-user"})
	c.Set("user_id", "member-user")
	c.Params = gin.Params{{Key: "dialog_id", Value: "dialog-1"}}

	GetChatbotInfo(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code 0, got %v: %v", resp["code"], resp["message"])
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %T", resp["data"])
	}
	if data["title"] != "Team Chatbot" {
		t.Errorf("expected title 'Team Chatbot', got %v", data["title"])
	}
}

// sp returns a pointer to the given string.
func sp(s string) *string { return &s }
