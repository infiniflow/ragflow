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
	if err := db.Create(&entity.Tenant{ID: "tenant-1", ParserIDs: ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.SystemSettings{Name: "api_key.max_count", Value: "24", Source: "admin", DataType: "integer"}).Error; err != nil {
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
