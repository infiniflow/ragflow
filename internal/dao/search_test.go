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
	"fmt"
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

// TestSearchDAOListByResourceIDsOrderByExpressionFallsBack checks that an
// `orderby` value that is an expression rather than an allowed column name
// leaves the rows in the default create_time order.
func TestSearchDAOListByResourceIDsOrderByExpressionFallsBack(t *testing.T) {
	db := setupSearchDetailDAOTestDB(t)
	pushDB(t, db)
	seedOrderableSearches(t, db)
	ctx := t.Context()
	d := NewSearchDAO()

	for _, orderby := range searchOrderExpressions {
		t.Run(orderby, func(t *testing.T) {
			rows, _, err := d.ListByResourceIDs(ctx, db, []string{"s-1", "s-2", "s-3"}, nil, 1, 10, []OrderTerm{{Column: orderby}}, "")
			if err != nil {
				t.Fatalf("ListByResourceIDs with orderby %q: %v", orderby, err)
			}
			assertSearchOrder(t, rows, []string{"s-1", "s-2", "s-3"}, orderby)
		})
	}
}

// TestSearchDAOListByResourceIDsWithOwnerFilterOrderByExpressionFallsBack
// covers the same query with an owner_ids filter.
func TestSearchDAOListByResourceIDsWithOwnerFilterOrderByExpressionFallsBack(t *testing.T) {
	db := setupSearchDetailDAOTestDB(t)
	pushDB(t, db)
	seedOrderableSearches(t, db)
	ctx := t.Context()
	d := NewSearchDAO()

	for _, orderby := range searchOrderExpressions {
		t.Run(orderby, func(t *testing.T) {
			rows, _, err := d.ListByResourceIDs(ctx, db, []string{"s-1", "s-2", "s-3"}, []string{"t1"}, 1, 10, []OrderTerm{{Column: orderby}}, "")
			if err != nil {
				t.Fatalf("ListByResourceIDs with orderby %q: %v", orderby, err)
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

	rows, _, err := d.ListByResourceIDs(ctx, db, []string{"s-1", "s-2", "s-3"}, nil, 1, 10, []OrderTerm{{Column: "name"}}, "")
	if err != nil {
		t.Fatalf("ListByResourceIDs ascending by name: %v", err)
	}
	assertSearchOrder(t, rows, []string{"s-3", "s-2", "s-1"}, "name")

	rows, _, err = d.ListByResourceIDs(ctx, db, []string{"s-1", "s-2", "s-3"}, nil, 1, 10, []OrderTerm{{Column: "name", Desc: true}}, "")
	if err != nil {
		t.Fatalf("ListByResourceIDs descending by name: %v", err)
	}
	assertSearchOrder(t, rows, []string{"s-1", "s-2", "s-3"}, "name")
}

// assertSearchIDs compares the returned row IDs against wantIDs, naming the
// call under test in the failure message. assertSearchOrder is for the
// ORDER BY allowlist tests, whose failure message talks about expressions
// reaching the ORDER BY clause.
func assertSearchIDs(t *testing.T, rows []*entity.SearchListItem, wantIDs []string, call string) {
	t.Helper()
	gotIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		gotIDs = append(gotIDs, row.ID)
	}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("%s returned %v, want %v", call, gotIDs, wantIDs)
	}
	for i := range gotIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("%s returned %v, want %v", call, gotIDs, wantIDs)
		}
	}
}

// seedSearchRowsWithCreateTime creates count searches owned by tenantID whose
// create_time grows with their index, so page boundaries are deterministic.
func seedSearchRowsWithCreateTime(t *testing.T, db *gorm.DB, tenantID string, count int) {
	t.Helper()
	for i := 1; i <= count; i++ {
		createTime := int64(i * 100)
		search := entity.Search{
			ID:           fmt.Sprintf("s-%d", i),
			TenantID:     tenantID,
			Name:         fmt.Sprintf("search-%d", i),
			CreatedBy:    tenantID,
			SearchConfig: entity.JSONMap{},
			Status:       stringPtr("1"),
			BaseModel:    entity.BaseModel{CreateTime: &createTime},
		}
		if err := db.Create(&search).Error; err != nil {
			t.Fatalf("failed to create search %s: %v", search.ID, err)
		}
	}
}

// TestSearchDAOListByResourceIDsPaginatesInSQL checks that page/pageSize reach the
// query as LIMIT/OFFSET while total keeps reporting the full COUNT(*).
// SearchService.ListSearches returns that total as the response's total field
// and no longer slices the returned rows itself.
func TestSearchDAOListByResourceIDsPaginatesInSQL(t *testing.T) {
	db := setupSearchDetailDAOTestDB(t)
	pushDB(t, db)
	seedSearchRowsWithCreateTime(t, db, "t1", 5)
	ctx := t.Context()
	d := NewSearchDAO()
	terms := []OrderTerm{{Column: "create_time", Desc: true}}

	tests := []struct {
		name     string
		page     int
		pageSize int
		wantIDs  []string
	}{
		{name: "first page", page: 1, pageSize: 2, wantIDs: []string{"s-5", "s-4"}},
		{name: "middle page", page: 2, pageSize: 2, wantIDs: []string{"s-3", "s-2"}},
		{name: "partial last page", page: 3, pageSize: 2, wantIDs: []string{"s-1"}},
		{name: "page past the end", page: 4, pageSize: 2, wantIDs: []string{}},
		{name: "unpaginated returns every row", page: 0, pageSize: 0, wantIDs: []string{"s-5", "s-4", "s-3", "s-2", "s-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, total, err := d.ListByResourceIDs(ctx, db, []string{"s-1", "s-2", "s-3", "s-4", "s-5"}, nil, tt.page, tt.pageSize, terms, "")
			if err != nil {
				t.Fatalf("ListByResourceIDs(page=%d, pageSize=%d): %v", tt.page, tt.pageSize, err)
			}
			if total != 5 {
				t.Fatalf("total = %d, want 5: total must count every matching row, not just the page", total)
			}
			assertSearchIDs(t, rows, tt.wantIDs, fmt.Sprintf("page=%d pageSize=%d", tt.page, tt.pageSize))
		})
	}
}

// TestSearchDAOListByResourceIDsKeepsJoinedOwnerColumnsWhenPaginated guards the
// COUNT followed by the paged Scan: the second statement must still select the
// joined user columns instead of inheriting count(*) from the first one.
func TestSearchDAOListByResourceIDsKeepsJoinedOwnerColumnsWhenPaginated(t *testing.T) {
	db := setupSearchDetailDAOTestDB(t)
	pushDB(t, db)
	seedSearchRowsWithCreateTime(t, db, "t1", 3)
	ctx := t.Context()

	if err := db.Create(&entity.User{ID: "t1", Nickname: "owner of t1", Status: stringPtr("1")}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	rows, total, err := NewSearchDAO().ListByResourceIDs(ctx, db, []string{"s-1", "s-2", "s-3"}, nil, 1, 2, []OrderTerm{{Column: "create_time", Desc: true}}, "")
	if err != nil {
		t.Fatalf("ListByResourceIDs: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for i, row := range rows {
		if row.Name == "" || row.TenantID != "t1" {
			t.Fatalf("row %d = %+v, want the search columns of tenant t1", i, row.Search)
		}
		if row.Nickname == nil || *row.Nickname != "owner of t1" {
			t.Fatalf("row %d nickname = %v, want owner of t1: the paged scan dropped the joined owner column", i, row.Nickname)
		}
	}
}

// TestSearchDAOListByResourceIDsKeepsOwnerFilter pins the owner_ids filter: a
// search owned by the requesting user is still excluded when its tenant_id is
// not among ownerIDs, even when it is included in the authorized resource IDs.
func TestSearchDAOListByResourceIDsKeepsOwnerFilter(t *testing.T) {
	db := setupSearchDetailDAOTestDB(t)
	pushDB(t, db)
	ctx := t.Context()
	d := NewSearchDAO()

	for _, tenantID := range []string{"t1", "u-self"} {
		createTime := int64(100)
		search := entity.Search{
			ID:           "search-" + tenantID,
			TenantID:     tenantID,
			Name:         "search " + tenantID,
			CreatedBy:    tenantID,
			SearchConfig: entity.JSONMap{},
			Status:       stringPtr("1"),
			BaseModel:    entity.BaseModel{CreateTime: &createTime},
		}
		if err := db.Create(&search).Error; err != nil {
			t.Fatalf("failed to create search %s: %v", search.ID, err)
		}
	}

	rows, total, err := d.ListByResourceIDs(ctx, db, []string{"search-t1", "search-u-self"}, []string{"t1"}, 0, 0, nil, "")
	if err != nil {
		t.Fatalf("ListByResourceIDs: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1: only the t1 search has a tenant_id in ownerIDs", total)
	}
	assertSearchIDs(t, rows, []string{"search-t1"}, "owner filter")
}
