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

package dataset

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission/testsupport"
	"ragflow/internal/service"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type datasetBatchSQLCapture struct {
	mu   sync.Mutex
	sqls []string
}

func (c *datasetBatchSQLCapture) LogMode(logger.LogLevel) logger.Interface      { return c }
func (c *datasetBatchSQLCapture) Info(context.Context, string, ...interface{})  {}
func (c *datasetBatchSQLCapture) Warn(context.Context, string, ...interface{})  {}
func (c *datasetBatchSQLCapture) Error(context.Context, string, ...interface{}) {}
func (c *datasetBatchSQLCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	c.sqls = append(c.sqls, sql)
	c.mu.Unlock()
}

func (c *datasetBatchSQLCapture) knowledgebaseSelects() int {
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

// seedSearchBatchKBs inserts count datasets with distinct embedding models so
// SearchDatasets stops at the embedding-consistency check right after the
// authorization loop, keeping the test focused on that loop's query count.
func seedSearchBatchKBs(t *testing.T, db *gorm.DB, tenantID string, count int) []string {
	t.Helper()
	status := string(entity.StatusValid)
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("search-kb-%02d", i)
		ids = append(ids, id)
		if err := db.Create(&entity.Knowledgebase{
			ID: id, TenantID: tenantID, UserID: tenantID, CreatedBy: tenantID,
			Name: id, EmbdID: fmt.Sprintf("model-%d@Builtin", i),
			ChunkNum: 1, Permission: string(entity.TenantPermissionMe), Status: &status,
		}).Error; err != nil {
			t.Fatalf("seed knowledgebase %s: %v", id, err)
		}
	}
	ensureDatasetTestMembership(t, tenantID, tenantID, "owner")
	return ids
}

func TestSearchDatasetsBatchesDatasetQueries(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	ctx := withDatasetTestContext(t.Context(), "search-owner", "search-owner")
	ids := seedSearchBatchKBs(t, db, "search-owner", 5)

	capture := &datasetBatchSQLCapture{}
	dao.DB = db.Session(&gorm.Session{Logger: capture})

	// Distinct embedding models force an error after authorization. Asserting
	// that error proves the batched authorization actually completed before the
	// query count is checked — a discarded error would let an early denial pass.
	_, err := NewDatasetService().SearchDatasets(ctx, &service.SearchDatasetsRequest{
		Question: "q", DatasetIDs: ids,
	})
	if err == nil || !strings.Contains(err.Error(), "different embedding models") {
		t.Fatalf("SearchDatasets error = %v, want the embedding-model mismatch (authorization must have completed)", err)
	}

	if n, limit := capture.knowledgebaseSelects(), 1+testsupport.DatasetPermissionQueryCount(len(ids)); n > limit {
		t.Fatalf("knowledgebase SELECTs = %d for %d datasets, want <= %d; got SQL: %v", n, len(ids), limit, capture.sqls)
	}
}

// Adversarial: the first denied dataset still determines the error, and no
// metadata from a later dataset leaks through a batch authorization.
func TestSearchDatasetsPreservesFirstDeniedError(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	ctx := withDatasetTestContext(t.Context(), "search-owner", "search-owner")
	ids := seedSearchBatchKBs(t, db, "search-owner", 3)
	status := string(entity.StatusValid)
	foreign := "search-foreign-kb"
	if err := db.Create(&entity.Knowledgebase{
		ID: foreign, TenantID: "other", CreatedBy: "other", Name: foreign,
		EmbdID: "model-x@Builtin", ChunkNum: 1,
		Permission: string(entity.TenantPermissionMe), Status: &status,
	}).Error; err != nil {
		t.Fatal(err)
	}

	_, err := NewDatasetService().SearchDatasets(ctx, &service.SearchDatasetsRequest{
		Question: "q", DatasetIDs: []string{ids[0], foreign, ids[1]},
	})
	if err == nil {
		t.Fatal("expected permission error for foreign dataset")
	}
	if !strings.Contains(err.Error(), "Permission") && !strings.Contains(err.Error(), "denied") {
		t.Fatalf("error = %q, want a permission error", err.Error())
	}
}
