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

func TestUserServiceGetUserByBetaAPIToken(t *testing.T) {
	setupUserBetaServiceDB(t)

	accessToken := "access-token"
	status := "1"
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

	beta := "beta-token"
	if err := dao.DB.Create(&entity.APIToken{
		TenantID: "tenant-1",
		Token:    "token-1",
		Beta:     &beta,
	}).Error; err != nil {
		t.Fatalf("failed to create api token: %v", err)
	}

	svc := NewUserService()
	for _, auth := range []string{"Bearer " + beta, beta, "Bearer    " + beta} {
		user, code, err := svc.GetUserByBetaAPIToken(auth)
		if err != nil {
			t.Fatalf("GetUserByBetaAPIToken(%q) failed: %v", auth, err)
		}
		if code != common.CodeSuccess {
			t.Fatalf("code = %v, want %v", code, common.CodeSuccess)
		}
		if user == nil || user.ID != "tenant-1" {
			t.Fatalf("user = %+v, want tenant-1", user)
		}
	}
}

func TestUserServiceGetUserByBetaAPITokenRejectsInvalidToken(t *testing.T) {
	setupUserBetaServiceDB(t)

	_, code, err := NewUserService().GetUserByBetaAPIToken("Bearer missing")
	if err == nil {
		t.Fatal("expected error for invalid beta token")
	}
	if code != common.CodeUnauthorized {
		t.Fatalf("code = %v, want %v", code, common.CodeUnauthorized)
	}
}

func TestUserServiceGetUserByBetaAPITokenRejectsEmptyOrWhitespaceToken(t *testing.T) {
	setupUserBetaServiceDB(t)

	for _, auth := range []string{"Bearer ", "   ", "\t"} {
		_, code, err := NewUserService().GetUserByBetaAPIToken(auth)
		if err == nil {
			t.Fatalf("expected error for auth %q", auth)
		}
		if code != common.CodeUnauthorized {
			t.Fatalf("code = %v, want %v for auth %q", code, common.CodeUnauthorized, auth)
		}
	}
}

func TestUserServiceGetUserByBetaAPITokenRejectsUserWithEmptyAccessToken(t *testing.T) {
	setupUserBetaServiceDB(t)

	status := "1"
	emptyToken := ""
	if err := dao.DB.Create(&entity.User{
		ID:              "tenant-1",
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

	beta := "beta-token"
	if err := dao.DB.Create(&entity.APIToken{
		TenantID: "tenant-1",
		Token:    "token-1",
		Beta:     &beta,
	}).Error; err != nil {
		t.Fatalf("failed to create api token: %v", err)
	}

	_, code, err := NewUserService().GetUserByBetaAPIToken("Bearer " + beta)
	if err == nil {
		t.Fatal("expected error for empty access token")
	}
	if code != common.CodeUnauthorized {
		t.Fatalf("code = %v, want %v", code, common.CodeUnauthorized)
	}
}
