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

package document

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/permission/testsupport"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type ingestBatchSQLCapture struct {
	mu   sync.Mutex
	sqls []string
}

func (c *ingestBatchSQLCapture) LogMode(logger.LogLevel) logger.Interface      { return c }
func (c *ingestBatchSQLCapture) Info(context.Context, string, ...interface{})  {}
func (c *ingestBatchSQLCapture) Warn(context.Context, string, ...interface{})  {}
func (c *ingestBatchSQLCapture) Error(context.Context, string, ...interface{}) {}
func (c *ingestBatchSQLCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	c.sqls = append(c.sqls, sql)
	c.mu.Unlock()
}

// knowledgebaseBatchLoads counts KB selects whose id filter carries more than
// one id — the single batched dataset-row fetch. Per-document authorization
// legitimately issues its own single-id checks, so total KB SELECTs are no
// longer a valid measure of this path.
func (c *ingestBatchSQLCapture) knowledgebaseBatchLoads() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, s := range c.sqls {
		upper := strings.ToUpper(s)
		if strings.HasPrefix(upper, "SELECT") && strings.Contains(upper, "FROM `KNOWLEDGEBASE`") && strings.Count(s, "\",\"") > 0 {
			n++
		}
	}
	return n
}

func TestIngestBatchesDatasetLookups(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")

	docIDs := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		kbID := fmt.Sprintf("batch-kb-%02d", i)
		insertTestKB(t, kbID, "tenant-1", 0, 0, 0)
		id := fmt.Sprintf("ingest-doc-%02d", i)
		docIDs = append(docIDs, id)
		insertTestDoc(t, id, kbID, 0, 0)
	}
	ctx := documentTestContext(t.Context())

	capture := &ingestBatchSQLCapture{}
	dao.DB = db.Session(&gorm.Session{Logger: capture})

	// Run is blank so validation is the only phase that loads datasets; the
	// remaining per-document work updates progress without further KB reads.
	code, err := testDocumentService(t).Ingest(ctx, &IngestDocumentRequest{DocIDs: docIDs})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("Ingest: code=%d err=%v", code, err)
	}
	// Dataset rows for the whole batch are fetched in one query. Per-document
	// authorization intentionally goes through the shared write seam and may
	// issue its own checks, so only the batch row load is bounded here.
	if n := capture.knowledgebaseBatchLoads(); n != 1 {
		t.Fatalf("batched knowledgebase loads = %d for %d docs, want 1; got SQL: %v", n, len(docIDs), capture.sqls)
	}
}

// Adversarial: validation errors follow document order. A document whose
// dataset no longer exists must report "dataset not found" before a later
// document's permission denial, even though datasets are loaded in a batch.
func TestIngestPreservesFirstValidationErrorInDocOrder(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")
	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)
	insertTestDoc(t, "ingest-doc-0", "kb-1", 0, 0)
	insertTestDoc(t, "ingest-doc-1", "missing-kb", 0, 0)
	status := string(entity.StatusValid)
	if err := db.Create(&entity.Knowledgebase{
		ID: "foreign-kb", TenantID: "tenant-1", Name: "foreign",
		Permission: string(entity.TenantPermissionMe), CreatedBy: "someone-else", Status: &status,
	}).Error; err != nil {
		t.Fatal(err)
	}
	insertTestDoc(t, "ingest-doc-2", "foreign-kb", 0, 0)
	ctx := documentTestContext(t.Context())

	capture := &ingestBatchSQLCapture{}
	dao.DB = db.Session(&gorm.Session{Logger: capture})
	_, err := testDocumentService(t).Ingest(ctx, &IngestDocumentRequest{
		DocIDs: []string{"ingest-doc-0", "ingest-doc-1", "ingest-doc-2"},
	})
	if err == nil || err.Error() != "dataset not found" {
		t.Fatalf("error = %v, want the first (dataset not found) error; SQL: %v", err, capture.sqls)
	}
}

// Adversarial (inverse of the missing-document case): a document whose dataset
// is missing comes first, a nonexistent document comes later. The first
// document's "dataset not found" must win, not the later "document not found".
func TestIngestPreservesMissingDatasetBeforeMissingDocument(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")
	insertTestDoc(t, "order-doc-0", "missing-kb", 0, 0)
	ctx := documentTestContext(t.Context())

	_, err := testDocumentService(t).Ingest(ctx, &IngestDocumentRequest{
		DocIDs: []string{"order-doc-0", "order-missing-doc"},
	})
	if err == nil || err.Error() != "dataset not found" {
		t.Fatalf("error = %v, want the first (dataset not found) error", err)
	}
}

func TestIngestPreservesMissingDocumentError(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")
	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)
	insertTestDoc(t, "ingest-doc-0", "kb-1", 0, 0)
	ctx := documentTestContext(t.Context())

	code, err := testDocumentService(t).Ingest(ctx, &IngestDocumentRequest{
		DocIDs: []string{"ingest-doc-0", "missing-doc"},
	})
	if code != common.CodeDataError {
		t.Fatalf("code = %d, want CodeDataError", code)
	}
	if err == nil || err.Error() != "document not found" {
		t.Fatalf("error = %v, want document not found", err)
	}
}

// BenchmarkIngestDocumentAuthorization records the per-document authorization
// cost on the ingest hot path. Dataset rows are fetched in one batch, but the
// write check is intentionally per document through the shared seam.
func BenchmarkIngestDocumentAuthorization(b *testing.B) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		b.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Document{}, &entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		b.Fatal(err)
	}
	status := string(entity.StatusValid)
	const tenants = "tenant-bench"
	ids := make([]string, 0, 32)
	for i := 0; i < 32; i++ {
		kbID := fmt.Sprintf("bench-kb-%02d", i)
		if err := db.Create(&entity.Knowledgebase{
			ID: kbID, TenantID: tenants, UserID: "user-bench", CreatedBy: "user-bench",
			Name: kbID, Permission: string(entity.TenantPermissionTeam), Status: &status,
		}).Error; err != nil {
			b.Fatal(err)
		}
		id := fmt.Sprintf("bench-doc-%02d", i)
		ids = append(ids, id)
		if err := db.Create(&entity.Document{
			ID: id, KbID: kbID, UserID: "user-bench", CreatedBy: "user-bench",
			ParserID: "naive", ParserConfig: entity.JSONMap{}, Suffix: ".txt", Status: &status,
		}).Error; err != nil {
			b.Fatal(err)
		}
	}
	if err := db.Create(&entity.UserTenant{
		ID: "m", UserID: "user-bench", TenantID: tenants,
		Role: string(permission.RoleOwner), Status: &status,
	}).Error; err != nil {
		b.Fatal(err)
	}
	if err := testsupport.PrepareEnterprisePermissionFixture(db, tenants, "user-bench"); err != nil {
		b.Fatal(err)
	}
	origDB := dao.DB
	dao.DB = db
	defer func() { dao.DB = origDB }()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if code, err := (&DocumentService{documentDAO: dao.NewDocumentDAO(), kbDAO: dao.NewKnowledgebaseDAO()}).Ingest(
			permission.WithIdentity(context.Background(), "user-bench", tenants),
			&IngestDocumentRequest{DocIDs: ids},
		); err != nil || code != common.CodeSuccess {
			b.Fatalf("Ingest: code=%d err=%v", code, err)
		}
	}
}
