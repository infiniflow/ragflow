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
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// TestAsyncDeleteSpaceOutlivesTheRequestContext pins the cleanup goroutine to a
// context that survives the request. Gin cancels c.Request.Context() as soon as
// the handler returns, so inheriting it left the space at status "2" forever:
// the delete endpoint answers with the idempotent "already deleting" success on
// every retry and space creation refuses the same name.
func TestAsyncDeleteSpaceOutlivesTheRequestContext(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.SkillSpace{}); err != nil {
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
