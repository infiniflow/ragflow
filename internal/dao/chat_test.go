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

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func setupChatTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.User{}, &entity.Chat{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

// chatOrderExpressions are SQL expressions rather than column names, standing
// in for an `orderby` query parameter that reaches ChatDAO (issue #14268).
// The chat table is named dialog.
var chatOrderExpressions = []string{
	"(SELECT inner_dialog.name FROM dialog AS inner_dialog WHERE inner_dialog.id = dialog.id)",
}

// seedOrderableChats creates three chats whose create_time order is the reverse
// of their name order, so a test can tell which column the ORDER BY clause
// actually used.
func seedOrderableChats(t *testing.T, db *gorm.DB) {
	t.Helper()
	chats := []entity.Chat{
		{ID: "c-1", TenantID: "t1", Name: stringPtr("zulu"), BaseModel: entity.BaseModel{CreateTime: int64Ptr(100)}},
		{ID: "c-2", TenantID: "t1", Name: stringPtr("mike"), BaseModel: entity.BaseModel{CreateTime: int64Ptr(200)}},
		{ID: "c-3", TenantID: "t1", Name: stringPtr("alpha"), BaseModel: entity.BaseModel{CreateTime: int64Ptr(300)}},
	}
	for i := range chats {
		chats[i].LLMID = "llm-1"
		chats[i].LLMSetting = entity.JSONMap{}
		chats[i].PromptType = "simple"
		chats[i].PromptConfig = entity.JSONMap{}
		chats[i].DoRefer = "1"
		chats[i].KBIDs = entity.JSONSlice{}
		chats[i].Status = stringPtr("1")
		if err := db.Create(&chats[i]).Error; err != nil {
			t.Fatalf("failed to create chat %s: %v", chats[i].ID, err)
		}
	}
}

func assertChatOrder(t *testing.T, rows []*entity.ChatListItem, want []string, orderby string) {
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

// TestChatDAOListByTenantIDsOrderByExpressionFallsBack checks that an `orderby`
// value that is an expression rather than an allowed column name leaves the
// rows in the default create_time order.
func TestChatDAOListByTenantIDsOrderByExpressionFallsBack(t *testing.T) {
	db := setupChatTestDB(t)
	pushDB(t, db)
	seedOrderableChats(t, db)
	ctx := t.Context()
	d := NewChatDAO()

	for _, orderby := range chatOrderExpressions {
		t.Run(orderby, func(t *testing.T) {
			rows, _, err := d.ListByTenantIDs(ctx, db, []string{"t1"}, "t1", 1, 10, []OrderTerm{{Column: orderby}}, "")
			if err != nil {
				t.Fatalf("ListByTenantIDs with orderby %q: %v", orderby, err)
			}
			assertChatOrder(t, rows, []string{"c-1", "c-2", "c-3"}, orderby)
		})
	}
}

// TestChatDAOListByOwnerIDsOrderByExpressionFallsBack is the ListByOwnerIDs
// counterpart, the branch the endpoint takes when owner_ids is supplied.
func TestChatDAOListByOwnerIDsOrderByExpressionFallsBack(t *testing.T) {
	db := setupChatTestDB(t)
	pushDB(t, db)
	seedOrderableChats(t, db)
	ctx := t.Context()
	d := NewChatDAO()

	for _, orderby := range chatOrderExpressions {
		t.Run(orderby, func(t *testing.T) {
			rows, _, err := d.ListByOwnerIDs(ctx, db, []string{"t1"}, "t1", 1, 10, []OrderTerm{{Column: orderby}}, "")
			if err != nil {
				t.Fatalf("ListByOwnerIDs with orderby %q: %v", orderby, err)
			}
			assertChatOrder(t, rows, []string{"c-1", "c-2", "c-3"}, orderby)
		})
	}
}

// TestChatDAOListOrderByAllowedColumn checks that the allowlist still honors a
// real column in both directions.
func TestChatDAOListOrderByAllowedColumn(t *testing.T) {
	db := setupChatTestDB(t)
	pushDB(t, db)
	seedOrderableChats(t, db)
	ctx := t.Context()
	d := NewChatDAO()

	rows, _, err := d.ListByTenantIDs(ctx, db, []string{"t1"}, "t1", 1, 10, []OrderTerm{{Column: "name"}}, "")
	if err != nil {
		t.Fatalf("ListByTenantIDs ascending by name: %v", err)
	}
	assertChatOrder(t, rows, []string{"c-3", "c-2", "c-1"}, "name")

	rows, _, err = d.ListByTenantIDs(ctx, db, []string{"t1"}, "t1", 1, 10, []OrderTerm{{Column: "name", Desc: true}}, "")
	if err != nil {
		t.Fatalf("ListByTenantIDs descending by name: %v", err)
	}
	assertChatOrder(t, rows, []string{"c-1", "c-2", "c-3"}, "name")
}

// assertChatIDs compares the returned row IDs against wantIDs, naming the call
// under test in the failure message. assertChatOrder is for the ORDER BY
// allowlist tests, whose failure message talks about expressions reaching the
// ORDER BY clause.
func assertChatIDs(t *testing.T, rows []*entity.ChatListItem, wantIDs []string, call string) {
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

// seedChatRowsWithCreateTime creates count chats owned by tenantID whose
// create_time grows with their index, so page boundaries are deterministic.
func seedChatRowsWithCreateTime(t *testing.T, db *gorm.DB, tenantID string, count int) {
	t.Helper()
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("chat-%d", i)
		createTime := int64(i * 100)
		chat := entity.Chat{
			ID:           fmt.Sprintf("c-%d", i),
			TenantID:     tenantID,
			Name:         &name,
			LLMID:        "llm-1",
			LLMSetting:   entity.JSONMap{},
			PromptType:   "simple",
			PromptConfig: entity.JSONMap{},
			DoRefer:      "1",
			KBIDs:        entity.JSONSlice{},
			Status:       stringPtr("1"),
			BaseModel:    entity.BaseModel{CreateTime: &createTime},
		}
		if err := db.Create(&chat).Error; err != nil {
			t.Fatalf("failed to create chat %s: %v", chat.ID, err)
		}
	}
}

// TestChatDAOListByOwnerIDsPaginatesInSQL checks that page/pageSize reach the
// query as LIMIT/OFFSET while total keeps reporting the full COUNT(*).
// ChatService.ListChats returns that total as the response's total field and no
// longer slices the returned rows itself.
func TestChatDAOListByOwnerIDsPaginatesInSQL(t *testing.T) {
	db := setupChatTestDB(t)
	pushDB(t, db)
	seedChatRowsWithCreateTime(t, db, "t1", 5)
	ctx := t.Context()
	d := NewChatDAO()
	terms := []OrderTerm{{Column: "create_time", Desc: true}}

	tests := []struct {
		name     string
		page     int
		pageSize int
		wantIDs  []string
	}{
		{name: "first page", page: 1, pageSize: 2, wantIDs: []string{"c-5", "c-4"}},
		{name: "middle page", page: 2, pageSize: 2, wantIDs: []string{"c-3", "c-2"}},
		{name: "partial last page", page: 3, pageSize: 2, wantIDs: []string{"c-1"}},
		{name: "page past the end", page: 4, pageSize: 2, wantIDs: []string{}},
		{name: "unpaginated returns every row", page: 0, pageSize: 0, wantIDs: []string{"c-5", "c-4", "c-3", "c-2", "c-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, total, err := d.ListByOwnerIDs(ctx, db, []string{"t1"}, "t1", tt.page, tt.pageSize, terms, "")
			if err != nil {
				t.Fatalf("ListByOwnerIDs(page=%d, pageSize=%d): %v", tt.page, tt.pageSize, err)
			}
			if total != 5 {
				t.Fatalf("total = %d, want 5: total must count every matching row, not just the page", total)
			}
			assertChatIDs(t, rows, tt.wantIDs, fmt.Sprintf("page=%d pageSize=%d", tt.page, tt.pageSize))
		})
	}
}

// TestChatDAOListByOwnerIDsKeepsJoinedOwnerColumnsWhenPaginated guards the COUNT
// followed by the paged Scan: the second statement must still select the joined
// user columns instead of inheriting count(*) from the first one.
func TestChatDAOListByOwnerIDsKeepsJoinedOwnerColumnsWhenPaginated(t *testing.T) {
	db := setupChatTestDB(t)
	pushDB(t, db)
	seedChatRowsWithCreateTime(t, db, "t1", 3)
	ctx := t.Context()

	if err := db.Create(&entity.User{ID: "t1", Nickname: "owner of t1", Status: stringPtr("1")}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	rows, total, err := NewChatDAO().ListByOwnerIDs(ctx, db, []string{"t1"}, "t1", 1, 2, []OrderTerm{{Column: "create_time", Desc: true}}, "")
	if err != nil {
		t.Fatalf("ListByOwnerIDs: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for i, row := range rows {
		if row.Name == nil || *row.Name == "" || row.TenantID != "t1" {
			t.Fatalf("row %d = %+v, want the chat columns of tenant t1", i, row.Chat)
		}
		if row.Nickname == nil || *row.Nickname != "owner of t1" {
			t.Fatalf("row %d nickname = %v, want owner of t1: the paged scan dropped the joined owner column", i, row.Nickname)
		}
	}
}
