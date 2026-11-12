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

//go:build !ee

package service

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// userIDHex is a real RAGFlow user id: a 32-char hex UUID string produced by
// common.GenerateToken(). It is NOT an integer, so any code path that treats it
// as uint/int is fundamentally broken.
const apiTokenUserIDHex = "0a1b2c3d4e5f60718293a4b5c6d7e8f90"

func newAPITokenTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}
	return db
}

// TestResolveAPITokenIdentityResolvesUserByStringID guards that the API-token
// identity path resolves a user by its real string id (a 32-char hex UUID). It
// must use the single canonical string-ID lookup on UserDAO, never a numeric
// contract that would reject every real id.
func TestResolveAPITokenIdentityResolvesUserByStringID(t *testing.T) {
	db := newAPITokenTestDB(t)
	accessToken := "secret-token-value"
	if err := db.Create(&entity.User{
		ID:          apiTokenUserIDHex,
		Email:       "a@example.com",
		Nickname:    "alice",
		AccessToken: &accessToken,
	}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	uid := apiTokenUserIDHex
	apiToken := &entity.APIToken{UserID: &uid, TenantID: "tenant-1"}
	identity, code, err := (&UserService{}).resolveAPITokenIdentity(context.Background(), apiToken)
	if err != nil {
		t.Fatalf("resolveAPITokenIdentity() error = %v, code = %v; want nil", err, code)
	}
	if code != common.CodeSuccess {
		t.Fatalf("resolveAPITokenIdentity() code = %v, want success", code)
	}
	if identity == nil || identity.User == nil || identity.User.ID != apiTokenUserIDHex {
		t.Fatalf("resolveAPITokenIdentity() identity user id = %v, want %q", identityUserID(identity), apiTokenUserIDHex)
	}
}

// TestResolveAPITokenIdentityUnknownUserReturnsUnauthorized is adversarial: a
// well-formed but unknown UUID must surface an unauthorized error, never a
// numeric-coercion failure that would mask the lookup.
func TestResolveAPITokenIdentityUnknownUserReturnsUnauthorized(t *testing.T) {
	db := newAPITokenTestDB(t)

	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	missing := "ffffffffffffffffffffffffffffffff"
	apiToken := &entity.APIToken{UserID: &missing, TenantID: "tenant-1"}
	identity, code, err := (&UserService{}).resolveAPITokenIdentity(context.Background(), apiToken)
	if err == nil {
		t.Fatalf("resolveAPITokenIdentity() = nil error, want unauthorized")
	}
	if code != common.CodeUnauthorized {
		t.Fatalf("resolveAPITokenIdentity() code = %v, want unauthorized", code)
	}
	if identity != nil {
		t.Fatalf("resolveAPITokenIdentity() identity = %v, want nil", identity)
	}
}

func identityUserID(identity *AuthIdentity) string {
	if identity == nil || identity.User == nil {
		return ""
	}
	return identity.User.ID
}
