package file

import (
	"errors"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func seedFileBrowsing(t *testing.T) {
	t.Helper()
	db := setupFolderTestDB(t)
	for _, membership := range []entity.UserTenant{
		{ID: "owner-membership", UserID: "owner", TenantID: "owner", Role: string(permission.RoleOwner)},
		{ID: "reader-membership", UserID: "reader", TenantID: "owner", Role: string(permission.RoleNormal)},
	} {
		if err := db.Create(&membership).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []entity.File{
		{ID: "root", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "/", Type: FileTypeFolder},
		{ID: "tags", ParentID: "root", TenantID: "owner", CreatedBy: "owner", Name: "tags.txt", Type: "doc"},
		{ID: "foreign-root", ParentID: "foreign-root", TenantID: "foreign", Name: "/", Type: FileTypeFolder},
		{ID: "foreign-tags", ParentID: "foreign-root", TenantID: "foreign", Name: "tags.txt", Type: "doc"},
	} {
		if err := db.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestTenantMemberCanBrowseTagFiles(t *testing.T) {
	seedFileBrowsing(t)
	svc := testFileService()
	ctx := t.Context()
	list, err := svc.ListFilesForTenant(ctx, "reader", "owner", "", 1, 100, nil, "")
	if err != nil {
		t.Fatalf("list shared tenant files: %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("file count = %d, want 1", list.Total)
	}
	ancestors, err := svc.GetAllParentFoldersForTenant(ctx, "reader", "owner", "tags")
	if err != nil {
		t.Fatalf("resolve shared tenant tag file: %v", err)
	}
	if len(ancestors) != 2 {
		t.Fatalf("ancestor count = %d, want 2", len(ancestors))
	}
	if _, err := svc.GetFileContent(ctx, "reader", "tags"); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("read file contents error = %v, want permission denied", err)
	}
	if err := svc.MoveFiles(ctx, "reader", []string{"tags"}, "", "new-tags.txt"); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("rename file error = %v, want permission denied", err)
	}
}

func TestTenantFileBrowsingRejectsNonmember(t *testing.T) {
	seedFileBrowsing(t)
	svc := testFileService()
	if _, err := svc.ListFilesForTenant(t.Context(), "outsider", "owner", "", 1, 100, nil, ""); err == nil {
		t.Fatal("nonmember listed another tenant's files")
	}
	if _, err := svc.GetAllParentFoldersForTenant(t.Context(), "outsider", "owner", "tags"); err == nil {
		t.Fatal("nonmember resolved another tenant's file path")
	}
	if err := dao.DB.Model(&entity.UserTenant{}).Where("id = ?", "reader-membership").Update("status", "0").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListFilesForTenant(t.Context(), "reader", "owner", "", 1, 100, nil, ""); err == nil {
		t.Fatal("inactive member listed another tenant's files")
	}
}

func TestTenantFileBrowsingRejectsForeignEntries(t *testing.T) {
	seedFileBrowsing(t)
	svc := testFileService()
	if _, err := svc.ListFilesForTenant(t.Context(), "reader", "owner", "foreign-root", 1, 100, nil, ""); !errors.Is(err, permission.ErrResourceNotFound) {
		t.Fatalf("foreign folder error = %v, want resource not found", err)
	}
	if _, err := svc.GetAllParentFoldersForTenant(t.Context(), "reader", "owner", "foreign-tags"); !errors.Is(err, permission.ErrResourceNotFound) {
		t.Fatalf("foreign file error = %v, want resource not found", err)
	}
}
