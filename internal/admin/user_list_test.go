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

package admin

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestListUsersRejectsInvalidQueryBeforeDatabaseAccess(t *testing.T) {
	for _, tt := range []struct {
		name, status, direction, field string
		page, pageSize                 int
	}{
		{name: "status", status: "unknown", page: 1, pageSize: 10},
		{name: "direction", direction: "unknown", page: 1, pageSize: 10},
		{name: "direction injection", direction: "desc; DROP TABLE user", page: 1, pageSize: 10},
		{name: "field injection", field: "email DESC; DROP TABLE user", page: 1, pageSize: 10},
		{name: "email", field: "email", page: 1, pageSize: 10},
		{name: "nickname", field: "nickname", page: 1, pageSize: 10},
		{name: "is_active", field: "is_active", page: 1, pageSize: 10},
		{name: "is_superuser", field: "is_superuser", page: 1, pageSize: 10},
		{name: "id", field: "id", page: 1, pageSize: 10},
		{name: "create_time", field: "create_time", page: 1, pageSize: 10},
		{name: "role", field: "role", page: 1, pageSize: 10},
		{name: "plan", field: "plan", page: 1, pageSize: 10},
		{name: "storage", field: "storage", page: 1, pageSize: 10},
		{name: "documents", field: "documents", page: 1, pageSize: 10},
		{name: "index", field: "index", page: 1, pageSize: 10},
		{name: "zero page", pageSize: 10},
		{name: "negative page", page: -1, pageSize: 10},
		{name: "zero page size", page: 1},
		{name: "negative page size", page: 1, pageSize: -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// No DAO or database is configured: invalid input must fail in the service.
			var svc Service
			users, total, err := svc.ListUsers(t.Context(), tt.page, tt.pageSize, "", tt.status, tt.direction, tt.field)
			if !errors.Is(err, errInvalidUserListQuery) || users != nil || total != 0 {
				t.Fatalf("ListUsers() = (%v, %d, %v)", users, total, err)
			}
		})
	}
}

func TestListUsersFiltersAndOrdering(t *testing.T) {
	handler := setupAdminUserDetailTest(t)
	created := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.UTC)
	lastLogin := created.Add(24 * time.Hour)
	if err := dao.DB.Model(&entity.User{}).Where("id = ?", "user-1").Update("last_login_time", lastLogin.Add(24*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	deleted := "0"
	for _, user := range []entity.User{
		{ID: "user-2", Email: "second@example.com", Nickname: "USER Two", IsActive: "0", LastLoginTime: &lastLogin, BaseModel: entity.BaseModel{CreateDate: &created}},
		{ID: "user-3", Email: "third@example.com", Nickname: "Other", IsActive: "1", LastLoginTime: &created, BaseModel: entity.BaseModel{CreateDate: &lastLogin}},
		{ID: "deleted", Email: "deleted@example.com", Nickname: "User Deleted", IsActive: "1", Status: &deleted},
	} {
		if err := dao.DB.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		name, keyword, status, direction, field string
		page, pageSize                          int
		emails                                  []string
		total                                   int64
	}{
		{name: "defaults", page: 1, pageSize: 10, emails: []string{"third@example.com", "second@example.com", "user@example.com"}, total: 3},
		{name: "blank defaults", status: " ", direction: " ", field: " ", page: 1, pageSize: 10, emails: []string{"third@example.com", "second@example.com", "user@example.com"}, total: 3},
		{name: "keyword normalization", keyword: "  usER  ", page: 1, pageSize: 10, emails: []string{"second@example.com", "user@example.com"}, total: 2},
		{name: "active", status: "active", page: 1, pageSize: 10, emails: []string{"third@example.com", "user@example.com"}, total: 2},
		{name: "inactive", status: " INACTIVE ", page: 1, pageSize: 10, emails: []string{"second@example.com"}, total: 1},
		{name: "combined normalization", keyword: " USER ", status: " AcTiVe ", direction: " ASC ", field: " LAST_LOGIN_TIME ", page: 1, pageSize: 10, emails: []string{"user@example.com"}, total: 1},
		{name: "ascending creation", direction: "asc", field: "create_date", page: 1, pageSize: 10, emails: []string{"user@example.com", "second@example.com", "third@example.com"}, total: 3},
		{name: "ascending last login", direction: "asc", field: "last_login_time", page: 1, pageSize: 10, emails: []string{"third@example.com", "second@example.com", "user@example.com"}, total: 3},
		{name: "descending last login", field: "last_login_time", page: 1, pageSize: 10, emails: []string{"user@example.com", "second@example.com", "third@example.com"}, total: 3},
		{name: "filtered page", keyword: "user", page: 2, pageSize: 1, emails: []string{"user@example.com"}, total: 2},
		{name: "no matches", keyword: "missing", page: 1, pageSize: 10, emails: []string{}, total: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			users, total, err := handler.service.ListUsers(t.Context(), tt.page, tt.pageSize, tt.keyword, tt.status, tt.direction, tt.field)
			if err != nil {
				t.Fatal(err)
			}
			emails := make([]string, 0, len(users))
			for _, user := range users {
				emails = append(emails, user["email"].(string))
			}
			if total != tt.total || !reflect.DeepEqual(emails, tt.emails) {
				t.Fatalf("emails = %v, total = %d; want %v, %d", emails, total, tt.emails, tt.total)
			}
		})
	}
}
