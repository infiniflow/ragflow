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

package storage

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// refusingBucketStorage refuses every bucket removal with `err`, the way a
// backend does when it cannot remove one logical bucket on its own.
type refusingBucketStorage struct {
	Storage
	err error
}

func (r refusingBucketStorage) RemoveBucket(context.Context, string) error {
	return r.err
}

// TestRemoveLogicalBucketKeepsASharedBucket pins the refusal as an outcome: the
// caller has already removed the dataset's own objects, so a shared bucket must
// not turn the deletion into a failure (#22513).
func TestRemoveLogicalBucketKeepsASharedBucket(t *testing.T) {
	store := refusingBucketStorage{
		Storage: NewMemoryStorage(),
		err:     fmt.Errorf("cannot remove logical OSS bucket kb-1: %w", ErrSharedBucketRemoval),
	}

	kept, err := RemoveLogicalBucket(context.Background(), store, "kb-1")
	if err != nil {
		t.Fatalf("shared bucket refusal returned an error: %v", err)
	}
	if !kept {
		t.Fatal("shared bucket refusal did not report the bucket as kept")
	}
}

// TestRemoveLogicalBucketReturnsAnyOtherFailure keeps the tolerance narrow: a
// removal that failed for another reason still reaches the caller.
func TestRemoveLogicalBucketReturnsAnyOtherFailure(t *testing.T) {
	store := refusingBucketStorage{
		Storage: NewMemoryStorage(),
		err:     errors.New("storage service unavailable"),
	}

	kept, err := RemoveLogicalBucket(context.Background(), store, "kb-1")
	if err == nil {
		t.Fatal("bucket removal failure was swallowed")
	}
	if kept {
		t.Fatal("failed bucket removal reported the bucket as kept")
	}
}

// TestRemoveLogicalBucketRemovesAnOrdinaryBucket covers the backend that can
// remove the bucket: it goes, and kept stays false.
func TestRemoveLogicalBucketRemovesAnOrdinaryBucket(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStorage()
	if err := store.Put(ctx, "kb-1", "doc", []byte("content")); err != nil {
		t.Fatal(err)
	}

	kept, err := RemoveLogicalBucket(ctx, store, "kb-1")
	if err != nil {
		t.Fatalf("remove ordinary bucket: %v", err)
	}
	if kept {
		t.Fatal("ordinary bucket was reported as kept")
	}
	if store.BucketExists(ctx, "kb-1") {
		t.Fatal("ordinary bucket survived its removal")
	}
}
