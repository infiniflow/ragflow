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
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// userID is a real RAGFlow user id: a 32-char hex UUID string produced by
// common.GenerateToken(). It is NOT an integer, so any code path that treats
// it as uint/int is fundamentally broken.
const userIDHex = "0a1b2c3d4e5f60718293a4b5c6d7e8f90"

func newUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}
	return db
}

// TestUserDAOGetByIDAcceptsUUIDString guards the contract that user ids are
// strings (VARCHAR(32) in MySQL, 32-char hex). The DAO must accept the real
// id type and bind it by value, returning the matching row.
func TestUserDAOGetByIDAcceptsUUIDString(t *testing.T) {
	db := newUserTestDB(t)
	if err := db.Create(&entity.User{ID: userIDHex, Email: "a@example.com", Nickname: "alice"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	got, err := NewUserDAO().GetByID(t.Context(), db, userIDHex)
	if err != nil {
		t.Fatalf("GetByID(%q) error = %v", userIDHex, err)
	}
	if got.ID != userIDHex {
		t.Fatalf("GetByID(%q).ID = %q, want %q", userIDHex, got.ID, userIDHex)
	}
	if got.Nickname != "alice" {
		t.Fatalf("GetByID(%q).Nickname = %q, want alice", userIDHex, got.Nickname)
	}
}

// TestUserDAOGetByIDUnknownUUIDReturnsNotFound is adversarial: a different
// (still validly-formatted) UUID must NOT match and must surface a not-found
// error rather than silently coercing to a numeric lookup.
func TestUserDAOGetByIDUnknownUUIDReturnsNotFound(t *testing.T) {
	db := newUserTestDB(t)
	if err := db.Create(&entity.User{ID: userIDHex, Email: "a@example.com", Nickname: "alice"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	other := "ffffffffffffffffffffffffffffffff"
	_, err := NewUserDAO().GetByID(t.Context(), db, other)
	if err == nil {
		t.Fatalf("GetByID(%q) = nil error, want not-found", other)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetByID(%q) error = %v, want ErrRecordNotFound", other, err)
	}
}

// TestUserDAODeleteAcceptsString guards that Delete also takes the real string
// id type and removes exactly the targeted row.
func TestUserDAODeleteAcceptsString(t *testing.T) {
	db := newUserTestDB(t)
	if err := db.Create(&entity.User{ID: userIDHex, Email: "a@example.com", Nickname: "alice"}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := NewUserDAO().Delete(t.Context(), db, userIDHex); err != nil {
		t.Fatalf("Delete(%q) error = %v", userIDHex, err)
	}

	_, err := NewUserDAO().GetByIDString(t.Context(), db, userIDHex)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("after Delete, GetByIDString(%q) error = %v, want ErrRecordNotFound", userIDHex, err)
	}
}
