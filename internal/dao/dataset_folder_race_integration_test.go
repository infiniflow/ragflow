//go:build integration

package dao

import (
	"context"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDatasetFolderMySQLFirstCreation(t *testing.T) {
	dsn := os.Getenv("DATASET_FOLDER_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("DATASET_FOLDER_TEST_MYSQL_DSN is required")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(8)
	var isolation string
	if err := db.Raw("SELECT @@transaction_isolation").Scan(&isolation).Error; err != nil || isolation != "REPEATABLE-READ" {
		t.Fatalf("isolation=%s err=%v", isolation, err)
	}
	for _, model := range []any{&entity.File{}, &entity.Knowledgebase{}, &entity.File2Document{}, &entity.Document{}, &entity.FileCommit{}, &entity.FileCommitItem{}} {
		if err := db.AutoMigrate(model); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"missing_shared_root", "missing_tenant_root", "same_dataset"} {
		t.Run(mode, func(t *testing.T) {
			tenant := common.GenerateToken()
			valid := "1"
			a := &entity.Knowledgebase{ID: tenant[:28] + "-a", TenantID: tenant, CreatedBy: tenant, Name: "alpha", ParserID: "general", Status: &valid}
			b := &entity.Knowledgebase{ID: tenant[:28] + "-b", TenantID: tenant, CreatedBy: tenant, Name: "beta", ParserID: "general", Status: &valid}
			for _, kb := range []*entity.Knowledgebase{a, b} {
				if err := db.Create(kb).Error; err != nil {
					t.Fatal(err)
				}
			}
			if mode != "missing_tenant_root" {
				id := common.GenerateToken()
				if err := db.Create(&entity.File{ID: id, ParentID: id, TenantID: tenant, CreatedBy: tenant, Name: "/", Type: "folder"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			ready := make(chan struct{}, 2)
			release := make(chan struct{})
			var reads int32
			callback := "test:first_creation_" + mode
			if err := db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "file" && strings.Contains(tx.Statement.SQL.String(), "parent_id = id") && atomic.AddInt32(&reads, 1) <= 2 {
					ready <- struct{}{}
					<-release
				}
			}); err != nil {
				t.Fatal(err)
			}
			results := make(chan error, 2)
			folders := make(chan string, 2)
			second := b
			if mode == "same_dataset" {
				second = a
			}
			for _, kb := range []*entity.Knowledgebase{a, second} {
				go func(kb *entity.Knowledgebase) {
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					folder, err := NewFileDAO().EnsureDatasetFolder(ctx, db, kb)
					if err == nil {
						folders <- folder.ID
					}
					results <- err
				}(kb)
			}
			for i := 0; i < 2; i++ {
				select {
				case <-ready:
				case <-time.After(10 * time.Second):
					close(release)
					t.Fatal("concurrent readers did not reach barrier")
				}
			}
			close(release)
			for i := 0; i < 2; i++ {
				if err := <-results; err != nil {
					t.Error(err)
				}
			}
			db.Callback().Query().Remove(callback)
			if t.Failed() {
				return
			}
			if mode == "same_dataset" {
				first, second := <-folders, <-folders
				if first != second {
					t.Fatalf("different bindings %s %s", first, second)
				}
				var n int64
				db.Model(&entity.File{}).Where("tenant_id = ? AND name = ?", tenant, "alpha").Count(&n)
				if n != 1 {
					t.Fatalf("losing transaction left %d dataset folders", n)
				}
			}
			c := &entity.Knowledgebase{ID: tenant[:28] + "-c", TenantID: tenant, CreatedBy: tenant, Name: "gamma", ParserID: "general", Status: &valid}
			if err := db.Create(c).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := NewFileDAO().EnsureDatasetFolder(t.Context(), db, c); err != nil {
				t.Fatalf("later dataset cannot resolve shared roots: %v", err)
			}
		})
	}
}
