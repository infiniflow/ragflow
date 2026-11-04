package handler_test

import (
	"net/url"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/handler"
	filesvc "ragflow/internal/service/file"
	"ragflow/internal/storage"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestEmptyDatasetHistoryRemainsResolvable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&entity.File{}, &entity.File2Document{}, &entity.Document{}, &entity.Knowledgebase{}, &entity.FileCommit{}, &entity.FileCommitItem{}, &entity.SkillSpace{}); err != nil {
		t.Fatal(err)
	}
	previous := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = previous })
	factory := storage.GetStorageFactory()
	priorStorage := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(priorStorage) })
	put := func(v any) {
		t.Helper()
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	status := "1"
	put(&entity.Knowledgebase{ID: "dataset", Name: "alpha", TenantID: "tenant", CreatedBy: "tenant", Status: &status})
	put(&entity.File{ID: "root", ParentID: "root", Name: "/", Type: "folder", TenantID: "tenant", CreatedBy: "tenant"})
	put(&entity.File{ID: "shared", ParentID: "root", Name: ".knowledgebase", Type: "folder", TenantID: "tenant", CreatedBy: "tenant", SourceType: "knowledgebase"})
	put(&entity.File{ID: "folder", ParentID: "shared", Name: "alpha", Type: "folder", TenantID: "tenant", CreatedBy: "tenant", SourceType: "knowledgebase"})
	commit, err := filesvc.NewFileCommitService().CreateCommit(t.Context(), "folder", "tenant", "initial", nil)
	if err != nil {
		t.Fatal(err)
	}
	commits, total, err := filesvc.NewFileCommitService().ListCommits(t.Context(), "folder", 1, 10, "create_time", true)
	if err != nil || total != 1 || len(commits) != 1 || commits[0].ID != commit.ID {
		t.Fatalf("commit history not stored: total=%d err=%v", total, err)
	}
	got, err := handler.NewFileCommitHandler(nil).ResolveFolderID(t.Context(), "datasets", "dataset")
	if err != nil || got != "folder" {
		t.Fatalf("read-only dataset history resolution failed despite existing history: folder=%s err=%v", got, err)
	}
}
