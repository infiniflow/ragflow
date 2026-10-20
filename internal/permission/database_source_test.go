//go:build !enterprise

package permission

import (
	"context"
	"errors"
	"net/url"
	"strings"
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
	for _, operation := range []Operation{OperationRead, OperationUpdate, OperationDeleteContent, OperationRun, OperationUse} {
		if err := checker.CheckResource(context.Background(), subject, ref, operation); err != nil {
			t.Errorf("shared dataset CheckResource(%s) error = %v, want nil", operation, err)
		}
	}
	for _, operation := range []Operation{OperationCreate, OperationDelete, OperationShare} {
		if err := checker.CheckResource(context.Background(), subject, ref, operation); !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("shared dataset CheckResource(%s) error = %v, want permission denied", operation, err)
		}
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
	if err := checker.CheckResource(context.Background(), Subject{UserID: ownerMembership.UserID}, ref, OperationDelete); err != nil {
		t.Errorf("tenant owner CheckResource(delete dataset) error = %v, want nil", err)
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
		{ID: "admin", UserID: "admin-user", TenantID: "member-tenant", Role: string(RoleAdmin), Status: &valid},
		{ID: "owner", UserID: "tenant-owner", TenantID: "member-tenant", Role: string(RoleOwner), Status: &valid},
		{ID: "other-owner", UserID: "other-owner", TenantID: "other-tenant", Role: string(RoleOwner), Status: &valid},
	}
	for i := range memberships {
		if err := db.Create(&memberships[i]).Error; err != nil {
			t.Fatalf("create membership %q: %v", memberships[i].ID, err)
		}
	}

	checker := NewDatabaseChecker(db)
	scope, err := checker.Scope(context.Background(), Subject{UserID: "member-user"}, ScopeQuery{
		Kind: ResourceKindDataset, Operation: OperationRead,
	})
	if err != nil {
		t.Fatalf("Scope(datasets) error = %v", err)
	}
	if scope.Mode != ScopeIDs || len(scope.ResourceIDs) != 1 || scope.ResourceIDs[0] != "team-visible" {
		t.Fatalf("Scope(datasets) = %+v, want only the team-visible dataset", scope)
	}

	adminScope, err := checker.Scope(context.Background(), Subject{UserID: "admin-user"}, ScopeQuery{
		Kind: ResourceKindDataset, Operation: OperationRead,
	})
	if err != nil {
		t.Fatalf("Scope(datasets, admin) error = %v", err)
	}
	if adminScope.Mode != ScopeNone {
		t.Fatalf("Scope(datasets, admin) = %+v, want no accessible datasets", adminScope)
	}

	queryScope, err := checker.QueryScope(context.Background(), Subject{UserID: "member-user"}, ScopeQuery{
		Kind: ResourceKindDataset, Operation: OperationRead,
	})
	if err != nil {
		t.Fatalf("QueryScope(datasets) error = %v", err)
	}
	var queryIDs []string
	if err := queryScope.Apply(db.Model(&entity.Knowledgebase{})).Order("knowledgebase.id").Pluck("knowledgebase.id", &queryIDs).Error; err != nil {
		t.Fatalf("query authorized datasets: %v", err)
	}
	if len(queryIDs) != 1 || queryIDs[0] != "team-visible" {
		t.Fatalf("QueryScope(datasets) returned %v, want only team-visible", queryIDs)
	}
	adminQueryScope, err := checker.QueryScope(context.Background(), Subject{UserID: "admin-user"}, ScopeQuery{
		Kind: ResourceKindDataset, Operation: OperationRead,
	})
	if err != nil {
		t.Fatalf("QueryScope(datasets, admin) error = %v", err)
	}
	var adminQueryIDs []string
	if err := adminQueryScope.Apply(db.Model(&entity.Knowledgebase{})).Order("knowledgebase.id").Pluck("knowledgebase.id", &adminQueryIDs).Error; err != nil {
		t.Fatalf("query authorized datasets for admin: %v", err)
	}
	if len(adminQueryIDs) != 0 {
		t.Fatalf("QueryScope(datasets, admin) returned %v, want no datasets", adminQueryIDs)
	}
	dryRun := db.Session(&gorm.Session{DryRun: true})
	statement := queryScope.Apply(dryRun.Model(&entity.Knowledgebase{})).Find(&[]entity.Knowledgebase{}).Statement
	if !strings.Contains(strings.ToUpper(statement.SQL.String()), "EXISTS") {
		t.Fatalf("QueryScope should compile authorization into SQL, got: %s", statement.SQL.String())
	}
}

func TestCheckerQueryScopeFallsBackToAuthorizedIDs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("CREATE TABLE dialog (id TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatalf("create dialog table: %v", err)
	}
	if err := db.Exec("INSERT INTO dialog (id) VALUES (?), (?)", "visible-chat", "hidden-chat").Error; err != nil {
		t.Fatalf("insert chats: %v", err)
	}

	chatRef := ResourceRef{Kind: ResourceKindChat, ID: "visible-chat"}
	source := &permissionTestSource{
		memberships: map[string]*Membership{
			"member\x00tenant": {UserID: "member", TenantID: "tenant", Role: RoleNormal, Active: true},
		},
		resources: map[ResourceRef]Resource{
			chatRef: {
				Ref: chatRef, TenantID: "tenant", OwnerUserID: "tenant", Visibility: VisibilityTenant,
				TenantRequirement: TenantMember, OwnerOperations: []Operation{OperationRead}, Active: true,
			},
		},
		candidates: []ResourceRef{chatRef},
	}
	queryScope, err := NewChecker(source).QueryScope(context.Background(), Subject{UserID: "member"}, ScopeQuery{
		Kind: ResourceKindChat, Operation: OperationRead,
	})
	if err != nil {
		t.Fatalf("QueryScope(chats) error = %v", err)
	}
	var ids []string
	if err := queryScope.Apply(db.Table("dialog")).Order("dialog.id").Pluck("dialog.id", &ids).Error; err != nil {
		t.Fatalf("query chats with fallback scope: %v", err)
	}
	if len(ids) != 1 || ids[0] != "visible-chat" {
		t.Fatalf("QueryScope(chats) returned %v, want only visible-chat", ids)
	}
}

func TestDatabaseCheckerSharedCanvasRestrictsOwnerOperations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}
	valid := string(entity.StatusValid)
	canvases := []entity.UserCanvas{
		{ID: "team-canvas", UserID: "owner-1", Permission: string(entity.TenantPermissionTeam)},
		{ID: "private-canvas", UserID: "owner-1", Permission: string(entity.TenantPermissionMe)},
		{ID: "other-team-canvas", UserID: "owner-2", Permission: string(entity.TenantPermissionTeam)},
	}
	for i := range canvases {
		if err := db.Create(&canvases[i]).Error; err != nil {
			t.Fatalf("create canvas %q: %v", canvases[i].ID, err)
		}
	}
	membership := entity.UserTenant{
		ID: "member-owner-1", UserID: "member-1", TenantID: "owner-1",
		Role: string(RoleNormal), Status: &valid,
	}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	adminMembership := entity.UserTenant{
		ID: "admin-owner-1", UserID: "admin-1", TenantID: "owner-1",
		Role: string(RoleAdmin), Status: &valid,
	}
	if err := db.Create(&adminMembership).Error; err != nil {
		t.Fatalf("create admin membership: %v", err)
	}
	checker := NewDatabaseChecker(db)
	teamCanvas := ResourceRef{Kind: ResourceKindCanvas, ID: "team-canvas"}
	privateCanvas := ResourceRef{Kind: ResourceKindCanvas, ID: "private-canvas"}

	for _, operation := range []Operation{OperationRead, OperationUpdate, OperationRun, OperationUse} {
		if err := checker.CheckResource(context.Background(), Subject{UserID: membership.UserID}, teamCanvas, operation); err != nil {
			t.Errorf("team member CheckResource(%s) error = %v, want nil", operation, err)
		}
	}
	for _, operation := range []Operation{OperationShare, OperationDelete} {
		if err := checker.CheckResource(context.Background(), Subject{UserID: membership.UserID}, teamCanvas, operation); !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("team member CheckResource(%s) error = %v, want permission denied", operation, err)
		}
	}
	for _, operation := range []Operation{OperationShare, OperationDelete} {
		if err := checker.CheckResource(context.Background(), Subject{UserID: "owner-1"}, teamCanvas, operation); err != nil {
			t.Errorf("canvas owner CheckResource(%s) error = %v, want nil", operation, err)
		}
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: adminMembership.UserID}, teamCanvas, OperationRead); err != nil {
		t.Errorf("tenant admin CheckResource(read) error = %v, want nil", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "owner-1"}, privateCanvas, OperationDelete); err != nil {
		t.Errorf("owner CheckResource(delete) error = %v, want nil", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "outsider"}, teamCanvas, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("outsider CheckResource(read) error = %v, want permission denied", err)
	}

	scope, err := checker.Scope(context.Background(), Subject{UserID: membership.UserID}, ScopeQuery{
		Kind: ResourceKindCanvas, Operation: OperationRead,
	})
	if err != nil {
		t.Fatalf("Scope(canvases) error = %v", err)
	}
	if scope.Mode != ScopeIDs || len(scope.ResourceIDs) != 1 || scope.ResourceIDs[0] != teamCanvas.ID {
		t.Fatalf("Scope(canvases) = %+v, want only team-canvas", scope)
	}
}

func TestDatabaseCheckerRestrictsAgentSessionRunToCreator(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.API4Conversation{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}
	if err := db.Create(&entity.UserCanvas{ID: "canvas-1", UserID: "canvas-owner"}).Error; err != nil {
		t.Fatalf("create canvas: %v", err)
	}
	if err := db.Create(&entity.API4Conversation{ID: "session-1", DialogID: "canvas-1", UserID: "session-owner"}).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	checker := NewDatabaseChecker(db)
	session := ResourceRef{Kind: ResourceKindAgentSession, ID: "session-1"}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "session-owner"}, session, OperationRun); err != nil {
		t.Fatalf("session owner CheckResource(run) error = %v, want nil", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "canvas-owner"}, session, OperationRun); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("canvas owner CheckResource(run) error = %v, want permission denied for another user's session", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "outsider"}, session, OperationRun); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("outsider CheckResource(run) error = %v, want permission denied", err)
	}

	scope, err := checker.Scope(context.Background(), Subject{UserID: "session-owner"}, ScopeQuery{
		Kind: ResourceKindAgentSession, Parent: ResourceRef{Kind: ResourceKindCanvas, ID: "canvas-1"}, Operation: OperationRun,
	})
	if err != nil {
		t.Fatalf("Scope(agent sessions) error = %v", err)
	}
	if scope.Mode != ScopeIDs || len(scope.ResourceIDs) != 1 || scope.ResourceIDs[0] != session.ID {
		t.Fatalf("Scope(agent sessions) = %+v, want only session-1", scope)
	}
}

func TestDatabaseCheckerAuthorizesMemoryFromPersistedRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Memory{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}

	valid := string(entity.StatusValid)
	memories := []entity.Memory{
		{ID: "team-memory", Name: "team", TenantID: "team-1", StorageType: "table", EmbdID: "embedding", LLMID: "chat", Permissions: string(entity.TenantPermissionTeam), MemorySize: 1024, ForgettingPolicy: "FIFO", Temperature: 0.5},
		{ID: "private-memory", Name: "private", TenantID: "owner-1", StorageType: "table", EmbdID: "embedding", LLMID: "chat", Permissions: string(entity.TenantPermissionMe), MemorySize: 1024, ForgettingPolicy: "FIFO", Temperature: 0.5},
	}
	for i := range memories {
		if err := db.Create(&memories[i]).Error; err != nil {
			t.Fatalf("create memory %q: %v", memories[i].ID, err)
		}
	}
	membership := entity.UserTenant{
		ID: "member-team-1", UserID: "member-1", TenantID: "team-1",
		Role: string(RoleNormal), Status: &valid,
	}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}

	checker := NewDatabaseChecker(db)
	teamRef := ResourceRef{Kind: ResourceKindMemory, ID: "team-memory"}
	privateRef := ResourceRef{Kind: ResourceKindMemory, ID: "private-memory"}
	resources, err := (&databaseSource{db: db}).GetResources(context.Background(), []ResourceRef{teamRef, privateRef})
	if err != nil {
		t.Fatalf("load memory authorization facts: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("loaded %d memory resources, want 2", len(resources))
	}
	for _, resource := range resources {
		if resource.TenantID != resource.OwnerUserID {
			t.Errorf("memory resource %q has tenant %q and owner %q; current memory schema uses tenant_id as owner identity", resource.Ref.ID, resource.TenantID, resource.OwnerUserID)
		}
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: membership.UserID}, teamRef, OperationRead); err != nil {
		t.Errorf("team member CheckResource(read) error = %v, want nil", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: membership.UserID}, teamRef, OperationUpdate); err != nil {
		t.Errorf("team member CheckResource(update) error = %v, want nil", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: membership.UserID}, teamRef, OperationShare); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("team member CheckResource(share) error = %v, want permission denied", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "outsider"}, teamRef, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("outsider CheckResource(read) error = %v, want permission denied", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "owner-1"}, privateRef, OperationRead); err != nil {
		t.Errorf("memory owner CheckResource(read) error = %v, want nil", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: membership.UserID}, privateRef, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("team member CheckResource(private read) error = %v, want permission denied", err)
	}
}

func TestDatabaseCheckerScopesMemoriesToOwnerAndTeamMembership(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Memory{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}

	memories := []entity.Memory{
		{ID: "owner-private", Name: "owner", TenantID: "member-1", StorageType: "table", EmbdID: "embedding", LLMID: "chat", Permissions: string(entity.TenantPermissionMe), MemorySize: 1024, ForgettingPolicy: "FIFO", Temperature: 0.5},
		{ID: "team-visible", Name: "team", TenantID: "team-1", StorageType: "table", EmbdID: "embedding", LLMID: "chat", Permissions: string(entity.TenantPermissionTeam), MemorySize: 1024, ForgettingPolicy: "FIFO", Temperature: 0.5},
		{ID: "team-private", Name: "private", TenantID: "team-1", StorageType: "table", EmbdID: "embedding", LLMID: "chat", Permissions: string(entity.TenantPermissionMe), MemorySize: 1024, ForgettingPolicy: "FIFO", Temperature: 0.5},
		{ID: "other-team", Name: "other", TenantID: "other-tenant", StorageType: "table", EmbdID: "embedding", LLMID: "chat", Permissions: string(entity.TenantPermissionTeam), MemorySize: 1024, ForgettingPolicy: "FIFO", Temperature: 0.5},
	}
	for i := range memories {
		if err := db.Create(&memories[i]).Error; err != nil {
			t.Fatalf("create memory %q: %v", memories[i].ID, err)
		}
	}
	valid := string(entity.StatusValid)
	memberships := []entity.UserTenant{
		{ID: "member-team-1", UserID: "member-1", TenantID: "team-1", Role: string(RoleNormal), Status: &valid},
		{ID: "owner-other", UserID: "other-owner", TenantID: "other-tenant", Role: string(RoleOwner), Status: &valid},
	}
	for i := range memberships {
		if err := db.Create(&memberships[i]).Error; err != nil {
			t.Fatalf("create membership %q: %v", memberships[i].ID, err)
		}
	}

	scope, err := NewDatabaseChecker(db).Scope(context.Background(), Subject{UserID: "member-1"}, ScopeQuery{
		Kind: ResourceKindMemory, Operation: OperationRead,
	})
	if err != nil {
		t.Fatalf("Scope(memories) error = %v", err)
	}
	if scope.Mode != ScopeIDs || len(scope.ResourceIDs) != 2 {
		t.Fatalf("Scope(memories) = %+v, want owner-private and team-visible", scope)
	}
	got := map[string]bool{}
	for _, id := range scope.ResourceIDs {
		got[id] = true
	}
	for _, id := range []string{"owner-private", "team-visible"} {
		if !got[id] {
			t.Errorf("Scope(memories) missing authorized memory %q", id)
		}
	}
	for _, id := range []string{"team-private", "other-team"} {
		if got[id] {
			t.Errorf("Scope(memories) includes unauthorized memory %q", id)
		}
	}
}

func TestDatabaseCheckerAuthorizesFilesThroughLinkedDatasetAccess(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.File{}, &entity.File2Document{}, &entity.Document{}, &entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}

	valid := string(entity.StatusValid)
	datasets := []entity.Knowledgebase{
		{ID: "team-dataset", TenantID: "team-tenant", Name: "Team", EmbdID: "embedding", Permission: string(entity.TenantPermissionTeam), CreatedBy: "team-owner", Status: &valid},
		{ID: "private-dataset", TenantID: "private-tenant", Name: "Private", EmbdID: "embedding", Permission: string(entity.TenantPermissionMe), CreatedBy: "private-owner", Status: &valid},
	}
	for i := range datasets {
		if err := db.Create(&datasets[i]).Error; err != nil {
			t.Fatalf("create dataset %q: %v", datasets[i].ID, err)
		}
	}
	files := []entity.File{
		{ID: "team-file", ParentID: "team-root", TenantID: "file-owner", CreatedBy: "file-creator", Name: "team.pdf", Type: "pdf"},
		{ID: "private-file", ParentID: "private-root", TenantID: "file-owner", CreatedBy: "file-creator", Name: "private.pdf", Type: "pdf"},
	}
	for i := range files {
		if err := db.Create(&files[i]).Error; err != nil {
			t.Fatalf("create file %q: %v", files[i].ID, err)
		}
	}
	documents := []entity.Document{
		{ID: "team-document", KbID: "team-dataset", ParserID: "general", ParserConfig: entity.JSONMap{}, SourceType: "local", Type: "pdf", CreatedBy: "team-owner", Suffix: "pdf", Status: &valid},
		{ID: "private-document", KbID: "private-dataset", ParserID: "general", ParserConfig: entity.JSONMap{}, SourceType: "local", Type: "pdf", CreatedBy: "private-owner", Suffix: "pdf", Status: &valid},
	}
	for i := range documents {
		if err := db.Create(&documents[i]).Error; err != nil {
			t.Fatalf("create document %q: %v", documents[i].ID, err)
		}
	}
	for i, file := range files {
		fileID, documentID := file.ID, documents[i].ID
		mapping := entity.File2Document{ID: "mapping-" + file.ID, FileID: &fileID, DocumentID: &documentID}
		if err := db.Create(&mapping).Error; err != nil {
			t.Fatalf("create file-document mapping for %q: %v", file.ID, err)
		}
	}
	memberships := []entity.UserTenant{
		{ID: "team-owner-membership", UserID: "team-owner", TenantID: "team-tenant", Role: string(RoleOwner), Status: &valid},
		{ID: "team-member-membership", UserID: "team-member", TenantID: "team-tenant", Role: string(RoleNormal), Status: &valid},
		{ID: "team-admin-membership", UserID: "team-admin", TenantID: "team-tenant", Role: string(RoleAdmin), Status: &valid},
		{ID: "private-owner-membership", UserID: "private-owner", TenantID: "private-tenant", Role: string(RoleOwner), Status: &valid},
	}
	for i := range memberships {
		if err := db.Create(&memberships[i]).Error; err != nil {
			t.Fatalf("create membership %q: %v", memberships[i].ID, err)
		}
	}

	checker := NewDatabaseChecker(db)
	teamFile := ResourceRef{Kind: ResourceKindFile, ID: "team-file"}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "team-member"}, teamFile, OperationRead); err != nil {
		t.Errorf("team member CheckResource(file read) error = %v, want nil", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "team-admin"}, teamFile, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("team admin CheckResource(file read) error = %v, want permission denied (dataset access requires a normal member)", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "outsider"}, teamFile, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("outsider CheckResource(file read) error = %v, want permission denied", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "private-owner"}, ResourceRef{Kind: ResourceKindFile, ID: "private-file"}, OperationRead); err != nil {
		t.Errorf("private dataset owner CheckResource(file read) error = %v, want nil", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "team-member"}, ResourceRef{Kind: ResourceKindFile, ID: "private-file"}, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("team member CheckResource(private file read) error = %v, want permission denied", err)
	}

	resources, err := (&databaseSource{db: db}).GetResources(context.Background(), []ResourceRef{teamFile})
	if err != nil {
		t.Fatalf("load file authorization facts: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("loaded %d file resources, want 1", len(resources))
	}
	resource := resources[0]
	if resource.TenantID != "file-owner" || resource.OwnerUserID != "file-owner" || resource.CreatedBy != "file-creator" {
		t.Errorf("file authorization facts = %+v, want tenant/owner file-owner and creator file-creator", resource)
	}
	if resource.Visibility != VisibilityShared || len(resource.SharedWithTenantIDs) != 1 || resource.SharedWithTenantIDs[0] != "team-tenant" || len(resource.SharedWithUserIDs) != 1 || resource.SharedWithUserIDs[0] != "team-owner" {
		t.Errorf("file sharing facts = %+v, want team tenant and its owner", resource)
	}
}

func TestDatabaseCheckerScopesFilesAndFoldersToOwnedFolder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.File{}, &entity.File2Document{}, &entity.Document{}, &entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}
	files := []entity.File{
		{ID: "root", ParentID: "root", TenantID: "user-1", CreatedBy: "user-1", Name: "/", Type: "folder"},
		{ID: "child-file", ParentID: "root", TenantID: "user-1", CreatedBy: "user-1", Name: "notes.pdf", Type: "pdf"},
		{ID: "child-folder", ParentID: "root", TenantID: "user-1", CreatedBy: "user-1", Name: "notes", Type: "folder"},
		{ID: "nested-file", ParentID: "child-folder", TenantID: "user-1", CreatedBy: "user-1", Name: "nested.pdf", Type: "pdf"},
		{ID: "other-tenant-file", ParentID: "root", TenantID: "user-2", CreatedBy: "user-2", Name: "private.pdf", Type: "pdf"},
	}
	for i := range files {
		if err := db.Create(&files[i]).Error; err != nil {
			t.Fatalf("create file %q: %v", files[i].ID, err)
		}
	}
	checker := NewDatabaseChecker(db)
	parent := ResourceRef{Kind: ResourceKindFolder, ID: "root"}
	for _, test := range []struct {
		kind ResourceKind
		want string
	}{
		{kind: ResourceKindFile, want: "child-file"},
		{kind: ResourceKindFolder, want: "child-folder"},
	} {
		scope, err := checker.Scope(context.Background(), Subject{UserID: "user-1"}, ScopeQuery{
			Kind: test.kind, Parent: parent, Operation: OperationRead,
		})
		if err != nil {
			t.Fatalf("Scope(%s) error = %v", test.kind, err)
		}
		if scope.Mode != ScopeIDs || len(scope.ResourceIDs) != 1 || scope.ResourceIDs[0] != test.want {
			t.Errorf("Scope(%s) = %+v, want only %q", test.kind, scope, test.want)
		}
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "user-1"}, ResourceRef{Kind: ResourceKindFolder, ID: "child-file"}, OperationRead); !errors.Is(err, ErrResourceNotFound) {
		t.Errorf("folder ref to a file CheckResource error = %v, want resource not found", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "user-1"}, ResourceRef{Kind: ResourceKindFile, ID: "child-folder"}, OperationRead); !errors.Is(err, ErrResourceNotFound) {
		t.Errorf("file ref to a folder CheckResource error = %v, want resource not found", err)
	}
	otherParent := ResourceRef{Kind: ResourceKindFolder, ID: "root"}
	if scope, err := checker.Scope(context.Background(), Subject{UserID: "user-2"}, ScopeQuery{
		Kind: ResourceKindFile, Parent: otherParent, Operation: OperationRead,
	}); err != nil || scope.Mode != ScopeNone {
		t.Errorf("Scope(other user's folder) = (%+v, %v), want empty scope", scope, err)
	}
}

func TestDatabaseCheckerKeepsChatsPrivateToTheirOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Chat{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}
	valid := string(entity.StatusValid)
	invalid := string(entity.StatusInvalid)
	chats := []entity.Chat{
		{ID: "team-chat", TenantID: "team-tenant", LLMSetting: entity.JSONMap{}, PromptConfig: entity.JSONMap{}, KBIDs: entity.JSONSlice{}, DoRefer: "1", Status: &valid},
		{ID: "inactive-chat", TenantID: "team-tenant", LLMSetting: entity.JSONMap{}, PromptConfig: entity.JSONMap{}, KBIDs: entity.JSONSlice{}, DoRefer: "1", Status: &invalid},
		{ID: "other-chat", TenantID: "other-tenant", LLMSetting: entity.JSONMap{}, PromptConfig: entity.JSONMap{}, KBIDs: entity.JSONSlice{}, DoRefer: "1", Status: &valid},
	}
	for i := range chats {
		if err := db.Create(&chats[i]).Error; err != nil {
			t.Fatalf("create chat %q: %v", chats[i].ID, err)
		}
	}
	membership := entity.UserTenant{
		ID: "member-team", UserID: "member-1", TenantID: "team-tenant", Role: string(RoleNormal), Status: &valid,
	}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	inviteMembership := entity.UserTenant{
		ID: "invite-team", UserID: "invite-1", TenantID: "team-tenant", Role: string(RoleInvite), Status: &valid,
	}
	if err := db.Create(&inviteMembership).Error; err != nil {
		t.Fatalf("create invite membership: %v", err)
	}

	checker := NewDatabaseChecker(db)
	teamChat := ResourceRef{Kind: ResourceKindChat, ID: "team-chat"}
	member := Subject{UserID: membership.UserID}
	for _, operation := range []Operation{OperationRead, OperationCreate, OperationRun, OperationUse} {
		if err := checker.CheckResource(context.Background(), member, teamChat, operation); !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("team member CheckResource(chat, %s) error = %v, want permission denied", operation, err)
		}
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "team-tenant"}, teamChat, OperationUpdate); err != nil {
		t.Errorf("chat owner CheckResource(update) error = %v, want nil", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "invite-1"}, teamChat, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("invited user CheckResource(read) error = %v, want permission denied", err)
	}
	if err := checker.CheckResource(context.Background(), member, ResourceRef{Kind: ResourceKindChat, ID: "inactive-chat"}, OperationRead); !errors.Is(err, ErrResourceNotFound) {
		t.Errorf("inactive chat CheckResource(read) error = %v, want resource not found", err)
	}
	scope, err := checker.Scope(context.Background(), member, ScopeQuery{Kind: ResourceKindChat, Operation: OperationRead})
	if err != nil {
		t.Fatalf("Scope(chats) error = %v", err)
	}
	if scope.Mode != ScopeNone || len(scope.ResourceIDs) != 0 {
		t.Fatalf("team member Scope(chats) = %+v, want empty scope", scope)
	}

	ownerScope, err := checker.Scope(context.Background(), Subject{UserID: "team-tenant"}, ScopeQuery{Kind: ResourceKindChat, Operation: OperationRead})
	if err != nil {
		t.Fatalf("owner Scope(chats) error = %v", err)
	}
	if ownerScope.Mode != ScopeIDs || len(ownerScope.ResourceIDs) != 1 || ownerScope.ResourceIDs[0] != "team-chat" {
		t.Fatalf("owner Scope(chats) = %+v, want only team-chat", ownerScope)
	}
}

func TestDatabaseCheckerScopesChatSessionsAndRestrictsMutations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Chat{}, &entity.ChatSession{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}
	valid := string(entity.StatusValid)
	chat := entity.Chat{
		ID: "team-chat", TenantID: "team-tenant", LLMSetting: entity.JSONMap{},
		PromptConfig: entity.JSONMap{}, KBIDs: entity.JSONSlice{}, DoRefer: "1", Status: &valid,
	}
	if err := db.Create(&chat).Error; err != nil {
		t.Fatalf("create chat: %v", err)
	}
	creatorID := "session-creator"
	removedCreatorID := "removed-creator"
	sessions := []entity.ChatSession{
		{ID: "creator-session", DialogID: chat.ID, UserID: &creatorID},
		{ID: "removed-creator-session", DialogID: chat.ID, UserID: &removedCreatorID},
	}
	for i := range sessions {
		if err := db.Create(&sessions[i]).Error; err != nil {
			t.Fatalf("create chat session %q: %v", sessions[i].ID, err)
		}
	}
	memberships := []entity.UserTenant{
		{ID: "creator-membership", UserID: creatorID, TenantID: chat.TenantID, Role: string(RoleNormal), Status: &valid},
		{ID: "reader-membership", UserID: "team-reader", TenantID: chat.TenantID, Role: string(RoleNormal), Status: &valid},
	}
	for i := range memberships {
		if err := db.Create(&memberships[i]).Error; err != nil {
			t.Fatalf("create membership %q: %v", memberships[i].ID, err)
		}
	}
	checker := NewDatabaseChecker(db)
	creatorSession := ResourceRef{Kind: ResourceKindChatSession, ID: "creator-session"}
	reader := Subject{UserID: "team-reader"}
	if err := checker.CheckResource(context.Background(), reader, creatorSession, OperationRead); err != nil {
		t.Errorf("tenant member CheckResource(session read) error = %v, want nil", err)
	}
	if err := checker.CheckResource(context.Background(), reader, creatorSession, OperationUpdate); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("tenant member CheckResource(session update) error = %v, want permission denied", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: creatorID}, creatorSession, OperationUpdate); err != nil {
		t.Errorf("session creator CheckResource(update) error = %v, want nil", err)
	}
	creatorAccess, err := checker.ResolveAccess(context.Background(), Subject{UserID: creatorID}, creatorSession)
	if err != nil {
		t.Fatalf("session creator ResolveAccess() error = %v", err)
	}
	if creatorAccess.Source != AccessSourceCreator {
		t.Errorf("session creator access source = %q, want %q", creatorAccess.Source, AccessSourceCreator)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: chat.TenantID}, creatorSession, OperationDelete); err != nil {
		t.Errorf("chat owner CheckResource(session delete) error = %v, want nil", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: removedCreatorID}, ResourceRef{Kind: ResourceKindChatSession, ID: "removed-creator-session"}, OperationUpdate); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("creator without active membership CheckResource(update) error = %v, want permission denied", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "outsider"}, creatorSession, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("outsider CheckResource(session read) error = %v, want permission denied", err)
	}

	scope, err := checker.Scope(context.Background(), reader, ScopeQuery{
		Kind: ResourceKindChatSession, Parent: ResourceRef{Kind: ResourceKindChat, ID: chat.ID}, Operation: OperationRead,
	})
	if err != nil {
		t.Fatalf("Scope(chat sessions) error = %v", err)
	}
	if scope.Mode != ScopeIDs || len(scope.ResourceIDs) != 2 {
		t.Fatalf("Scope(chat sessions) = %+v, want both sessions visible read-only", scope)
	}
}

func TestDatabaseCheckerKeepsSearchAppsPrivateToTheirCreator(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Search{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}
	valid := string(entity.StatusValid)
	invalid := string(entity.StatusInvalid)
	searchApps := []entity.Search{
		{ID: "team-search", TenantID: "team-tenant", CreatedBy: "team-owner", Name: "Team search", SearchConfig: entity.JSONMap{}, Status: &valid},
		{ID: "member-search", TenantID: "member-1", CreatedBy: "member-1", Name: "Personal search", SearchConfig: entity.JSONMap{}, Status: &valid},
		{ID: "other-search", TenantID: "other-tenant", CreatedBy: "other-owner", Name: "Other search", SearchConfig: entity.JSONMap{}, Status: &valid},
		{ID: "inactive-search", TenantID: "team-tenant", CreatedBy: "team-owner", Name: "Inactive search", SearchConfig: entity.JSONMap{}, Status: &invalid},
	}
	for i := range searchApps {
		if err := db.Create(&searchApps[i]).Error; err != nil {
			t.Fatalf("create search app %q: %v", searchApps[i].ID, err)
		}
	}
	memberships := []entity.UserTenant{
		{ID: "member-team", UserID: "member-1", TenantID: "team-tenant", Role: string(RoleNormal), Status: &valid},
		{ID: "owner-team", UserID: "team-owner", TenantID: "team-tenant", Role: string(RoleOwner), Status: &valid},
		{ID: "invite-team", UserID: "invite-1", TenantID: "team-tenant", Role: string(RoleInvite), Status: &valid},
	}
	for i := range memberships {
		if err := db.Create(&memberships[i]).Error; err != nil {
			t.Fatalf("create membership %q: %v", memberships[i].ID, err)
		}
	}

	checker := NewDatabaseChecker(db)
	teamSearch := ResourceRef{Kind: ResourceKindSearchApp, ID: "team-search"}
	member := Subject{UserID: "member-1"}
	if err := checker.CheckResource(context.Background(), member, teamSearch, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("tenant member CheckResource(search read) error = %v, want permission denied", err)
	}
	for _, operation := range []Operation{OperationUpdate, OperationDelete, OperationRun, OperationUse} {
		if err := checker.CheckResource(context.Background(), member, teamSearch, operation); !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("tenant member CheckResource(search %s) error = %v, want permission denied", operation, err)
		}
	}
	for _, operation := range []Operation{OperationRead, OperationUpdate, OperationDelete, OperationRun, OperationUse} {
		if err := checker.CheckResource(context.Background(), Subject{UserID: "team-owner"}, teamSearch, operation); err != nil {
			t.Errorf("creator CheckResource(search %s) error = %v, want nil", operation, err)
		}
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "invite-1"}, teamSearch, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("invited user CheckResource(search read) error = %v, want permission denied", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "outsider"}, teamSearch, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("outsider CheckResource(search read) error = %v, want permission denied", err)
	}
	if err := checker.CheckResource(context.Background(), member, ResourceRef{Kind: ResourceKindSearchApp, ID: "inactive-search"}, OperationRead); !errors.Is(err, ErrResourceNotFound) {
		t.Errorf("inactive search CheckResource(read) error = %v, want resource not found", err)
	}

	resources, err := (&databaseSource{db: db}).GetResources(context.Background(), []ResourceRef{teamSearch})
	if err != nil {
		t.Fatalf("load search authorization facts: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("loaded %d search resources, want 1", len(resources))
	}
	if got := resources[0]; got.TenantID != "team-tenant" || got.CreatedBy != "team-owner" || got.OwnerUserID != "team-owner" {
		t.Errorf("search authorization facts = %+v, want tenant team-tenant and creator/owner team-owner", got)
	}

	readScope, err := checker.Scope(context.Background(), member, ScopeQuery{Kind: ResourceKindSearchApp, Operation: OperationRead})
	if err != nil {
		t.Fatalf("Scope(search read) error = %v", err)
	}
	if readScope.Mode != ScopeIDs || len(readScope.ResourceIDs) != 1 || readScope.ResourceIDs[0] != "member-search" {
		t.Fatalf("Scope(search read) = %+v, want only member-search", readScope)
	}
	runScope, err := checker.Scope(context.Background(), member, ScopeQuery{Kind: ResourceKindSearchApp, Operation: OperationRun})
	if err != nil {
		t.Fatalf("Scope(search run) error = %v", err)
	}
	if runScope.Mode != ScopeIDs || len(runScope.ResourceIDs) != 1 || runScope.ResourceIDs[0] != "member-search" {
		t.Fatalf("Scope(search run) = %+v, want only member-search", runScope)
	}
}

func TestDatabaseCheckerAuthorizesConnectorsAndSyncTasks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Connector{}, &entity.Connector2Kb{}, &entity.SyncLogs{}, &entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}

	valid := string(entity.StatusValid)
	connectors := []entity.Connector{
		{ID: "personal-connector", TenantID: "team-member", Name: "Personal", Source: "rest", InputType: "poll", Config: entity.JSONMap{}, Status: "5"},
		{ID: "team-connector", TenantID: "team-tenant", Name: "Team", Source: "rest", InputType: "poll", Config: entity.JSONMap{}, Status: "5"},
		{ID: "other-connector", TenantID: "other-tenant", Name: "Other", Source: "rest", InputType: "poll", Config: entity.JSONMap{}, Status: "5"},
	}
	for i := range connectors {
		if err := db.Create(&connectors[i]).Error; err != nil {
			t.Fatalf("create connector %q: %v", connectors[i].ID, err)
		}
	}
	links := []entity.Connector2Kb{
		{ID: "link-1", ConnectorID: "team-connector", KbID: "dataset-1", AutoParse: "1"},
	}
	for i := range links {
		if err := db.Create(&links[i]).Error; err != nil {
			t.Fatalf("create connector-dataset link: %v", err)
		}
	}
	dataset := entity.Knowledgebase{
		ID: "dataset-1", TenantID: "team-tenant", CreatedBy: "team-owner", Name: "Team dataset",
		EmbdID: "embedding", Permission: string(entity.TenantPermissionTeam), Status: &valid,
	}
	if err := db.Create(&dataset).Error; err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	tasks := []entity.SyncLogs{
		{ID: "linked-task", ConnectorID: "team-connector", TaskType: "sync", Status: string(entity.TaskStatusFail), ErrorMsg: "failed", KbID: "dataset-1"},
		{ID: "completed-task", ConnectorID: "team-connector", TaskType: "sync", Status: string(entity.TaskStatusDone), ErrorMsg: "", KbID: "dataset-1"},
		{ID: "prune-task", ConnectorID: "team-connector", TaskType: "prune", Status: "3", ErrorMsg: "", KbID: "dataset-1"},
		{ID: "unlinked-task", ConnectorID: "team-connector", TaskType: "sync", Status: "3", ErrorMsg: "", KbID: "dataset-unlinked"},
	}
	for i := range tasks {
		if err := db.Create(&tasks[i]).Error; err != nil {
			t.Fatalf("create sync task %q: %v", tasks[i].ID, err)
		}
	}
	memberships := []entity.UserTenant{
		{ID: "member-link", UserID: "team-member", TenantID: "team-tenant", Role: string(RoleNormal), Status: &valid},
		{ID: "owner-link", UserID: "team-owner", TenantID: "team-tenant", Role: string(RoleOwner), Status: &valid},
		{ID: "invite-link", UserID: "invited-user", TenantID: "team-tenant", Role: string(RoleInvite), Status: &valid},
	}
	for i := range memberships {
		if err := db.Create(&memberships[i]).Error; err != nil {
			t.Fatalf("create membership %q: %v", memberships[i].ID, err)
		}
	}

	checker := NewDatabaseChecker(db)
	member := Subject{UserID: "team-member"}
	teamConnector := ResourceRef{Kind: ResourceKindConnector, ID: "team-connector"}
	for _, operation := range []Operation{OperationRun, OperationUse} {
		if err := checker.CheckResource(context.Background(), member, teamConnector, operation); err != nil {
			t.Errorf("team member CheckResource(connector %s) error = %v, want nil", operation, err)
		}
	}
	for _, operation := range []Operation{OperationRead, OperationUpdate, OperationDelete} {
		if err := checker.CheckResource(context.Background(), member, teamConnector, operation); !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("team member CheckResource(connector %s) error = %v, want permission denied", operation, err)
		}
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "invited-user"}, teamConnector, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("invited user CheckResource(connector read) error = %v, want permission denied", err)
	}
	if err := checker.CheckResource(context.Background(), Subject{UserID: "outsider"}, teamConnector, OperationRead); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("outsider CheckResource(connector read) error = %v, want permission denied", err)
	}
	if err := checker.CheckResource(context.Background(), member, ResourceRef{Kind: ResourceKindConnector, ID: "personal-connector"}, OperationRead); err != nil {
		t.Errorf("connector owner CheckResource(read) error = %v, want nil", err)
	}

	linkedTask := ResourceRef{Kind: ResourceKindSyncTask, ID: "linked-task"}
	for _, operation := range []Operation{OperationRead, OperationRun} {
		if err := checker.CheckResource(context.Background(), member, linkedTask, operation); err != nil {
			t.Errorf("team member CheckResource(sync task %s) error = %v, want nil", operation, err)
		}
	}
	if err := checker.CheckResource(context.Background(), member, linkedTask, OperationDelete); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("team member CheckResource(sync task delete) error = %v, want permission denied", err)
	}
	if err := checker.CheckResource(context.Background(), member, ResourceRef{Kind: ResourceKindSyncTask, ID: "unlinked-task"}, OperationRead); !errors.Is(err, ErrResourceNotFound) {
		t.Errorf("unlinked sync task CheckResource(read) error = %v, want resource not found", err)
	}
	if err := checker.CheckResource(context.Background(), member, ResourceRef{Kind: ResourceKindSyncTask, ID: "prune-task"}, OperationRun); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("team member CheckResource(prune task run) error = %v, want permission denied", err)
	}
	if err := checker.CheckResource(context.Background(), member, ResourceRef{Kind: ResourceKindSyncTask, ID: "completed-task"}, OperationRun); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("team member CheckResource(completed task run) error = %v, want permission denied", err)
	}

	connectorScope, err := checker.Scope(context.Background(), member, ScopeQuery{Kind: ResourceKindConnector, Operation: OperationRead})
	if err != nil {
		t.Fatalf("Scope(connectors) error = %v", err)
	}
	if connectorScope.Mode != ScopeIDs || len(connectorScope.ResourceIDs) != 1 || connectorScope.ResourceIDs[0] != "personal-connector" {
		t.Fatalf("Scope(connectors) = %+v, want personal connector only", connectorScope)
	}
	taskScope, err := checker.Scope(context.Background(), member, ScopeQuery{
		Kind: ResourceKindSyncTask, Parent: teamConnector, Operation: OperationRead,
	})
	if err != nil {
		t.Fatalf("Scope(sync tasks) error = %v", err)
	}
	if taskScope.Mode != ScopeIDs || len(taskScope.ResourceIDs) != 3 {
		t.Fatalf("Scope(sync tasks) = %+v, want linked sync and prune task logs", taskScope)
	}
	if _, err := checker.Scope(context.Background(), member, ScopeQuery{Kind: ResourceKindSyncTask, Operation: OperationRead}); !errors.Is(err, ErrInvalidPermission) {
		t.Errorf("Scope(sync tasks without connector) error = %v, want invalid permission", err)
	}
}

func TestDatabaseCheckerAuthorizesTenantModelsAndSeparatesInstanceUseFromRead(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}

	providers := []entity.TenantModelProvider{
		{ID: "team-provider", UserID: "team-owner", ProviderName: "openai", TenantID: "team-tenant"},
		{ID: "other-provider", UserID: "other-owner", ProviderName: "openai", TenantID: "other-tenant"},
	}
	for i := range providers {
		if err := db.Create(&providers[i]).Error; err != nil {
			t.Fatalf("create provider %q: %v", providers[i].ID, err)
		}
	}
	instances := []entity.TenantModelInstance{
		{ID: "team-instance", InstanceName: "team-instance", ProviderID: "team-provider", APIKey: "secret-team-key", Status: "active"},
		{ID: "other-instance", InstanceName: "other-instance", ProviderID: "other-provider", APIKey: "secret-other-key", Status: "active"},
	}
	for i := range instances {
		if err := db.Create(&instances[i]).Error; err != nil {
			t.Fatalf("create instance %q: %v", instances[i].ID, err)
		}
	}
	models := []entity.TenantModel{
		{ID: "team-model", ModelName: "chat-model", ProviderID: "team-provider", InstanceID: "team-instance", ModelType: int(entity.ModelTypeChat), Status: "active"},
		{ID: "malformed-model", ModelName: "cross-provider-model", ProviderID: "team-provider", InstanceID: "other-instance", ModelType: int(entity.ModelTypeChat), Status: "active"},
	}
	for i := range models {
		if err := db.Create(&models[i]).Error; err != nil {
			t.Fatalf("create model %q: %v", models[i].ID, err)
		}
	}
	valid := string(entity.StatusValid)
	memberships := []entity.UserTenant{
		{ID: "team-owner-membership", UserID: "team-owner", TenantID: "team-tenant", Role: string(RoleOwner), Status: &valid},
		{ID: "team-member-membership", UserID: "team-member", TenantID: "team-tenant", Role: string(RoleNormal), Status: &valid},
		{ID: "team-invite-membership", UserID: "team-invite", TenantID: "team-tenant", Role: string(RoleInvite), Status: &valid},
		{ID: "other-owner-membership", UserID: "other-owner", TenantID: "other-tenant", Role: string(RoleOwner), Status: &valid},
	}
	for i := range memberships {
		if err := db.Create(&memberships[i]).Error; err != nil {
			t.Fatalf("create membership %q: %v", memberships[i].ID, err)
		}
	}

	checker := NewDatabaseChecker(db)
	member := Subject{UserID: "team-member"}
	providerRef := ResourceRef{Kind: ResourceKindModelProvider, ID: "team-provider"}
	instanceRef := ResourceRef{Kind: ResourceKindModelInstance, ID: "team-instance"}
	modelRef := ResourceRef{Kind: ResourceKindModel, ID: "team-model"}
	for _, test := range []struct {
		name      string
		subject   Subject
		ref       ResourceRef
		operation Operation
		wantErr   error
	}{
		{name: "member reads provider", subject: member, ref: providerRef, operation: OperationRead},
		{name: "member cannot manage provider", subject: member, ref: providerRef, operation: OperationUpdate, wantErr: ErrPermissionDenied},
		{name: "member uses instance without reading credentials", subject: member, ref: instanceRef, operation: OperationUse},
		{name: "member cannot read instance credentials", subject: member, ref: instanceRef, operation: OperationRead, wantErr: ErrPermissionDenied},
		{name: "member reads model", subject: member, ref: modelRef, operation: OperationRead},
		{name: "member uses model", subject: member, ref: modelRef, operation: OperationUse},
		{name: "member cannot manage model", subject: member, ref: modelRef, operation: OperationUpdate, wantErr: ErrPermissionDenied},
		{name: "outsider cannot use model", subject: Subject{UserID: "outsider"}, ref: modelRef, operation: OperationUse, wantErr: ErrPermissionDenied},
		{name: "invite cannot use model", subject: Subject{UserID: "team-invite"}, ref: modelRef, operation: OperationUse, wantErr: ErrPermissionDenied},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := checker.CheckResource(context.Background(), test.subject, test.ref, test.operation)
			if test.wantErr == nil && err != nil {
				t.Fatalf("CheckResource(%s) error = %v, want nil", test.name, err)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("CheckResource(%s) error = %v, want %v", test.name, err, test.wantErr)
			}
		})
	}

	owner := Subject{UserID: "team-owner"}
	if err := checker.CheckResource(context.Background(), owner, instanceRef, OperationRead); err != nil {
		t.Errorf("tenant owner cannot read instance credentials: %v", err)
	}
	if err := checker.CheckResource(context.Background(), member, ResourceRef{Kind: ResourceKindModel, ID: "malformed-model"}, OperationRead); !errors.Is(err, ErrResourceNotFound) {
		t.Errorf("cross-provider model CheckResource error = %v, want resource not found", err)
	}

	for _, query := range []struct {
		name      string
		kind      ResourceKind
		parent    ResourceRef
		operation Operation
		want      string
	}{
		{name: "providers", kind: ResourceKindModelProvider, want: "team-provider"},
		{name: "usable instances under provider", kind: ResourceKindModelInstance, parent: providerRef, operation: OperationUse, want: "team-instance"},
		{name: "models under instance", kind: ResourceKindModel, parent: instanceRef, want: "team-model"},
		{name: "models across tenant", kind: ResourceKindModel, want: "team-model"},
	} {
		t.Run("scope "+query.name, func(t *testing.T) {
			operation := query.operation
			if operation == "" {
				operation = OperationRead
			}
			scope, err := checker.Scope(context.Background(), member, ScopeQuery{
				Kind: query.kind, Parent: query.parent, Operation: operation,
			})
			if err != nil {
				t.Fatalf("Scope(%s) error = %v", query.name, err)
			}
			if scope.Mode != ScopeIDs || len(scope.ResourceIDs) != 1 || scope.ResourceIDs[0] != query.want {
				t.Fatalf("Scope(%s) = %+v, want only %q", query.name, scope, query.want)
			}
		})
	}
	if _, err := checker.Scope(context.Background(), member, ScopeQuery{
		Kind:      ResourceKindModelInstance,
		Parent:    ResourceRef{Kind: ResourceKindModel, ID: "team-model"},
		Operation: OperationRead,
	}); !errors.Is(err, ErrInvalidPermission) {
		t.Errorf("Scope(instances with wrong parent) error = %v, want invalid permission", err)
	}
}

func TestDatabaseCheckerAuthorizesMCPServersAndSkillSpaces(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.MCPServer{},
		&entity.SkillSpace{},
		&entity.File{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}

	valid := string(entity.StatusValid)
	memberships := []entity.UserTenant{
		{ID: "team-owner-membership", UserID: "team-owner", TenantID: "team-tenant", Role: string(RoleOwner), Status: &valid},
		{ID: "team-member-membership", UserID: "team-member", TenantID: "team-tenant", Role: string(RoleNormal), Status: &valid},
		{ID: "team-invite-membership", UserID: "team-invite", TenantID: "team-tenant", Role: string(RoleInvite), Status: &valid},
		{ID: "other-owner-membership", UserID: "other-owner", TenantID: "other-tenant", Role: string(RoleOwner), Status: &valid},
	}
	for i := range memberships {
		if err := db.Create(&memberships[i]).Error; err != nil {
			t.Fatalf("create membership %q: %v", memberships[i].ID, err)
		}
	}

	servers := []entity.MCPServer{
		{ID: "team-mcp", TenantID: "team-tenant", Name: "Team MCP", URL: "https://mcp.example", ServerType: "streamable-http", Headers: entity.JSONMap{"Authorization": "secret"}},
		{ID: "other-mcp", TenantID: "other-tenant", Name: "Other MCP", URL: "https://other.example", ServerType: "streamable-http"},
	}
	for i := range servers {
		if err := db.Create(&servers[i]).Error; err != nil {
			t.Fatalf("create MCP server %q: %v", servers[i].ID, err)
		}
	}

	folders := []entity.File{
		{ID: "active-folder", ParentID: "root", TenantID: "team-tenant", CreatedBy: "team-owner", Name: "active", Type: "folder", SourceType: "skill_space"},
		{ID: "deleting-folder", ParentID: "root", TenantID: "team-tenant", CreatedBy: "team-owner", Name: "deleting", Type: "folder", SourceType: "skill_space"},
		{ID: "deleted-folder", ParentID: "root", TenantID: "team-tenant", CreatedBy: "team-owner", Name: "deleted", Type: "folder", SourceType: "skill_space"},
		{ID: "wrong-type-file", ParentID: "root", TenantID: "team-tenant", CreatedBy: "team-owner", Name: "not-a-folder", Type: "file", SourceType: "skill_space"},
		{ID: "other-tenant-folder", ParentID: "root", TenantID: "other-tenant", CreatedBy: "other-owner", Name: "other", Type: "folder", SourceType: "skill_space"},
	}
	for i := range folders {
		if err := db.Create(&folders[i]).Error; err != nil {
			t.Fatalf("create folder %q: %v", folders[i].ID, err)
		}
	}
	spaces := []entity.SkillSpace{
		{ID: "active-space", TenantID: "team-tenant", Name: "Active", FolderID: "active-folder", Status: entity.SpaceStatusActive},
		{ID: "deleting-space", TenantID: "team-tenant", Name: "Deleting", FolderID: "deleting-folder", Status: entity.SpaceStatusDeleting},
		{ID: "deleted-space", TenantID: "team-tenant", Name: "Deleted", FolderID: "deleted-folder", Status: entity.SpaceStatusDeleted},
		{ID: "wrong-folder-type-space", TenantID: "team-tenant", Name: "Wrong type", FolderID: "wrong-type-file", Status: entity.SpaceStatusActive},
		{ID: "cross-tenant-folder-space", TenantID: "team-tenant", Name: "Cross tenant", FolderID: "other-tenant-folder", Status: entity.SpaceStatusActive},
		{ID: "missing-folder-space", TenantID: "team-tenant", Name: "Missing folder", FolderID: "missing-folder", Status: entity.SpaceStatusActive},
	}
	for i := range spaces {
		if err := db.Create(&spaces[i]).Error; err != nil {
			t.Fatalf("create skill space %q: %v", spaces[i].ID, err)
		}
	}

	checker := NewDatabaseChecker(db)
	member := Subject{UserID: "team-member"}
	owner := Subject{UserID: "team-owner"}
	outsider := Subject{UserID: "outsider"}
	invite := Subject{UserID: "team-invite"}
	mcpRef := ResourceRef{Kind: ResourceKindMCPServer, ID: "team-mcp"}
	spaceRef := ResourceRef{Kind: ResourceKindSkillSpace, ID: "active-space"}
	for _, test := range []struct {
		name      string
		subject   Subject
		ref       ResourceRef
		operation Operation
		wantErr   error
	}{
		{name: "member cannot directly read MCP config", subject: member, ref: mcpRef, operation: OperationRead, wantErr: ErrPermissionDenied},
		{name: "member uses MCP server", subject: member, ref: mcpRef, operation: OperationUse},
		{name: "member cannot manage MCP server", subject: member, ref: mcpRef, operation: OperationUpdate, wantErr: ErrPermissionDenied},
		{name: "owner manages MCP server", subject: owner, ref: mcpRef, operation: OperationUpdate},
		{name: "outsider cannot use MCP server", subject: outsider, ref: mcpRef, operation: OperationUse, wantErr: ErrPermissionDenied},
		{name: "invite cannot use MCP server", subject: invite, ref: mcpRef, operation: OperationUse, wantErr: ErrPermissionDenied},
		{name: "member reads skill space", subject: member, ref: spaceRef, operation: OperationRead},
		{name: "member uses skill space", subject: member, ref: spaceRef, operation: OperationUse},
		{name: "member cannot manage skill space", subject: member, ref: spaceRef, operation: OperationUpdate, wantErr: ErrPermissionDenied},
		{name: "owner manages skill space", subject: owner, ref: spaceRef, operation: OperationDelete},
		{name: "outsider cannot use skill space", subject: outsider, ref: spaceRef, operation: OperationUse, wantErr: ErrPermissionDenied},
		{name: "invite cannot use skill space", subject: invite, ref: spaceRef, operation: OperationUse, wantErr: ErrPermissionDenied},
		{name: "deleting skill space remains addressable", subject: member, ref: ResourceRef{Kind: ResourceKindSkillSpace, ID: "deleting-space"}, operation: OperationRead},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := checker.CheckResource(context.Background(), test.subject, test.ref, test.operation)
			if test.wantErr == nil && err != nil {
				t.Fatalf("CheckResource error = %v, want nil", err)
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("CheckResource error = %v, want %v", err, test.wantErr)
			}
		})
	}

	for _, id := range []string{"deleted-space", "wrong-folder-type-space", "cross-tenant-folder-space", "missing-folder-space"} {
		ref := ResourceRef{Kind: ResourceKindSkillSpace, ID: id}
		if err := checker.CheckResource(context.Background(), member, ref, OperationRead); !errors.Is(err, ErrResourceNotFound) {
			t.Errorf("CheckResource(%q) error = %v, want resource not found", id, err)
		}
	}

	resources, err := (&databaseSource{db: db}).GetResources(context.Background(), []ResourceRef{mcpRef, spaceRef})
	if err != nil {
		t.Fatalf("load integration authorization facts: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("loaded %d integration resources, want 2", len(resources))
	}
	for _, resource := range resources {
		if resource.TenantID != "team-tenant" || resource.OwnerUserID != "team-owner" {
			t.Errorf("resource %v facts = tenant %q, owner %q; want team tenant/owner", resource.Ref, resource.TenantID, resource.OwnerUserID)
		}
	}

	for _, kind := range []ResourceKind{ResourceKindMCPServer, ResourceKindSkillSpace} {
		t.Run("scope "+string(kind), func(t *testing.T) {
			scope, err := checker.Scope(context.Background(), member, ScopeQuery{Kind: kind, Operation: OperationRead})
			if err != nil {
				t.Fatalf("Scope(%s) error = %v", kind, err)
			}
			want := []string{}
			if kind == ResourceKindSkillSpace {
				want = []string{"active-space"}
			}
			if len(want) == 0 {
				if scope.Mode != ScopeNone || len(scope.ResourceIDs) != 0 {
					t.Fatalf("Scope(%s) = %+v, want no visible resources", kind, scope)
				}
				return
			}
			if scope.Mode != ScopeIDs || len(scope.ResourceIDs) != len(want) || scope.ResourceIDs[0] != want[0] {
				t.Fatalf("Scope(%s) = %+v, want IDs %v", kind, scope, want)
			}
		})
		if _, err := checker.Scope(context.Background(), member, ScopeQuery{
			Kind: kind, Parent: ResourceRef{Kind: ResourceKindCanvas, ID: "unexpected-parent"}, Operation: OperationRead,
		}); !errors.Is(err, ErrInvalidPermission) {
			t.Errorf("Scope(%s with parent) error = %v, want invalid permission", kind, err)
		}
	}
}
