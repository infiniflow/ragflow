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
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/permission/testsupport"
	"ragflow/internal/storage"

	"gorm.io/gorm"
)

// TestAsyncDeleteSpaceOutlivesTheRequestContext pins the cleanup goroutine to a
// context that survives the request. Gin cancels c.Request.Context() as soon as
// the handler returns, so inheriting it left the space at status "2" forever:
// the delete endpoint answers with the idempotent "already deleting" success on
// every retry and space creation refuses the same name.
func TestAsyncDeleteSpaceOutlivesTheRequestContext(t *testing.T) {
	db := setupServiceTestDB(t)
	models := append([]interface{}{&entity.SkillSpace{}}, permissionChangeLogTestModels()...)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("failed to migrate skill space: %v", err)
	}
	pushServiceDB(t, db)

	space := &entity.SkillSpace{
		ID:       "space-1",
		TenantID: "tenant-1",
		Name:     "skills",
		FolderID: "folder-1",
		Status:   entity.SpaceStatusDeleting,
	}
	if err := db.Create(space).Error; err != nil {
		t.Fatalf("create space: %v", err)
	}

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()

	NewSkillSpaceService(nil).asyncDeleteSpace(requestCtx, space.ID, space.FolderID, space.TenantID, nil)

	stored, err := dao.NewSkillSpaceDAO().GetByIDAnyStatus(context.Background(), db, space.ID)
	if err != nil {
		t.Fatalf("reload space: %v", err)
	}
	if stored.Status != entity.SpaceStatusDeleted {
		t.Fatalf("space status = %q, want %q: cleanup still inherits the canceled request context", stored.Status, entity.SpaceStatusDeleted)
	}
}

// TestAsyncDeleteSpaceDeletesTheFolderAsItsCreator pins the acting user of the
// folder cleanup to the folder creator. DeleteFiles authorizes every entry in
// the tree against the creator's grant, so a tenant member can create the space
// while the tenant owner deletes it; passing the tenant ID left the folder rows
// and storage objects behind even though the space was marked deleted.
func TestAsyncDeleteSpaceDeletesTheFolderAsItsCreator(t *testing.T) {
	db := setupServiceTestDB(t)
	models := append([]interface{}{&entity.SkillSpace{}}, permissionChangeLogTestModels()...)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("failed to migrate skill space: %v", err)
	}
	pushServiceDB(t, db)
	if err := testsupport.PrepareEnterprisePermissionFixture(db, "tenant-1", "owner-1", "member-1"); err != nil {
		t.Fatalf("prepare skill space permission fixture: %v", err)
	}

	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(previous) })

	for _, file := range []*entity.File{
		{ID: "skills-root", ParentID: "skills-root", TenantID: "tenant-1", CreatedBy: "owner-1", Name: "/", Type: "folder"},
		{ID: "folder-1", ParentID: "skills-root", TenantID: "tenant-1", CreatedBy: "member-1", Name: "skills", Type: "folder"},
		{ID: "file-1", ParentID: "folder-1", TenantID: "tenant-1", CreatedBy: "member-1", Name: "skill.md", Type: "doc"},
	} {
		if err := db.Create(file).Error; err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}

	space := &entity.SkillSpace{
		ID:       "space-1",
		TenantID: "tenant-1",
		Name:     "skills",
		FolderID: "folder-1",
		Status:   entity.SpaceStatusDeleting,
	}
	if err := db.Create(space).Error; err != nil {
		t.Fatalf("create space: %v", err)
	}

	NewSkillSpaceService(nil).asyncDeleteSpace(context.Background(), space.ID, space.FolderID, space.TenantID, nil)

	fileDAO := dao.NewFileDAO()
	for _, id := range []string{"folder-1", "file-1"} {
		if _, err := fileDAO.GetByID(context.Background(), db, id); err == nil {
			t.Fatalf("file %q survived the space cleanup: DeleteFiles ran as a non-creator", id)
		}
	}
	stored, err := dao.NewSkillSpaceDAO().GetByIDAnyStatus(context.Background(), db, space.ID)
	if err != nil {
		t.Fatalf("reload space: %v", err)
	}
	if stored.Status != entity.SpaceStatusDeleted {
		t.Fatalf("space status = %q, want %q", stored.Status, entity.SpaceStatusDeleted)
	}
}

// failFolderLookupsOnce fails the first budget reads of the file table with a
// non-not-found error, standing in for a transient database failure that leaves
// the folder row intact.
func failFolderLookupsOnce(t *testing.T, db *gorm.DB, budget int32) {
	t.Helper()
	var calls int32
	err := db.Callback().Query().Before("gorm:query").Register("test:fail_folder_lookup", func(tx *gorm.DB) {
		if tx.Statement.Table != "file" {
			return
		}
		if atomic.AddInt32(&calls, 1) <= budget {
			tx.AddError(errors.New("transient lookup failure"))
		}
	})
	if err != nil {
		t.Fatalf("register lookup failure hook: %v", err)
	}
}

// TestAsyncDeleteSpaceRetriesTheFolderLookup pins the creator lookup to a retry.
// Falling back to the tenant ID on a transient read error made DeleteFiles
// authorize against a principal that does not own a member-created folder, so
// the retry is what keeps the space and its folder in agreement.
func TestAsyncDeleteSpaceRetriesTheFolderLookup(t *testing.T) {
	db := setupServiceTestDB(t)
	models := append([]interface{}{&entity.SkillSpace{}}, permissionChangeLogTestModels()...)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("failed to migrate skill space: %v", err)
	}
	pushServiceDB(t, db)
	if err := testsupport.PrepareEnterprisePermissionFixture(db, "tenant-1", "owner-1", "member-1"); err != nil {
		t.Fatalf("prepare skill space permission fixture: %v", err)
	}

	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(previous) })

	for _, file := range []*entity.File{
		{ID: "skills-root", ParentID: "skills-root", TenantID: "tenant-1", CreatedBy: "owner-1", Name: "/", Type: "folder"},
		{ID: "folder-1", ParentID: "skills-root", TenantID: "tenant-1", CreatedBy: "member-1", Name: "skills", Type: "folder"},
		{ID: "file-1", ParentID: "folder-1", TenantID: "tenant-1", CreatedBy: "member-1", Name: "skill.md", Type: "doc"},
	} {
		if err := db.Create(file).Error; err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}

	space := &entity.SkillSpace{
		ID:       "space-1",
		TenantID: "tenant-1",
		Name:     "skills",
		FolderID: "folder-1",
		Status:   entity.SpaceStatusDeleting,
	}
	if err := db.Create(space).Error; err != nil {
		t.Fatalf("create space: %v", err)
	}

	failFolderLookupsOnce(t, db, 1)

	NewSkillSpaceService(nil).asyncDeleteSpace(context.Background(), space.ID, space.FolderID, space.TenantID, nil)

	fileDAO := dao.NewFileDAO()
	for _, id := range []string{"folder-1", "file-1"} {
		if _, err := fileDAO.GetByID(context.Background(), db, id); err == nil {
			t.Fatalf("file %q survived the space cleanup: the lookup retry did not run", id)
		}
	}
	stored, err := dao.NewSkillSpaceDAO().GetByIDAnyStatus(context.Background(), db, space.ID)
	if err != nil {
		t.Fatalf("reload space: %v", err)
	}
	if stored.Status != entity.SpaceStatusDeleted {
		t.Fatalf("space status = %q, want %q", stored.Status, entity.SpaceStatusDeleted)
	}
}

// TestAsyncDeleteSpaceKeepsTheDeletingStatusWhenTheFolderCreatorCannotBeRead
// pins the outcome of an unreadable folder creator: the space must stay at "2"
// with its folder intact rather than be reported deleted while the rows and
// storage objects survive.
func TestAsyncDeleteSpaceKeepsTheDeletingStatusWhenTheFolderCreatorCannotBeRead(t *testing.T) {
	db := setupServiceTestDB(t)
	models := append([]interface{}{&entity.SkillSpace{}}, permissionChangeLogTestModels()...)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("failed to migrate skill space: %v", err)
	}
	pushServiceDB(t, db)

	for _, file := range []*entity.File{
		{ID: "skills-root", ParentID: "skills-root", TenantID: "tenant-1", CreatedBy: "owner-1", Name: "/", Type: "folder"},
		{ID: "folder-1", ParentID: "skills-root", TenantID: "tenant-1", CreatedBy: "member-1", Name: "skills", Type: "folder"},
	} {
		if err := db.Create(file).Error; err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}

	space := &entity.SkillSpace{
		ID:       "space-1",
		TenantID: "tenant-1",
		Name:     "skills",
		FolderID: "folder-1",
		Status:   entity.SpaceStatusDeleting,
	}
	if err := db.Create(space).Error; err != nil {
		t.Fatalf("create space: %v", err)
	}

	// Both lookup attempts fail with a non-not-found error.
	failFolderLookupsOnce(t, db, 2)

	NewSkillSpaceService(nil).asyncDeleteSpace(context.Background(), space.ID, space.FolderID, space.TenantID, nil)

	if _, err := dao.NewFileDAO().GetByID(context.Background(), db, "folder-1"); err != nil {
		t.Fatalf("folder was deleted without knowing its creator: %v", err)
	}
	stored, err := dao.NewSkillSpaceDAO().GetByIDAnyStatus(context.Background(), db, space.ID)
	if err != nil {
		t.Fatalf("reload space: %v", err)
	}
	if stored.Status != entity.SpaceStatusDeleting {
		t.Fatalf("space status = %q, want %q: the space was reported deleted while its folder survived", stored.Status, entity.SpaceStatusDeleting)
	}
}

// TestDeleteSpaceRestartsCleanupLeftInTheDeletingState pins the retry path for a
// space an earlier cleanup could not finish. Answering the repeat delete with an
// idempotent success left the space at "2" for good: cleanup never ran again and
// space creation kept refusing the name.
func TestDeleteSpaceRestartsCleanupLeftInTheDeletingState(t *testing.T) {
	db := setupServiceTestDB(t)
	models := append([]interface{}{&entity.SkillSpace{}}, permissionChangeLogTestModels()...)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("failed to migrate skill space: %v", err)
	}
	pushServiceDB(t, db)

	// The cleanup goroutine and the poll below hit the database at the same
	// time, and an in-memory SQLite database only carries its schema on the
	// connection that created it.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("unwrap test database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	valid := string(entity.StatusValid)
	// The enterprise policy resolves the tenant role through the user row, so
	// the acting owner needs one; the community policy never reads it.
	if err := db.Create(&entity.User{
		ID: "owner-1", Nickname: "owner", Email: "owner-1@test.local",
		IsAuthenticated: "1", IsActive: "1", IsAnonymous: "0",
	}).Error; err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	if err := db.Create(&entity.UserTenant{
		ID:       "owner-membership",
		UserID:   "owner-1",
		TenantID: "tenant-1",
		Role:     string(permission.RoleOwner),
		Status:   &valid,
	}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	// The cleanup runs as member-1, the folder creator; the enterprise policy
	// authorizes that principal through its tenant membership. The owner seeded
	// above keeps the fixture from promoting member-1 to owner.
	if err := testsupport.PrepareEnterprisePermissionFixture(db, "tenant-1", "member-1"); err != nil {
		t.Fatalf("prepare skill space permission fixture: %v", err)
	}

	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(previous) })

	for _, file := range []*entity.File{
		{ID: "skills-root", ParentID: "skills-root", TenantID: "tenant-1", CreatedBy: "owner-1", Name: "/", Type: "folder"},
		{ID: "folder-1", ParentID: "skills-root", TenantID: "tenant-1", CreatedBy: "member-1", Name: "skills", Type: "folder"},
		{ID: "file-1", ParentID: "folder-1", TenantID: "tenant-1", CreatedBy: "member-1", Name: "skill.md", Type: "doc"},
	} {
		if err := db.Create(file).Error; err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}

	space := &entity.SkillSpace{
		ID:       "space-1",
		TenantID: "tenant-1",
		Name:     "skills",
		FolderID: "folder-1",
		Status:   entity.SpaceStatusDeleting,
	}
	if err := db.Create(space).Error; err != nil {
		t.Fatalf("create space: %v", err)
	}

	code, err := NewSkillSpaceService(nil).DeleteSpace(context.Background(), "owner-1", space.ID, nil)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("DeleteSpace = (%v, %v), want success", code, err)
	}

	spaceDAO := dao.NewSkillSpaceDAO()
	deadline := time.Now().Add(3 * time.Second)
	for {
		stored, reloadErr := spaceDAO.GetByIDAnyStatus(context.Background(), db, space.ID)
		if reloadErr != nil {
			t.Fatalf("reload space: %v", reloadErr)
		}
		if stored.Status == entity.SpaceStatusDeleted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("space status = %q, want %q: the repeat delete did not restart the cleanup", stored.Status, entity.SpaceStatusDeleted)
		}
		time.Sleep(20 * time.Millisecond)
	}

	fileDAO := dao.NewFileDAO()
	for _, id := range []string{"folder-1", "file-1"} {
		if _, err := fileDAO.GetByID(context.Background(), db, id); err == nil {
			t.Fatalf("file %q survived the restarted cleanup", id)
		}
	}
}
