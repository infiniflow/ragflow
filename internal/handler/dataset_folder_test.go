package handler_test

import (
	"net/url"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/handler"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestResolveDatasetFolder(t *testing.T) {
	for _, scenario := range []string{"parent_scope", "reserved_name", "duplicate_candidates", "empty_history"} {
		t.Run(scenario, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { raw.Close() })
			if err = db.AutoMigrate(&entity.File{}, &entity.File2Document{}, &entity.Document{}, &entity.Knowledgebase{}, &entity.FileCommit{}, &entity.FileCommitItem{}, &entity.SkillSpace{}); err != nil {
				t.Fatal(err)
			}
			old := dao.DB
			dao.DB = db
			t.Cleanup(func() { dao.DB = old })
			put := func(v any) {
				t.Helper()
				if err := db.Create(v).Error; err != nil {
					t.Fatal(err)
				}
			}
			name := "alpha"
			if scenario == "reserved_name" {
				name = ".knowledgebase"
			}
			status := "1"
			put(&entity.Knowledgebase{ID: "dataset", TenantID: "tenant", CreatedBy: "tenant", Name: name, Status: &status})
			put(&entity.File{ID: "root", ParentID: "root", TenantID: "tenant", CreatedBy: "tenant", Name: "/", Type: "folder"})
			if scenario == "parent_scope" {
				put(&entity.File{ID: "wrong", ParentID: "root", TenantID: "tenant", CreatedBy: "tenant", Name: name, Type: "folder", SourceType: "knowledgebase"})
			}
			put(&entity.File{ID: "shared-root", ParentID: "root", TenantID: "tenant", CreatedBy: "tenant", Name: ".knowledgebase", Type: "folder", SourceType: "knowledgebase"})
			put(&entity.File{ID: "correct", ParentID: "shared-root", TenantID: "tenant", CreatedBy: "tenant", Name: name, Type: "folder", SourceType: "knowledgebase"})
			if scenario == "duplicate_candidates" {
				put(&entity.File{ID: common.GenerateToken(), ParentID: "shared-root", TenantID: "tenant", CreatedBy: "tenant", Name: name, Type: "folder", SourceType: "knowledgebase"})
			}
			if scenario == "empty_history" {
				put(&entity.FileCommit{ID: "history", FolderID: "correct", AuthorID: "tenant"})
			}
			got, err := handler.NewFileCommitHandler(nil).ResolveFolderID(t.Context(), "datasets", "dataset")
			if scenario == "duplicate_candidates" {
				if err == nil {
					t.Fatalf("ambiguous folder resolved as %s", got)
				}
				return
			}
			if err != nil || got != "correct" {
				t.Fatalf("resolved id=%s err=%v; expected correct child", got, err)
			}
		})
	}
}
