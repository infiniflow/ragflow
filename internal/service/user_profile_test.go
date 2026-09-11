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

	"ragflow/internal/entity"
)

// Regression for #14789: GetUserProfile is serialized into user-facing API
// responses (login/register/oauth/profile). It must never expose the password
// hash or the live access token; the token is delivered via the Authorization
// header + cookie instead.
func TestGetUserProfileOmitsCredentials(t *testing.T) {
	token := "live-access-token"
	pw := "bcrypt$hash$secret"
	user := &entity.User{
		ID:          "u1",
		Nickname:    "alice",
		Email:       "alice@example.com",
		AccessToken: &token,
		Password:    &pw,
	}

	profile := NewUserService().GetUserProfile(user)

	if _, ok := profile["access_token"]; ok {
		t.Errorf("GetUserProfile leaked access_token: %v", profile["access_token"])
	}
	if _, ok := profile["password"]; ok {
		t.Errorf("GetUserProfile leaked password: %v", profile["password"])
	}
	if profile["nickname"] != "alice" {
		t.Errorf("expected nickname preserved, got %v", profile["nickname"])
	}
}
