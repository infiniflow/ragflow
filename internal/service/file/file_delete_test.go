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

package file

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/storage"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type failingObjectCheckStorage struct{ storage.Storage }

func (f failingObjectCheckStorage) ObjectExists(context.Context, string, string) (bool, error) {
	return false, errors.New("storage unavailable")
}

func TestDeleteFolderLeavesNonemptyBucket(t *testing.T) {
	db := setupFolderTestDB(t)
	folder, err := testFileService().fileDAO.CreateFolder(t.Context(), db, "root", "tenant-1", "tenant-1", "folder", FileTypeFolder)
	if err != nil {
		t.Fatal(err)
	}
	insertFolderTestFile(t, "file-1", folder.ID, "tracked")
	store := storage.NewMemoryStorage()
	if err := store.Put(t.Context(), folder.ID, "tracked", []byte("content")); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(t.Context(), folder.ID, "untracked", []byte("content")); err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })
	core, logs := observer.New(zapcore.InfoLevel)
	previousLogger := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() { common.Logger = previousLogger })

	service := testFileService()
	if err := service.deleteFolderRecursive(t.Context(), folder); err != nil {
		t.Fatalf("delete folder with nonempty bucket: %v", err)
	}
	if !store.ObjExist(t.Context(), folder.ID, "untracked") {
		t.Fatal("folder deletion removed an untracked object")
	}
	if store.ObjExist(t.Context(), folder.ID, "tracked") {
		t.Fatal("folder deletion retained a file object")
	}
	if _, err := service.fileDAO.GetByID(t.Context(), db, folder.ID); err == nil {
		t.Fatal("folder record retained after storage failure")
	}
	if entries := logs.FilterMessage("Deleted file").All(); len(entries) != 1 || entries[0].ContextMap()["file"] != "tracked (file-1)" {
		t.Errorf("file deletion logs = %v", entries)
	}
	if entries := logs.FilterMessage("Deleted folder").All(); len(entries) != 1 || entries[0].ContextMap()["folder"] != "folder ("+folder.ID+")" {
		t.Errorf("folder deletion logs = %v", entries)
	}
}

func TestDeleteFolderStopsOnStorageCheckError(t *testing.T) {
	db := setupFolderTestDB(t)
	folder, err := testFileService().fileDAO.CreateFolder(t.Context(), db, "root", "tenant-1", "tenant-1", "folder", FileTypeFolder)
	if err != nil {
		t.Fatal(err)
	}
	insertFolderTestFile(t, "file-1", folder.ID, "tracked")
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(failingObjectCheckStorage{storage.NewMemoryStorage()})
	t.Cleanup(func() { factory.SetStorage(previous) })
	if err := testFileService().deleteFolderRecursive(t.Context(), folder); err == nil {
		t.Fatal("expected storage check error")
	}
	if _, err := testFileService().fileDAO.GetByID(t.Context(), db, "file-1"); err != nil {
		t.Fatalf("file record deleted after storage check failed: %v", err)
	}
}

func TestDeleteFolderContinuesWhenStorageIsMissing(t *testing.T) {
	db := setupFolderTestDB(t)
	service := testFileService()
	folder, err := service.fileDAO.CreateFolder(t.Context(), db, "root", "tenant-1", "tenant-1", "folder", FileTypeFolder)
	if err != nil {
		t.Fatal(err)
	}
	insertFolderTestFile(t, "file-1", folder.ID, "missing")
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(previous) })
	if err := service.deleteFolderRecursive(t.Context(), folder); err != nil {
		t.Fatalf("delete folder with missing storage: %v", err)
	}
	if _, err := service.fileDAO.GetByID(t.Context(), db, folder.ID); err == nil {
		t.Fatal("folder record retained after missing storage")
	}
}

func TestDeleteFilesAuthorizesEveryDescendantBeforeDeleting(t *testing.T) {
	db := setupFolderTestDB(t)
	for _, file := range []*entity.File{
		{ID: "folder", ParentID: "root", TenantID: "user-1", Name: "folder", Type: FileTypeFolder},
		{ID: "child-folder", ParentID: "folder", TenantID: "user-1", CreatedBy: "user-1", Name: "child", Type: FileTypeFolder},
		{ID: "foreign-child", ParentID: "child-folder", TenantID: "user-2", Name: "private.pdf", Type: "pdf"},
	} {
		if err := db.Create(file).Error; err != nil {
			t.Fatalf("seed %s: %v", file.ID, err)
		}
	}

	err := testFileService().DeleteFiles(fileTestContext(t, "user-1", "user-1"), "user-1", []string{"folder"})
	if !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("DeleteFiles error = %v, want permission denied", err)
	}
	for _, id := range []string{"folder", "child-folder", "foreign-child"} {
		if _, err := dao.NewFileDAO().GetByID(t.Context(), db, id); err != nil {
			t.Fatalf("file %q was removed before authorization completed: %v", id, err)
		}
	}
}

func TestFileService_CheckFileTreeAccessBatchesWideSubtrees(t *testing.T) {
	db := setupFolderTestDB(t)
	entries := make([]entity.File, 0, 1+2*510)
	entries = append(entries,
		entity.File{ID: "tenant-root", ParentID: "tenant-root", TenantID: "tenant-1", CreatedBy: "tenant-1", Name: "/", Type: FileTypeFolder},
		entity.File{ID: "tree-root", ParentID: "tenant-root", TenantID: "tenant-1", CreatedBy: "owner", Name: "tree", Type: FileTypeFolder},
	)
	for i := 0; i < 510; i++ {
		folderID := fmt.Sprintf("folder-%04d", i)
		entries = append(entries,
			entity.File{ID: folderID, ParentID: "tree-root", TenantID: "tenant-1", CreatedBy: "owner", Name: folderID, Type: FileTypeFolder},
			entity.File{ID: fmt.Sprintf("file-%04d", i), ParentID: folderID, TenantID: "tenant-1", CreatedBy: "owner", Name: "child.txt", Type: "doc"},
		)
	}
	if err := db.CreateInBatches(&entries, 100).Error; err != nil {
		t.Fatalf("seed wide file tree: %v", err)
	}

	root := &entity.File{ID: "tree-root", ParentID: "tree-root", TenantID: "tenant-1", CreatedBy: "owner", Name: "/", Type: FileTypeFolder}
	if err := testFileService().checkFileTreeAccess(fileTestContext(t, "owner", "tenant-1"), "owner", root, permission.OperationDelete); err != nil {
		t.Fatalf("check permissions for wide subtree: %v", err)
	}
}
