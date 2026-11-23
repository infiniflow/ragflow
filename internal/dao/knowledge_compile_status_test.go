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
	"net/url"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func setupCompilationStatusCountTestDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatalf("access sqlite pool: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	table := (entity.KnowledgeCompileBacklog{}).TableName()
	if err := db.AutoMigrate(&entity.KnowledgeCompileBacklog{}); err != nil {
		t.Fatalf("migrate %s: %v", table, err)
	}
	return db, table
}

func insertCompilationStatusCountEntries(t *testing.T, db *gorm.DB, table string, entries []entity.KnowledgeCompileEntry) {
	t.Helper()
	if err := db.Table(table).Create(&entries).Error; err != nil {
		t.Fatalf("insert entries: %v", err)
	}
}

// TestListKnowledgeCompileEntries locks the data-access contract: rows are
// scoped by dataset, duplicates are preserved, and serializer:json columns
// (Variants/TaskTypes) round-trip through Find.
func TestListKnowledgeCompileEntries(t *testing.T) {
	db, table := setupCompilationStatusCountTestDB(t)
	insertCompilationStatusCountEntries(t, db, table, []entity.KnowledgeCompileEntry{
		{DatasetID: "kb", DocID: "same", QueuePosition: 1, TaskTypes: []string{"Graph"}},
		{DatasetID: "kb", DocID: "same", QueuePosition: 2, TaskTypes: []string{"Graph"}},
		{DatasetID: "kb", DocID: "timeline", QueuePosition: 3, Variants: []string{"structure"}, TaskTypes: []string{"Timeline"}},
		{DatasetID: "kb", DocID: "unscoped", QueuePosition: 4},
		{DatasetID: "other", DocID: "x", QueuePosition: 1, TaskTypes: []string{"Graph"}},
	})

	got, err := ListKnowledgeCompileEntries(t.Context(), db, table, "kb")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len=%d want 4 (duplicates kept, other dataset excluded)", len(got))
	}
	graphCount := 0
	foundStructure := false
	for _, entry := range got {
		for _, taskType := range entry.TaskTypes {
			if strings.EqualFold(taskType, "Graph") {
				graphCount++
			}
		}
		for _, variant := range entry.Variants {
			if variant == "structure" {
				foundStructure = true
			}
		}
	}
	if graphCount != 2 {
		t.Fatalf("graph entries=%d want 2; TaskTypes serializer:json did not round-trip", graphCount)
	}
	if !foundStructure {
		t.Fatal("Variants serializer:json did not round-trip (structure missing)")
	}
}

// TestCountKnowledgeCompileEntries locks the SQL aggregation path: dataset
// scoping and duplicate preservation.
func TestCountKnowledgeCompileEntries(t *testing.T) {
	db, table := setupCompilationStatusCountTestDB(t)
	insertCompilationStatusCountEntries(t, db, table, []entity.KnowledgeCompileEntry{
		{DatasetID: "kb", DocID: "same", QueuePosition: 1},
		{DatasetID: "kb", DocID: "same", QueuePosition: 2},
		{DatasetID: "kb", DocID: "third", QueuePosition: 3},
		{DatasetID: "other", DocID: "x", QueuePosition: 1},
	})

	got, err := CountKnowledgeCompileEntries(t.Context(), db, table, "kb")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 3 {
		t.Fatalf("count=%d want 3 (duplicates kept, other dataset excluded)", got)
	}
}

// TestCompilationStatusEntriesEmptyAndError locks the edge paths: an unknown
// dataset yields an empty set with no error, while a broken table surfaces the
// database error instead of being swallowed.
func TestCompilationStatusEntriesEmptyAndError(t *testing.T) {
	db, table := setupCompilationStatusCountTestDB(t)
	insertCompilationStatusCountEntries(t, db, table, []entity.KnowledgeCompileEntry{
		{DatasetID: "kb", DocID: "d1", QueuePosition: 1, TaskTypes: []string{"Graph"}},
	})

	entries, err := ListKnowledgeCompileEntries(t.Context(), db, table, "missing")
	if err != nil || len(entries) != 0 {
		t.Fatalf("list missing dataset: entries=%d err=%v", len(entries), err)
	}
	count, err := CountKnowledgeCompileEntries(t.Context(), db, table, "missing")
	if err != nil || count != 0 {
		t.Fatalf("count missing dataset: count=%d err=%v", count, err)
	}

	if _, err := CountKnowledgeCompileEntries(t.Context(), db, "no_such_compile_table", "kb"); err == nil {
		t.Fatal("expected database error for missing table, got nil")
	}
}

// TestKnowledgeCompileEntriesHonorContext is the adversarial guard for the
// context contract: a cancelled context must abort both queries. An
// implementation that drops ctx would still pass every happy-path fixture but
// fail here.
func TestKnowledgeCompileEntriesHonorContext(t *testing.T) {
	db, table := setupCompilationStatusCountTestDB(t)
	insertCompilationStatusCountEntries(t, db, table, []entity.KnowledgeCompileEntry{
		{DatasetID: "kb", DocID: "d1", QueuePosition: 1, TaskTypes: []string{"Graph"}},
	})

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := ListKnowledgeCompileEntries(cancelled, db, table, "kb"); err == nil {
		t.Fatal("ListKnowledgeCompileEntries ignored cancelled context")
	}
	if _, err := CountKnowledgeCompileEntries(cancelled, db, table, "kb"); err == nil {
		t.Fatal("CountKnowledgeCompileEntries ignored cancelled context")
	}
}
