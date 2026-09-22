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
	"testing"

	"ragflow/internal/storage"
)

type failingObjectCheckStorage struct{ storage.Storage }

func (f failingObjectCheckStorage) ObjectExists(context.Context, string, string) (bool, error) {
	return false, errors.New("storage unavailable")
}

func TestDeleteFolderLeavesNonemptyBucket(t *testing.T) {
	db := setupFolderTestDB(t)
	folder, err := testFileService().fileDAO.CreateFolder(t.Context(), db, "root", "tenant-1", "folder", FileTypeFolder)
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

	service := testFileService()
	if err := service.deleteFolderRecursive(t.Context(), folder, "tenant-1"); err != nil {
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
}

func TestDeleteFolderStopsOnStorageCheckError(t *testing.T) {
	db := setupFolderTestDB(t)
	folder, err := testFileService().fileDAO.CreateFolder(t.Context(), db, "root", "tenant-1", "folder", FileTypeFolder)
	if err != nil {
		t.Fatal(err)
	}
	insertFolderTestFile(t, "file-1", folder.ID, "tracked")
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(failingObjectCheckStorage{storage.NewMemoryStorage()})
	t.Cleanup(func() { factory.SetStorage(previous) })
	if err := testFileService().deleteFolderRecursive(t.Context(), folder, "tenant-1"); err == nil {
		t.Fatal("expected storage check error")
	}
	if _, err := testFileService().fileDAO.GetByID(t.Context(), db, "file-1"); err != nil {
		t.Fatalf("file record deleted after storage check failed: %v", err)
	}
}

func TestDeleteFolderContinuesWhenStorageIsMissing(t *testing.T) {
	db := setupFolderTestDB(t)
	service := testFileService()
	folder, err := service.fileDAO.CreateFolder(t.Context(), db, "root", "tenant-1", "folder", FileTypeFolder)
	if err != nil {
		t.Fatal(err)
	}
	insertFolderTestFile(t, "file-1", folder.ID, "missing")
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(previous) })
	if err := service.deleteFolderRecursive(t.Context(), folder, "tenant-1"); err != nil {
		t.Fatalf("delete folder with missing storage: %v", err)
	}
	if _, err := service.fileDAO.GetByID(t.Context(), db, folder.ID); err == nil {
		t.Fatal("folder record retained after missing storage")
	}
}
