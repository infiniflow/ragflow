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
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// TestListVersions_Success verifies that ListVersions returns all versions
// for a canvas, ordered by update_time DESC.
func TestListVersions_Success(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	// Migrate tables needed for agent versions
	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	now := time.Now()

	// Insert canvas owner
	testDB.Create(&entity.User{ID: "user-1", Nickname: "owner", Email: "owner@test.com"})

	// Insert canvas
	testDB.Create(&entity.UserCanvas{
		ID:     "canvas-1",
		UserID: "user-1",
		Title:  sptr("Test Agent"),
	})

	// Insert 3 versions with staggered timestamps
	testDB.Create(&entity.UserCanvasVersion{
		ID:           "v1",
		UserCanvasID: "canvas-1",
		Title:        sptr("v1_oldest"),
		BaseModel: entity.BaseModel{
			CreateTime: ptr(now.Add(-2 * time.Hour).UnixMilli()),
			UpdateTime: ptr(now.Add(-2 * time.Hour).UnixMilli()),
		},
	})
	testDB.Create(&entity.UserCanvasVersion{
		ID:           "v2",
		UserCanvasID: "canvas-1",
		Title:        sptr("v2_middle"),
		BaseModel: entity.BaseModel{
			CreateTime: ptr(now.Add(-1 * time.Hour).UnixMilli()),
			UpdateTime: ptr(now.Add(-1 * time.Hour).UnixMilli()),
		},
	})
	testDB.Create(&entity.UserCanvasVersion{
		ID:           "v3",
		UserCanvasID: "canvas-1",
		Title:        sptr("v3_newest"),
		BaseModel: entity.BaseModel{
			CreateTime: ptr(now.UnixMilli()),
			UpdateTime: ptr(now.UnixMilli()),
		},
	})

	svc := NewAgentService()
	versions, err := svc.ListVersions("canvas-1")
	if err != nil {
		t.Fatalf("ListVersions failed: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	// Verify DESC order
	if *versions[0].Title != "v3_newest" {
		t.Errorf("expected v3_newest first, got %s", *versions[0].Title)
	}
	if *versions[1].Title != "v2_middle" {
		t.Errorf("expected v2_middle second, got %s", *versions[1].Title)
	}
	if *versions[2].Title != "v1_oldest" {
		t.Errorf("expected v1_oldest last, got %s", *versions[2].Title)
	}
}

// TestListVersions_Empty verifies that ListVersions returns an empty slice
// when no versions exist.
func TestListVersions_Empty(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.UserCanvas{},
		&entity.UserCanvasVersion{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	// Insert canvas with no versions
	testDB.Create(&entity.UserCanvas{
		ID:     "canvas-empty",
		UserID: "user-1",
		Title:  sptr("Empty Agent"),
	})

	svc := NewAgentService()
	versions, err := svc.ListVersions("canvas-empty")
	if err != nil {
		t.Fatalf("ListVersions failed: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected 0 versions, got %d", len(versions))
	}
}

// TestCheckCanvasAccess_Owner verifies that the canvas owner gets access.
func TestCheckCanvasAccess_Owner(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	testDB.Create(&entity.User{ID: "user-1", Nickname: "owner", Email: "a@b.com"})
	testDB.Create(&entity.UserCanvas{ID: "c-1", UserID: "user-1", Title: sptr("My Agent")})

	svc := NewAgentService()
	ok, err := svc.CheckCanvasAccess("user-1", "c-1")
	if err != nil {
		t.Fatalf("CheckCanvasAccess failed: %v", err)
	}
	if !ok {
		t.Error("expected owner to have access")
	}
}

// TestCheckCanvasAccess_NotOwner verifies that another user is denied.
func TestCheckCanvasAccess_NotOwner(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	testDB.Create(&entity.User{ID: "user-1", Nickname: "owner", Email: "a@b.com"})
	testDB.Create(&entity.User{ID: "user-2", Nickname: "attacker", Email: "c@d.com"})
	testDB.Create(&entity.UserCanvas{ID: "c-1", UserID: "user-1", Title: sptr("My Agent")})

	svc := NewAgentService()
	ok, err := svc.CheckCanvasAccess("user-2", "c-1")
	if err != nil {
		t.Fatalf("CheckCanvasAccess failed: %v", err)
	}
	if ok {
		t.Error("expected non-owner to be denied access")
	}
}

// TestCheckCanvasAccess_NotFound verifies behavior for non-existent canvas.
func TestCheckCanvasAccess_NotFound(t *testing.T) {
	testDB := setupServiceTestDB(t)
	t.Helper()

	if err := testDB.AutoMigrate(
		&entity.User{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() { dao.DB = orig })

	testDB.Create(&entity.User{ID: "user-1", Nickname: "tester", Email: "a@b.com"})

	svc := NewAgentService()
	_, err := svc.CheckCanvasAccess("user-1", "non-existent")
	if err == nil {
		t.Error("expected error for non-existent canvas")
	}
}

// ptr returns a pointer to the given int64.
func ptr(v int64) *int64 { return &v }