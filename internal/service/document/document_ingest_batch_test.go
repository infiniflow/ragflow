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

func (c *ingestBatchSQLCapture) knowledgebaseSelects() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, s := range c.sqls {
		upper := strings.ToUpper(s)
		if strings.HasPrefix(upper, "SELECT") && strings.Contains(upper, "FROM `KNOWLEDGEBASE`") {
			n++
		}
	}
	return n
}

func TestIngestBatchesDatasetLookups(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")
	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)

	docIDs := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("ingest-doc-%02d", i)
		docIDs = append(docIDs, id)
		insertTestDoc(t, id, "kb-1", 0, 0)
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
	// One permission batch load and one lookup for the single shared KB.
	if n := capture.knowledgebaseSelects(); n > 2 {
		t.Fatalf("knowledgebase SELECTs = %d for %d docs in one KB, want <= 2 (batch); got SQL: %v", n, len(docIDs), capture.sqls)
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
