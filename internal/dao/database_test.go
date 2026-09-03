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

package dao

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAutoMigrateSafelyCreatesIngestionTaskTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// Verify tables do not exist initially
	if db.Migrator().HasTable(&entity.IngestionTask{}) {
		t.Fatal("expected ingestion_task to not exist initially")
	}
	if db.Migrator().HasTable(&entity.IngestionTaskLog{}) {
		t.Fatal("expected ingestion_task_log to not exist initially")
	}

	ctx := context.Background()
	for _, m := range []interface{}{&entity.IngestionTask{}, &entity.IngestionTaskLog{}} {
		if err = autoMigrateSafely(ctx, db, m); err != nil {
			t.Fatalf("autoMigrateSafely failed for %T: %v", m, err)
		}
	}

	// Verify tables exist after auto migration
	if !db.Migrator().HasTable(&entity.IngestionTask{}) {
		t.Fatal("expected ingestion_task to exist after autoMigrateSafely")
	}
	if !db.Migrator().HasTable(&entity.IngestionTaskLog{}) {
		t.Fatal("expected ingestion_task_log to exist after autoMigrateSafely")
	}

	// Verify idempotency
	for _, m := range []interface{}{&entity.IngestionTask{}, &entity.IngestionTaskLog{}} {
		if err = autoMigrateSafely(ctx, db, m); err != nil {
			t.Fatalf("second autoMigrateSafely failed for %T: %v", m, err)
		}
	}
}

func TestLoadTemplatesFromDirErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Valid JSON template
	validPath := filepath.Join(tmpDir, "valid.json")
	validContent := `{"id": "test_1", "title": {"en": "Test"}, "description": {"en": "Desc"}}`
	if err := os.WriteFile(validPath, []byte(validContent), 0644); err != nil {
		t.Fatalf("write valid file: %v", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("read tmp dir: %v", err)
	}
	tmpls, ids, err := loadTemplatesFromDir(tmpDir, entries)
	if err != nil {
		t.Fatalf("unexpected error loading valid template: %v", err)
	}
	if len(tmpls) != 1 || ids[0] != "test_1" {
		t.Fatalf("expected 1 template with id test_1, got %v", ids)
	}

	// 2. Corrupt JSON template
	corruptPath := filepath.Join(tmpDir, "corrupt.json")
	if err := os.WriteFile(corruptPath, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	entries, err = os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("read tmp dir: %v", err)
	}
	_, _, err = loadTemplatesFromDir(tmpDir, entries)
	if err == nil {
		t.Fatal("expected error loading corrupt template, got nil")
	}
	_ = os.Remove(corruptPath)

	// 3. Template with missing/empty ID
	emptyIDPath := filepath.Join(tmpDir, "empty_id.json")
	if err := os.WriteFile(emptyIDPath, []byte(`{"title": {"en": "No ID"}}`), 0644); err != nil {
		t.Fatalf("write empty ID file: %v", err)
	}
	entries, err = os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("read tmp dir: %v", err)
	}
	_, _, err = loadTemplatesFromDir(tmpDir, entries)
	if err == nil {
		t.Fatal("expected error loading template with missing ID, got nil")
	}
	_ = os.Remove(emptyIDPath)

	// 4. Template with trailing data
	trailingPath := filepath.Join(tmpDir, "trailing.json")
	if err := os.WriteFile(trailingPath, []byte(`{"id": "t1"} trailing content`), 0644); err != nil {
		t.Fatalf("write trailing file: %v", err)
	}
	entries, err = os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("read tmp dir: %v", err)
	}
	_, _, err = loadTemplatesFromDir(tmpDir, entries)
	if err == nil {
		t.Fatal("expected error loading template with trailing content, got nil")
	}
}

func TestParseCanvasTemplateFileRejectsInvalidIDTypes(t *testing.T) {
	for _, raw := range []string{
		`{"id": null}`,
		`{"id": true}`,
		`{"id": {"value": "template"}}`,
		`{"id": ["template"]}`,
	} {
		if _, err := parseCanvasTemplateFile([]byte(raw)); err == nil {
			t.Fatalf("parseCanvasTemplateFile(%s) succeeded for an invalid ID type", raw)
		}
	}
}

func TestParseCanvasTemplateFileAcceptsNumericID(t *testing.T) {
	tmpl, err := parseCanvasTemplateFile([]byte(`{"id": 45}`))
	if err != nil {
		t.Fatalf("parseCanvasTemplateFile returned error for numeric ID: %v", err)
	}
	if tmpl.ID != "45" {
		t.Fatalf("template ID = %q, want 45", tmpl.ID)
	}
}
