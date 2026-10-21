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

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupUserBetaServiceDB(t *testing.T) {
	t.Helper()
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.APIToken{}); err != nil {
		t.Fatalf("failed to migrate api token: %v", err)
	}
	pushServiceDB(t, db)
}

func TestUserServiceResolveBetaAPIToken(t *testing.T) {
	ctx := t.Context()
	setupUserBetaServiceDB(t)

	accessToken := "access-token"
	status := "1"
	if err := dao.DB.Create(&entity.User{
		ID:              "user-1",
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
	if err := dao.DB.Create(&entity.UserTenant{
		ID:       "membership-1",
		UserID:   "user-1",
		TenantID: "tenant-1",
		Role:     "normal",
		Status:   &status,
	}).Error; err != nil {
		t.Fatalf("failed to create membership: %v", err)
	}

	beta := "beta-token"
	userID := "user-1"
	if err := dao.DB.Create(&entity.APIToken{
		TenantID: "tenant-1",
		UserID:   &userID,
		Token:    "token-1",
		Beta:     &beta,
	}).Error; err != nil {
		t.Fatalf("failed to create api token: %v", err)
	}
	expectedTenantID, err := resolveUserAuthTenant(ctx, &entity.User{ID: "user-1"}, "tenant-1")
	if err != nil {
		t.Fatalf("resolveUserAuthTenant() failed: %v", err)
	}

	svc := NewUserService()
	for _, auth := range []string{"Bearer " + beta, beta, "Bearer    " + beta} {
		identity, code, err := svc.ResolveBetaAPIToken(ctx, auth)
		if err != nil {
			t.Fatalf("ResolveBetaAPIToken(%q) failed: %v", auth, err)
		}
		if code != common.CodeSuccess {
			t.Fatalf("code = %v, want %v", code, common.CodeSuccess)
		}
		if identity == nil || identity.User == nil || identity.User.ID != "user-1" || identity.TenantID != expectedTenantID || identity.APIToken == nil || identity.APIToken.TenantID != expectedTenantID {
			t.Fatalf("identity = %+v, want user-1 with tenant context %q", identity, expectedTenantID)
		}
	}

	identity, code, err := svc.ResolveAPIToken(ctx, "Bearer token-1")
	if err != nil {
		t.Fatalf("ResolveAPIToken() failed: %v", err)
	}
	if code != common.CodeSuccess || identity.User.ID != "user-1" || identity.TenantID != expectedTenantID || identity.APIToken == nil || identity.APIToken.TenantID != expectedTenantID {
		t.Fatalf("ResolveAPIToken() = %+v, %v; want user-1 with tenant context %q", identity, code, expectedTenantID)
	}
}

func TestUserServiceResolveBetaAPITokenRejectsInvalidToken(t *testing.T) {
	ctx := t.Context()
	setupUserBetaServiceDB(t)

	_, code, err := NewUserService().ResolveBetaAPIToken(ctx, "Bearer missing")
	if err == nil {
		t.Fatal("expected error for invalid beta token")
	}
	if code != common.CodeUnauthorized {
		t.Fatalf("code = %v, want %v", code, common.CodeUnauthorized)
	}
}

func TestUserServiceResolveBetaAPITokenRejectsEmptyOrWhitespaceToken(t *testing.T) {
	ctx := t.Context()
	setupUserBetaServiceDB(t)

	for _, auth := range []string{"Bearer ", "   ", "\t"} {
		_, code, err := NewUserService().ResolveBetaAPIToken(ctx, auth)
		if err == nil {
			t.Fatalf("expected error for auth %q", auth)
		}
		if code != common.CodeUnauthorized {
			t.Fatalf("code = %v, want %v for auth %q", code, common.CodeUnauthorized, auth)
		}
	}
}

func TestUserServiceResolveBetaAPITokenRejectsUserWithEmptyAccessToken(t *testing.T) {
	ctx := t.Context()
	setupUserBetaServiceDB(t)

	status := "1"
	emptyToken := ""
	if err := dao.DB.Create(&entity.User{
		ID:              "user-1",
		Email:           "tenant@example.com",
		Nickname:        "Tenant",
		AccessToken:     &emptyToken,
		IsAuthenticated: "1",
		IsActive:        "1",
		IsAnonymous:     "0",
		Status:          &status,
	}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if err := dao.DB.Create(&entity.UserTenant{
		ID:       "membership-1",
		UserID:   "user-1",
		TenantID: "tenant-1",
		Role:     "normal",
		Status:   &status,
	}).Error; err != nil {
		t.Fatalf("failed to create membership: %v", err)
	}

	beta := "beta-token"
	userID := "user-1"
	if err := dao.DB.Create(&entity.APIToken{
		TenantID: "tenant-1",
		UserID:   &userID,
		Token:    "token-1",
		Beta:     &beta,
	}).Error; err != nil {
		t.Fatalf("failed to create api token: %v", err)
	}

	_, code, err := NewUserService().ResolveBetaAPIToken(ctx, "Bearer "+beta)
	if err == nil {
		t.Fatal("expected error for empty access token")
	}
	if code != common.CodeUnauthorized {
		t.Fatalf("code = %v, want %v", code, common.CodeUnauthorized)
	}
}
