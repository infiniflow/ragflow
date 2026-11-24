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

package service

import (
	"testing"

	"ragflow/internal/entity"
)

func TestListConnectorsScopesRequestedUserToReadableResources(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.Connector{}); err != nil {
		t.Fatalf("migrate connectors: %v", err)
	}
	pushServiceDB(t, db)

	active := string(entity.StatusValid)
	for _, membership := range []entity.UserTenant{
		{ID: "user-a-tenant", UserID: "user-a", TenantID: "tenant-1", Role: "owner", Status: &active},
		{ID: "user-b-tenant", UserID: "user-b", TenantID: "tenant-1", Role: "normal", Status: &active},
	} {
		if err := db.Create(&membership).Error; err != nil {
			t.Fatalf("create membership: %v", err)
		}
	}
	for _, connector := range []entity.Connector{
		{ID: "connector-a", UserID: "user-a", TenantID: "tenant-1", Name: "A", Source: "rest", InputType: "poll", Config: entity.ConnectorConfig{}, Status: "5"},
		{ID: "connector-b", UserID: "user-b", TenantID: "tenant-1", Name: "B", Source: "rest", InputType: "poll", Config: entity.ConnectorConfig{}, Status: "5"},
	} {
		if err := db.Create(&connector).Error; err != nil {
			t.Fatalf("create connector: %v", err)
		}
	}

	ctx := serviceTestContext(t, "user-a", "tenant-1")
	listed, err := NewConnectorService().ListConnectors(ctx, "")
	if err != nil || len(listed.Connectors) != 1 || listed.Connectors[0].ID != "connector-a" {
		t.Fatalf("ListConnectors() = (%#v, %v), want only caller's connector", listed, err)
	}
	listed, err = NewConnectorService().ListConnectors(ctx, "user-b")
	if err != nil || len(listed.Connectors) != 1 || listed.Connectors[0].ID != "connector-b" {
		t.Fatalf("ListConnectors(access_user_id=user-b) = (%#v, %v), want user-b's connector", listed, err)
	}
}
