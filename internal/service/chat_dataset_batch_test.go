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
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/permission/testsupport"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// batchSQLCapture records every statement GORM executes so the test can assert
// that dataset authorization and loading use a bounded number of round-trips
// instead of one query per dataset (N+1).
type batchSQLCapture struct {
	mu   sync.Mutex
	sqls []string
}

func (c *batchSQLCapture) LogMode(logger.LogLevel) logger.Interface      { return c }
func (c *batchSQLCapture) Info(context.Context, string, ...interface{})  {}
func (c *batchSQLCapture) Warn(context.Context, string, ...interface{})  {}
func (c *batchSQLCapture) Error(context.Context, string, ...interface{}) {}
func (c *batchSQLCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	c.sqls = append(c.sqls, sql)
	c.mu.Unlock()
}

func (c *batchSQLCapture) knowledgebaseSelects() int {
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

// seedChatBatchKBs inserts count datasets owned by tenantID, all with parsed
// chunks so validation succeeds, and returns their IDs in insertion order.
func seedChatBatchKBs(t *testing.T, db *gorm.DB, tenantID string, count int) []string {
	t.Helper()
	status := string(entity.StatusValid)
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("batch-kb-%02d", i)
		ids = append(ids, id)
		if err := db.Create(&entity.Knowledgebase{
			ID: id, TenantID: tenantID, CreatedBy: tenantID,
			Name: id, ChunkNum: 1, Permission: string(entity.TenantPermissionMe),
			Status: &status,
		}).Error; err != nil {
			t.Fatalf("seed knowledgebase %s: %v", id, err)
		}
	}
	return ids
}

func TestValidateCreateDatasetIDsBatchesDatasetQueries(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	ctx := serviceTestContext(t, "u-batch", "u-batch")
	ids := seedChatBatchKBs(t, db, "u-batch", 5)

	capture := &batchSQLCapture{}
	dao.DB = db.Session(&gorm.Session{Logger: capture})

	got, err := NewChatService().validateCreateDatasetIDs(ctx, toDatasetAnySlice(ids))
	if err != nil {
		t.Fatalf("validateCreateDatasetIDs: %v", err)
	}
	if len(got) != len(ids) {
		t.Fatalf("dataset count = %d, want %d", len(got), len(ids))
	}
	for i := range ids {
		if got[i] != ids[i] {
			t.Fatalf("dataset[%d] = %q, want %q (order must be preserved)", i, got[i], ids[i])
		}
	}

	// Authorization loads the datasets once and the service loads them once:
	// two dataset queries regardless of how many datasets are requested.
	if n := capture.knowledgebaseSelects(); n > 2 {
		t.Fatalf("knowledgebase SELECTs = %d for %d datasets, want <= 2 (batch); got SQL: %v", n, len(ids), capture.sqls)
	}
}

func TestValidateCreateDatasetIDsBatchesRESTDatasetQueries(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	ctx := serviceTestContext(t, "u-batch", "u-batch")
	ids := seedChatBatchKBs(t, db, "u-batch", 5)

	capture := &batchSQLCapture{}
	dao.DB = db.Session(&gorm.Session{Logger: capture})

	got, err := NewChatService().validateRESTDatasetIDs(ctx, toDatasetAnySlice(ids))
	if err != nil {
		t.Fatalf("validateRESTDatasetIDs: %v", err)
	}
	if len(got) != len(ids) {
		t.Fatalf("dataset count = %d, want %d", len(got), len(ids))
	}
	for i := range ids {
		if got[i] != ids[i] {
			t.Fatalf("dataset[%d] = %q, want %q (order must be preserved)", i, got[i], ids[i])
		}
	}
	if n := capture.knowledgebaseSelects(); n > 2 {
		t.Fatalf("knowledgebase SELECTs = %d for %d datasets, want <= 2 (batch); got SQL: %v", n, len(ids), capture.sqls)
	}
}

// Adversarial: a returned dataset with no parsed chunks must still produce the
// exact legacy error, even though datasets are now loaded in one batch.
func TestValidateCreateDatasetIDsPreservesEmptyDatasetError(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	ctx := serviceTestContext(t, "u-batch", "u-batch")
	ids := seedChatBatchKBs(t, db, "u-batch", 3)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", ids[1]).Update("chunk_num", 0).Error; err != nil {
		t.Fatal(err)
	}

	_, err := NewChatService().validateCreateDatasetIDs(ctx, toDatasetAnySlice(ids))
	if err == nil {
		t.Fatal("expected error for dataset without parsed chunks")
	}
	want := fmt.Sprintf("the dataset %s doesn't own parsed file", ids[1])
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

// Adversarial: authorization errors win over later validation errors in
// request order. A denied dataset at index 1 must be reported before an
// unparsed dataset at index 2, even though datasets are now loaded in a batch.
func TestValidateCreateDatasetIDsPreservesFirstDeniedError(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	ctx := serviceTestContext(t, "u-batch", "u-batch")
	ids := seedChatBatchKBs(t, db, "u-batch", 3)
	// A dataset in a foreign tenant is not accessible to u-batch.
	status := string(entity.StatusValid)
	foreign := "foreign-batch-kb"
	if err := db.Create(&entity.Knowledgebase{
		ID: foreign, TenantID: "other-tenant", CreatedBy: "other-tenant",
		Name: foreign, ChunkNum: 1, Permission: string(entity.TenantPermissionMe),
		Status: &status,
	}).Error; err != nil {
		t.Fatal(err)
	}
	// A later dataset has no parsed chunks; its error must not mask the denial.
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", ids[1]).Update("chunk_num", 0).Error; err != nil {
		t.Fatal(err)
	}

	_, err := NewChatService().validateCreateDatasetIDs(ctx, toDatasetAnySlice([]string{ids[0], foreign, ids[1]}))
	if err == nil {
		t.Fatal("expected permission error for foreign dataset")
	}
	if err.Error() != "Permission check failed" {
		t.Fatalf("error = %q, want the first (permission) error, not a later validation error", err.Error())
	}
}

// BenchmarkValidateCreateDatasetIDsBatches guards the hot authorization path:
// ns/op should stay flat as the dataset count grows, because the batch loads
// datasets once regardless of how many are requested.
func BenchmarkValidateCreateDatasetIDsBatches(b *testing.B) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		b.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		b.Fatal(err)
	}
	status := string(entity.StatusValid)
	ids := make([]string, 0, 32)
	for i := 0; i < 32; i++ {
		id := fmt.Sprintf("bench-kb-%02d", i)
		ids = append(ids, id)
		if err := db.Create(&entity.Knowledgebase{
			ID: id, TenantID: "t", CreatedBy: "t", Name: id,
			ChunkNum: 1, Permission: string(entity.TenantPermissionMe), Status: &status,
		}).Error; err != nil {
			b.Fatal(err)
		}
	}
	if err := db.Create(&entity.UserTenant{
		ID: "m", UserID: "t", TenantID: "t", Role: "owner", Status: &status,
	}).Error; err != nil {
		b.Fatal(err)
	}
	if err := testsupport.PrepareEnterprisePermissionFixture(db, "t", "t"); err != nil {
		b.Fatal(err)
	}
	origDB := dao.DB
	dao.DB = db
	defer func() { dao.DB = origDB }()
	svc := NewChatService()
	ctx := permission.WithIdentity(context.Background(), "t", "t")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.validateCreateDatasetIDs(ctx, toDatasetAnySlice(ids)); err != nil {
			b.Fatal(err)
		}
	}
}

func toDatasetAnySlice(ids []string) []interface{} {
	out := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		out = append(out, id)
	}
	return out
}
