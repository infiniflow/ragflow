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
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupUserListDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	role := int32(7)
	users := []entity.User{
		{ID: "a", Email: "alice@example.com", Nickname: "Reader", IsActive: "1", RoleID: &role},
		{ID: "b", Email: "bob@example.com", Nickname: "ALICE", IsActive: "0"},
		{ID: "c", Email: "carol@example.com", Nickname: "100%_Ready!", IsActive: "1", RoleID: &role},
		{ID: "d", Email: "deleted@example.com", Nickname: "Alice", Status: userListTestString("0")},
	}
	for i := range users {
		users[i].CreateDate = &created
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func userListTestString(value string) *string { return &value }

func TestUserListFiltersAndPagination(t *testing.T) {
	db := setupUserListDB(t)
	for i, id := range []string{"a", "b", "c"} {
		lastLogin := time.Date(2026, 10, 1, i, 0, 0, 0, time.UTC)
		if err := db.Model(&entity.User{}).Where("id = ?", id).Update("last_login_time", lastLogin).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		name, keyword, status, direction, order string
		offset, limit                           int
		ids                                     []string
		total                                   int64
	}{
		{name: "default order excludes deleted", ids: []string{"a", "b", "c"}, total: 3},
		{name: "email or nickname case insensitive", keyword: " ALiCe ", ids: []string{"a", "b"}, total: 2},
		{name: "combined", keyword: "alice", status: "active", ids: []string{"a"}, total: 1},
		{name: "disabled account", status: "inactive", ids: []string{"b"}, total: 1},
		{name: "literal wildcard", keyword: "%_", ids: []string{"c"}, total: 1},
		{name: "escape character", keyword: "!", ids: []string{"c"}, total: 1},
		{name: "bound keyword", keyword: "' OR 1=1 --", ids: []string{}, total: 0},
		{name: "descending last login", order: "last_login_time", direction: "desc", ids: []string{"c", "b", "a"}, total: 3},
		{name: "ascending last login", order: "last_login_time", direction: "asc", ids: []string{"a", "b", "c"}, total: 3},
		{name: "creation date tie breaker", order: "create_date", direction: "asc", ids: []string{"a", "b", "c"}, total: 3},
		{name: "filtered page", keyword: "alice", offset: 1, limit: 1, ids: []string{"b"}, total: 2},
		{name: "past last page", offset: 10, limit: 2, ids: []string{}, total: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			users, total, err := NewUserDAO().List(t.Context(), db, tt.offset, tt.limit, tt.keyword, tt.status, tt.direction, tt.order)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, 0, len(users))
			for _, user := range users {
				ids = append(ids, user.ID)
			}
			if total != tt.total || !reflect.DeepEqual(ids, tt.ids) {
				t.Fatalf("ids = %v, total = %d; want %v, %d", ids, total, tt.ids, tt.total)
			}
		})
	}
}

func TestUserListCountsCallerConditionsBeforePagination(t *testing.T) {
	db := setupUserListDB(t)
	query := db.Where("user.role_id = ?", 7)
	users, total, err := NewUserDAO().List(t.Context(), query, 1, 1, "", "active", "asc", "create_date")
	if err != nil || total != 2 || len(users) != 1 || users[0].ID != "c" {
		t.Fatalf("List() = (%v, %d, %v)", users, total, err)
	}
	// Reusing the caller's query must not inherit LIMIT/OFFSET or COUNT selection.
	users, total, err = NewUserDAO().List(t.Context(), query, 0, 10, "alice", "", "", "")
	if err != nil || total != 1 || len(users) != 1 || users[0].ID != "a" {
		t.Fatalf("reused List() = (%v, %d, %v)", users, total, err)
	}
}

func TestUserListRejectsInvalidQuery(t *testing.T) {
	db := setupUserListDB(t)
	for _, tt := range []struct{ status, direction, field string }{
		{status: "unknown"}, {direction: "desc; DROP TABLE user"}, {field: "email DESC; DROP TABLE user"},
		{field: "email"}, {field: "nickname"}, {field: "is_active"}, {field: "is_superuser"},
		{field: "id"}, {field: "create_time"}, {field: "role"}, {field: "plan"},
	} {
		_, _, err := NewUserDAO().List(t.Context(), db, 0, 10, "", tt.status, tt.direction, tt.field)
		if !errors.Is(err, ErrInvalidUserListQuery) {
			t.Fatalf("List(%+v) error = %v", tt, err)
		}
	}
}

func TestUserListQueryDryRun(t *testing.T) {
	db := setupUserListDB(t)
	query, err := userListQuery(t.Context(), db.Session(&gorm.Session{DryRun: true}).Where("user.role_id = ?", 7), "a%_", "inactive")
	if err != nil {
		t.Fatal(err)
	}
	result := query.Find(&[]entity.User{})
	sql := result.Statement.SQL.String()
	if !strings.Contains(sql, "LOWER(`user`.`email`) LIKE ? ESCAPE '!' OR LOWER(`user`.`nickname`) LIKE ? ESCAPE '!'") ||
		!strings.Contains(sql, "`user`.`is_active` = ?") || !strings.Contains(sql, "`user`.`status` <> ?") {
		t.Fatalf("unexpected SQL: %s", sql)
	}
	if !reflect.DeepEqual(result.Statement.Vars, []interface{}{7, "0", "%a!%!_%", "%a!%!_%", "0"}) {
		t.Fatalf("unexpected bindings: %#v", result.Statement.Vars)
	}
}
