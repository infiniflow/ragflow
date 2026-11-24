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

package dataset

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqlCaptureLogger records every SQL statement GORM executes so the test can
// assert that deleteDataset no longer pulls the whole document row (SELECT *)
// but only the columns it actually reads (id, name, location).
type sqlCaptureLogger struct {
	mu   sync.Mutex
	sqls []string
}

func (l *sqlCaptureLogger) LogMode(logger.LogLevel) logger.Interface { return l }
func (l *sqlCaptureLogger) Info(ctx context.Context, msg string, data ...interface{}) {
}
func (l *sqlCaptureLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
}
func (l *sqlCaptureLogger) Error(ctx context.Context, msg string, data ...interface{}) {
}
func (l *sqlCaptureLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sqls = append(l.sqls, sql)
}

func TestDeleteDatasetProjectsDocumentColumns(t *testing.T) {
	db := setupServiceTestDB(t)
	cl := &sqlCaptureLogger{}
	captureDB := db.Session(&gorm.Session{Logger: cl})
	oldDB := dao.DB
	dao.DB = captureDB
	t.Cleanup(func() { dao.DB = oldDB })

	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Dataset")
	// Seed a document with an oversized parser_config to make the over-read real.
	big := `{"huge":"` + strings.Repeat("x", 4096) + `"}`
	if err := db.Exec("INSERT INTO document (id, kb_id, name, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"doc-1", "kb-1", "Document", "general", big, "pdf", "tenant-1", "pdf", "tracked").Error; err != nil {
		t.Fatal(err)
	}
	store := storage.NewMemoryStorage()
	if err := store.Put(t.Context(), "kb-1", "tracked", []byte("content")); err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })

	kb, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), db, "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := testDatasetUpdateService(t).deleteDataset(t.Context(), kb); err != nil {
		t.Fatalf("delete dataset: %v", err)
	}

	// Locate the SELECT issued against the document table.
	var docSQL string
	for _, s := range cl.sqls {
		upper := strings.ToUpper(s)
		if strings.Contains(upper, "SELECT") && strings.Contains(upper, "FROM `DOCUMENT`") {
			docSQL = s
		}
	}
	if docSQL == "" {
		t.Fatalf("no query against the document table captured; got: %v", cl.sqls)
	}

	// RED target: before the fix this is "SELECT * FROM \"document\" ..." which
	// drags the entire row (including the large parser_config) across the wire.
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(docSQL)), "SELECT *") {
		t.Errorf("deleteDataset selects the whole document row (SELECT *) instead of projecting only the columns it reads; SQL: %s", docSQL)
	}
	if !strings.Contains(docSQL, "location") || !strings.Contains(docSQL, "name") {
		t.Errorf("deleteDataset projection is missing required columns (location/name); SQL: %s", docSQL)
	}
	if strings.Contains(docSQL, "parser_config") {
		t.Errorf("deleteDataset must not load parser_config; SQL: %s", docSQL)
	}

	// Behavior must remain intact: the KB is gone and its storage object removed.
	if store.ObjExist(t.Context(), "kb-1", "tracked") {
		t.Fatal("dataset deletion retained a document object")
	}
	var count int64
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("dataset retained after deletion: count=%d, err=%v", count, err)
	}
}
