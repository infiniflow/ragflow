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

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupSearchbotDetailHandlerDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.User{}, &entity.UserTenant{}, &entity.Search{}, &entity.APIToken{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() {
		dao.DB = orig
	})
}

func newSearchbotDetailRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewSearchBotHandler(service.NewSearchService(), nil, nil, nil)
	r := gin.New()
	r.GET("/api/v1/searchbots/detail", h.SearchbotDetail)
	return r
}

func TestSearchbotDetailSuccess(t *testing.T) {
	setupSearchbotDetailHandlerDB(t)

	status := "1"
	accessToken := "access-token"
	tenantAvatar := "tenant-avatar"
	beta := "beta-token"
	searchAvatar := "search-avatar"

	if err := dao.DB.Create(&entity.User{
		ID:              "tenant-1",
		Email:           "tenant@example.com",
		Nickname:        "Tenant",
		Avatar:          &tenantAvatar,
		AccessToken:     &accessToken,
		IsAuthenticated: "1",
		IsActive:        "1",
		IsAnonymous:     "0",
		Status:          &status,
	}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if err := dao.DB.Create(&entity.UserTenant{
		ID:        "ut-1",
		UserID:    "tenant-1",
		TenantID:  "tenant-1",
		Role:      "owner",
		InvitedBy: "tenant-1",
		Status:    &status,
	}).Error; err != nil {
		t.Fatalf("failed to create user_tenant: %v", err)
	}

	if err := dao.DB.Create(&entity.APIToken{
		TenantID: "tenant-1",
		Token:    "token-1",
		Beta:     &beta,
	}).Error; err != nil {
		t.Fatalf("failed to create api token: %v", err)
	}

	if err := dao.DB.Create(&entity.Search{
		ID:           "search-1",
		Avatar:       &searchAvatar,
		TenantID:     "tenant-1",
		Name:         "Search App",
		CreatedBy:    "tenant-1",
		SearchConfig: entity.JSONMap{"summary": true},
		Status:       &status,
	}).Error; err != nil {
		t.Fatalf("failed to create search: %v", err)
	}

	r := newSearchbotDetailRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/searchbots/detail?search_id=search-1", nil)
	req.Header.Set("Authorization", "Bearer "+beta)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("code = %v, want %v", resp["code"], common.CodeSuccess)
	}
	data := resp["data"].(map[string]interface{})
	if data["id"] != "search-1" {
		t.Fatalf("id = %v, want search-1", data["id"])
	}
	if _, ok := data["avatar"]; !ok {
		t.Fatal("expected avatar key in response")
	}
	if data["avatar"] != searchAvatar {
		t.Fatalf("avatar = %v, want %q", data["avatar"], searchAvatar)
	}
	if _, ok := data["nickname"]; ok {
		t.Fatalf("did not expect nickname in response, got %v", data["nickname"])
	}
	if _, ok := data["tenant_avatar"]; ok {
		t.Fatalf("did not expect tenant_avatar in response, got %v", data["tenant_avatar"])
	}
}

func TestSearchbotDetailRejectsMissingSearchID(t *testing.T) {
	setupSearchbotDetailHandlerDB(t)

	r := newSearchbotDetailRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/searchbots/detail", nil)
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["code"] != float64(common.CodeArgumentError) {
		t.Fatalf("code = %v, want %v", resp["code"], common.CodeArgumentError)
	}
}

func TestSearchbotDetailRejectsInvalidBetaToken(t *testing.T) {
	setupSearchbotDetailHandlerDB(t)

	r := newSearchbotDetailRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/searchbots/detail?search_id=search-1", nil)
	req.Header.Set("Authorization", "Bearer missing")
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["code"] != float64(common.CodeUnauthorized) {
		t.Fatalf("code = %v, want %v", resp["code"], common.CodeUnauthorized)
	}
}

func TestSearchbotDetailRejectsUnauthorizedSearchAccess(t *testing.T) {
	setupSearchbotDetailHandlerDB(t)

	status := "1"
	beta := "beta-token"
	accessToken := "access-token"

	for _, u := range []entity.User{
		{
			ID:              "tenant-1",
			Email:           "tenant1@example.com",
			Nickname:        "Tenant1",
			IsAuthenticated: "1",
			IsActive:        "1",
			IsAnonymous:     "0",
			Status:          &status,
		},
		{
			ID:              "user-2",
			Email:           "user2@example.com",
			Nickname:        "User2",
			AccessToken:     &accessToken,
			IsAuthenticated: "1",
			IsActive:        "1",
			IsAnonymous:     "0",
			Status:          &status,
		},
	} {
		user := u
		if err := dao.DB.Create(&user).Error; err != nil {
			t.Fatalf("failed to create user %s: %v", user.ID, err)
		}
	}

	if err := dao.DB.Create(&entity.UserTenant{
		ID:        "ut-1",
		UserID:    "user-2",
		TenantID:  "user-2",
		Role:      "owner",
		InvitedBy: "user-2",
		Status:    &status,
	}).Error; err != nil {
		t.Fatalf("failed to create user_tenant: %v", err)
	}

	if err := dao.DB.Create(&entity.APIToken{
		TenantID: "user-2",
		Token:    "token-1",
		Beta:     &beta,
	}).Error; err != nil {
		t.Fatalf("failed to create api token: %v", err)
	}

	if err := dao.DB.Create(&entity.Search{
		ID:           "search-1",
		TenantID:     "tenant-1",
		Name:         "Search App",
		CreatedBy:    "tenant-1",
		SearchConfig: entity.JSONMap{"summary": true},
		Status:       &status,
	}).Error; err != nil {
		t.Fatalf("failed to create search: %v", err)
	}

	r := newSearchbotDetailRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/searchbots/detail?search_id=search-1", nil)
	req.Header.Set("Authorization", "Bearer "+beta)
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["code"] != float64(common.CodeOperatingError) {
		t.Fatalf("code = %v, want %v", resp["code"], common.CodeOperatingError)
	}
}

func TestSearchbotDetailDoesNotExposeInternalErrorText(t *testing.T) {
	setupSearchbotDetailHandlerDB(t)

	status := "1"
	accessToken := "access-token"
	beta := "beta-token"
	if err := dao.DB.Create(&entity.User{
		ID:              "tenant-1",
		Email:           "tenant@example.com",
		Nickname:        "Tenant",
		AccessToken:     &accessToken,
		IsAuthenticated: "1",
		IsActive:        "1",
		IsAnonymous:     "0",
		Status:          &status,
	}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if err := dao.DB.Create(&entity.APIToken{
		TenantID: "tenant-1",
		Token:    "token-1",
		Beta:     &beta,
	}).Error; err != nil {
		t.Fatalf("failed to create api token: %v", err)
	}
	if err := dao.DB.Exec("DROP TABLE user_tenant").Error; err != nil {
		t.Fatalf("failed to drop user_tenant table: %v", err)
	}

	r := newSearchbotDetailRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/searchbots/detail?search_id=search-1", nil)
	req.Header.Set("Authorization", "Bearer "+beta)
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if strings.Contains(body, "no such table") {
		t.Fatalf("response leaked internal error text: %s", body)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["code"] != float64(common.CodeServerError) {
		t.Fatalf("code = %v, want %v", resp["code"], common.CodeServerError)
	}
}
