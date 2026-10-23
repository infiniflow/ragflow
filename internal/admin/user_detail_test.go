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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"

	"github.com/gin-gonic/gin"
)

type adminTestResponse struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

func setupAdminUserDetailTest(t *testing.T) *Handler {
	t.Helper()
	db := testutil.SetupTestDB(t,
		&entity.User{},
	)
	t.Cleanup(testutil.ReplaceDBForTest(t, db))

	now := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	isSuperuser := false
	statusValid := string(entity.StatusValid)
	language := "English"
	loginChannel := "password"
	user := entity.User{
		ID:              "user-1",
		Nickname:        "User One",
		Email:           "user@example.com",
		Language:        &language,
		IsAuthenticated: "1",
		IsActive:        "1",
		IsAnonymous:     "0",
		LoginChannel:    &loginChannel,
		Status:          &statusValid,
		IsSuperuser:     &isSuperuser,
		BaseModel:       entity.BaseModel{CreateDate: &now, UpdateDate: &now},
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	return NewHandler(NewService())
}

func callAdminUserHandler(t *testing.T, handler func(*gin.Context)) adminTestResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "username", Value: "user@example.com"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handler(ctx)

	var response adminTestResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != int(common.CodeSuccess) {
		t.Fatalf("response code = %d, want %d; body = %s", response.Code, common.CodeSuccess, recorder.Body.String())
	}
	return response
}

func TestGetUserReturnsDetailList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := setupAdminUserDetailTest(t)
	response := callAdminUserHandler(t, handler.GetUser)

	var details []struct {
		Email       string `json:"email"`
		IsActive    string `json:"is_active"`
		CreateDate  string `json:"create_date"`
		IsAnonymous string `json:"is_anonymous"`
	}
	if err := json.Unmarshal(response.Data, &details); err != nil {
		t.Fatalf("decode user details: %v", err)
	}
	if len(details) != 1 {
		t.Fatalf("user details length = %d, want 1", len(details))
	}
	if details[0].Email != "user@example.com" || details[0].IsActive != "1" || details[0].CreateDate == "" || details[0].IsAnonymous != "0" {
		t.Fatalf("unexpected user details: %+v", details[0])
	}
}
