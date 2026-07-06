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
	"strings"
	"testing"

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

// TestFileComponent_Invoke_HappyPath verifies File remains metadata-only
// even when explicit storage overrides are supplied.
func TestFileComponent_Invoke_HappyPath(t *testing.T) {
	ms := withMemoryStorage(t)
	if err := ms.Put("bucketA", "path/to/file.txt", []byte("hello, ragflow")); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c := &FileComponent{}
	out, err := c.Invoke(context.Background(), map[string]any{
		"file":   []map[string]any{{"name": "file.txt"}},
		"bucket": "bucketA",
		"path":   "path/to/file.txt",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	if got := out["name"]; got != "file.txt" {
		t.Errorf("name = %v, want file.txt", got)
	}
	if out["bucket"] != "bucketA" {
		t.Errorf("bucket = %v, want bucketA", out["bucket"])
	}
	if out["path"] != "path/to/file.txt" {
		t.Errorf("path = %v, want path/to/file.txt", out["path"])
	}
	if _, ok := out["binary"]; ok {
		t.Fatalf("binary should not be emitted by File: %v", out["binary"])
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
	if out["name"] != "report.pdf" {
		t.Fatalf("name = %v, want report.pdf", out["name"])
	}
	if _, ok := out["bucket"]; ok {
		t.Fatalf("bucket should not be emitted for doc_id-only path: %v", out["bucket"])
	}
	if _, ok := out["path"]; ok {
		t.Fatalf("path should not be emitted for doc_id-only path: %v", out["path"])
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
	if out["name"] != "deck.pptx" {
		t.Fatalf("name = %v, want deck.pptx", out["name"])
	}
	if _, ok := out["bucket"]; ok {
		t.Fatalf("bucket should not be emitted for doc_id-only path: %v", out["bucket"])
	}
	if _, ok := out["path"]; ok {
		t.Fatalf("path should not be emitted for doc_id-only path: %v", out["path"])
	}
}

func TestFileComponent_Invoke_DocIDWithoutStorageLocationStillSucceeds(t *testing.T) {
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
	out, err := c.Invoke(context.Background(), map[string]any{"doc_id": "doc-empty"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["name"]; got != "doc-empty" {
		t.Fatalf("name = %v, want doc-empty", got)
	}
}

// TestFileComponent_Invoke_MissingDoc covers the input-validation
// branch when neither doc_id nor file is supplied.
func TestFileComponent_Invoke_MissingDoc(t *testing.T) {
	withMemoryStorage(t)
	c := &FileComponent{}
	_, err := c.Invoke(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for empty inputs, got nil")
	}
	if !strings.Contains(err.Error(), "doc_id") {
		t.Errorf("error should mention doc_id/file: %v", err)
	}
}

// TestFileComponent_Invoke_EchoesStorageOverride verifies explicit
// bucket/path overrides still flow through for downstream Parser use.
func TestFileComponent_Invoke_IncludesCheckpointPath(t *testing.T) {
	ms := withMemoryStorage(t)
	const wantPath = "checkpoint/expected/path.bin"
	if err := ms.Put("b", wantPath, []byte("x")); err != nil {
		t.Fatalf("seed: %v", err)
	}
	c := &FileComponent{}
	out, err := c.Invoke(context.Background(), map[string]any{
		"file":   []map[string]any{{"name": "checkpoint.bin"}},
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
	if _, ok := out["binary"]; ok {
		t.Fatalf("binary should not be emitted by File: %v", out["binary"])
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
	for _, key := range []string{"name", "file"} {
		if _, ok := outs[key]; !ok {
			t.Errorf("Outputs() missing %q", key)
		}
	}
}

// TestFileComponent_Parallelism asserts the fan-out is locked to
// 1 — File is metadata-only and intentionally non-fanned-out.
func TestFileComponent_Parallelism(t *testing.T) {
	c := &FileComponent{}
	if got := c.Parallelism(); got != 1 {
		t.Errorf("Parallelism() = %d, want 1", got)
	}
}
