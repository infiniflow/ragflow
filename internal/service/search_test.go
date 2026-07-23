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

func setupSearchServiceTestDB(t *testing.T) {
	t.Helper()
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.Search{}); err != nil {
		t.Fatalf("failed to migrate search: %v", err)
	}
	pushServiceDB(t, db)
}

func createSearchServiceTestSearch(t *testing.T, id, tenantID, name string) {
	t.Helper()

	status := string(entity.StatusValid)
	if err := dao.DB.Create(&entity.Search{
		ID:           id,
		TenantID:     tenantID,
		Name:         name,
		CreatedBy:    tenantID,
		SearchConfig: entity.JSONMap{},
		Status:       &status,
	}).Error; err != nil {
		t.Fatalf("failed to create search: %v", err)
	}
}

func TestSearchServiceCreateRejectsEmptyName(t *testing.T) {
	setupSearchServiceTestDB(t)

	_, err := NewSearchService().CreateSearch("tenant-1", "   ", nil)
	if err == nil {
		t.Fatal("expected empty name validation error")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchServiceUpdateRejectsUnauthorizedSearchID(t *testing.T) {
	setupSearchServiceTestDB(t)

	req := &UpdateSearchRequest{
		Name:         "New Name",
		SearchConfig: map[string]interface{}{},
	}
	_, err := NewSearchService().UpdateSearch("user-2", "invalid_search_id", req)
	if err == nil {
		t.Fatal("expected authorization error")
	}
	if err.Error() != "no authorization" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchServiceCreateAndUpdateRoundTrip(t *testing.T) {
	setupSearchServiceTestDB(t)

	created, err := NewSearchService().CreateSearch("tenant-1", "My Search", nil)
	if err != nil {
		t.Fatalf("CreateSearch failed: %v", err)
	}
	if created.SearchID == "" {
		t.Fatal("expected non-empty search_id")
	}

	// A different user must not be able to update it.
	req := &UpdateSearchRequest{
		Name:         "Hijacked Name",
		SearchConfig: map[string]interface{}{},
	}
	_, err = NewSearchService().UpdateSearch("user-2", created.SearchID, req)
	if err == nil || err.Error() != "no authorization" {
		t.Fatalf("expected no authorization, got %v", err)
	}

	// The owner can update name + merge config.
	req = &UpdateSearchRequest{
		Name:         "Updated Name",
		SearchConfig: map[string]interface{}{"summary": true},
	}
	updated, err := NewSearchService().UpdateSearch("tenant-1", created.SearchID, req)
	if err != nil {
		t.Fatalf("owner UpdateSearch failed: %v", err)
	}
	if updated.Name != "Updated Name" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}
	if updated.SearchConfig["summary"] != true {
		t.Fatalf("expected merged search_config, got %#v", updated.SearchConfig)
	}

	persisted, err := dao.NewSearchDAO().GetByID(created.SearchID)
	if err != nil {
		t.Fatalf("get updated search: %v", err)
	}
	if persisted.Name != "Updated Name" {
		t.Fatalf("expected persisted name, got %q", persisted.Name)
	}
}

func TestSearchServiceListSearchesReturnsOwnerDisplayFields(t *testing.T) {
	setupSearchServiceTestDB(t)

	if err := dao.DB.Create(&entity.User{
		ID:       "user-1",
		Nickname: "search owner",
		Status:   sptr("1"),
	}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	createSearchServiceTestSearch(t, "search-1", "user-1", "Search One")

	result, err := NewSearchService().ListSearches("user-1", "", 0, 0, "create_time", true, nil)
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}
	if result.Total != 1 || len(result.SearchApps) != 1 {
		t.Fatalf("expected one search app, got total=%d len=%d", result.Total, len(result.SearchApps))
	}
	if got, want := result.SearchApps[0]["nickname"], "search owner"; got != want {
		t.Fatalf("nickname = %v, want %v", got, want)
	}
}

func TestSearchServiceListSearchesNicknameFallsBackToTenantID(t *testing.T) {
	setupSearchServiceTestDB(t)

	createSearchServiceTestSearch(t, "search-1", "user-1", "Search One")

	result, err := NewSearchService().ListSearches("user-1", "", 0, 0, "create_time", true, nil)
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}
	if result.Total != 1 || len(result.SearchApps) != 1 {
		t.Fatalf("expected one search app, got total=%d len=%d", result.Total, len(result.SearchApps))
	}
	if got, want := result.SearchApps[0]["nickname"], "user-1"; got != want {
		t.Fatalf("nickname = %v, want %v", got, want)
	}
}
