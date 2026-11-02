//go:build !enterprise

package permission

import (
	"context"
	"net/url"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestFileQueryScopeFiltersInSQLAndReturnsBatchAccessMasks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.File{}, &entity.File2Document{}, &entity.Document{}, &entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}

	valid := string(entity.StatusValid)
	for _, membership := range []entity.UserTenant{
		{ID: "file-owner-membership", UserID: "file-owner", TenantID: "file-tenant", Role: string(RoleOwner), Status: &valid},
		{ID: "file-reader-membership", UserID: "reader", TenantID: "file-tenant", Role: string(RoleNormal), Status: &valid},
		{ID: "dataset-owner-membership", UserID: "dataset-owner", TenantID: "dataset-tenant", Role: string(RoleOwner), Status: &valid},
		{ID: "dataset-reader-membership", UserID: "reader", TenantID: "dataset-tenant", Role: string(RoleNormal), Status: &valid},
	} {
		if err := db.Create(&membership).Error; err != nil {
			t.Fatalf("create membership %q: %v", membership.ID, err)
		}
	}

	for _, dataset := range []entity.Knowledgebase{
		{ID: "team-dataset", TenantID: "dataset-tenant", Name: "Team", EmbdID: "embedding", Permission: string(entity.TenantPermissionTeam), CreatedBy: "dataset-owner", Status: &valid},
		{ID: "private-dataset", TenantID: "dataset-tenant", Name: "Private", EmbdID: "embedding", Permission: string(entity.TenantPermissionMe), CreatedBy: "dataset-owner", Status: &valid},
	} {
		if err := db.Create(&dataset).Error; err != nil {
			t.Fatalf("create dataset %q: %v", dataset.ID, err)
		}
	}

	for _, file := range []entity.File{
		{ID: "root", ParentID: "root", TenantID: "file-tenant", CreatedBy: "file-owner", Name: "/", Type: "folder"},
		{ID: "reader-owned", ParentID: "root", TenantID: "file-tenant", CreatedBy: "reader", Name: "reader.pdf", Type: "pdf"},
		{ID: "owner-private", ParentID: "root", TenantID: "file-tenant", CreatedBy: "file-owner", Name: "private.pdf", Type: "pdf"},
		{ID: "empty-owner", ParentID: "root", TenantID: "file-tenant", CreatedBy: "", Name: "legacy.pdf", Type: "pdf"},
		{ID: "team-linked", ParentID: "root", TenantID: "file-tenant", CreatedBy: "file-owner", Name: "team.pdf", Type: "pdf"},
		{ID: "private-linked", ParentID: "root", TenantID: "file-tenant", CreatedBy: "file-owner", Name: "private-dataset.pdf", Type: "pdf"},
	} {
		if err := db.Create(&file).Error; err != nil {
			t.Fatalf("create file %q: %v", file.ID, err)
		}
	}

	for _, document := range []entity.Document{
		{ID: "team-document", KbID: "team-dataset", ParserID: "general", ParserConfig: entity.JSONMap{}, SourceType: "local", Type: "pdf", CreatedBy: "dataset-owner", Suffix: "pdf", Status: &valid},
		{ID: "private-document", KbID: "private-dataset", ParserID: "general", ParserConfig: entity.JSONMap{}, SourceType: "local", Type: "pdf", CreatedBy: "dataset-owner", Suffix: "pdf", Status: &valid},
	} {
		if err := db.Create(&document).Error; err != nil {
			t.Fatalf("create document %q: %v", document.ID, err)
		}
	}
	for _, link := range []entity.File2Document{
		{ID: "team-link", FileID: stringPointer("team-linked"), DocumentID: stringPointer("team-document")},
		{ID: "private-link", FileID: stringPointer("private-linked"), DocumentID: stringPointer("private-document")},
	} {
		if err := db.Create(&link).Error; err != nil {
			t.Fatalf("create file-dataset link %q: %v", link.ID, err)
		}
	}

	checker := newCommunityDatabaseChecker(db)
	ctx := context.Background()
	scope, err := checker.QueryScope(ctx, Subject{UserID: "reader", TenantID: "file-tenant"}, ScopeQuery{
		Kind:      ResourceKindFile,
		Parent:    ResourceRef{Kind: ResourceKindFolder, ID: "root"},
		Operation: OperationRead,
	})
	if err != nil {
		t.Fatalf("build file query scope: %v", err)
	}
	var visible []entity.File
	query := scope.Apply(db.Model(&entity.File{})).
		Where("file.tenant_id = ? AND file.parent_id = ? AND file.id <> ?", "file-tenant", "root", "root").
		Order("file.id")
	if err := query.Find(&visible).Error; err != nil {
		t.Fatalf("query visible files: %v", err)
	}
	if len(visible) != 2 || visible[0].ID != "reader-owned" || visible[1].ID != "team-linked" {
		t.Fatalf("visible file IDs = %v, want [reader-owned team-linked]", fileIDs(visible))
	}

	refs := []ResourceRef{
		{Kind: ResourceKindFile, ID: "reader-owned"},
		{Kind: ResourceKindFile, ID: "team-linked"},
	}
	accesses, err := checker.FilterAccesses(ctx, Subject{UserID: "reader", TenantID: "file-tenant"}, refs, OperationRead)
	if err != nil {
		t.Fatalf("resolve page access masks: %v", err)
	}
	if accesses[refs[0]].PermissionMask != ResourcePermissionOwner {
		t.Errorf("owner access mask = %d, want %d", accesses[refs[0]].PermissionMask, ResourcePermissionOwner)
	}
	sharedMask := ResourcePermissionRead | ResourcePermissionWrite | ResourcePermissionManage
	if accesses[refs[1]].PermissionMask != sharedMask {
		t.Errorf("shared access mask = %d, want %d", accesses[refs[1]].PermissionMask, sharedMask)
	}

	statement := scope.Apply(db.Session(&gorm.Session{DryRun: true}).Model(&entity.File{})).
		Where("file.tenant_id = ? AND file.parent_id = ? AND file.id <> ?", "file-tenant", "root", "root").
		Find(&[]entity.File{}).Statement
	if statement.SQL.String() == "" {
		t.Fatal("file query scope produced no SQL")
	}
	if len(statement.Vars) > 40 {
		t.Fatalf("file query scope used %d bind variables, expected a fixed-size predicate", len(statement.Vars))
	}
}

func stringPointer(value string) *string { return &value }

func fileIDs(files []entity.File) []string {
	ids := make([]string, 0, len(files))
	for _, file := range files {
		ids = append(ids, file.ID)
	}
	return ids
}
