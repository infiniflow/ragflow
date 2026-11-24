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

//go:build integration

package dao

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/entity"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestAPITokenLimitConcurrentMySQL(t *testing.T) {
	dsn := os.Getenv("RAGFLOW_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RAGFLOW_TEST_MYSQL_DSN is required")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid MySQL DSN")
	}
	config.DBName = ""
	admin, err := gorm.Open(mysql.Open(config.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatal("connect to test MySQL")
	}
	sqlAdmin, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlAdmin.Close() })
	// Own a fresh database; never alter the database named in the supplied DSN.
	name := fmt.Sprintf("api_token_limit_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE DATABASE `" + name + "`").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP DATABASE `" + name + "`").Error; err != nil {
			t.Errorf("drop disposable database: %v", err)
		}
	})
	config.DBName = name
	config.ParseTime = true
	db, err := gorm.Open(mysql.Open(config.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatal("connect to disposable database")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&entity.Tenant{}, &entity.APIToken{}, &entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	if !t.Run("atomic default seed", func(t *testing.T) {
		seed := entity.SystemSettings{Name: "api_key.max_count", Value: "16", Source: "variable", DataType: "integer"}
		existing := entity.SystemSettings{Name: "API_KEY.MAX_COUNT", Value: "37", Source: "admin", DataType: "integer"}
		if err := db.Create(&existing).Error; err != nil {
			t.Fatal(err)
		}
		if err := insertMissingSystemSettings(t.Context(), db, []entity.SystemSettings{seed}); err != nil {
			t.Fatal(err)
		}
		var stored entity.SystemSettings
		if err := db.Where("name = ?", seed.Name).Take(&stored).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Name != existing.Name || stored.Value != existing.Value || stored.Source != existing.Source || stored.DataType != existing.DataType {
			t.Fatalf("existing setting overwritten: got %+v, want %+v", stored, existing)
		}
		if err := db.Where("name = ?", seed.Name).Delete(&entity.SystemSettings{}).Error; err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		for range 16 {
			wg.Go(func() {
				<-start
				if err := insertMissingSystemSettings(t.Context(), db, []entity.SystemSettings{seed}); err != nil {
					t.Errorf("concurrent seed: %v", err)
				}
			})
		}
		close(start)
		wg.Wait()
		var rows []entity.SystemSettings
		if err := db.Where("name = ?", seed.Name).Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].Value != "16" {
			t.Fatalf("unexpected seeded rows: %+v", rows)
		}
		if err := db.Where("name = ?", seed.Name).Delete(&entity.SystemSettings{}).Error; err != nil {
			t.Fatal(err)
		}
	}) {
		return
	}
	if err := db.Create(&entity.Tenant{ID: "tenant-1", ParserIDs: ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.SystemSettings{Name: "api_key.max_count", Value: "24", Source: "admin", DataType: "integer"}).Error; err != nil {
		t.Fatal(err)
	}
	// MySQL matches mixed-case names, so they must retain quota validation.
	if err := db.Model(&entity.SystemSettings{}).Where("name = ?", "api_key.max_count").Updates(map[string]any{"name": "Api_Key.Max_Count", "value": "0"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := GetAPITokenLimit(t.Context(), db); err == nil {
		t.Fatal("accepted non-positive limit through a mixed-case setting name")
	}
	if err := db.Model(&entity.SystemSettings{}).Where("name = ?", "api_key.max_count").Update("value", "24").Error; err != nil {
		t.Fatal(err)
	}
	limit, err := GetAPITokenLimit(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var created atomic.Int32
	for i := range 48 {
		wg.Go(func() {
			<-start
			err := NewAPITokenDAO().Create(t.Context(), db, &entity.APIToken{TenantID: "tenant-1", Token: fmt.Sprint(i)})
			switch {
			case err == nil:
				created.Add(1)
			case !errors.Is(err, ErrAPITokenLimitReached):
				t.Errorf("unexpected creation error: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()
	var count int64
	if err := db.Model(&entity.APIToken{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if created.Load() != int32(limit) || count != int64(limit) {
		t.Fatalf("concurrent requests exceeded quota: created=%d stored=%d", created.Load(), count)
	}
}
