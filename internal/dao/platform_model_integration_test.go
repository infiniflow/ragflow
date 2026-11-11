//go:build integration

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
	"fmt"
	"os"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"ragflow/internal/entity"
)

func TestPlatformProviderConcurrentCreation(t *testing.T) {
	for _, tc := range []struct{ name, env string }{
		{"mysql", "RAGFLOW_TEST_MYSQL_DSN"},
		{"postgres", "RAGFLOW_TEST_POSTGRES_DSN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if dsn == "" {
				t.Skipf("%s requires a disposable database", tc.env)
			}
			var dialector gorm.Dialector
			if tc.name == "mysql" {
				dialector = newMigrationAwareDialector(dsn)
			} else {
				dialector = postgres.Open(dsn)
			}
			db, err := gorm.Open(dialector, &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { sqlDB.Close() })
			for _, table := range []string{"system_settings", "tenant_model_provider", "tenant", "user_tenant", "user"} {
				if db.Migrator().HasTable(table) {
					t.Fatalf("refusing to use populated database: %s exists", table)
				}
			}
			models := []interface{}{&entity.SystemSettings{}, &entity.TenantModelProvider{}, &entity.Tenant{}, &entity.UserTenant{}, &entity.User{}}
			t.Cleanup(func() {
				if err := db.Migrator().DropTable(models...); err != nil {
					t.Error(err)
				}
			})
			if err := db.Migrator().CreateTable(&platformTestUser{}); err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(models[:4]...); err != nil {
				t.Fatal(err)
			}
			const workers = 8
			ids := make([]string, workers)
			errs := make([]error, workers)
			var wg sync.WaitGroup
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					provider, err := (PlatformModelSettings{}).CreateProvider(t.Context(), db, "OpenAI", fmt.Sprintf("admin-%d", i))
					errs[i] = err
					if err == nil {
						ids[i] = provider.ID
					}
				}(i)
			}
			wg.Wait()
			for i, err := range errs {
				if err != nil {
					t.Errorf("worker %d: %v", i, err)
				} else if ids[i] != ids[0] {
					t.Errorf("worker %d id %q differs from %q", i, ids[i], ids[0])
				}
			}
			var count int64
			if err := db.Model(&entity.TenantModelProvider{}).Where("tenant_id = ?", entity.PlatformTenantID).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("providers = %d, error %v", count, err)
			}
		})
	}
}

// openPlatformTestDB opens a disposable database and creates the tables the
// platform checks read, dropping them afterwards.
func openPlatformTestDB(t *testing.T, name, env string) *gorm.DB {
	t.Helper()
	dsn := os.Getenv(env)
	if dsn == "" {
		t.Skipf("%s requires a disposable database", env)
	}
	var dialector gorm.Dialector
	if name == "mysql" {
		dialector = newMigrationAwareDialector(dsn)
	} else {
		dialector = postgres.Open(dsn)
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	for _, table := range []string{"system_settings", "tenant_model_provider", "tenant", "user_tenant", "user"} {
		if db.Migrator().HasTable(table) {
			t.Fatalf("refusing to use populated database: %s exists", table)
		}
	}
	models := []interface{}{&entity.SystemSettings{}, &entity.TenantModelProvider{}, &entity.Tenant{}, &entity.UserTenant{}, &platformTestUser{}}
	t.Cleanup(func() {
		if err := db.Migrator().DropTable(models...); err != nil {
			t.Error(err)
		}
	})
	if err := db.Migrator().CreateTable(&platformTestUser{}); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(models[:4]...); err != nil {
		t.Fatal(err)
	}
	return db
}

// The reserved platform ID, or records only the platform may own, stop the
// initialization on a real database, including MySQL's case-insensitive
// comparison; nothing is marked ready.
func TestPlatformInitializationRejectsReservedRecords(t *testing.T) {
	joined := entity.TenantMembershipStatusJoined
	cases := []struct {
		name  string
		mysql bool
		seed  []any
	}{
		{name: "tenant", seed: []any{&entity.Tenant{ID: entity.PlatformTenantID}}},
		{name: "membership", seed: []any{&entity.UserTenant{ID: "m1", UserID: "u1", TenantID: entity.PlatformTenantID, Role: "owner", Status: &joined}}},
		{name: "user", seed: []any{&platformTestUser{ID: entity.PlatformTenantID}}},
		{name: "tenant in other case", mysql: true, seed: []any{&entity.Tenant{ID: "__PLATFORM__"}}},
		{name: "unknown default", seed: []any{&entity.SystemSettings{Name: entity.PlatformModelDefaultPrefix + "chat", Value: "m1", Source: "admin", DataType: "string"}}},
		{name: "unknown provider", seed: []any{&entity.TenantModelProvider{ID: "p1", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"}}},
	}
	for _, db := range []struct{ name, env string }{
		{"mysql", "RAGFLOW_TEST_MYSQL_DSN"},
		{"postgres", "RAGFLOW_TEST_POSTGRES_DSN"},
	} {
		for _, tc := range cases {
			if tc.mysql && db.name != "mysql" {
				continue
			}
			t.Run(db.name+"/"+tc.name, func(t *testing.T) {
				conn := openPlatformTestDB(t, db.name, db.env)
				for _, row := range tc.seed {
					if err := conn.Create(row).Error; err != nil {
						t.Fatal(err)
					}
				}
				if err := (PlatformModelSettings{}).Initialize(t.Context(), conn); err == nil {
					t.Fatal("initialization accepted a reserved record")
				}
				var ready int64
				if err := conn.Model(&entity.SystemSettings{}).Where("name = ? AND value = ?", entity.PlatformProviderLockSetting, "ready").Count(&ready).Error; err != nil || ready != 0 {
					t.Fatalf("marked ready: %d, %v", ready, err)
				}
			})
		}
	}
}

type platformTestUser struct {
	ID string `gorm:"column:id;primaryKey;size:32"`
}

func (platformTestUser) TableName() string { return "user" }
