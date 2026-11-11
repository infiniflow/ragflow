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
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type platformEmbeddingFixture struct {
	ID           string  `gorm:"primaryKey;size:32"`
	TenantID     string  `gorm:"size:32;index"`
	Status       string  `gorm:"size:1"`
	EmbdID       string  `gorm:"size:128"`
	TenantEmbdID *string `gorm:"size:32"`
	ChunkNum     int64
	entity.BaseModel
}

func (platformEmbeddingFixture) TableName() string { return "knowledgebase" }

func TestBindDatasetEmbeddingConcurrentDefaults(t *testing.T) {
	for _, tc := range []struct{ name, env string }{{"mysql", "RAGFLOW_TEST_MYSQL_DSN"}, {"postgres", "RAGFLOW_TEST_POSTGRES_DSN"}} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if dsn == "" {
				t.Skipf("%s requires disposable database", tc.env)
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
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			models := []any{&entity.SystemSettings{}, &entity.Tenant{}, &entity.UserTenant{}, &entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}, &platformEmbeddingFixture{}}
			for _, model := range models {
				if db.Migrator().HasTable(model) {
					t.Fatalf("refusing populated database: %T exists", model)
				}
			}
			t.Cleanup(func() {
				if err := db.Migrator().DropTable(models...); err != nil {
					t.Error(err)
				}
			})
			if err := db.AutoMigrate(models...); err != nil {
				t.Fatal(err)
			}
			previous := dao.DB
			dao.DB = db
			t.Cleanup(func() { dao.DB = previous })
			active := "1"
			modelA, modelB := "embedding-a", "embedding-b"
			rows := []any{
				&entity.SystemSettings{Name: entity.PlatformProviderLockSetting, Source: "system", DataType: "string", Value: "ready"},
				&entity.Tenant{ID: "tenant", Status: &active},
				&entity.UserTenant{ID: "owner", TenantID: "tenant", UserID: "owner", Role: "owner", Status: &active, TenantEmbdID: &modelA},
				&entity.UserTenant{ID: "member", TenantID: "tenant", UserID: "member", Role: "normal", Status: &active, TenantEmbdID: &modelB},
				&entity.TenantModelProvider{ID: "provider", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
				&entity.TenantModelInstance{ID: "instance", ProviderID: "provider", InstanceName: "default", APIKey: "secret", Status: "active", Extra: "{}"},
				&entity.TenantModel{ID: modelA, ProviderID: "provider", InstanceID: "instance", ModelName: "text-embedding-3-small", ModelType: int(entity.ModelTypeEmbedding), Status: "active"},
				&entity.TenantModel{ID: modelB, ProviderID: "provider", InstanceID: "instance", ModelName: "text-embedding-3-large", ModelType: int(entity.ModelTypeEmbedding), Status: "active"},
				&platformEmbeddingFixture{ID: "dataset", TenantID: "tenant", Status: active},
			}
			for _, row := range rows {
				if err := db.Create(row).Error; err != nil {
					t.Fatalf("seed %T: %v", row, err)
				}
			}
			const n = 8
			refs := make([]string, n)
			errs := make([]error, n)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < n; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					user := "owner"
					if i%2 != 0 {
						user = "member"
					}
					ctx := permission.WithIdentity(t.Context(), user, "tenant")
					refs[i], errs[i] = BindDatasetEmbedding(ctx, db, fakeChatDocEngine{}, "dataset", "tenant")
				}(i)
			}
			close(start)
			wg.Wait()
			for i, err := range errs {
				if err != nil {
					t.Errorf("worker %d: %v", i, err)
				} else if refs[i] != refs[0] {
					t.Errorf("worker %d resolved %q, want %q", i, refs[i], refs[0])
				}
			}
			var stored platformEmbeddingFixture
			if err := db.Take(&stored, "id = ?", "dataset").Error; err != nil {
				t.Fatal(err)
			}
			if stored.TenantEmbdID == nil || stored.EmbdID != refs[0] || *stored.TenantEmbdID != refs[0] || (refs[0] != modelA && refs[0] != modelB) {
				t.Fatal(fmt.Sprintf("inconsistent binding: %+v refs=%v", stored, refs))
			}
			// Hold an uncommitted model change while a writer tries to use the
			// previous reference. It must wait, then reject the stale vector.
			next := modelA
			if refs[0] == modelA {
				next = modelB
			}
			tx := db.Begin()
			if tx.Error != nil {
				t.Fatal(tx.Error)
			}
			defer tx.Rollback()
			if err := tx.Model(&platformEmbeddingFixture{}).Where("id = ?", "dataset").Updates(map[string]any{"embd_id": next, "tenant_embd_id": next}).Error; err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			wrote := make(chan struct{}, 1)
			go func() {
				result <- WithStableDatasetEmbedding(t.Context(), db, "dataset", "tenant", refs[0], func() error { wrote <- struct{}{}; return nil })
			}()
			select {
			case err := <-result:
				t.Fatalf("writer did not wait for model change: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			if err := tx.Commit().Error; err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("stale vector write was accepted")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("writer did not resume after model change")
			}
			select {
			case <-wrote:
				t.Fatal("stale vector reached index")
			default:
			}
			// A model change must wait for a retrieval already using that model.
			readStarted := make(chan struct{})
			releaseRead := make(chan struct{})
			readDone := make(chan error, 1)
			changeDone := make(chan error, 1)
			go func() {
				kb := &entity.Knowledgebase{ID: "dataset", TenantID: "tenant", EmbdID: next}
				readDone <- dao.WithStableDatasetEmbeddingBindings(t.Context(), db, []*entity.Knowledgebase{kb}, func(context.Context) error {
					close(readStarted)
					<-releaseRead
					return nil
				})
			}()
			select {
			case <-readStarted:
			case err := <-readDone:
				t.Fatalf("retrieval failed: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("retrieval did not start")
			}
			parallelRead := make(chan error, 1)
			go func() {
				kb := &entity.Knowledgebase{ID: "dataset", TenantID: "tenant", EmbdID: next}
				parallelRead <- dao.WithStableDatasetEmbeddingBindings(t.Context(), db, []*entity.Knowledgebase{kb}, func(context.Context) error { return nil })
			}()
			select {
			case err := <-parallelRead:
				if err != nil {
					t.Fatalf("concurrent retrieval failed: %v", err)
				}
			case <-time.After(5 * time.Second):
				close(releaseRead)
				t.Fatal("concurrent retrieval blocked by another reader")
			}
			parallelWrite := make(chan error, 1)
			go func() {
				parallelWrite <- WithStableDatasetEmbedding(t.Context(), db, "dataset", "tenant", next, func() error { return nil })
			}()
			select {
			case err := <-parallelWrite:
				if err != nil {
					t.Fatalf("same-model vector write failed: %v", err)
				}
			case <-time.After(5 * time.Second):
				close(releaseRead)
				t.Fatal("same-model vector write blocked by retrieval")
			}
			go func() {
				changeDone <- db.Model(&platformEmbeddingFixture{}).Where("id = ?", "dataset").Updates(map[string]any{"embd_id": refs[0], "tenant_embd_id": refs[0]}).Error
			}()
			select {
			case err := <-changeDone:
				t.Fatalf("model change did not wait for retrieval: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			close(releaseRead)
			if err := <-readDone; err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-changeDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("model change remained locked")
			}

		})
	}
}

// A dataset bound through the platform default keeps that embedding model when
// the platform default later changes; only a dataset bound afterwards gets the
// new one.
func TestPlatformDefaultChangeKeepsDatasetEmbedding(t *testing.T) {
	for _, tc := range []struct{ name, env string }{{"mysql", "RAGFLOW_TEST_MYSQL_DSN"}, {"postgres", "RAGFLOW_TEST_POSTGRES_DSN"}} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if dsn == "" {
				t.Skipf("%s requires disposable database", tc.env)
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
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			models := []any{&entity.SystemSettings{}, &entity.Tenant{}, &entity.UserTenant{}, &entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{}, &platformEmbeddingFixture{}}
			for _, model := range models {
				if db.Migrator().HasTable(model) {
					t.Fatalf("refusing populated database: %T exists", model)
				}
			}
			t.Cleanup(func() {
				if err := db.Migrator().DropTable(models...); err != nil {
					t.Error(err)
				}
			})
			if err := db.AutoMigrate(models...); err != nil {
				t.Fatal(err)
			}
			previous := dao.DB
			dao.DB = db
			t.Cleanup(func() { dao.DB = previous })
			active := "1"
			modelA, modelB := "embedding-a", "embedding-b"
			rows := []any{
				&entity.SystemSettings{Name: entity.PlatformProviderLockSetting, Source: "system", DataType: "string", Value: "ready"},
				&entity.Tenant{ID: "tenant", Status: &active},
				// The owner has no embedding default of their own.
				&entity.UserTenant{ID: "owner", TenantID: "tenant", UserID: "owner", Role: "owner", Status: &active},
				&entity.TenantModelProvider{ID: "provider", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
				&entity.TenantModelInstance{ID: "instance", ProviderID: "provider", InstanceName: "default", APIKey: "secret", Status: "active", Extra: "{}"},
				&entity.TenantModel{ID: modelA, ProviderID: "provider", InstanceID: "instance", ModelName: "text-embedding-3-small", ModelType: int(entity.ModelTypeEmbedding), Status: "active"},
				&entity.TenantModel{ID: modelB, ProviderID: "provider", InstanceID: "instance", ModelName: "text-embedding-3-large", ModelType: int(entity.ModelTypeEmbedding), Status: "active"},
				&platformEmbeddingFixture{ID: "first", TenantID: "tenant", Status: active},
				&platformEmbeddingFixture{ID: "second", TenantID: "tenant", Status: active},
			}
			for _, row := range rows {
				if err := db.Create(row).Error; err != nil {
					t.Fatalf("seed %T: %v", row, err)
				}
			}
			settings := dao.PlatformModelSettings{}
			ctx := permission.WithIdentity(t.Context(), "owner", "tenant")
			if err := settings.SetDefault(t.Context(), db, "embedding", modelA); err != nil {
				t.Fatal(err)
			}
			ref, err := BindDatasetEmbedding(ctx, db, fakeChatDocEngine{}, "first", "tenant")
			if err != nil || ref != modelA {
				t.Fatalf("first binding = %q, %v", ref, err)
			}
			if err := settings.SetDefault(t.Context(), db, "embedding", modelB); err != nil {
				t.Fatal(err)
			}
			if ref, err := BindDatasetEmbedding(ctx, db, fakeChatDocEngine{}, "first", "tenant"); err != nil || ref != modelA {
				t.Fatalf("bound dataset followed the new default: %q, %v", ref, err)
			}
			if ref, err := BindDatasetEmbedding(ctx, db, fakeChatDocEngine{}, "second", "tenant"); err != nil || ref != modelB {
				t.Fatalf("new dataset binding = %q, %v", ref, err)
			}
			var first platformEmbeddingFixture
			if err := db.Take(&first, "id = ?", "first").Error; err != nil || first.EmbdID != modelA {
				t.Fatalf("stored binding changed: %+v, %v", first, err)
			}
		})
	}
}
