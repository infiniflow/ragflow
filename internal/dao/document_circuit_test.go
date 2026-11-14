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
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// openDocumentCircuitTestDB returns a handle whose tables do not exist, so any
// query that actually reaches the database fails loudly instead of returning an
// empty result the caller could mistake for an open circuit.
func openDocumentCircuitTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	return db
}

func TestDocumentQueriesResolveOpenCircuitWithoutQuery(t *testing.T) {
	db := openDocumentCircuitTestDB(t)
	documentDAO := NewDocumentDAO()
	opts := DocumentListOptions{KbID: "kb-1", DocIDFilterApplied: true}
	ctx := context.Background()

	documents, total, err := documentDAO.ListByKBIDWithOptions(ctx, db, opts)
	if err != nil {
		t.Fatalf("ListByKBIDWithOptions queried the database: %v", err)
	}
	if len(documents) != 0 || total != 0 {
		t.Fatalf("ListByKBIDWithOptions = (%v, %d), want (empty, 0)", documents, total)
	}

	filters, total, err := documentDAO.GetFilterByKBID(ctx, db, opts)
	if err != nil {
		t.Fatalf("GetFilterByKBID queried the database: %v", err)
	}
	if total != 0 {
		t.Fatalf("GetFilterByKBID total = %d, want 0", total)
	}
	if _, ok := filters["suffix"].(map[string]int64); !ok {
		t.Fatalf("GetFilterByKBID suffix bucket type = %T, want map[string]int64", filters["suffix"])
	}
	if _, ok := filters["ingestion_status"].(map[string]int64); !ok {
		t.Fatalf("GetFilterByKBID ingestion_status bucket type = %T, want map[string]int64", filters["ingestion_status"])
	}
	if _, ok := filters["metadata"].(map[string]interface{}); !ok {
		t.Fatalf("GetFilterByKBID metadata bucket type = %T, want map[string]interface{}", filters["metadata"])
	}

	ids, err := documentDAO.ListIDsByKBIDWithOptions(ctx, db, opts)
	if err != nil {
		t.Fatalf("ListIDsByKBIDWithOptions queried the database: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("ListIDsByKBIDWithOptions = %v, want empty", ids)
	}
}

func TestDocumentQueriesStillQueryWhenCircuitClosed(t *testing.T) {
	db := openDocumentCircuitTestDB(t)
	documentDAO := NewDocumentDAO()
	ctx := context.Background()

	if _, _, err := documentDAO.ListByKBIDWithOptions(ctx, db, DocumentListOptions{KbID: "kb-1"}); err == nil {
		t.Fatal("ListByKBIDWithOptions answered from a circuit that should have stayed closed")
	}
}
