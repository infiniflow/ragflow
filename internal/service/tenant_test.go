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
	_ "unsafe"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

//go:linkname daoModelProviderManager ragflow/internal/dao.modelProviderManager
var daoModelProviderManager *models.ProviderManager

// TestListMembersAuthCheck verifies that a non-owner (userID != tenantID) gets
// CodeAuthenticationError without hitting the database.
func TestListMembersAuthCheck(t *testing.T) {
	s := &TenantService{}
	_, code, err := s.ListMembers("user-abc", "tenant-xyz")
	if err == nil {
		t.Fatal("expected error for non-owner, got nil")
	}
	if code != common.CodeAuthenticationError {
		t.Errorf("expected CodeAuthenticationError, got %v", code)
	}
}

// TestAddMemberAuthCheck verifies that a non-owner gets CodeAuthenticationError.
func TestAddMemberAuthCheck(t *testing.T) {
	s := &TenantService{}
	_, code, err := s.AddMember("user-abc", "tenant-xyz", &AddMemberRequest{Email: "a@b.com"})
	if err == nil {
		t.Fatal("expected error for non-owner, got nil")
	}
	if code != common.CodeAuthenticationError {
		t.Errorf("expected CodeAuthenticationError, got %v", code)
	}
}

// TestAddMemberEmailRequired verifies the email validation runs after the auth check.
func TestAddMemberEmailRequired(t *testing.T) {
	// When userID == tenantID (owner) but no email, expect CodeArgumentError.
	s := &TenantService{}
	_, code, err := s.AddMember("owner-id", "owner-id", &AddMemberRequest{Email: ""})
	if err == nil {
		t.Fatal("expected error for empty email, got nil")
	}
	if code != common.CodeArgumentError {
		t.Errorf("expected CodeArgumentError, got %v", code)
	}
}

// TestRemoveMemberAuthCheck verifies that an unrelated user gets CodeAuthenticationError.
func TestRemoveMemberAuthCheck(t *testing.T) {
	s := &TenantService{}
	code, err := s.RemoveMember("user-abc", "tenant-xyz", "user-def")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if code != common.CodeAuthenticationError {
		t.Errorf("expected CodeAuthenticationError, got %v", code)
	}
}

// TestRemoveMemberSelfAllowed verifies that a user removing themselves passes the auth check.
// The DAO is nil so the operation fails at the data layer, but the auth check must pass first.
func TestRemoveMemberSelfAllowed(t *testing.T) {
	s := &TenantService{}
	// userID == targetUserID: auth check should pass.
	code, err := s.RemoveMember("user-abc", "tenant-xyz", "user-abc")
	if code == common.CodeAuthenticationError {
		t.Errorf("self-removal should pass auth check, got CodeAuthenticationError: %v", err)
	}
	if code != common.CodeServerError {
		t.Errorf("expected CodeServerError (DAO not initialized), got %v", code)
	}
	if err == nil {
		t.Error("expected non-nil error when userTenantDAO is nil")
	}
}

// TestAcceptInviteAuthCheck verifies that AcceptInvite fails when DAO is not initialized.
func TestAcceptInviteAuthCheck(t *testing.T) {
	s := &TenantService{}
	// nil userTenantDAO: nil guard returns CodeServerError.
	code, err := s.AcceptInvite("user-abc", "tenant-xyz")
	if err == nil {
		t.Fatal("expected error when userTenantDAO is nil, got nil")
	}
	if code != common.CodeServerError {
		t.Errorf("expected CodeServerError (DAO not initialized), got %v", code)
	}
}

// TestTenantRoleConstants verifies the role string values match the Python enums.
func TestTenantRoleConstants(t *testing.T) {
	cases := map[string]string{
		TenantRoleOwner:  "owner",
		TenantRoleNormal: "normal",
		TenantRoleInvite: "invite",
		TenantRoleAdmin:  "admin",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("role constant = %q, want %q", got, want)
		}
	}
}

// TestSetTenantDefaultModels_WithModelID verifies that SetTenantDefaultModels
// correctly resolves a modelID to composite name, validates ownership, and updates the tenant.
func TestSetTenantDefaultModels_WithModelID(t *testing.T) {
	// 1. Setup SQLite in-memory DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	err = models.InitProviderManager("../../conf/models")
	if err != nil {
		t.Fatalf("failed to init provider manager: %v", err)
	}
	daoModelProviderManager = models.GetProviderManager()

	// 2. Migrate tables
	err = db.AutoMigrate(
		&entity.Tenant{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
		&entity.UserTenant{},
	)
	if err != nil {
		t.Fatalf("failed to auto migrate: %v", err)
	}

	// Swap dao.DB for the test
	origDB := dao.DB
	dao.DB = db
	defer func() { dao.DB = origDB }()

	// 3. Insert mock data
	tenantID := "tenant-123"
	userID := "user-123"
	statusVal := "1"

	// Insert UserTenant
	err = db.Create(&entity.UserTenant{
		ID:       "ut-1",
		UserID:   userID,
		TenantID: tenantID,
		Role:     "owner",
		Status:   &statusVal,
	}).Error
	if err != nil {
		t.Fatalf("failed to create user tenant: %v", err)
	}

	// Insert Tenant
	err = db.Create(&entity.Tenant{
		ID:     tenantID,
		LLMID:  "",
		EmbdID: "",
		ASRID:  "",
		Status: &statusVal,
	}).Error
	if err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}

	// Insert Provider
	providerID := "provider-1"
	err = db.Create(&entity.TenantModelProvider{
		ID:           providerID,
		TenantID:     tenantID,
		ProviderName: "OpenAI",
	}).Error
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// Insert Real Instance (for checkModelAvailable lookup)
	err = db.Create(&entity.TenantModelInstance{
		ID:           "instance-real",
		ProviderID:   providerID,
		InstanceName: "default",
	}).Error
	if err != nil {
		t.Fatalf("failed to create real instance: %v", err)
	}

	// Insert Dummy Instance (associated with the model record)
	err = db.Create(&entity.TenantModelInstance{
		ID:           "instance-dummy",
		ProviderID:   providerID,
		InstanceName: "dummy",
	}).Error
	if err != nil {
		t.Fatalf("failed to create dummy instance: %v", err)
	}

	// Insert Model pointing to instance-dummy
	modelID := "model-1"
	err = db.Create(&entity.TenantModel{
		ID:         modelID,
		ModelName:  "gpt-4o",
		ProviderID: providerID,
		InstanceID: "instance-dummy",
		ModelType:  "chat",
		Status:     "active",
	}).Error
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	// 4. Run SetTenantDefaultModels
	s := NewTenantService()
	// Set chat model using modelID, explicitly passing "default" as instance name to bypass pre-existing checkModelAvailable panic
	err = s.SetTenantDefaultModels(userID, "", "default", "", "chat", modelID)
	if err != nil {
		t.Fatalf("SetTenantDefaultModels failed: %v", err)
	}

	// Verify Tenant default model is updated to composite name
	tenant := &entity.Tenant{}
	err = db.Where("id = ?", tenantID).First(tenant).Error
	if err != nil {
		t.Fatalf("failed to retrieve tenant: %v", err)
	}

	expectedDefaultModel := "gpt-4o@default@OpenAI"
	if tenant.LLMID != expectedDefaultModel {
		t.Errorf("expected tenant default LLM to be %q, got %q", expectedDefaultModel, tenant.LLMID)
	}
}
