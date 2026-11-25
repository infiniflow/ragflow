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
	"testing/synctest"
	"unicode/utf8"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
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

func TestMoveFilesKeepsLongNameWithinLocationLimit(t *testing.T) {
	const limit = utility.StorageKeyMaxBytes
	for label, name := range map[string]string{
		"near-limit":     strings.Repeat("a", limit-5),
		"ascii-at-limit": strings.Repeat("a", limit-len(".txt")) + ".txt",
		"utf8-at-limit":  strings.Repeat("界", 83) + "ab.txt",
	} {
		t.Run(label, func(t *testing.T) {
			seedMoveUniqueTestDB(t)
			// SQLite does not enforce the location column's declared width.
			if err := dao.DB.Exec(`CREATE TRIGGER check_move_location BEFORE UPDATE OF location ON file
				WHEN length(NEW.location) > 255
				BEGIN SELECT RAISE(ABORT, 'location too long'); END`).Error; err != nil {
				t.Fatal(err)
			}
			if err := dao.DB.Model(&entity.File{}).Where("id = ?", "s1").Update("name", name).Error; err != nil {
				t.Fatal(err)
			}
			store := installCountingStorage(t, name)
			ctx := fileTestContext(t, "tenant-1", "tenant-1")
			svc := testFileService()
			if err := svc.MoveFiles(ctx, []string{"s1"}, "d1", ""); err != nil {
				t.Fatal(err)
			}
			file, err := svc.GetFileContent(ctx, "s1")
			if err != nil {
				t.Fatal(err)
			}
			if file.Name != name || file.ParentID != "d1" || file.Location == nil {
				t.Fatalf("unexpected moved file: %+v", file)
			}
			key := *file.Location
			if len(key) > limit || !utf8.ValidString(key) {
				t.Fatalf("invalid destination key %q (%d bytes)", key, len(key))
			}
			if data, err := store.Get(ctx, file.ParentID, key); err != nil || string(data) != "payload" {
				t.Fatalf("reading moved file: got %q, err %v", data, err)
			}
			if data, err := store.Get(ctx, "d1", name); err != nil || string(data) != "taken" {
				t.Fatalf("reading pre-existing object: got %q, err %v", data, err)
			}
		})
	}
}

func TestUniqueStorageLocationRepeatedCollisionsAtByteLimit(t *testing.T) {
	// Freeze the clock so successive calls collide on the timestamp candidate.
	synctest.Test(t, func(t *testing.T) {
		store := storage.NewMemoryStorage()
		name := strings.Repeat("a", utility.StorageKeyMaxBytes)
		for i := 0; i < 4; i++ {
			key := uniqueStorageLocation(t.Context(), store, "dest", name)
			if len(key) > utility.StorageKeyMaxBytes || store.ObjExist(t.Context(), "dest", key) {
				t.Fatalf("attempt %d: invalid or occupied key %q", i, key)
			}
			if i == 0 && key != name {
				t.Fatalf("free destination: got %q, want original name", key)
			}
			if err := store.Put(t.Context(), "dest", key, []byte("taken")); err != nil {
				t.Fatal(err)
			}
		}
	})
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
