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

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func setupSearchDetailDAOTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.User{}, &entity.Search{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func TestSearchDAOGetDetailByID(t *testing.T) {
	db := setupSearchDetailDAOTestDB(t)
	pushDB(t, db)

	userStatus := "1"
	userAvatar := "tenant-avatar"
	if err := db.Create(&entity.User{
		ID:              "tenant-1",
		Email:           "tenant@example.com",
		Nickname:        "Tenant Nick",
		Avatar:          &userAvatar,
		IsAuthenticated: "1",
		IsActive:        "1",
		IsAnonymous:     "0",
		Status:          &userStatus,
	}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	searchStatus := "1"
	searchAvatar := "search-avatar"
	if err := db.Create(&entity.Search{
		ID:           "search-1",
		Avatar:       &searchAvatar,
		TenantID:     "tenant-1",
		Name:         "Search App",
		CreatedBy:    "tenant-1",
		SearchConfig: entity.JSONMap{"summary": true},
		Status:       &searchStatus,
	}).Error; err != nil {
		t.Fatalf("failed to create search: %v", err)
	}

	detail, err := NewSearchDAO().GetDetailByID("search-1")
	if err != nil {
		t.Fatalf("GetDetailByID failed: %v", err)
	}
	if detail == nil {
		t.Fatal("expected detail, got nil")
	}
	if detail.ID != "search-1" {
		t.Fatalf("ID = %q, want search-1", detail.ID)
	}
	if detail.Avatar == nil || *detail.Avatar != searchAvatar {
		t.Fatalf("Avatar = %v, want %q", detail.Avatar, searchAvatar)
	}
	if detail.Nickname == nil || *detail.Nickname != "Tenant Nick" {
		t.Fatalf("Nickname = %v, want Tenant Nick", detail.Nickname)
	}
	if detail.TenantAvatar == nil || *detail.TenantAvatar != userAvatar {
		t.Fatalf("TenantAvatar = %v, want %q", detail.TenantAvatar, userAvatar)
	}
	if got := detail.SearchConfig["summary"]; got != true {
		t.Fatalf("search_config.summary = %v, want true", got)
	}
}

func TestSearchDAOGetDetailByIDReturnsNilWhenJoinedUserIsInactive(t *testing.T) {
	db := setupSearchDetailDAOTestDB(t)
	pushDB(t, db)

	userStatus := "0"
	if err := db.Create(&entity.User{
		ID:              "tenant-1",
		Email:           "tenant@example.com",
		Nickname:        "Tenant Nick",
		IsAuthenticated: "1",
		IsActive:        "1",
		IsAnonymous:     "0",
		Status:          &userStatus,
	}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	searchStatus := "1"
	if err := db.Create(&entity.Search{
		ID:           "search-1",
		TenantID:     "tenant-1",
		Name:         "Search App",
		CreatedBy:    "tenant-1",
		SearchConfig: entity.JSONMap{"summary": true},
		Status:       &searchStatus,
	}).Error; err != nil {
		t.Fatalf("failed to create search: %v", err)
	}

	detail, err := NewSearchDAO().GetDetailByID("search-1")
	if err != nil {
		t.Fatalf("GetDetailByID failed: %v", err)
	}
	if detail != nil {
		t.Fatalf("expected nil detail when joined user is inactive, got %+v", detail)
	}
}
