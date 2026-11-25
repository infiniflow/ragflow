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
	"testing"

	"ragflow/internal/dao"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newCaseTestDB builds the minimal table the batched KB lookup reads. A
// COLLATE NOCASE id column models the case-insensitive collation MySQL uses by
// default: GetByIDs can return a row whose stored id differs in case from the
// requested id.
func newCaseTestDB(t *testing.T, collate string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE knowledgebase (id TEXT PRIMARY KEY COLLATE " + collate + ", status TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLoadKBsInOrderMatchesCaseInsensitiveID(t *testing.T) {
	db := newCaseTestDB(t, "NOCASE")
	if err := db.Exec("INSERT INTO knowledgebase (id, status) VALUES (?, ?)", "ABC123", "1").Error; err != nil {
		t.Fatal(err)
	}

	got, err := LoadKBsInOrder(context.Background(), db, dao.NewKnowledgebaseDAO(), []string{"abc123"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] == nil || got[0].ID != "ABC123" {
		t.Fatalf("case-insensitive ID did not resolve to its stored row: %#v", got)
	}
}

// Repeated IDs must keep their positions, including when the variants differ
// only by case and the database returns a single canonical row.
func TestLoadKBsInOrderKeepsRepeatedCaseVariants(t *testing.T) {
	db := newCaseTestDB(t, "NOCASE")
	if err := db.Exec("INSERT INTO knowledgebase (id, status) VALUES (?, ?)", "ABC123", "1").Error; err != nil {
		t.Fatal(err)
	}

	got, err := LoadKBsInOrder(context.Background(), db, dao.NewKnowledgebaseDAO(), []string{"abc123", "ABC123", "AbC123"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("result length = %d, want 3", len(got))
	}
	for i, kb := range got {
		if kb == nil || kb.ID != "ABC123" {
			t.Fatalf("result[%d] = %#v, want the stored row for every case variant", i, kb)
		}
	}
}

// On a case-sensitive database an exact match must win over a case-folded
// near-match; a case-insensitive fallback is only for when exact lookup misses.
func TestLoadKBsInOrderPrefersExactIDCase(t *testing.T) {
	db := newCaseTestDB(t, "BINARY")
	for _, id := range []string{"ABC", "abc"} {
		if err := db.Exec("INSERT INTO knowledgebase (id, status) VALUES (?, ?)", id, "1").Error; err != nil {
			t.Fatal(err)
		}
	}

	got, err := LoadKBsInOrder(context.Background(), db, dao.NewKnowledgebaseDAO(), []string{"abc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] == nil || got[0].ID != "abc" {
		t.Fatalf("exact-case match did not win: %#v", got)
	}
}
