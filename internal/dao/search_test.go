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
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func int64Ptr(v int64) *int64 {
	return &v
}

// searchOrderExpressions are SQL expressions rather than column names. They
// stand in for the `orderby` query parameter of GET /api/v1/searches, which
// reaches SearchDAO straight from the request (issue #14268).
var searchOrderExpressions = []string{
	"(SELECT inner_search.name FROM search AS inner_search WHERE inner_search.id = search.id)",
}

// seedOrderableSearches creates three searches whose create_time order is the
// reverse of their name order, so a test can tell which column the ORDER BY
// clause actually used.
func seedOrderableSearches(t *testing.T, db *gorm.DB) {
	t.Helper()
	searches := []entity.Search{
		{ID: "s-1", TenantID: "t1", Name: "zulu", CreatedBy: "t1", SearchConfig: entity.JSONMap{}, Status: stringPtr("1"), BaseModel: entity.BaseModel{CreateTime: int64Ptr(100)}},
		{ID: "s-2", TenantID: "t1", Name: "mike", CreatedBy: "t1", SearchConfig: entity.JSONMap{}, Status: stringPtr("1"), BaseModel: entity.BaseModel{CreateTime: int64Ptr(200)}},
		{ID: "s-3", TenantID: "t1", Name: "alpha", CreatedBy: "t1", SearchConfig: entity.JSONMap{}, Status: stringPtr("1"), BaseModel: entity.BaseModel{CreateTime: int64Ptr(300)}},
	}
	for i := range searches {
		if err := db.Create(&searches[i]).Error; err != nil {
			t.Fatalf("failed to create search %s: %v", searches[i].ID, err)
		}
	}
}

func assertSearchOrder(t *testing.T, rows []*entity.SearchListItem, want []string, orderby string) {
	t.Helper()
	if len(rows) != len(want) {
		t.Fatalf("orderby %q returned %d rows, want %d", orderby, len(rows), len(want))
	}
	for i, row := range rows {
		if row.ID != want[i] {
			got := make([]string, 0, len(rows))
			for _, r := range rows {
				got = append(got, r.ID)
			}
			t.Fatalf("orderby %q produced order %v, want %v: the expression reached the ORDER BY clause", orderby, got, want)
		}
	}
}

// TestSearchDAOListByTenantIDsOrderByExpressionFallsBack checks that an
// `orderby` value that is an expression rather than an allowed column name
// leaves the rows in the default create_time order.
func TestSearchDAOListByTenantIDsOrderByExpressionFallsBack(t *testing.T) {
	db := setupSearchDetailDAOTestDB(t)
	pushDB(t, db)
	seedOrderableSearches(t, db)
	ctx := t.Context()
	d := NewSearchDAO()

	for _, orderby := range searchOrderExpressions {
		t.Run(orderby, func(t *testing.T) {
			rows, _, err := d.ListByTenantIDs(ctx, db, []string{"t1"}, "t1", 1, 10, orderby, false, "")
			if err != nil {
				t.Fatalf("ListByTenantIDs with orderby %q: %v", orderby, err)
			}
			assertSearchOrder(t, rows, []string{"s-1", "s-2", "s-3"}, orderby)
		})
	}
}

// TestSearchDAOListByOwnerIDsOrderByExpressionFallsBack is the ListByOwnerIDs
// counterpart, the branch the endpoint takes when owner_ids is supplied.
func TestSearchDAOListByOwnerIDsOrderByExpressionFallsBack(t *testing.T) {
	db := setupSearchDetailDAOTestDB(t)
	pushDB(t, db)
	seedOrderableSearches(t, db)
	ctx := t.Context()
	d := NewSearchDAO()

	for _, orderby := range searchOrderExpressions {
		t.Run(orderby, func(t *testing.T) {
			rows, _, err := d.ListByOwnerIDs(ctx, db, []string{"t1"}, "t1", orderby, false, "")
			if err != nil {
				t.Fatalf("ListByOwnerIDs with orderby %q: %v", orderby, err)
			}
			assertSearchOrder(t, rows, []string{"s-1", "s-2", "s-3"}, orderby)
		})
	}
}

// TestSearchDAOListOrderByAllowedColumn checks that the allowlist still honors
// a real column in both directions.
func TestSearchDAOListOrderByAllowedColumn(t *testing.T) {
	db := setupSearchDetailDAOTestDB(t)
	pushDB(t, db)
	seedOrderableSearches(t, db)
	ctx := t.Context()
	d := NewSearchDAO()

	rows, _, err := d.ListByTenantIDs(ctx, db, []string{"t1"}, "t1", 1, 10, "name", false, "")
	if err != nil {
		t.Fatalf("ListByTenantIDs ascending by name: %v", err)
	}
	assertSearchOrder(t, rows, []string{"s-3", "s-2", "s-1"}, "name")

	rows, _, err = d.ListByTenantIDs(ctx, db, []string{"t1"}, "t1", 1, 10, "name", true, "")
	if err != nil {
		t.Fatalf("ListByTenantIDs descending by name: %v", err)
	}
	assertSearchOrder(t, rows, []string{"s-1", "s-2", "s-3"}, "name")
}
