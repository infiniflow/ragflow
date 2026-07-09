//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package testutil

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ──────────────────────────────────────────────────────────
// Simple Helper Functions
// ──────────────────────────────────────────────────────────

// StrPtr returns a pointer to the given string.
func StrPtr(s string) *string {
	return &s
}

// ──────────────────────────────────────────────────────────
// Database Helpers
// ──────────────────────────────────────────────────────────

// SetupTestDB sets up an in-memory SQLite database for testing.
// It auto-migrates the given tables (or all common tables if none provided).
func SetupTestDB(t *testing.T, tables ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if len(tables) == 0 {
		tables = []any{
			&entity.IngestionTask{},
			&entity.IngestionTaskLog{},
			&entity.Task{},
			&entity.Document{},
			&entity.Knowledgebase{},
			&entity.Tenant{},
			&entity.File{},
			&entity.File2Document{},
		}
	}

	if err := db.AutoMigrate(tables...); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	return db
}

// ReplaceDBForTest replaces dao.DB with the given test database
// and returns a cleanup function to restore the original.
func ReplaceDBForTest(t *testing.T, db *gorm.DB) func() {
	t.Helper()
	origDB := dao.DB
	dao.DB = db
	return func() { dao.DB = origDB }
}

// ──────────────────────────────────────────────────────────
// Test Data Configuration (Options Pattern)
// ──────────────────────────────────────────────────────────

// TestDataOption configures the test data setup.
type TestDataOption func(*testDataConfig)

type testDataConfig struct {
	tenantID   string
	kbID       string
	docID      string
	taskID     string
	pipelineID *string
	docName    string
	docType    string
}

// WithTenantID sets the tenant ID for test data.
func WithTenantID(id string) TestDataOption {
	return func(c *testDataConfig) { c.tenantID = id }
}

// WithKBID sets the knowledgebase ID for test data.
func WithKBID(id string) TestDataOption {
	return func(c *testDataConfig) { c.kbID = id }
}

// WithDocID sets the document ID for test data.
func WithDocID(id string) TestDataOption {
	return func(c *testDataConfig) { c.docID = id }
}

// WithTaskID sets the task ID for test data.
func WithTaskID(id string) TestDataOption {
	return func(c *testDataConfig) { c.taskID = id }
}

// WithPipelineID sets the pipeline ID for test data.
func WithPipelineID(id string) TestDataOption {
	return func(c *testDataConfig) {
		pid := id
		c.pipelineID = &pid
	}
}

// WithDocName sets the document name for test data.
func WithDocName(name string) TestDataOption {
	return func(c *testDataConfig) { c.docName = name }
}

// WithDocType sets the document type for test data.
func WithDocType(docType string) TestDataOption {
	return func(c *testDataConfig) { c.docType = docType }
}

// SeedTestData seeds the database with test data.
// Returns: tenantID, kbID, docID, taskID
func SeedTestData(t *testing.T, db *gorm.DB, opts ...TestDataOption) (string, string, string, string) {
	t.Helper()
	cfg := &testDataConfig{
		tenantID: "tenant-1",
		kbID:     "kb-1",
		docID:    "doc-1",
		taskID:   "task-1",
		docName:  "test.pdf",
		docType:  "pdf",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// Create Tenant
	if err := db.Create(&entity.Tenant{
		ID:     cfg.tenantID,
		LLMID:  "gpt-4",
		Status: StrPtr("1"),
	}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	// Create Knowledgebase
	if err := db.Create(&entity.Knowledgebase{
		ID:           cfg.kbID,
		TenantID:     cfg.tenantID,
		EmbdID:       "embd-1",
		Status:       StrPtr("1"),
		ParserConfig: entity.JSONMap{},
	}).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}

	// Create Document
	doc := &entity.Document{
		ID:           cfg.docID,
		KbID:         cfg.kbID,
		Name:         &cfg.docName,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		PipelineID:   cfg.pipelineID,
		Status:       StrPtr("1"),
		Type:         cfg.docType,
	}
	loc := "doc_store/" + cfg.docID
	doc.Location = &loc
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("create doc: %v", err)
	}

	// Create Task
	if err := db.Create(&entity.Task{
		ID:       cfg.taskID,
		DocID:    cfg.docID,
		TaskType: "dataflow",
		FromPage: 0,
		ToPage:   100000,
	}).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Create IngestionTask
	if err := db.Create(&entity.IngestionTask{
		ID:         cfg.taskID,
		UserID:     "u1",
		DocumentID: cfg.docID,
		DatasetID:  cfg.kbID,
		Status:     common.RUNNING,
	}).Error; err != nil {
		t.Fatalf("create ingestion task: %v", err)
	}

	return cfg.tenantID, cfg.kbID, cfg.docID, cfg.taskID
}

// ──────────────────────────────────────────────────────────
// Test Entity Helpers
// ──────────────────────────────────────────────────────────

// TestTask creates a test IngestionTask.
func TestTask(docID string) *entity.IngestionTask {
	return &entity.IngestionTask{
		ID:         "task-1",
		DocumentID: docID,
		DatasetID:  "kb-1",
	}
}

// TestDoc creates a test Document.
func TestDoc(id string, docType string, suffix string, kbID ...string) *entity.Document {
	name := "test." + suffix
	loc := "doc_store/" + id
	kb := "kb-1"
	if len(kbID) > 0 {
		kb = kbID[0]
	}
	return &entity.Document{
		ID:       id,
		KbID:     kb,
		Type:     docType,
		Suffix:   suffix,
		Name:     &name,
		Location: &loc,
		ParserID: "naive",
	}
}
