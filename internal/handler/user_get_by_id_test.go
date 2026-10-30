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

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// userIDHex is a real RAGFlow user id: a 32-char hex UUID string. The previous
// implementation parsed it with strconv.ParseUint, which rejects any
// non-numeric id and returned HTTP 400 for every real user. This test pins the
// corrected behavior: a UUID string id must resolve the user (HTTP 200).
const userIDHex = "0a1b2c3d4e5f60718293a4b5c6d7e8f90"

func TestGetUserByIDResolvesUUIDString(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}
	if err := db.Create(&entity.User{ID: userIDHex, Email: "a@example.com", Nickname: "alice"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	r := gin.New()
	r.GET("/users/:id", NewUserHandler(service.NewUserService()).GetUserByID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/"+userIDHex, nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /users/%s status = %d, want 200 (body=%s)", userIDHex, w.Code, w.Body.String())
	}

	var body struct {
		Data struct {
			ID       string `json:"id"`
			Nickname string `json:"nickname"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (body=%s)", err, w.Body.String())
	}
	if body.Data.ID != userIDHex {
		t.Fatalf("response id = %q, want %q", body.Data.ID, userIDHex)
	}
	if body.Data.Nickname != "alice" {
		t.Fatalf("response nickname = %q, want alice", body.Data.Nickname)
	}
}

// TestGetUserByIDMissingUUIDReturnsNotFound is adversarial: a well-formed but
// unknown UUID string must surface a not-found error (body code 404), never the
// "invalid user id" 400 that the old numeric ParseUint path produced for any
// non-integer id. ResponseWithCodeData always returns HTTP 200, so we assert on
// the body's business code field.
func TestGetUserByIDMissingUUIDReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}

	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	r := gin.New()
	r.GET("/users/:id", NewUserHandler(service.NewUserService()).GetUserByID)

	missing := "ffffffffffffffffffffffffffffffff"
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/"+missing, nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /users/%s status = %d, want 200 (body=%s)", missing, w.Code, w.Body.String())
	}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (body=%s)", err, w.Body.String())
	}
	if body.Code != 404 {
		t.Fatalf("GET /users/%s body code = %d, want 404 not-found (message=%q)", missing, body.Code, body.Message)
	}
}
