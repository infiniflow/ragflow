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
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

func TestCompletePasswordLoginRequiresUnchangedPassword(t *testing.T) {
	db := testutil.SetupTestDB(t, &entity.User{})
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	user := &entity.User{ID: "user-id", Password: new("old-hash"), AccessToken: new("old-token")}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	if _, code, err := completePasswordLogin(t.Context(), user); err != nil || code != common.CodeSuccess || *user.AccessToken == "old-token" {
		t.Fatalf("login with matching password: code=%v, error=%v", code, err)
	}
	if err := db.Model(user).Updates(map[string]any{"password": "new-hash", "access_token": "invalidated-token"}).Error; err != nil {
		t.Fatal(err)
	}
	user.Password = new("old-hash")
	if _, code, err := completePasswordLogin(t.Context(), user); err == nil || code != common.CodeAuthenticationError {
		t.Fatalf("login with stale password: code=%v, error=%v", code, err)
	}
	var saved entity.User
	if err := db.Take(&saved, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if *saved.Password != "new-hash" || *saved.AccessToken != "invalidated-token" {
		t.Fatal("stale login modified account credentials")
	}
}

func TestUpdateUserSettingsDoesNotRestoreStaleCredentials(t *testing.T) {
	db := testutil.SetupTestDB(t, &entity.User{})
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	user := &entity.User{ID: "user-id", Password: new("old-hash"), AccessToken: new("old-token")}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&entity.User{}).Where("id = ?", user.ID).
		Updates(map[string]any{"password": "new-hash", "access_token": "invalidated-token"}).Error; err != nil {
		t.Fatal(err)
	}
	code, err := NewUserService().UpdateUserSettings(t.Context(), user, &UpdateSettingsRequest{Nickname: new("Updated")})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("update settings: code=%v, error=%v", code, err)
	}
	var saved entity.User
	if err := db.Take(&saved, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Nickname != "Updated" || *saved.Password != "new-hash" || *saved.AccessToken != "invalidated-token" {
		t.Fatal("profile update did not preserve the current credentials")
	}
}
