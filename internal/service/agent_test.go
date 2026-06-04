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

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

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
	testDB.Create(&entity.UserCanvas{ID: "c-1", UserID: "user-1", Title: sp("My Agent")})

	svc := NewAgentService()
	ok, err := svc.CheckCanvasAccess("user-1", "c-1")
	if err != nil {
		t.Fatalf("CheckCanvasAccess failed: %v", err)
	}
	if !ok {
		t.Error("expected owner to have access")
	}
}

// TestCheckCanvasAccess_NotOwner verifies that a tenant member can access
// a team-level canvas.
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
	testDB.Create(&entity.User{ID: "user-2", Nickname: "member", Email: "c@d.com"})
	testDB.Create(&entity.UserTenant{ID: "ut-1", UserID: "user-2", TenantID: "user-1", Role: "member", Status: sp("1")})
	testDB.Create(&entity.UserCanvas{ID: "c-1", UserID: "user-1", Permission: "team", Title: sp("Team Agent")})

	svc := NewAgentService()
	ok, err := svc.CheckCanvasAccess("user-2", "c-1")
	if err != nil {
		t.Fatalf("CheckCanvasAccess failed: %v", err)
	}
	if !ok {
		t.Error("expected tenant member to have access to team canvas")
	}
}

// TestCheckCanvasAccess_PrivateCanvas_Denied verifies that a tenant member
// cannot access a private canvas.
func TestCheckCanvasAccess_PrivateCanvas_Denied(t *testing.T) {
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
	testDB.Create(&entity.User{ID: "user-2", Nickname: "member", Email: "c@d.com"})
	testDB.Create(&entity.UserTenant{ID: "ut-1", UserID: "user-2", TenantID: "user-1", Role: "member", Status: sp("1")})
	testDB.Create(&entity.UserCanvas{ID: "c-1", UserID: "user-1", Title: sp("Private Agent")})

	svc := NewAgentService()
	ok, err := svc.CheckCanvasAccess("user-2", "c-1")
	if err != nil {
		t.Fatalf("CheckCanvasAccess failed: %v", err)
	}
	if ok {
		t.Error("expected tenant member to be denied access to private canvas")
	}
}

// TestCheckCanvasAccess_NotFound verifies error for non-existent canvas.
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

// sp returns a pointer to the given string.
func sp(s string) *string { return &s }