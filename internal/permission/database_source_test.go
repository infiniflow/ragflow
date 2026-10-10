//go:build !enterprise

package permission

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDatabaseCheckerAuthorizesDatasetFromPersistedRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}

	valid := string(entity.StatusValid)
	dataset := entity.Knowledgebase{
		ID: "dataset-1", TenantID: "tenant-1", CreatedBy: "creator-1",
		Permission: string(entity.TenantPermissionTeam), Status: &valid,
	}
	ownerMembership := entity.UserTenant{
		ID: "owner-membership", UserID: "tenant-owner", TenantID: dataset.TenantID,
		Role: string(RoleOwner), Status: &valid,
	}
	membership := entity.UserTenant{
		ID: "membership-1", UserID: "member-1", TenantID: dataset.TenantID,
		Role: string(RoleNormal), Status: &valid,
	}
	if err := db.Create(&dataset).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	if err := db.Create(&ownerMembership).Error; err != nil {
		t.Fatalf("create owner membership: %v", err)
	}

	checker := NewDatabaseChecker(db)
	subject := Subject{UserID: membership.UserID}
	resources, err := (&databaseSource{db: db}).GetResources(context.Background(), []ResourceRef{{
		Kind: ResourceKindDataset,
		ID:   dataset.ID,
	}})
	if err != nil {
		t.Fatalf("load dataset authorization facts: %v", err)
	}
	if len(resources) != 1 || resources[0].OwnerUserID != ownerMembership.UserID || resources[0].CreatedBy != dataset.CreatedBy {
		t.Fatalf("dataset resource = %+v, want tenant owner %q and creator %q", resources, ownerMembership.UserID, dataset.CreatedBy)
	}

	ref := ResourceRef{Kind: ResourceKindDataset, ID: dataset.ID}
	if err := checker.CheckResource(context.Background(), subject, ref, OperationRead); err != nil {
		t.Errorf("CheckResource(%+v) error = %v, want nil", ref, err)
	}
	adminMembership := entity.UserTenant{
		ID: "admin-membership", UserID: "admin-1", TenantID: dataset.TenantID,
		Role: string(RoleAdmin), Status: &valid,
	}
	if err := db.Create(&adminMembership).Error; err != nil {
		t.Fatalf("create admin membership: %v", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: adminMembership.UserID}, ref, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("CheckResource(admin member) error = %v, want %v", err, ErrPermissionDenied)
	}

	access, err := checker.ResolveAccess(context.Background(), subject, ref)
	if err != nil {
		t.Fatalf("ResolveAccess(dataset) error = %v", err)
	}
	if access.TenantID != dataset.TenantID || access.Source != AccessSourceTenant {
		t.Fatalf("dataset access = %+v, want tenant access", access)
	}
}

func TestDatabaseCheckerRejectsPrivateDatasetForOtherTenantMember(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}

	valid := string(entity.StatusValid)
	dataset := entity.Knowledgebase{
		ID: "dataset-private", TenantID: "tenant-1", CreatedBy: "creator-1",
		Permission: string(entity.TenantPermissionMe), Status: &valid,
	}
	membership := entity.UserTenant{
		ID: "membership-1", UserID: "member-1", TenantID: "tenant-1",
		Role: string(RoleNormal), Status: &valid,
	}
	ownerMembership := entity.UserTenant{
		ID: "owner-membership", UserID: "tenant-owner", TenantID: "tenant-1",
		Role: string(RoleOwner), Status: &valid,
	}
	if err := db.Create(&dataset).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	if err := db.Create(&ownerMembership).Error; err != nil {
		t.Fatalf("create owner membership: %v", err)
	}

	err = NewDatabaseChecker(db).CheckResource(context.Background(), Subject{UserID: membership.UserID}, ResourceRef{
		Kind: ResourceKindDataset,
		ID:   dataset.ID,
	}, OperationRead)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("CheckResource(private dataset) error = %v, want %v", err, ErrPermissionDenied)
	}
}

func TestDatabaseCheckerScopesDatasetsToMembershipAndVisibility(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}

	valid := string(entity.StatusValid)
	datasets := []entity.Knowledgebase{
		{ID: "team-visible", TenantID: "member-tenant", CreatedBy: "tenant-owner", Permission: string(entity.TenantPermissionTeam), Status: &valid},
		{ID: "private-hidden", TenantID: "member-tenant", CreatedBy: "tenant-owner", Permission: string(entity.TenantPermissionMe), Status: &valid},
		{ID: "outside-tenant", TenantID: "other-tenant", CreatedBy: "other-owner", Permission: string(entity.TenantPermissionTeam), Status: &valid},
	}
	for i := range datasets {
		if err := db.Create(&datasets[i]).Error; err != nil {
			t.Fatalf("create dataset %q: %v", datasets[i].ID, err)
		}
	}
	memberships := []entity.UserTenant{
		{ID: "member", UserID: "member-user", TenantID: "member-tenant", Role: string(RoleNormal), Status: &valid},
		{ID: "owner", UserID: "tenant-owner", TenantID: "member-tenant", Role: string(RoleOwner), Status: &valid},
		{ID: "other-owner", UserID: "other-owner", TenantID: "other-tenant", Role: string(RoleOwner), Status: &valid},
	}
	for i := range memberships {
		if err := db.Create(&memberships[i]).Error; err != nil {
			t.Fatalf("create membership %q: %v", memberships[i].ID, err)
		}
	}

	scope, err := NewDatabaseChecker(db).Scope(context.Background(), Subject{UserID: "member-user"}, ScopeQuery{
		Kind: ResourceKindDataset, Operation: OperationRead,
	})
	if err != nil {
		t.Fatalf("Scope(datasets) error = %v", err)
	}
	if scope.Mode != ScopeIDs || len(scope.ResourceIDs) != 1 || scope.ResourceIDs[0] != "team-visible" {
		t.Fatalf("Scope(datasets) = %+v, want only the team-visible dataset", scope)
	}
}
