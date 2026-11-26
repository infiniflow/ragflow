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
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

// countingObjExistStorage counts existence probes without changing the
// backend's behavior, so the test measures the I/O cost, not a mock's opinion.
type countingObjExistStorage struct {
	storage.Storage
	objExistCalls int
}

func (s *countingObjExistStorage) ObjExist(ctx context.Context, bucket, fnm string, tenantID ...string) bool {
	s.objExistCalls++
	return s.Storage.ObjExist(ctx, bucket, fnm, tenantID...)
}

func seedMoveUniqueTestDB(t *testing.T) {
	t.Helper()
	setupFolderTestDB(t)
	for _, f := range []*entity.File{
		{ID: "pf1", ParentID: "pf1", TenantID: "tenant-1", CreatedBy: "tenant-1", Name: "root", Type: FileTypeFolder},
		{ID: "d1", ParentID: "pf1", TenantID: "tenant-1", CreatedBy: "tenant-1", Name: "dest", Type: FileTypeFolder},
		{ID: "s1", ParentID: "pf1", TenantID: "tenant-1", CreatedBy: "tenant-1", Name: "report.txt", Location: sptr("report.txt"), Type: "pdf"},
	} {
		if err := dao.DB.Create(f).Error; err != nil {
			t.Fatalf("seed %s: %v", f.ID, err)
		}
	}
}

func installCountingStorage(t *testing.T, existingKeys ...string) *countingObjExistStorage {
	t.Helper()
	mem := storage.NewMemoryStorage()
	ctx := t.Context()
	if err := mem.Put(ctx, "pf1", "report.txt", []byte("payload")); err != nil {
		t.Fatalf("seed source object: %v", err)
	}
	for _, key := range existingKeys {
		if err := mem.Put(ctx, "d1", key, []byte("taken")); err != nil {
			t.Fatalf("seed destination object %q: %v", key, err)
		}
	}
	store := &countingObjExistStorage{Storage: mem}
	factory := storage.GetStorageFactory()
	original := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(original) })
	return store
}

func movedUniqueLocation(t *testing.T) string {
	t.Helper()
	var f entity.File
	if err := dao.DB.Where("id = ?", "s1").Take(&f).Error; err != nil {
		t.Fatalf("read moved file: %v", err)
	}
	if f.Location == nil || *f.Location == "" {
		t.Fatal("moved file has no location")
	}
	return *f.Location
}

func moveUniqueTest(t *testing.T, existingKeys ...string) (*countingObjExistStorage, string) {
	t.Helper()
	seedMoveUniqueTestDB(t)
	store := installCountingStorage(t, existingKeys...)
	svc := testFileService()
	if err := svc.MoveFiles(fileTestContext(t, "tenant-1", "tenant-1"), []string{"s1"}, "d1", ""); err != nil {
		t.Fatalf("MoveFiles: %v", err)
	}
	return store, movedUniqueLocation(t)
}

// K occupied names used to cost K+1 serial storage round-trips. A unique
// candidate must be chosen in a bounded number of probes.
func TestMoveFilesUsesBoundedExistenceProbes(t *testing.T) {
	taken := []string{"report.txt", "report.txt_", "report.txt__"}
	store, location := moveUniqueTest(t, taken...)
	if store.objExistCalls > 2 {
		t.Errorf("ObjExist calls = %d, want at most 2", store.objExistCalls)
	}
	for _, key := range taken {
		if location == key {
			t.Errorf("location %q collides with an existing object", location)
		}
	}
	if !store.Storage.ObjExist(t.Context(), "d1", location) {
		t.Errorf("no object at the recorded location %q", location)
	}
}

// The conflict-free path is the common path and must stay byte-identical:
// no suffix, exactly one existence probe.
func TestMoveFilesKeepsOriginalNameWhenFree(t *testing.T) {
	store, location := moveUniqueTest(t)
	if location != "report.txt" {
		t.Errorf("location = %q, want unchanged report.txt", location)
	}
	if store.objExistCalls != 1 {
		t.Errorf("ObjExist calls = %d, want 1", store.objExistCalls)
	}
}

// A long run of underscore names is the adversarial case for the old loop;
// the probe count must stay flat as the number of collisions grows.
// A near-limit name plus a collision must not produce a location longer than
// the database column: the storage move happens before the DB update, so an
// over-long location would leave the file moved but untrackable.
func TestMoveFilesKeepsLongNameWithinLocationLimit(t *testing.T) {
	longName := strings.Repeat("a", common.NameLimit255-5)
	setupFolderTestDB(t)
	for _, f := range []*entity.File{
		{ID: "pf1", ParentID: "pf1", TenantID: "tenant-1", CreatedBy: "tenant-1", Name: "root", Type: FileTypeFolder},
		{ID: "d1", ParentID: "pf1", TenantID: "tenant-1", CreatedBy: "tenant-1", Name: "dest", Type: FileTypeFolder},
		{ID: "s1", ParentID: "pf1", TenantID: "tenant-1", CreatedBy: "tenant-1", Name: longName, Location: sptr(longName), Type: "pdf"},
	} {
		if err := dao.DB.Create(f).Error; err != nil {
			t.Fatalf("seed %s: %v", f.ID, err)
		}
	}

	ctx := t.Context()
	mem := storage.NewMemoryStorage()
	if err := mem.Put(ctx, "pf1", longName, []byte("payload")); err != nil {
		t.Fatalf("seed source object: %v", err)
	}
	if err := mem.Put(ctx, "d1", longName, []byte("taken")); err != nil {
		t.Fatalf("seed colliding object: %v", err)
	}
	store := &countingObjExistStorage{Storage: mem}
	factory := storage.GetStorageFactory()
	original := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(original) })

	if err := testFileService().MoveFiles(fileTestContext(t, "tenant-1", "tenant-1"), []string{"s1"}, "d1", ""); err != nil {
		t.Fatalf("MoveFiles: %v", err)
	}
	location := movedUniqueLocation(t)
	if len(location) > common.NameLimit255 {
		t.Errorf("location length = %d, want at most %d", len(location), common.NameLimit255)
	}
	if location == longName {
		t.Error("location still collides with the existing object")
	}
	if !store.Storage.ObjExist(ctx, "d1", location) {
		t.Errorf("no object at the recorded location %q", location)
	}
}

func TestMoveFilesProbeCountIsIndependentOfCollisionCount(t *testing.T) {
	taken := []string{"report.txt"}
	for i := 1; i <= 15; i++ {
		taken = append(taken, "report.txt"+strings.Repeat("_", i))
	}
	store, location := moveUniqueTest(t, taken...)
	if store.objExistCalls > 2 {
		t.Errorf("ObjExist calls = %d with %d collisions, want at most 2", store.objExistCalls, len(taken))
	}
	for _, key := range taken {
		if location == key {
			t.Fatalf("location %q collides with an existing object", location)
		}
	}
}
