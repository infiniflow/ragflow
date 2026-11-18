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
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	"strings"
	"testing"
)

func TestDeleteUserPreservesAdminRestrictions(t *testing.T) {
	for _, tt := range []struct {
		name, active, want string
		admin              bool
	}{
		{name: "active", active: "1", want: "deactivate"},
		{name: "admin", active: "0", admin: true, want: "admin account"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := testutil.SetupTestDB(t, &entity.User{})
			t.Cleanup(testutil.ReplaceDBForTest(t, db))
			user := &entity.User{ID: "actor", Email: "actor@example.test", Nickname: "Actor", IsActive: tt.active, IsAuthenticated: "1", IsAnonymous: "0", IsSuperuser: new(tt.admin)}
			if err := db.Create(user).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := NewService().DeleteUser(t.Context(), user.Email); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error=%v", err)
			}
			if _, err := dao.NewUserDAO().GetByID(t.Context(), db, user.ID); err != nil {
				t.Fatalf("user removed: %v", err)
			}
		})
	}
}
