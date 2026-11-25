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

package tool

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
	"ragflow/internal/service/nlp"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type retrievalBatchSQLCapture struct {
	mu   sync.Mutex
	sqls []string
}

func (c *retrievalBatchSQLCapture) LogMode(logger.LogLevel) logger.Interface      { return c }
func (c *retrievalBatchSQLCapture) Info(context.Context, string, ...interface{})  {}
func (c *retrievalBatchSQLCapture) Warn(context.Context, string, ...interface{})  {}
func (c *retrievalBatchSQLCapture) Error(context.Context, string, ...interface{}) {}
func (c *retrievalBatchSQLCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	c.sqls = append(c.sqls, sql)
	c.mu.Unlock()
}

func (c *retrievalBatchSQLCapture) knowledgebaseSelects() int {
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

func setupRetrievalBatchDB(t *testing.T, count int) (*gorm.DB, []string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}
	status := string(entity.StatusValid)
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("retrieval-kb-%02d", i)
		ids = append(ids, id)
		if err := db.Create(&entity.Knowledgebase{
			ID: id, TenantID: "owner", Name: id,
			EmbdID:     fmt.Sprintf("model-%d@Builtin", i),
			Permission: string(entity.TenantPermissionMe), CreatedBy: "owner", Status: &status,
		}).Error; err != nil {
			t.Fatalf("create dataset %s: %v", id, err)
		}
	}
	if err := db.Create(&entity.UserTenant{
		ID: "owner-membership", UserID: "owner", TenantID: "owner",
		Role: string(permission.RoleOwner), InvitedBy: "owner", Status: &status,
	}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	if err := testsupport.PrepareEnterprisePermissionFixture(db, "owner", "owner"); err != nil {
		t.Fatalf("prepare permission fixture: %v", err)
	}
	return db, ids
}

func TestNLPRetrievalAdapterBatchesDatasetAuthorization(t *testing.T) {
	db, ids := setupRetrievalBatchDB(t, 5)
	capture := &retrievalBatchSQLCapture{}
	captureDB := db.Session(&gorm.Session{Logger: capture})
	adapter := &NLPRetrievalAdapter{
		svc:   &nlp.RetrievalService{},
		kbDAO: dao.NewKnowledgebaseDAO(),
	}
	ctx := permission.WithIdentity(t.Context(), "owner", "owner")

	// Distinct embedding models stop Search at validation after authorization.
	// Asserting that error proves authorization completed before the count is
	// checked; a discarded error would let an early denial pass this test.
	_, err := adapter.Search(ctx, captureDB, RetrievalRequest{Query: "q", DatasetIDs: ids})
	if err == nil || !strings.Contains(err.Error(), "different embedding models") {
		t.Fatalf("Search error = %v, want the embedding-model mismatch (authorization must have completed)", err)
	}

	// One dataset load for resolveDatasets and one batch authorization.
	if n := capture.knowledgebaseSelects(); n > 2 {
		t.Fatalf("knowledgebase SELECTs = %d for %d datasets, want <= 2 (batch); got SQL: %v", n, len(ids), capture.sqls)
	}
}

// Adversarial: a dataset the caller cannot use is still rejected even when
// authorization is done in one batch, and the first error is preserved.
func TestNLPRetrievalAdapterRejectsUnauthorizedDatasetInBatch(t *testing.T) {
	db, ids := setupRetrievalBatchDB(t, 2)
	status := string(entity.StatusValid)
	foreign := "retrieval-foreign-kb"
	if err := db.Create(&entity.Knowledgebase{
		ID: foreign, TenantID: "foreign", Name: foreign,
		EmbdID: "model-x@Builtin", Permission: string(entity.TenantPermissionMe),
		CreatedBy: "foreign", Status: &status,
	}).Error; err != nil {
		t.Fatal(err)
	}
	adapter := &NLPRetrievalAdapter{
		svc:   &nlp.RetrievalService{},
		kbDAO: dao.NewKnowledgebaseDAO(),
	}
	ctx := permission.WithIdentity(t.Context(), "owner", "owner")

	_, err := adapter.Search(ctx, db, RetrievalRequest{
		Query: "q", DatasetIDs: []string{ids[0], foreign, ids[1]},
	})
	if !errorsIsPermissionError(err) {
		t.Fatalf("error = %v, want a permission error", err)
	}
}

func errorsIsPermissionError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Permission") || strings.Contains(err.Error(), "denied") || strings.Contains(err.Error(), "not found")
}
