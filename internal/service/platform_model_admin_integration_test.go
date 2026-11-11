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

package service

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"ragflow/internal/entity"
)

func TestPlatformInstanceConcurrentCreation(t *testing.T) {
	for _, tc := range []struct{ name, env string }{{"mysql", "RAGFLOW_TEST_MYSQL_DSN"}, {"postgres", "RAGFLOW_TEST_POSTGRES_DSN"}} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if dsn == "" {
				t.Skipf("%s requires a disposable database", tc.env)
			}
			var dialect gorm.Dialector
			if tc.name == "mysql" {
				dialect = mysql.Open(dsn)
			} else {
				dialect = postgres.Open(dsn)
			}
			db, err := gorm.Open(dialect, &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			pool, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = pool.Close() })
			tables := []any{&entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}}
			for _, table := range tables {
				if db.Migrator().HasTable(table) {
					t.Fatalf("refusing populated database: %T exists", table)
				}
			}
			t.Cleanup(func() {
				if err := db.Migrator().DropTable(tables...); err != nil {
					t.Error(err)
				}
			})
			if err := db.AutoMigrate(tables...); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"}).Error; err != nil {
				t.Fatal(err)
			}
			const workers = 8
			start := make(chan struct{})
			errs := make([]error, workers)
			var wg sync.WaitGroup
			for i := range errs {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					name := "shared"
					if i%2 != 0 {
						name = "SHARED"
					}
					_, errs[i] = NewModelProviderService().CreatePlatformInstance(t.Context(), db, "platform", CreateProviderInstanceRequest{InstanceName: name})
				}(i)
			}
			close(start)
			wg.Wait()
			successes := 0
			for _, err := range errs {
				if err == nil {
					successes++
				} else if !errors.Is(err, ErrPlatformInstanceConflict) {
					t.Errorf("unexpected result: %v", err)
				}
			}
			var count int64
			if err := db.Model(&entity.TenantModelInstance{}).Where("provider_id = ?", "platform").Count(&count).Error; err != nil || count != 1 || successes != 1 {
				t.Fatalf("instances=%d successes=%d err=%v", count, successes, err)
			}
			var instance entity.TenantModelInstance
			if err := db.Where("provider_id = ?", "platform").Take(&instance).Error; err != nil {
				t.Fatal(err)
			}
			model := &entity.TenantModel{ID: "model", ProviderID: "platform", InstanceID: instance.ID, ModelName: "chat", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: "{}"}
			if err := db.Create(model).Error; err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 10; i++ {
				if err := db.Model(&entity.TenantModel{}).Where("id = ?", model.ID).Update("extra", "{}").Error; err != nil {
					t.Fatal(err)
				}
				start := make(chan struct{})
				results := make(chan error, 2)
				go func() {
					<-start
					models := []CreateInstanceModelInfo{{ModelName: "chat", Extra: map[string]interface{}{"thinking": true}}}
					results <- NewModelProviderService().UpdatePlatformInstance(t.Context(), db, "platform", instance.ID, UpdatePlatformInstanceRequest{ModelInfo: &models})
				}()
				go func() {
					<-start
					results <- UpdatePlatformModel(t.Context(), db, "platform", instance.ID, model.ID, UpdatePlatformModelRequest{Extra: map[string]interface{}{"context_length": 4096}})
				}()
				close(start)
				for j := 0; j < 2; j++ {
					if err := <-results; err != nil {
						t.Fatal(err)
					}
				}
				var stored entity.TenantModel
				if err := db.Take(&stored, "id = ?", model.ID).Error; err != nil {
					t.Fatal(err)
				}
				var fields map[string]interface{}
				if err := json.Unmarshal([]byte(stored.Extra), &fields); err != nil || fields["thinking"] != true || fields["context_length"] != float64(4096) {
					t.Fatalf("concurrent changes lost: %s, %v", stored.Extra, err)
				}
			}
			// Concurrent adds of one name leave one model; the others conflict.
			addErrs := make([]error, 16)
			addStart := make(chan struct{})
			var addWG sync.WaitGroup
			for i := range addErrs {
				addWG.Add(1)
				go func(i int) {
					defer addWG.Done()
					<-addStart
					_, addErrs[i] = NewModelProviderService().AddPlatformModel(t.Context(), db, "platform", instance.ID, AddModelRequest{ModelName: "added", ModelTypes: []string{"chat"}})
				}(i)
			}
			close(addStart)
			addWG.Wait()
			added := 0
			for _, err := range addErrs {
				if err == nil {
					added++
				} else if !errors.Is(err, ErrPlatformModelConflict) {
					t.Errorf("unexpected add result: %v", err)
				}
			}
			if err := db.Model(&entity.TenantModel{}).Where("instance_id = ? AND model_name = ?", instance.ID, "added").Count(&count).Error; err != nil || count != 1 || added != 1 {
				t.Fatalf("models=%d added=%d err=%v", count, added, err)
			}
		})
	}
}
