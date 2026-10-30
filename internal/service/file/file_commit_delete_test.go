package file

import (
	"encoding/json"
	"errors"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newCommitDeleteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, serr := db.DB(); serr == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(&entity.FileCommit{}, &entity.FileCommitItem{}, &entity.File{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	old := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = old })
	return db
}

// A committed file whose live row is already gone is the only shape
// GetUncommittedChanges reports as a deletion, so the delete operation must
// record the tombstone without touching the file table.
func TestCreateCommit_DeleteRecordsTombstoneWhenLiveFileIsGone(t *testing.T) {
	db := newCommitDeleteTestDB(t)
	tree := `{"f1":{"hash":"h1","location":".objects/h1","name":"a.txt","size":3,"status":"1","parent_id":"folder-1"}}`
	parent := &entity.FileCommit{ID: "c1", FolderID: "folder-1", Message: "init", AuthorID: "u1", FileCount: 1, TreeState: &tree}
	if err := db.Create(parent).Error; err != nil {
		t.Fatalf("seed parent commit: %v", err)
	}

	commit, err := NewFileCommitService().CreateCommit(t.Context(), "folder-1", "u1", "remove a.txt", []entity.FileChange{
		{FileID: "f1", FileName: "a.txt", Operation: "delete"},
	})
	if err != nil {
		t.Fatalf("CreateCommit delete: %v", err)
	}

	var items []entity.FileCommitItem
	if err := db.Where("commit_id = ?", commit.ID).Find(&items).Error; err != nil {
		t.Fatalf("load commit items: %v", err)
	}
	if len(items) != 1 || items[0].Operation != "delete" {
		t.Fatalf("expected one delete item, got %+v", items)
	}
	if items[0].OldHash == nil || *items[0].OldHash != "h1" {
		t.Fatalf("expected old hash h1, got %v", items[0].OldHash)
	}
	if items[0].OldLocation == nil || *items[0].OldLocation != ".objects/h1" {
		t.Fatalf("expected old location .objects/h1, got %v", items[0].OldLocation)
	}

	var saved entity.FileCommit
	if err := db.First(&saved, "id = ?", commit.ID).Error; err != nil {
		t.Fatalf("load commit: %v", err)
	}
	var state map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(*saved.TreeState), &state); err != nil {
		t.Fatalf("unmarshal tree state: %v", err)
	}
	if state["f1"]["status"] != "0" {
		t.Fatalf("expected f1 status 0 in tree state, got %v", state["f1"]["status"])
	}
	if state["f1"]["hash"] != "h1" {
		t.Fatalf("expected the tombstone to keep the old hash, got %v", state["f1"]["hash"])
	}
}

func seedLiveFile(t *testing.T, db *gorm.DB, id, parentID string) {
	t.Helper()
	if err := db.Create(&entity.File{ID: id, ParentID: parentID, TenantID: "t1", CreatedBy: "u1", Name: id + ".txt", Type: "file"}).Error; err != nil {
		t.Fatalf("seed file %s: %v", id, err)
	}
}

// A tombstone for a file that is still in the folder would hide that file from
// GetUncommittedChanges: the scan marks committed entries as processed and
// skips tombstones, so the live row is reported as neither deleted nor added.
func TestCreateCommit_RejectsDeleteOfLiveFile(t *testing.T) {
	db := newCommitDeleteTestDB(t)
	tree := `{
		"f2":{"hash":"h2","location":".objects/h2","name":"a.txt","size":1,"status":"1","parent_id":"folder-1"},
		"f4":{"hash":"h4","location":".objects/h4","name":"b.txt","size":1,"status":"1","parent_id":"sub-1"}
	}`
	parent := &entity.FileCommit{ID: "c1", FolderID: "folder-1", Message: "init", AuthorID: "u1", TreeState: &tree}
	if err := db.Create(parent).Error; err != nil {
		t.Fatalf("seed parent commit: %v", err)
	}
	seedLiveFile(t, db, "f2", "folder-1")
	seedLiveFile(t, db, "sub-1", "folder-1")
	seedLiveFile(t, db, "f4", "sub-1")

	for _, fileID := range []string{"f2", "f4"} {
		_, err := NewFileCommitService().CreateCommit(t.Context(), "folder-1", "u1", "remove "+fileID, []entity.FileChange{
			{FileID: fileID, FileName: fileID + ".txt", Operation: "delete"},
		})
		if !errors.Is(err, ErrDeleteLiveFile) {
			t.Fatalf("delete of live file %s: err = %v, want ErrDeleteLiveFile", fileID, err)
		}
	}

	var commits int64
	if err := db.Model(&entity.FileCommit{}).Where("id <> ?", parent.ID).Count(&commits).Error; err != nil {
		t.Fatalf("count commits: %v", err)
	}
	if commits != 0 {
		t.Fatalf("expected rejected commits to roll back, found %d extra commit rows", commits)
	}
}

// The same file ID may exist in another folder; only membership in this
// folder's subtree makes a delete invalid.
func TestCreateCommit_AllowsDeleteOfFileOutsideFolder(t *testing.T) {
	db := newCommitDeleteTestDB(t)
	tree := `{"f3":{"hash":"h3","location":".objects/h3","name":"c.txt","size":1,"status":"1","parent_id":"folder-1"}}`
	parent := &entity.FileCommit{ID: "c1", FolderID: "folder-1", Message: "init", AuthorID: "u1", TreeState: &tree}
	if err := db.Create(parent).Error; err != nil {
		t.Fatalf("seed parent commit: %v", err)
	}
	seedLiveFile(t, db, "f3", "other-folder")

	if _, err := NewFileCommitService().CreateCommit(t.Context(), "folder-1", "u1", "remove f3", []entity.FileChange{
		{FileID: "f3", FileName: "c.txt", Operation: "delete"},
	}); err != nil {
		t.Fatalf("delete of a file outside the folder: %v", err)
	}
}
