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

func TestGetOwnerByTenantIDFallsBackToTenantIDUserID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserTenant{}); err != nil {
		t.Fatalf("migrate user_tenant: %v", err)
	}

	status := entity.TenantMembershipStatusJoined
	if err := db.Create(&entity.UserTenant{
		ID: "owner-link", UserID: "tenant-1", TenantID: "tenant-1", Status: &status,
		LLMID: "owner-default@provider",
	}).Error; err != nil {
		t.Fatalf("create owner membership: %v", err)
	}

	membership, err := NewUserTenantDAO().GetOwnerByTenantID(t.Context(), db, "tenant-1")
	if err != nil {
		t.Fatalf("GetOwnerByTenantID() error = %v", err)
	}
	if membership.UserID != "tenant-1" || membership.LLMID != "owner-default@provider" {
		t.Fatalf("GetOwnerByTenantID() = %#v, want the tenant_id=user_id membership", membership)
	}
}
