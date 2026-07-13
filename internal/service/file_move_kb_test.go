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
	"errors"
	"sync"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

var errNotFound = errors.New("not found")

// fakeMoveStorage is a storage fake that supports Move and Copy by
// manipulating an in-memory map keyed by bucket/object.
type fakeMoveStorage struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeMoveStorage() *fakeMoveStorage {
	return &fakeMoveStorage{objects: map[string][]byte{}}
}

func (f *fakeMoveStorage) key(bucket, fnm string) string { return bucket + "/" + fnm }

func (f *fakeMoveStorage) Health() bool { return true }

func (f *fakeMoveStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[f.key(bucket, fnm)] = append([]byte(nil), binary...)
	return nil
}

func (f *fakeMoveStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.objects[f.key(bucket, fnm)]
	if !ok {
		return nil, errNotFound
	}
	return append([]byte(nil), v...), nil
}

func (f *fakeMoveStorage) Remove(bucket, fnm string, tenantID ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, f.key(bucket, fnm))
	return nil
}

func (f *fakeMoveStorage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[f.key(bucket, fnm)]
	return ok
}

func (f *fakeMoveStorage) ListObjects(bucket string, tenantID ...string) ([]string, error) {
	return nil, nil
}

func (f *fakeMoveStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	return "", nil
}

func (f *fakeMoveStorage) BucketExists(bucket string) bool { return true }

func (f *fakeMoveStorage) RemoveBucket(bucket string) error { return nil }

func (f *fakeMoveStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.objects[f.key(srcBucket, srcPath)]
	if !ok {
		return false
	}
	f.objects[f.key(destBucket, destPath)] = append([]byte(nil), v...)
	return true
}

func (f *fakeMoveStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	if !f.Copy(srcBucket, srcPath, destBucket, destPath) {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, f.key(srcBucket, srcPath))
	return true
}

func (f *fakeMoveStorage) Close() error { return nil }

func moveTestService(t *testing.T) *FileService {
	t.Helper()
	return &FileService{
		fileDAO:          dao.NewFileDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
		documentService:  testDocumentService(t),
	}
}

func setMoveTestStorage(t *testing.T) *fakeMoveStorage {
	t.Helper()
	fake := newFakeMoveStorage()
	factory := storage.GetStorageFactory()
	orig := factory.GetStorage()
	factory.SetStorage(fake)
	t.Cleanup(func() { factory.SetStorage(orig) })
	return fake
}

func insertMoveTestRoot(t *testing.T, id string) {
	t.Helper()
	root := &entity.File{
		ID:        id,
		ParentID:  id,
		TenantID:  "tenant-1",
		CreatedBy: "user-1",
		Name:      "/",
		Type:      "folder",
		SourceType: "",
	}
	if err := dao.DB.Create(root).Error; err != nil {
		t.Fatalf("insert root: %v", err)
	}
}

func insertMoveTestFolder(t *testing.T, id, parentID, name, sourceType string) {
	t.Helper()
	f := &entity.File{
		ID:         id,
		ParentID:   parentID,
		TenantID:   "tenant-1",
		CreatedBy:  "user-1",
		Name:       name,
		Type:       "folder",
		SourceType: sourceType,
	}
	if err := dao.DB.Create(f).Error; err != nil {
		t.Fatalf("insert folder %s: %v", name, err)
	}
}

func insertMoveTestFile(t *testing.T, id, parentID, name, location string, sourceType string) {
	t.Helper()
	loc := location
	f := &entity.File{
		ID:         id,
		ParentID:   parentID,
		TenantID:   "tenant-1",
		CreatedBy:  "user-1",
		Name:       name,
		Type:       "doc",
		Location:   &loc,
		Size:       12,
		SourceType: sourceType,
	}
	if err := dao.DB.Create(f).Error; err != nil {
		t.Fatalf("insert file %s: %v", name, err)
	}
}

// TestMoveFiles_ToKnowledgebaseSubfolder verifies that a regular file can be
// moved into a .knowledgebase subfolder (dataset mirror folder).
func TestMoveFiles_ToKnowledgebaseSubfolder(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertMoveTestRoot(t, "root")
	insertMoveTestFolder(t, "regular", "root", "regular", "")
	insertMoveTestFolder(t, "kb-root", "root", ".knowledgebase", string(entity.FileSourceKnowledgebase))
	insertMoveTestFolder(t, "kb-sub", "kb-root", "dataset-a", string(entity.FileSourceKnowledgebase))
	insertMoveTestFile(t, "file-a", "regular", "report.pdf", "report.pdf", "")

	fake := setMoveTestStorage(t)
	fake.Put("regular", "report.pdf", []byte("data"))

	svc := moveTestService(t)
	ok, msg := svc.MoveFiles("tenant-1", []string{"file-a"}, "kb-sub", "")
	if !ok {
		t.Fatalf("MoveFiles failed: %s", msg)
	}

	file, err := dao.NewFileDAO().GetByID("file-a")
	if err != nil {
		t.Fatalf("get moved file: %v", err)
	}
	if file.ParentID != "kb-sub" {
		t.Fatalf("parent_id = %s, want kb-sub", file.ParentID)
	}
	if !fake.ObjExist("kb-sub", "report.pdf") {
		t.Fatal("expected blob to be moved to kb-sub/report.pdf")
	}
	if fake.ObjExist("regular", "report.pdf") {
		t.Fatal("old blob should be removed")
	}
}

// TestMoveFiles_BetweenKnowledgebaseSubfolders verifies that a file already
// inside a .knowledgebase subfolder can be moved to another subfolder.
func TestMoveFiles_BetweenKnowledgebaseSubfolders(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertMoveTestRoot(t, "root")
	insertMoveTestFolder(t, "kb-root", "root", ".knowledgebase", string(entity.FileSourceKnowledgebase))
	insertMoveTestFolder(t, "kb-sub-1", "kb-root", "dataset-a", string(entity.FileSourceKnowledgebase))
	insertMoveTestFolder(t, "kb-sub-2", "kb-root", "dataset-b", string(entity.FileSourceKnowledgebase))
	insertMoveTestFile(t, "file-b", "kb-sub-1", "report.pdf", "report.pdf", string(entity.FileSourceKnowledgebase))

	fake := setMoveTestStorage(t)
	fake.Put("kb-sub-1", "report.pdf", []byte("data"))

	svc := moveTestService(t)
	ok, msg := svc.MoveFiles("tenant-1", []string{"file-b"}, "kb-sub-2", "")
	if !ok {
		t.Fatalf("MoveFiles failed: %s", msg)
	}

	file, err := dao.NewFileDAO().GetByID("file-b")
	if err != nil {
		t.Fatalf("get moved file: %v", err)
	}
	if file.ParentID != "kb-sub-2" {
		t.Fatalf("parent_id = %s, want kb-sub-2", file.ParentID)
	}
	if !fake.ObjExist("kb-sub-2", "report.pdf") {
		t.Fatal("expected blob to be moved to kb-sub-2/report.pdf")
	}
	if fake.ObjExist("kb-sub-1", "report.pdf") {
		t.Fatal("old blob should be removed")
	}
}

// TestListFiles_KnowledgebaseHasChildFolder verifies that the root folder listing
// reports .knowledgebase as having child folders, so the move dialog can expand it.
func TestListFiles_KnowledgebaseHasChildFolder(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertMoveTestRoot(t, "root")
	insertMoveTestFolder(t, "kb-root", "root", ".knowledgebase", string(entity.FileSourceKnowledgebase))
	insertMoveTestFolder(t, "kb-sub", "kb-root", "dataset-a", string(entity.FileSourceKnowledgebase))

	svc := moveTestService(t)
	resp, err := svc.ListFiles("tenant-1", "root", 1, 100, "create_time", true, "")
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	var kbFolder map[string]interface{}
	for _, f := range resp.Files {
		if name, _ := f["name"].(string); name == ".knowledgebase" {
			kbFolder = f
			break
		}
	}
	if kbFolder == nil {
		t.Fatal(".knowledgebase folder not found in root listing")
	}

	hasChild, ok := kbFolder["has_child_folder"].(bool)
	if !ok {
		t.Fatalf("has_child_folder is not bool: %T", kbFolder["has_child_folder"])
	}
	if !hasChild {
		t.Fatalf("has_child_folder = %v, want true", hasChild)
	}
}

// TestMoveFiles_RenameToSameName verifies the duplicate-name check excludes the
// source file itself, so renaming a file to its current name succeeds.
func TestMoveFiles_RenameToSameName(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertMoveTestRoot(t, "root")
	insertMoveTestFolder(t, "regular", "root", "regular", "")
	insertMoveTestFile(t, "file-c", "regular", "report.pdf", "report.pdf", "")

	fake := setMoveTestStorage(t)
	fake.Put("regular", "report.pdf", []byte("data"))

	svc := moveTestService(t)
	ok, msg := svc.MoveFiles("tenant-1", []string{"file-c"}, "", "report.pdf")
	if !ok {
		t.Fatalf("rename to same name failed: %s", msg)
	}

	file, err := dao.NewFileDAO().GetByID("file-c")
	if err != nil {
		t.Fatalf("get file: %v", err)
	}
	if file.Name != "report.pdf" {
		t.Fatalf("name = %s, want report.pdf", file.Name)
	}
}
