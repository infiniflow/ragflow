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
)

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
