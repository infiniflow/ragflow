//go:build integration

package dataset

import (
	"context"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	filesvc "ragflow/internal/service/file"
)

// RAGFLOW_TEST_MYSQL_DSN must allow creating disposable databases and reading
// performance_schema.data_lock_waits. Every subtest owns and drops its schema.
func setupFolderConcurrencyDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dsn := os.Getenv("RAGFLOW_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RAGFLOW_TEST_MYSQL_DSN unset: requires real MySQL with performance_schema")
	}
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = ""
	cfg.ParseTime = true
	admin, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adminSQL.Close() })
	schema := "ragflow_folder_" + common.GenerateToken()
	if err := admin.Exec("CREATE DATABASE `" + schema + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP DATABASE `" + schema + "`").Error; err != nil {
			t.Error(err)
		}
	})
	cfg.DBName = schema
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(8)
	t.Cleanup(func() { _ = sqlDB.Close() })
	models := []interface{}{
		&entity.File{}, &entity.File2Document{}, &entity.Document{}, &entity.Knowledgebase{}, &entity.User{}, &entity.Tenant{},
		&entity.UserTenant{}, &entity.Connector{}, &entity.Connector2Kb{}, &entity.SyncLogs{},
	}
	models = append(models, datasetTestExtraModels()...)
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatal(err)
	}
	return db, schema
}

type folderConcurrencyActor struct{}

type folderConcurrencyResult struct {
	name string
	code common.ErrorCode
	err  error
}

func TestDatasetRenameFolderConcurrentCreateIntegration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		first    string
		fileType string
		rollback bool
	}{
		{name: "rename then create folder", first: "rename", fileType: "folder"},
		{name: "rename then create virtual", first: "rename", fileType: "virtual"},
		{name: "create folder then rename", first: "create", fileType: "folder"},
		{name: "create virtual then rename", first: "create", fileType: "virtual"},
		{name: "rename rollback releases parent", first: "rename", fileType: "folder", rollback: true},
		{name: "create rollback releases parent", first: "create", fileType: "folder", rollback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, schema := setupFolderConcurrencyDB(t)
			pushServiceDB(t, db)
			insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
			insertRenameTestFolder(t, "root", "root", "tenant-1", "/", "")
			insertRenameTestFolder(t, "kb-root", "root", "tenant-1", ".knowledgebase", "knowledgebase")
			insertRenameTestFolder(t, "original", "kb-root", "tenant-1", "Original", "knowledgebase")
			if tc.rollback {
				event := "UPDATE"
				if tc.first == "create" {
					event = "INSERT"
				}
				if err := db.Exec("CREATE TRIGGER reject_first BEFORE " + event + " ON file FOR EACH ROW BEGIN IF NEW.name = 'Renamed' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'reject first writer'; END IF; END").Error; err != nil {
					t.Fatal(err)
				}
			}

			ctx, cancel := context.WithTimeout(withDatasetTestContext(t.Context(), "tenant-1", "tenant-1"), 20*time.Second)
			defer cancel()
			checked, release := make(chan struct{}), make(chan struct{})
			var paused atomic.Bool
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			// Pause after the first writer's real name lookup. No SQL or service
			// implementation is replaced; the other writer runs on another connection.
			if err := db.Callback().Query().After("gorm:query").Register("folder_test:pause", func(tx *gorm.DB) {
				if tx.Statement.Context.Value(folderConcurrencyActor{}) != tc.first || tx.Statement.Table != "file" || !strings.Contains(tx.Statement.SQL.String(), "LOWER(name)") || !paused.CompareAndSwap(false, true) {
					return
				}
				close(checked)
				select {
				case <-release:
				case <-ctx.Done():
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Callback().Query().Remove("folder_test:pause") }()
			datasets := testDatasetUpdateService(t)
			files := filesvc.NewFileService(nil)
			run := func(actor string) folderConcurrencyResult {
				actorCtx := context.WithValue(ctx, folderConcurrencyActor{}, actor)
				if actor == "rename" {
					name := "Renamed"
					_, code, err := datasets.UpdateDataset(actorCtx, "kb-1", service.UpdateDatasetRequest{Name: &name})
					return folderConcurrencyResult{code: code, err: err}
				}
				result, err := files.CreateFolder(actorCtx, "Renamed", "kb-root", tc.fileType)
				if err != nil {
					return folderConcurrencyResult{err: err}
				}
				return folderConcurrencyResult{name: result["name"].(string)}
			}
			firstDone, secondDone := make(chan folderConcurrencyResult, 1), make(chan folderConcurrencyResult, 1)
			var workers sync.WaitGroup
			defer func() { unblock(); cancel(); workers.Wait() }()
			workers.Add(1)
			go func() { defer workers.Done(); firstDone <- run(tc.first) }()
			select {
			case <-checked:
			case result := <-firstDone:
				t.Fatalf("first writer stopped before checking name: %+v", result)
			case <-ctx.Done():
				t.Fatal("first writer did not reach name lookup")
			}
			// An unrelated parent must remain usable while this parent is held.
			if _, err := files.CreateFolder(ctx, "Independent", "root", "folder"); err != nil {
				t.Fatalf("unrelated parent was blocked: %v", err)
			}
			second := "create"
			if tc.first == "create" {
				second = "rename"
			}
			workers.Add(1)
			go func() { defer workers.Done(); secondDone <- run(second) }()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for waiting := false; !waiting; {
				select {
				case result := <-secondDone:
					t.Fatalf("second writer bypassed parent lock: %+v", result)
				case <-ctx.Done():
					t.Fatal("second writer never waited for parent lock")
				case <-ticker.C:
					var n int64
					if err := db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM performance_schema.data_lock_waits w
JOIN performance_schema.data_locks l ON l.ENGINE = w.ENGINE AND l.ENGINE_LOCK_ID = w.REQUESTING_ENGINE_LOCK_ID
WHERE l.OBJECT_SCHEMA = ? AND l.OBJECT_NAME = 'file'`, schema).Scan(&n).Error; err != nil {
						t.Fatal(err)
					}
					waiting = n > 0
				}
			}
			unblock()
			await := func(ch <-chan folderConcurrencyResult) folderConcurrencyResult {
				select {
				case result := <-ch:
					return result
				case <-ctx.Done():
					t.Fatal("writer did not complete after parent was released")
					return folderConcurrencyResult{}
				}
			}
			firstResult, secondResult := await(firstDone), await(secondDone)
			if tc.rollback {
				if firstResult.err == nil || secondResult.err != nil {
					t.Fatalf("rollback results: first=%+v second=%+v", firstResult, secondResult)
				}
			} else if firstResult.err != nil {
				t.Fatalf("first writer failed: %+v", firstResult)
			}
			wantKB, wantCreated := "Renamed", "Renamed(1)"
			if tc.first == "create" && !tc.rollback {
				wantKB, wantCreated = "Original", "Renamed"
				if secondResult.err == nil || secondResult.code != common.CodeDataError {
					t.Fatalf("rename should report collision: %+v", secondResult)
				}
			} else if secondResult.err != nil {
				t.Fatalf("second writer failed: %+v", secondResult)
			}
			if tc.rollback && tc.first == "rename" {
				wantKB, wantCreated = "Original", "Renamed"
			}
			if tc.rollback && tc.first == "create" {
				wantCreated = ""
			}
			kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, "kb-1")
			if err != nil || kb.Name != wantKB {
				t.Fatalf("dataset=%+v err=%v; want %s", kb, err, wantKB)
			}
			var original entity.File
			if err := db.Where("id = ?", "original").Take(&original).Error; err != nil || original.Name != wantKB {
				t.Fatalf("original=%+v err=%v; want %s", original, err, wantKB)
			}
			if !tc.rollback || tc.first == "rename" {
				created := secondResult
				if tc.first == "create" {
					created = firstResult
				}
				if created.name != wantCreated {
					t.Fatalf("created name=%q; want %q", created.name, wantCreated)
				}
			}
			wantEntries := int64(2)
			if wantCreated == "" {
				wantEntries = 1
			}
			var count int64
			if err := db.Model(&entity.File{}).Where("parent_id = ?", "kb-root").Count(&count).Error; err != nil || count != wantEntries {
				t.Fatalf("namespace entries=%d want=%d err=%v", count, wantEntries, err)
			}
			var duplicates []struct {
				Name  string
				Count int64
			}
			if err := db.Model(&entity.File{}).Select("LOWER(name) AS name, COUNT(*) AS count").Where("parent_id = ?", "kb-root").Group("LOWER(name)").Having("COUNT(*) > 1").Scan(&duplicates).Error; err != nil || len(duplicates) != 0 {
				t.Fatalf("duplicate names: %v err=%v", duplicates, err)
			}
			t.Logf("observed MySQL lock wait; final dataset=%s, create=%s", wantKB, wantCreated)
		})
	}
}
