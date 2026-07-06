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

func setupAPITokenBetaTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&entity.APIToken{}); err != nil {
		t.Fatalf("failed to migrate api_token: %v", err)
	}

	return db
}

func TestAPITokenDAOGetByBeta(t *testing.T) {
	db := setupAPITokenBetaTestDB(t)
	pushDB(t, db)

	beta := "beta-token"
	if err := db.Create(&entity.APIToken{
		TenantID: "tenant-1",
		Token:    "token-1",
		Beta:     &beta,
	}).Error; err != nil {
		t.Fatalf("failed to create api token: %v", err)
	}

	got, err := NewAPITokenDAO().GetByBeta(beta)
	if err != nil {
		t.Fatalf("GetByBeta failed: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected token(s), got empty list")
	}
	if got[0].TenantID != "tenant-1" {
		t.Fatalf("TenantID = %q, want tenant-1", got[0].TenantID)
	}
	if got[0].Beta == nil || *got[0].Beta != beta {
		t.Fatalf("Beta = %v, want %q", got[0].Beta, beta)
	}
}
