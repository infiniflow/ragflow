package dataset

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	filesvc "ragflow/internal/service/file"
	"testing"
)

func TestReviewSharedRootDuplicates(t *testing.T) {
	f := newDatasetFolderFixture(t)
	a := f.create("alpha")
	for _, v := range []entity.File{
		{ID: "root-a", ParentID: "root-a", TenantID: "tenant-1", Name: "/", Type: "folder"},
		{ID: "root-b", ParentID: "root-b", TenantID: "tenant-1", Name: "/", Type: "folder"},
		{ID: "shared-a", ParentID: "root-a", TenantID: "tenant-1", Name: ".knowledgebase", Type: "folder", SourceType: "knowledgebase"},
		{ID: "shared-b", ParentID: "root-a", TenantID: "tenant-1", Name: ".knowledgebase", Type: "folder", SourceType: "knowledgebase"},
	} {
		v.CreatedBy = "tenant-1"
		f.must(f.db.Create(&v).Error)
	}
	f.must(f.db.Model(&entity.File{}).Where("id = ?", "root-b").Update("create_time", 1).Error)
	folder, err := dao.NewFileDAO().EnsureDatasetFolder(f.ctx, f.db, f.kb(a))
	f.must(err)
	if folder.ParentID != "shared-a" {
		t.Fatalf("unexpected parent %s", folder.ParentID)
	}
	if f.count(&entity.File{}, "id IN ?", []string{"root-a", "root-b", "shared-a", "shared-b"}) != 4 {
		t.Fatal("duplicates were removed")
	}
}
func TestReviewDeleteBatchRefusal(t *testing.T) {
	for _, canonical := range []bool{false, true} {
		name := "ordinary"
		if canonical {
			name = "canonical"
		}
		t.Run(name, func(t *testing.T) {
			f := newDatasetFolderFixture(t)
			a := f.create("alpha")
			folder, err := dao.NewFileDAO().EnsureDatasetFolder(f.ctx, f.db, f.kb(a))
			f.must(err)
			fs := filesvc.NewFileService(f.docs)
			ordinary, err := fs.CreateFolder(f.ctx, "tenant-1", "ordinary", "", "folder")
			f.must(err)
			if !canonical {
				f.must(f.db.Model(&entity.File{}).Where("id = ?", folder.ID).Update("source_type", "").Error)
			}
			if err := fs.DeleteFiles(f.ctx, []string{ordinary["id"].(string), folder.ID}); err == nil {
				t.Fatal("delete unexpectedly succeeded")
			}
			if f.count(&entity.File{}, "id = ?", ordinary["id"]) != 1 {
				t.Fatal("batch partially deleted")
			}
			if f.count(&entity.File{}, "id = ?", folder.ID) != 1 {
				t.Fatal("bound directory deleted")
			}
		})
	}
}

func TestReviewBindingRaceRetry(t *testing.T) {
	for _, outer := range []bool{false, true} {
		name := "root_database"
		if outer {
			name = "existing_transaction"
		}
		t.Run(name, func(t *testing.T) {
			f := newDatasetFolderFixture(t)
			a := f.create("alpha")
			attempts := 0
			f.must(f.db.Callback().Update().Before("gorm:update").Register("test:binding_race", func(tx *gorm.DB) {
				if tx.Statement.Table == "knowledgebase" {
					if values, ok := tx.Statement.Dest.(map[string]interface{}); ok {
						if _, binding := values["folder_id"]; binding {
							attempts++
							if attempts == 1 {
								tx.Statement.AddClause(clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "1 = 0"}}})
							}
						}
					}
				}
			}))
			t.Cleanup(func() { f.db.Callback().Update().Remove("test:binding_race") })
			db := f.db
			if outer {
				db = f.db.Begin()
				defer db.Rollback()
			}
			folder, err := dao.NewFileDAO().EnsureDatasetFolder(f.ctx, db, f.kb(a))
			if outer {
				if !errors.Is(err, dao.ErrDatasetFolderBindingRace) || attempts != 1 {
					t.Fatalf("outer transaction retried or lost race: attempts=%d err=%v", attempts, err)
				}
				return
			}
			f.must(err)
			if attempts != 2 {
				t.Fatalf("attempts=%d", attempts)
			}
			if f.count(&entity.File{}, "name = ? AND type = ?", "alpha", "folder") != 1 {
				t.Fatal("rolled-back directory remains")
			}
			if f.kb(a).FolderID == nil || *f.kb(a).FolderID != folder.ID {
				t.Fatal("retry did not save binding")
			}
		})
	}
}
