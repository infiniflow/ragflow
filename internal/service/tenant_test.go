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
	_ "unsafe"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

//go:linkname daoModelProviderManager ragflow/internal/dao.modelProviderManager
var daoModelProviderManager *models.ProviderManager

func setupTenantPermissionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(append([]interface{}{&entity.UserTenant{}}, permissionChangeLogTestModels()...)...); err != nil {
		t.Fatalf("migrate tenant permission table: %v", err)
	}
	originalDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = originalDB })
	return db
}

func seedTenantMembership(t *testing.T, db *gorm.DB, id, userID, tenantID, role string) {
	t.Helper()
	status := "1"
	if err := db.Create(&entity.UserTenant{
		ID: id, UserID: userID, TenantID: tenantID, Role: role, Status: &status,
	}).Error; err != nil {
		t.Fatalf("seed membership: %v", err)
	}
}

// TestListMembersAuthCheck verifies a tenant member without the owner role is denied.
func TestListMembersAuthCheck(t *testing.T) {
	db := setupTenantPermissionTestDB(t)
	seedTenantMembership(t, db, "member", "user-abc", "tenant-xyz", "normal")
	seedTenantMembership(t, db, "owner", "owner-user", "tenant-xyz", "owner")
	s := &TenantService{userTenantDAO: dao.NewUserTenantDAO()}
	ctx := t.Context()
	_, code, err := s.ListMembers(ctx, "user-abc", "tenant-xyz")
	if err == nil {
		t.Fatal("expected error for non-owner, got nil")
	}
	if code != common.CodeForbidden {
		t.Errorf("expected CodeForbidden, got %v", code)
	}
}

// TestAddMemberAuthCheck verifies that a non-owner is denied by tenant policy.
func TestAddMemberAuthCheck(t *testing.T) {
	db := setupTenantPermissionTestDB(t)
	seedTenantMembership(t, db, "member", "user-abc", "tenant-xyz", "normal")
	seedTenantMembership(t, db, "owner", "owner-user", "tenant-xyz", "owner")
	s := &TenantService{userTenantDAO: dao.NewUserTenantDAO()}
	ctx := t.Context()
	_, code, err := s.AddMember(ctx, "user-abc", "tenant-xyz", &AddMemberRequest{Email: "a@b.com"})
	if err == nil {
		t.Fatal("expected error for non-owner, got nil")
	}
	if code != common.CodeForbidden {
		t.Errorf("expected CodeForbidden, got %v", code)
	}
}

// TestAddMemberEmailRequired verifies owner authorization runs before payload validation.
func TestAddMemberEmailRequired(t *testing.T) {
	db := setupTenantPermissionTestDB(t)
	seedTenantMembership(t, db, "owner", "owner-user", "tenant-xyz", "owner")
	s := &TenantService{userTenantDAO: dao.NewUserTenantDAO()}
	ctx := t.Context()
	_, code, err := s.AddMember(ctx, "owner-user", "tenant-xyz", &AddMemberRequest{Email: ""})
	if err == nil {
		t.Fatal("expected error for empty email, got nil")
	}
	if code != common.CodeArgumentError {
		t.Errorf("expected CodeArgumentError, got %v", code)
	}
}

// TestRemoveMemberAuthCheck verifies that a user outside the tenant is denied.
func TestRemoveMemberAuthCheck(t *testing.T) {
	db := setupTenantPermissionTestDB(t)
	seedTenantMembership(t, db, "owner", "owner-user", "tenant-xyz", "owner")
	s := &TenantService{userTenantDAO: dao.NewUserTenantDAO()}
	ctx := t.Context()
	code, err := s.RemoveMember(ctx, "user-abc", "tenant-xyz", "user-def")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if code != common.CodeForbidden {
		t.Errorf("expected CodeForbidden, got %v", code)
	}
}

// TestRemoveMemberSelfAllowed verifies a tenant member can remove their own membership.
func TestRemoveMemberSelfAllowed(t *testing.T) {
	db := setupTenantPermissionTestDB(t)
	seedTenantMembership(t, db, "member", "user-abc", "tenant-xyz", "normal")
	s := &TenantService{userTenantDAO: dao.NewUserTenantDAO()}
	ctx := t.Context()
	code, err := s.RemoveMember(ctx, "user-abc", "tenant-xyz", "user-abc")
	if code != common.CodeSuccess {
		t.Errorf("expected CodeSuccess, got %v: %v", code, err)
	}
	if err != nil {
		t.Fatalf("self-removal failed: %v", err)
	}
	var membership entity.UserTenant
	if err := db.Where("user_id = ? AND tenant_id = ?", "user-abc", "tenant-xyz").Take(&membership).Error; err != nil {
		t.Fatalf("read self-membership after leaving: %v", err)
	}
	if membership.Status == nil || *membership.Status != entity.TenantMembershipStatusLeft || membership.Role != "" {
		t.Fatalf("self-membership after leaving = %+v, want left status and empty role", membership)
	}
}

func TestDeclineOwnTenantInviteAllowed(t *testing.T) {
	db := setupTenantPermissionTestDB(t)
	status := entity.TenantMembershipStatusInvited
	if err := db.Create(&entity.UserTenant{ID: "invite", UserID: "invitee-user", TenantID: "tenant-xyz", Status: &status}).Error; err != nil {
		t.Fatalf("seed pending invitation: %v", err)
	}
	s := &TenantService{userTenantDAO: dao.NewUserTenantDAO()}

	code, err := s.RemoveMember(t.Context(), "invitee-user", "tenant-xyz", "invitee-user")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("RemoveMember(invitation self-decline) = (%v, %v), want success", code, err)
	}
	var membership entity.UserTenant
	if err := db.Where("user_id = ? AND tenant_id = ?", "invitee-user", "tenant-xyz").Take(&membership).Error; err != nil {
		t.Fatalf("read declined invitation: %v", err)
	}
	if membership.Status == nil || *membership.Status != entity.TenantMembershipStatusRejected || membership.Role != "" {
		t.Fatalf("declined invitation = %+v, want rejected status and empty role", membership)
	}
}

func TestRemoveMemberCannotRemoveTenantOwner(t *testing.T) {
	db := setupTenantPermissionTestDB(t)
	seedTenantMembership(t, db, "owner", "owner-user", "tenant-xyz", "owner")
	s := &TenantService{userTenantDAO: dao.NewUserTenantDAO()}

	code, err := s.RemoveMember(t.Context(), "owner-user", "tenant-xyz", "owner-user")
	if err == nil || code != common.CodeArgumentError {
		t.Fatalf("RemoveMember(owner) = (%v, %v), want argument error", code, err)
	}
}

// TestAcceptInviteAuthCheck verifies that AcceptInvite fails when DAO is not initialized.
func TestAcceptInviteAuthCheck(t *testing.T) {
	s := &TenantService{}
	// nil userTenantDAO: nil guard returns CodeServerError.
	ctx := t.Context()
	code, err := s.AcceptInvite(ctx, "user-abc", "tenant-xyz")
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
	err = db.Create(&entity.Tenant{ID: tenantID, Status: &statusVal}).Error
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
		ModelType:  int(entity.ModelTypeChat),
		Status:     "active",
	}).Error
	if err != nil {
		t.Fatalf("failed to create model: %v", err)
	}

	// 4. Run SetTenantDefaultModels
	s := NewTenantService()
	// Set chat model using modelID, explicitly passing "default" as instance name to bypass pre-existing checkModelAvailable panic
	ctx := serviceTestContext(t, userID, tenantID)
	err = s.SetTenantDefaultModels(ctx, userID, "", "default", "", "chat", modelID)
	if err != nil {
		t.Fatalf("SetTenantDefaultModels failed: %v", err)
	}

	// Verify the default is stored on the user's owner membership.
	var membership entity.UserTenant
	err = db.Where("id = ?", "ut-1").First(&membership).Error
	if err != nil {
		t.Fatalf("failed to retrieve owner membership: %v", err)
	}

	expectedDefaultModel := "gpt-4o@dummy@OpenAI"
	if membership.LLMID != expectedDefaultModel {
		t.Errorf("expected owner default LLM to be %q, got %q", expectedDefaultModel, membership.LLMID)
	}
	if membership.TenantLLMID == nil || *membership.TenantLLMID != modelID {
		t.Errorf("expected owner tenant_llm_id to be %q, got %v", modelID, membership.TenantLLMID)
	}
}
