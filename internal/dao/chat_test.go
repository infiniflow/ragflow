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
			rows, _, err := d.ListByOwnerIDs(ctx, db, []string{"t1"}, "t1", []OrderTerm{{Column: orderby}}, "")
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
