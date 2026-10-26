//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package service

import (
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestCreateAPIKeyStoresUserAndTenantContext(t *testing.T) {
	ctx := t.Context()
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.APIToken{}); err != nil {
		t.Fatalf("failed to migrate api_token: %v", err)
	}
	pushServiceDB(t, db)

	key, err := (&SystemService{}).CreateAPIKey(ctx, "user-1", "tenant-1", &CreateAPIKeyRequest{})
	if err != nil {
		t.Fatalf("CreateAPIKey() error = %v", err)
	}
	if key.UserID != "user-1" || key.TenantID != "tenant-1" {
		t.Fatalf("CreateAPIKey() context = (%q, %q), want (user-1, tenant-1)", key.UserID, key.TenantID)
	}

	stored, err := dao.NewAPITokenDAO().GetByAPIToken(ctx, dao.DB, key.Token)
	if err != nil {
		t.Fatalf("GetByAPIToken() error = %v", err)
	}
	if stored == nil || stored.UserID == nil || *stored.UserID != "user-1" || stored.TenantID != "tenant-1" {
		t.Fatalf("stored API token context = %+v, want user-1 in tenant-1", stored)
	}
}
