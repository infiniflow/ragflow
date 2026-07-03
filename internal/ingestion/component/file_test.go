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

package component

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// withMemoryStorage swaps the global storage factory for an
// in-memory implementation and restores the previous backend on
// cleanup. Mirrors the pattern in internal/service/file_test.go:80-83.
func withMemoryStorage(t *testing.T) *storage.MemoryStorage {
	t.Helper()
	factory := storage.GetStorageFactory()
	prev := factory.GetStorage()
	ms := storage.NewMemoryStorage().(*storage.MemoryStorage)
	factory.SetStorage(ms)
	t.Cleanup(func() { factory.SetStorage(prev) })
	return ms
}

func withFileComponentTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Document{}, &entity.File{}, &entity.File2Document{}); err != nil {
		t.Fatalf("failed to migrate sqlite: %v", err)
	}
	prev := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = prev })
	return db
}

// TestFileComponent_Registered verifies the init() registration
// is visible to the runtime registry (Phase 4 / API layer
// depends on this).
func TestFileComponent_Registered(t *testing.T) {
	factory, cat, md, ok := runtime.DefaultRegistry.Lookup("File")
	if !ok {
		t.Fatal("File not registered in runtime.DefaultRegistry")
	}
	if cat != runtime.CategoryIngestion {
		t.Errorf("category = %q, want %q", cat, runtime.CategoryIngestion)
	}
	if factory == nil {
		t.Error("factory is nil")
	}
	if md.Inputs == nil || len(md.Inputs) == 0 {
		t.Errorf("metadata.Inputs empty: %v", md.Inputs)
	}
	if md.Outputs == nil || len(md.Outputs) == 0 {
		t.Errorf("metadata.Outputs empty: %v", md.Outputs)
	}
}

// TestFileComponent_Invoke_HappyPath pre-loads memory storage
// with bytes, invokes the component, and verifies the binary
// output base64-decodes to the original bytes.
func TestFileComponent_Invoke_HappyPath(t *testing.T) {
	ms := withMemoryStorage(t)
	want := []byte("hello, ragflow")
	if err := ms.Put("bucketA", "path/to/file.txt", want); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c := &FileComponent{}
	out, err := c.Invoke(context.Background(), map[string]any{
		"doc_id": "doc-1",
		"bucket": "bucketA",
		"path":   "path/to/file.txt",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	enc, ok := out["binary"].(string)
	if !ok || enc == "" {
		t.Fatalf("binary not produced: %v", out["binary"])
	}
	got, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("binary = %q, want %q", got, want)
	}
	if out["name"] != "doc-1" {
		t.Errorf("name = %v, want doc-1", out["name"])
	}
	if out["bucket"] != "bucketA" {
		t.Errorf("bucket = %v, want bucketA", out["bucket"])
	}
	if out["path"] != "path/to/file.txt" {
		t.Errorf("path = %v, want path/to/file.txt", out["path"])
	}
	if out["_elapsed_time"] == nil {
		t.Error("_elapsed_time missing")
	}
	if out["_created_time"] == nil {
		t.Error("_created_time missing")
	}
}

func TestFileComponent_Invoke_ResolvesDocIDViaDocumentLocation(t *testing.T) {
	ms := withMemoryStorage(t)
	db := withFileComponentTestDB(t)
	location := "docs/from-document.bin"
	if err := ms.Put("kb-doc", location, []byte("doc-location")); err != nil {
		t.Fatalf("seed storage: %v", err)
	}
	docName := "report.pdf"
	if err := db.Create(&entity.Document{
		ID:           "doc-loc",
		KbID:         "kb-doc",
		ParserID:     "na",
		ParserConfig: entity.JSONMap{},
		Type:         "pdf",
		CreatedBy:    "u1",
		Name:         &docName,
		Location:     &location,
		Suffix:       ".pdf",
	}).Error; err != nil {
		t.Fatalf("seed doc: %v", err)
	}

	c := &FileComponent{}
	out, err := c.Invoke(context.Background(), map[string]any{"doc_id": "doc-loc"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, err := base64.StdEncoding.DecodeString(out["binary"].(string))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got) != "doc-location" {
		t.Fatalf("binary = %q, want %q", got, "doc-location")
	}
	if out["name"] != "report.pdf" {
		t.Fatalf("name = %v, want report.pdf", out["name"])
	}
	if out["bucket"] != "kb-doc" || out["path"] != location {
		t.Fatalf("bucket/path = %v/%v, want kb-doc/%s", out["bucket"], out["path"], location)
	}
}

func TestFileComponent_Invoke_ResolvesDocIDViaFileMapping(t *testing.T) {
	ms := withMemoryStorage(t)
	db := withFileComponentTestDB(t)
	location := "tenant-root/from-file.bin"
	if err := ms.Put("folder-1", location, []byte("file-mapping")); err != nil {
		t.Fatalf("seed storage: %v", err)
	}
	docName := "deck.pptx"
	if err := db.Create(&entity.Document{
		ID:           "doc-file",
		KbID:         "kb-1",
		ParserID:     "na",
		ParserConfig: entity.JSONMap{},
		Type:         "ppt",
		CreatedBy:    "u1",
		Name:         &docName,
		Suffix:       ".pptx",
	}).Error; err != nil {
		t.Fatalf("seed doc: %v", err)
	}
	if err := db.Create(&entity.File{
		ID:        "file-1",
		ParentID:  "folder-1",
		TenantID:  "tenant-1",
		CreatedBy: "u1",
		Name:      "deck.pptx",
		Location:  &location,
		Type:      "file",
	}).Error; err != nil {
		t.Fatalf("seed file: %v", err)
	}
	fileID := "file-1"
	docID := "doc-file"
	if err := db.Create(&entity.File2Document{
		ID:         "map-1",
		FileID:     &fileID,
		DocumentID: &docID,
	}).Error; err != nil {
		t.Fatalf("seed mapping: %v", err)
	}

	c := &FileComponent{}
	out, err := c.Invoke(context.Background(), map[string]any{"doc_id": "doc-file"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, err := base64.StdEncoding.DecodeString(out["binary"].(string))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(got) != "file-mapping" {
		t.Fatalf("binary = %q, want %q", got, "file-mapping")
	}
	if out["bucket"] != "folder-1" || out["path"] != location {
		t.Fatalf("bucket/path = %v/%v, want folder-1/%s", out["bucket"], out["path"], location)
	}
}

func TestFileComponent_Invoke_DocIDWithoutStorageLocationFails(t *testing.T) {
	withMemoryStorage(t)
	db := withFileComponentTestDB(t)
	if err := db.Create(&entity.Document{
		ID:           "doc-empty",
		KbID:         "kb-1",
		ParserID:     "na",
		ParserConfig: entity.JSONMap{},
		Type:         "txt",
		CreatedBy:    "u1",
		Suffix:       ".txt",
	}).Error; err != nil {
		t.Fatalf("seed doc: %v", err)
	}

	c := &FileComponent{}
	_, err := c.Invoke(context.Background(), map[string]any{"doc_id": "doc-empty"})
	if err == nil {
		t.Fatal("expected error for doc without storage location")
	}
	if !strings.Contains(err.Error(), "document location is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestFileComponent_Invoke_MissingDoc covers the input-validation
// branch when neither doc_id nor files is supplied.
func TestFileComponent_Invoke_MissingDoc(t *testing.T) {
	withMemoryStorage(t)
	c := &FileComponent{}
	_, err := c.Invoke(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty inputs, got nil")
	}
	if !strings.Contains(err.Error(), "doc_id") {
		t.Errorf("error should mention doc_id/files: %v", err)
	}
}

// slowStorage blocks Get() until either the channel is closed
// (test releases the lock) or the context expires. Used to
// exercise the timeout path in TestFileComponent_Invoke_HonorsTimeout.
type slowStorage struct {
	release chan struct{}
	calls   atomic.Int32
}

func (s *slowStorage) Health() bool                                { return true }
func (s *slowStorage) Put(string, string, []byte, ...string) error { return nil }
func (s *slowStorage) Remove(string, string, ...string) error      { return nil }
func (s *slowStorage) ObjExist(string, string, ...string) bool     { return true }
func (s *slowStorage) GetPresignedURL(string, string, time.Duration, ...string) (string, error) {
	return "", nil
}
func (s *slowStorage) BucketExists(string) bool                 { return true }
func (s *slowStorage) RemoveBucket(string) error                { return nil }
func (s *slowStorage) Copy(string, string, string, string) bool { return false }
func (s *slowStorage) Move(string, string, string, string) bool { return false }

func (s *slowStorage) Get(bucket, fnm string, _ ...string) ([]byte, error) {
	s.calls.Add(1)
	// Block on release; tests close the channel in t.Cleanup so
	// the worker goroutine never leaks. The component's fetchBinary
	// races this against ctx.Done() — when the test shrinks
	// fileFetchTimeout below the test ctx bound, ctx.Done() wins
	// and the worker is left waiting here (then released on cleanup).
	<-s.release
	return []byte("late"), nil
}

// TestFileComponent_Invoke_StorageError covers the wrap of an
// upstream storage-layer error: empty memory storage means
// bucket/path is missing, so Get returns ErrMemoryNotFound; the
// component should wrap it with the "file: ..." prefix.
func TestFileComponent_Invoke_StorageError(t *testing.T) {
	withMemoryStorage(t) // empty memory storage
	c := &FileComponent{}
	_, err := c.Invoke(context.Background(), map[string]any{
		"doc_id": "doc-x",
		"bucket": "empty-bucket",
		"path":   "missing.txt",
	})
	if err == nil {
		t.Fatal("expected error from empty storage, got nil")
	}
	if !strings.HasPrefix(err.Error(), "file:") {
		t.Errorf("error should be wrapped with 'file:' prefix, got %v", err)
	}
	if !errors.Is(err, storage.ErrMemoryNotFound) {
		t.Errorf("expected chain to include ErrMemoryNotFound, got %v", err)
	}
}

// TestFileComponent_Invoke_IncludesCheckpointPath verifies the
// path output is the same key the storage layer is queried with,
// so downstream components / the materialized-boundary
// checkpoint can re-resolve the binary without rerunning File.
func TestFileComponent_Invoke_IncludesCheckpointPath(t *testing.T) {
	ms := withMemoryStorage(t)
	const wantPath = "checkpoint/expected/path.bin"
	if err := ms.Put("b", wantPath, []byte("x")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	c := &FileComponent{}
	out, err := c.Invoke(context.Background(), map[string]any{
		"doc_id": "doc-1",
		"bucket": "b",
		"path":   wantPath,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out["path"] != wantPath {
		t.Errorf("path = %v, want %q", out["path"], wantPath)
	}
	if out["bucket"] != "b" {
		t.Errorf("bucket = %v, want b", out["bucket"])
	}
}

// TestFileComponent_Invoke_HonorsTimeout installs a storage
// that blocks well past the (test-shrunk) fileFetchTimeout, then
// asserts the component returns context.DeadlineExceeded. The
// timeout variable is restored on cleanup so other tests see the
// production value.
func TestFileComponent_Invoke_HonorsTimeout(t *testing.T) {
	prev := fileFetchTimeout
	fileFetchTimeout = 50 * time.Millisecond
	t.Cleanup(func() { fileFetchTimeout = prev })

	slow := &slowStorage{release: make(chan struct{})}
	t.Cleanup(func() { close(slow.release) }) // unblock goroutine on cleanup
	factory := storage.GetStorageFactory()
	prevStorage := factory.GetStorage()
	factory.SetStorage(slow)
	t.Cleanup(func() { factory.SetStorage(prevStorage) })

	c := &FileComponent{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Invoke(ctx, map[string]any{
		"doc_id": "doc-slow",
		"bucket": "b",
		"path":   "p",
	})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

// TestFileComponent_InputsOutputs_NonEmpty is the shape
// assertion Phase 4's API endpoint relies on.
func TestFileComponent_InputsOutputs_NonEmpty(t *testing.T) {
	c := &FileComponent{}
	ins := c.Inputs()
	outs := c.Outputs()
	if len(ins) == 0 {
		t.Error("Inputs() returned empty map")
	}
	if len(outs) == 0 {
		t.Error("Outputs() returned empty map")
	}
	for _, key := range []string{"binary", "name"} {
		if _, ok := outs[key]; !ok {
			t.Errorf("Outputs() missing %q", key)
		}
	}
}

// TestFileComponent_Parallelism asserts the fan-out is locked to
// 1 — File is a single MinIO fetch and the plan §AD-5a confirms
// this is intentional.
func TestFileComponent_Parallelism(t *testing.T) {
	c := &FileComponent{}
	if got := c.Parallelism(); got != 1 {
		t.Errorf("Parallelism() = %d, want 1", got)
	}
}
