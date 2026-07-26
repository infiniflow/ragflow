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
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestMemory returns a MemoryStorage typed pointer for direct Inspect access.
func newTestMemory(t *testing.T) *MemoryStorage {
	t.Helper()
	s := NewMemoryStorage()
	ms, ok := s.(*MemoryStorage)
	if !ok {
		t.Fatalf("NewMemoryStorage did not return *MemoryStorage")
	}
	return ms
}

func TestMemoryStorage_PutGet(t *testing.T) {
	ms := newTestMemory(t)

	payload := []byte("hello, world")
	if err := ms.Put("b1", "k1", payload); err != nil {
		t.Fatalf("Put returned error: %v", err)
	}

	got, err := ms.Get("b1", "k1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("Get returned %q, want %q", got, payload)
	}

	// Mutating the caller's slice after Put must not affect stored data.
	payload[0] = 'X'
	got2, err := ms.Get("b1", "k1")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if string(got2) != "hello, world" {
		t.Fatalf("stored bytes were mutated; got %q", got2)
	}
}

func TestMemoryStorage_GetMissing(t *testing.T) {
	ms := newTestMemory(t)

	if _, err := ms.Get("missing-bucket", "k"); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("Get on missing bucket: expected ErrMemoryNotFound, got %v", err)
	}

	if err := ms.Put("b1", "exists", []byte("data")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if _, err := ms.Get("b1", "missing-key"); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("Get on missing key: expected ErrMemoryNotFound, got %v", err)
	}
}

func TestMemoryStorage_ObjExist(t *testing.T) {
	ms := newTestMemory(t)

	if ms.ObjExist("b1", "k1") {
		t.Fatalf("ObjExist on empty bucket returned true")
	}

	if err := ms.Put("b1", "k1", []byte("v")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !ms.ObjExist("b1", "k1") {
		t.Fatalf("ObjExist after Put returned false")
	}
	if ms.ObjExist("b1", "other") {
		t.Fatalf("ObjExist for sibling key returned true")
	}
	if ms.ObjExist("other-bucket", "k1") {
		t.Fatalf("ObjExist for sibling bucket returned true")
	}
}

func TestMemoryStorage_Remove(t *testing.T) {
	ms := newTestMemory(t)

	// Idempotent: removing a key from a missing bucket is a no-op.
	if err := ms.Remove("ghost", "k"); err != nil {
		t.Fatalf("Remove on missing bucket returned error: %v", err)
	}

	if err := ms.Put("b1", "k1", []byte("v")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if err := ms.Remove("b1", "k1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if ms.ObjExist("b1", "k1") {
		t.Fatalf("ObjExist after Remove returned true")
	}

	// Removing the same key again must not error.
	if err := ms.Remove("b1", "k1"); err != nil {
		t.Fatalf("Remove on already-removed key returned error: %v", err)
	}
}

func TestMemoryStorage_RemoveBucket(t *testing.T) {
	ms := newTestMemory(t)

	for _, k := range []string{"a", "b", "c"} {
		if err := ms.Put("b1", k, []byte(k)); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}
	if err := ms.Put("b2", "x", []byte("x")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if err := ms.RemoveBucket("b1"); err != nil {
		t.Fatalf("RemoveBucket failed: %v", err)
	}
	if ms.BucketExists("b1") {
		t.Fatalf("BucketExists returned true after RemoveBucket")
	}
	if !ms.BucketExists("b2") {
		t.Fatalf("sibling bucket was removed unexpectedly")
	}

	// Idempotent: removing a missing bucket is a no-op.
	if err := ms.RemoveBucket("b1"); err != nil {
		t.Fatalf("RemoveBucket on missing bucket returned error: %v", err)
	}
}

func TestMemoryStorage_CopyMove(t *testing.T) {
	ms := newTestMemory(t)

	if err := ms.Put("src", "k", []byte("payload")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Copy preserves source.
	if !ms.Copy("src", "k", "dst", "k2") {
		t.Fatalf("Copy returned false on existing source")
	}
	if !ms.ObjExist("src", "k") {
		t.Fatalf("source missing after Copy")
	}
	if !ms.ObjExist("dst", "k2") {
		t.Fatalf("destination missing after Copy")
	}
	got, err := ms.Get("dst", "k2")
	if err != nil {
		t.Fatalf("Get copy failed: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("copy content mismatch: %q", got)
	}

	// Move deletes the source.
	if !ms.Move("src", "k", "dst2", "k3") {
		t.Fatalf("Move returned false on existing source")
	}
	if ms.ObjExist("src", "k") {
		t.Fatalf("source still exists after Move")
	}
	if !ms.ObjExist("dst2", "k3") {
		t.Fatalf("destination missing after Move")
	}

	// Copy/Move on missing source returns false.
	if ms.Copy("src", "k", "dst", "k4") {
		t.Fatalf("Copy on missing source returned true")
	}
	if ms.Move("src", "k", "dst", "k4") {
		t.Fatalf("Move on missing source returned true")
	}
}

func TestMemoryStorage_BucketExists(t *testing.T) {
	ms := newTestMemory(t)

	if ms.BucketExists("b1") {
		t.Fatalf("BucketExists returned true for empty backend")
	}
	if err := ms.Put("b1", "k", []byte("v")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !ms.BucketExists("b1") {
		t.Fatalf("BucketExists returned false after Put")
	}
	if err := ms.RemoveBucket("b1"); err != nil {
		t.Fatalf("RemoveBucket failed: %v", err)
	}
	if ms.BucketExists("b1") {
		t.Fatalf("BucketExists returned true after RemoveBucket")
	}
}

func TestMemoryStorage_PresignedURL(t *testing.T) {
	ms := newTestMemory(t)

	if err := ms.Put("b1", "k1", []byte("v")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	url, err := ms.GetPresignedURL("b1", "k1", time.Minute)
	if err != nil {
		t.Fatalf("GetPresignedURL failed: %v", err)
	}
	if !strings.Contains(url, "b1") {
		t.Fatalf("presigned URL missing bucket: %s", url)
	}
	if !strings.Contains(url, "k1") {
		t.Fatalf("presigned URL missing key: %s", url)
	}
	if !strings.HasPrefix(url, "memory://") {
		t.Fatalf("presigned URL has unexpected scheme: %s", url)
	}

	if _, err := ms.GetPresignedURL("b1", "missing", time.Minute); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("GetPresignedURL on missing key: expected ErrMemoryNotFound, got %v", err)
	}
}

func TestMemoryStorage_Health(t *testing.T) {
	ms := newTestMemory(t)
	if !ms.Health() {
		t.Fatalf("Health returned false for in-memory backend")
	}
}

func TestMemoryStorage_Concurrent(t *testing.T) {
	ms := newTestMemory(t)

	const writers = 100
	var wg sync.WaitGroup
	wg.Add(writers)

	for i := 0; i < writers; i++ {
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("k-%d", i)
			payload := []byte(fmt.Sprintf("payload-%d", i))
			if err := ms.Put("race", key, payload); err != nil {
				t.Errorf("Put failed for %s: %v", key, err)
				return
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < writers; i++ {
		key := fmt.Sprintf("k-%d", i)
		want := fmt.Sprintf("payload-%d", i)
		got, err := ms.Get("race", key)
		if err != nil {
			t.Fatalf("Get %s failed: %v", key, err)
		}
		if string(got) != want {
			t.Fatalf("Get %s returned %q, want %q", key, got, want)
		}
	}
}

func TestMemoryStorage_Inspect(t *testing.T) {
	ms := newTestMemory(t)

	if got := ms.Inspect(); len(got) != 0 {
		t.Fatalf("Inspect on empty backend returned %d entries", len(got))
	}

	if err := ms.Put("b1", "k1", []byte("12345")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if err := ms.Put("b1", "k2", []byte("hello")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if err := ms.Put("b2", "only", []byte("x")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	entries := ms.Inspect()
	if len(entries) != 3 {
		t.Fatalf("Inspect returned %d entries, want 3", len(entries))
	}

	want := map[MemoryEntry]bool{
		{Bucket: "b1", Key: "k1", Size: 5}:   true,
		{Bucket: "b1", Key: "k2", Size: 5}:   true,
		{Bucket: "b2", Key: "only", Size: 1}: true,
	}
	for _, e := range entries {
		if !want[e] {
			t.Fatalf("unexpected Inspect entry: %+v", e)
		}
	}

	// Mutating the returned slice must not affect internal state.
	entries[0].Bucket = "tampered"
	again := ms.Inspect()
	for _, e := range again {
		if e.Bucket == "tampered" {
			t.Fatalf("Inspect returned a shared mutable snapshot")
		}
	}

	// After cleanup, Inspect should be empty again.
	if err := ms.RemoveBucket("b1"); err != nil {
		t.Fatalf("RemoveBucket failed: %v", err)
	}
	if err := ms.RemoveBucket("b2"); err != nil {
		t.Fatalf("RemoveBucket failed: %v", err)
	}
	if got := ms.Inspect(); len(got) != 0 {
		t.Fatalf("Inspect after cleanup returned %d entries", len(got))
	}
}
