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

package service

import (
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupSearchShareServiceDB(t *testing.T) {
	t.Helper()
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.Search{}); err != nil {
		t.Fatalf("failed to migrate search: %v", err)
	}
	pushServiceDB(t, db)
}

func TestSearchServiceGetSearchShareDetail(t *testing.T) {
	setupSearchShareServiceDB(t)

	status := "1"
	avatar := "tenant-avatar"
	accessToken := "access-token"
	if err := dao.DB.Create(&entity.User{
		ID:              "tenant-1",
		Email:           "tenant@example.com",
		Nickname:        "Tenant",
		Avatar:          &avatar,
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

	searchAvatar := "search-avatar"
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

	detail, err := NewSearchService().GetSearchShareDetail("tenant-1", "search-1")
	if err != nil {
		t.Fatalf("GetSearchShareDetail failed: %v", err)
	}
	if detail == nil {
		t.Fatal("expected detail, got nil")
	}
	if detail.Avatar == nil || *detail.Avatar != searchAvatar {
		t.Fatalf("Avatar = %v, want %q", detail.Avatar, searchAvatar)
	}
	if got := detail.SearchConfig["summary"]; got != true {
		t.Fatalf("search_config.summary = %v, want true", got)
	}
}

func TestSearchServiceGetSearchShareDetailRejectsUnauthorizedUser(t *testing.T) {
	setupSearchShareServiceDB(t)

	status := "1"
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

	_, err := NewSearchService().GetSearchShareDetail("user-2", "search-1")
	if err == nil {
		t.Fatal("expected permission error")
	}
	if !strings.Contains(err.Error(), "has no permission") {
		t.Fatalf("err = %v, want permission error", err)
	}
}
