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
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

// countingRemoveStorage records the existence probes and removals issued by
// removeFileObject. Storage backends treat a missing object as a successful
// Remove, so probing first is a redundant round trip.
type countingRemoveStorage struct {
	storage.Storage
	existsCalls int
	removeCalls []string
	exists      bool
	existsErr   error
	removeErr   error
}

func (s *countingRemoveStorage) ObjectExists(context.Context, string, string) (bool, error) {
	s.existsCalls++
	return s.exists, s.existsErr
}

func (s *countingRemoveStorage) Remove(_ context.Context, bucket, name string, _ ...string) error {
	s.removeCalls = append(s.removeCalls, bucket+"/"+name)
	return s.removeErr
}

// TestRemoveFileObjectSkipsExistenceCheck is the core RED assertion: the
// object is reported missing, yet the idempotent Remove must still be issued
// and no existence probe may happen.
func TestRemoveFileObjectSkipsExistenceCheck(t *testing.T) {
	loc := "doc.pdf"
	file := &entity.File{ID: "file-1", ParentID: "bucket-1", Location: &loc}
	store := &countingRemoveStorage{exists: false}

	if err := removeFileObject(t.Context(), store, file); err != nil {
		t.Fatalf("removeFileObject on missing object: %v", err)
	}
	if store.existsCalls != 0 {
		t.Fatalf("ObjectExists called %d times, want 0", store.existsCalls)
	}
	if want := []string{"bucket-1/doc.pdf"}; !reflect.DeepEqual(store.removeCalls, want) {
		t.Fatalf("Remove calls = %v, want %v", store.removeCalls, want)
	}
}

// TestRemoveFileObjectPropagatesRemoveError pins that dropping the probe does
// not swallow a real removal failure, and keeps its error wrapping.
func TestRemoveFileObjectPropagatesRemoveError(t *testing.T) {
	loc := "doc.pdf"
	file := &entity.File{ID: "file-1", ParentID: "bucket-1", Location: &loc}
	store := &countingRemoveStorage{removeErr: errors.New("storage unavailable")}

	err := removeFileObject(t.Context(), store, file)
	if err == nil || !strings.Contains(err.Error(), "remove file file-1: storage unavailable") {
		t.Fatalf("removeFileObject error = %v, want wrapped remove failure", err)
	}
	if store.existsCalls != 0 {
		t.Fatalf("ObjectExists called %d times, want 0", store.existsCalls)
	}
}

// TestRemoveFileObjectIgnoresEmptyLocation guards the no-op branch: a file
// without a stored object must not touch storage at all.
func TestRemoveFileObjectIgnoresEmptyLocation(t *testing.T) {
	empty := ""
	for _, file := range []*entity.File{
		{ID: "nil-location", ParentID: "bucket-1"},
		{ID: "empty-location", ParentID: "bucket-1", Location: &empty},
	} {
		store := &countingRemoveStorage{}
		if err := removeFileObject(t.Context(), store, file); err != nil {
			t.Fatalf("%s: %v", file.ID, err)
		}
		if store.existsCalls != 0 || len(store.removeCalls) != 0 {
			t.Fatalf("%s touched storage: exists=%d remove=%v", file.ID, store.existsCalls, store.removeCalls)
		}
	}
}

// TestRemoveFileObjectRequiresStorage pins the nil-storage guard: the missing
// storage configuration is still reported, independent of existence probing.
func TestRemoveFileObjectRequiresStorage(t *testing.T) {
	loc := "doc.pdf"
	file := &entity.File{ID: "file-1", ParentID: "bucket-1", Location: &loc}
	if err := removeFileObject(t.Context(), nil, file); err == nil {
		t.Fatal("expected error when storage is not configured")
	}
}
