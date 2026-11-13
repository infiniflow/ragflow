//go:build !ee

package service

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

func TestListMembersAuthCheck(t *testing.T) {
	db := setupTenantPermissionTestDB(t)
	seedTenantMembership(t, db, "member", "user-abc", "tenant-xyz", "normal")
	seedTenantMembership(t, db, "owner", "owner-user", "tenant-xyz", "owner")
	s := &TenantService{userTenantDAO: dao.NewUserTenantDAO()}
	ctx := permission.WithIdentity(t.Context(), "user-abc", "tenant-xyz")
	_, code, err := s.ListMembers(ctx, "tenant-xyz")
	if err == nil {
		t.Fatal("expected error for non-owner, got nil")
	}
	if code != common.CodeForbidden {
		t.Errorf("code = %v, want %v", code, common.CodeForbidden)
	}
}
