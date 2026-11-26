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
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/storage"
)

// countingDeletionStorage records every existence probe and removal. Storage
// backends treat a missing object/bucket as a successful removal, so the
// pre-delete ObjectExists/BucketExistsWithError probes are redundant.
type countingDeletionStorage struct {
	storage.Storage
	existsCalls    int
	bucketCalls    int
	removed        []string
	removedBuckets []string
	removedEmpty   []string
	objectErr      error
	bucketErr      error
	emptyBucketErr error
	bucketMissing  bool
}

func (s *countingDeletionStorage) ObjectExists(context.Context, string, string) (bool, error) {
	s.existsCalls++
	return true, nil
}

func (s *countingDeletionStorage) BucketExistsWithError(context.Context, string) (bool, error) {
	s.bucketCalls++
	return !s.bucketMissing, nil
}

func (s *countingDeletionStorage) Remove(_ context.Context, bucket, name string, _ ...string) error {
	if s.objectErr != nil {
		return s.objectErr
	}
	s.removed = append(s.removed, bucket+"/"+name)
	return nil
}

func (s *countingDeletionStorage) RemoveBucket(_ context.Context, bucket string) error {
	if s.bucketErr != nil {
		return s.bucketErr
	}
	s.removedBuckets = append(s.removedBuckets, bucket)
	return nil
}

func (s *countingDeletionStorage) RemoveEmptyBucket(_ context.Context, bucket string) error {
	if s.emptyBucketErr != nil {
		return s.emptyBucketErr
	}
	s.removedEmpty = append(s.removedEmpty, bucket)
	return nil
}

// deletionFixture exercises all five existence checks in deleteExternalData:
// document object (:251), dataset bucket (:266), downloads bucket (:281),
// file object (:296) and folder bucket (:314).
func deletionFixture() *userDeletionData {
	return &userDeletionData{
		ownedTenantID: "tenant-1",
		datasetIDs:    []string{"dataset-1"},
		datasets: []entity.Knowledgebase{
			{ID: "dataset-1", TenantID: "tenant-1", Name: "Dataset"},
		},
		documents: []entity.Document{
			{ID: "doc-1", KbID: "dataset-1", Name: sptr("Doc"), Location: sptr("doc.pdf")},
		},
		files: []entity.File{
			{ID: "file-1", ParentID: "dataset-1", TenantID: "tenant-1", Name: "file.pdf", Location: sptr("file.pdf"), Type: "file"},
			{ID: "folder-1", ParentID: "folder-1", TenantID: "tenant-1", Name: "/", Type: "folder"},
		},
		datasetNames: map[string]string{"dataset-1": "Dataset"},
	}
}

// TestDeleteExternalDataSkipsExistenceProbes is the core RED assertion: all
// five pre-delete probes must be gone while every removal still happens.
func TestDeleteExternalDataSkipsExistenceProbes(t *testing.T) {
	store := &countingDeletionStorage{}
	if err := deletionFixture().deleteExternalData(t.Context(), &deletionEngine{}, store); err != nil {
		t.Fatalf("deleteExternalData: %v", err)
	}
	// Object-level probes are gone; the two bucket-level guards remain for
	// backends that refuse logical-bucket removal.
	if store.existsCalls != 0 || store.bucketCalls != 2 {
		t.Fatalf("existence probes = ObjectExists:%d BucketExists:%d, want 0/2", store.existsCalls, store.bucketCalls)
	}
	if want := []string{"dataset-1/doc.pdf", "dataset-1/file.pdf"}; !reflect.DeepEqual(store.removed, want) {
		t.Fatalf("removed objects = %v, want %v", store.removed, want)
	}
	if want := []string{"dataset-1", "tenant-1-downloads"}; !reflect.DeepEqual(store.removedBuckets, want) {
		t.Fatalf("removed buckets = %v, want %v", store.removedBuckets, want)
	}
	if want := []string{"folder-1"}; !reflect.DeepEqual(store.removedEmpty, want) {
		t.Fatalf("removed empty buckets = %v, want %v", store.removedEmpty, want)
	}
}

// TestDeleteExternalDataSkipsUnremovableMissingBucket is the adversarial
// backend case: some backends refuse logical-bucket removal outright. When
// the physical bucket is already absent, the caller must skip the removal
// instead of failing user deletion — the baseline did exactly that.
func TestDeleteExternalDataSkipsUnremovableMissingBucket(t *testing.T) {
	store := &countingDeletionStorage{
		bucketMissing: true,
		bucketErr:     errors.New("cannot remove logical bucket"),
	}
	if err := deletionFixture().deleteExternalData(t.Context(), &deletionEngine{}, store); err != nil {
		t.Fatalf("deleteExternalData with a missing unremovable bucket: %v", err)
	}
	if len(store.removedBuckets) != 0 {
		t.Fatalf("RemoveBucket called for missing buckets: %v", store.removedBuckets)
	}
	if store.bucketCalls == 0 {
		t.Fatal("expected existence checks before bucket removal")
	}
}

// TestDeleteExternalDataKeepsRemovalErrors pins that removing the probes does
// not swallow a real storage failure (the user row must stay for retry).
func TestDeleteExternalDataKeepsRemovalErrors(t *testing.T) {
	for _, tc := range []struct {
		name            string
		store           *countingDeletionStorage
		want            string
		wantBucketCalls int
	}{
		{"object", &countingDeletionStorage{objectErr: errors.New("storage unavailable")}, "storage unavailable", 0},
		{"bucket", &countingDeletionStorage{bucketErr: errors.New("storage unavailable")}, "storage unavailable", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := deletionFixture().deleteExternalData(t.Context(), &deletionEngine{}, tc.store)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("deleteExternalData error = %v, want %q", err, tc.want)
			}
			if tc.store.existsCalls != 0 || tc.store.bucketCalls != tc.wantBucketCalls {
				t.Fatalf("existence probes = %d/%d, want 0/%d", tc.store.existsCalls, tc.store.bucketCalls, tc.wantBucketCalls)
			}
		})
	}
}
