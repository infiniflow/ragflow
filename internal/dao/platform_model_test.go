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

package dao

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"ragflow/internal/entity"
	"testing"
)

func TestPlatformProviderOwnershipAndDefaults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.SystemSettings{}, &entity.TenantModelProvider{}, &entity.Tenant{}, &entity.UserTenant{}, &entity.User{}); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	settings := PlatformModelSettings{}
	first, err := settings.CreateProvider(ctx, db, "OpenAI", "admin-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := settings.CreateProvider(ctx, db, "openai", "admin-b")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || second.UserID != "admin-a" || second.TenantID != entity.PlatformTenantID {
		t.Fatalf("unexpected providers: %+v %+v", first, second)
	}
	if err := settings.SetDefault(ctx, db, "chat", "model-1"); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetDefault(ctx, db, "chat", "model-2"); err != nil {
		t.Fatal(err)
	}
	got, err := settings.Default(ctx, db, "chat")
	if err != nil || got != "model-2" {
		t.Fatalf("default = %q, %v", got, err)
	}
	if err := settings.SetDefault(ctx, db, "chat", ""); err != nil {
		t.Fatal(err)
	}
	got, err = settings.Default(ctx, db, "chat")
	if err != nil || got != "" {
		t.Fatalf("cleared default = %q, %v", got, err)
	}
	if err := NewTenantDAO().Create(ctx, db, &entity.Tenant{ID: entity.PlatformTenantID}); err == nil {
		t.Fatal("reserved tenant accepted")
	}
	if err := db.Create(&entity.Tenant{ID: entity.PlatformTenantID}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := settings.CreateProvider(ctx, db, "Another", "admin-a"); err == nil {
		t.Fatal("existing tenant collision accepted")
	}
}

func TestPlatformInitializationRejectsExistingRecords(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  any
	}{
		{"tenant", &entity.Tenant{ID: entity.PlatformTenantID}},
		{"user", &entity.User{ID: entity.PlatformTenantID, Email: "reserved@example.com", Nickname: "reserved"}},
		{"default setting", &entity.SystemSettings{Name: entity.PlatformModelDefaultPrefix + "chat", Source: "admin", DataType: "string", Value: "other"}},
		{"forged marker", &entity.SystemSettings{Name: entity.PlatformProviderLockSetting, Source: "admin", DataType: "string", Value: "ready"}},
		{"membership", &entity.UserTenant{ID: "membership", UserID: "user", TenantID: entity.PlatformTenantID, Role: "normal"}},
		{"provider", &entity.TenantModelProvider{ID: "provider", TenantID: entity.PlatformTenantID, UserID: "user", ProviderName: "OpenAI"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&entity.SystemSettings{}, &entity.Tenant{}, &entity.UserTenant{}, &entity.User{}, &entity.TenantModelProvider{}); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(tc.row).Error; err != nil {
				t.Fatal(err)
			}
			if err := (PlatformModelSettings{}).Initialize(t.Context(), db); err == nil {
				t.Fatal("reserved ID conflict accepted")
			}
		})
	}
}
