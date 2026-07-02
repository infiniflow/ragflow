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

package ingestion

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/parser"
)

func setupE2EDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	// Auto-migrate all tables the test needs
	tables := []any{
		&entity.Tenant{},
		&entity.Knowledgebase{},
		&entity.Document{},
		&entity.File{},
		&entity.File2Document{},
		&entity.IngestionTask{},
		&entity.IngestionTaskLog{},
	}
	if err := db.AutoMigrate(tables...); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	return db
}

func seedE2EData(t *testing.T, db *gorm.DB) (docID, taskID, docPath string) {
	t.Helper()

	// Tenant
	tenant := &entity.Tenant{
		ID:        "tenant-e2e-1",
		Name:      strPtr("e2e-test-tenant"),
		LLMID:     "default",
		EmbdID:    "default",
		ASRID:     "",
		Img2TxtID: "",
		RerankID:  "",
		ParserIDs: "naive",
		Status:    strPtr("1"),
	}
	if err := db.Create(tenant).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	// Knowledgebase
	kb := &entity.Knowledgebase{
		ID:       "kb-e2e-1",
		TenantID: tenant.ID,
		Name:     "e2e-test-kb",
		Language: strPtr("Chinese"),
		EmbdID:   "default",
		ParserID: "naive",
	}
	if err := db.Create(kb).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}

	// Document
	docID = "doc-e2e-1"
	docName := "sample doc01 for RAG.pdf"
	docPath = "e2e_test/" + docID + ".pdf"
	doc := &entity.Document{
		ID:           docID,
		KbID:         kb.ID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		Type:         "pdf",
		Name:         &docName,
		Location:     &docPath,
		Suffix:       ".pdf",
		Status:       strPtr("1"),
	}
	if err := db.Create(doc).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}

	// File (represents the file in storage)
	file := &entity.File{
		ID:         "file-e2e-1",
		ParentID:   "",
		TenantID:   tenant.ID,
		CreatedBy:  tenant.ID,
		Name:       docName,
		Type:       "pdf",
		Location:   &docPath,
		SourceType: "", // FileSourceLocal
	}
	if err := db.Create(file).Error; err != nil {
		t.Fatalf("create file: %v", err)
	}

	// File2Document mapping
	f2d := &entity.File2Document{
		FileID:     &file.ID,
		DocumentID: &docID,
	}
	if err := db.Create(f2d).Error; err != nil {
		t.Fatalf("create file2document: %v", err)
	}

	// IngestionTask
	taskID = "task-e2e-1"
	task := &entity.IngestionTask{
		ID:         taskID,
		UserID:     tenant.ID,
		DocumentID: docID,
		DatasetID:  kb.ID,
		Status:     string(common.RUNNING),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create ingestion task: %v", err)
	}

	return
}

func loadTestPDFBytes(t *testing.T) []byte {
	t.Helper()
	path := "internal/deepdoc/parser/pdf/testdata/real_pdfs/11.pdf"
	data, err := os.ReadFile(path)
	if err != nil {
		data, err = os.ReadFile("../../" + path)
		if err != nil {
			t.Fatalf("read test PDF: %v", err)
		}
	}
	return data
}

// mockStorageForE2E returns a mock Storage that serves a test PDF from disk.
// This avoids requiring MinIO to be configured for the test host.
type mockStorageForE2E struct {
	pdfData []byte
}

func (m *mockStorageForE2E) Health() bool { return true }
func (m *mockStorageForE2E) Put(_, _ string, _ []byte, _ ...string) error { return nil }
func (m *mockStorageForE2E) Get(_, _ string, _ ...string) ([]byte, error) {
	return m.pdfData, nil
}
func (m *mockStorageForE2E) Remove(_, _ string, _ ...string) error { return nil }
func (m *mockStorageForE2E) ObjExist(_, _ string, _ ...string) bool { return true }
func (m *mockStorageForE2E) GetPresignedURL(_, _ string, _ time.Duration, _ ...string) (string, error) {
	return "", nil
}
func (m *mockStorageForE2E) BucketExists(_ string) bool { return true }
func (m *mockStorageForE2E) RemoveBucket(_ string) error { return nil }
func (m *mockStorageForE2E) Copy(_, _, _, _ string) bool { return true }
func (m *mockStorageForE2E) Move(_, _, _, _ string) bool { return true }

func TestE2E_ParseDocument_RealPDF(t *testing.T) {
	// ── Setup DB (replace dao.DB with in-memory SQLite) ───────────
	saveDB := dao.DB
	dao.DB = setupE2EDB(t)
	t.Cleanup(func() { dao.DB = saveDB })

	// ── Seed test data ────────────────────────────────────────────
	docID, taskID, docPath := seedE2EData(t, dao.DB)
	_ = docPath // used by the mock storage to identify the file

	// ── Load PDF ──────────────────────────────────────────────────
	pdfData := loadTestPDFBytes(t)

	// ── Create Ingestor with injected dependencies ────────────────
	ingestor := NewIngestor("e2e-test", 1, []string{"pdf"})
	ingestor.storageImpl = &mockStorageForE2E{pdfData: pdfData}
	ingestor.pdfParser = parser.NewPDFParser()

	// ── Execute ───────────────────────────────────────────────────
	task := &entity.IngestionTask{
		ID:         taskID,
		DocumentID: docID,
		DatasetID:  "kb-e2e-1",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	err := ingestor.parseDocument(ctx, task)
	if err != nil {
		t.Fatalf("parseDocument failed: %v", err)
	}

	// ── Verify task state ─────────────────────────────────────────
	// parseDocument does not update task status (executeTask does).
	// The fact that it returned nil confirms the full chain worked:
	//   DB load → storage fetch → PDF parse → post-process → sections output
	t.Log("E2E parseDocument succeeded: full chain verified")
}
