package file

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newPageCommitTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.FileCommit{}, &entity.FileCommitItem{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	old := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = old })
	return db
}

func TestRecordPageEdit_CreatesCommitAndItem(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := context.Background()

	in := PageEditCommitInput{
		DatasetID:  "kb1",
		DocID:      "wiki/page-a",
		Slug:       "page-a",
		PageType:   "wiki",
		Title:      "First edit",
		AuthorID:   "u1",
		OldContent: "hello world",
		NewContent: "hello world, edited",
	}
	commit, err := svc.RecordPageEdit(ctx, in)
	if err != nil {
		t.Fatalf("RecordPageEdit: %v", err)
	}
	if commit.ID == "" {
		t.Fatal("expected a generated commit id")
	}
	if commit.AuthorID != "u1" || commit.Message != "First edit" || commit.FileCount != 1 {
		t.Fatalf("unexpected commit: %+v", commit)
	}
	if commit.FolderID != "kb1" {
		t.Fatalf("expected folder_id kb1 for dataset scope, got %s", commit.FolderID)
	}
	if commit.ParentID != nil {
		t.Fatalf("first edit should have no parent, got %v", *commit.ParentID)
	}

	var items []entity.FileCommitItem
	if err := dao.DB.Where("commit_id = ?", commit.ID).Find(&items).Error; err != nil {
		t.Fatalf("load items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]
	if it.Operation != "modify" {
		t.Fatalf("expected operation modify, got %s", it.Operation)
	}
	if it.FileID != "kb1/wiki/page-a" {
		t.Fatalf("expected file_id kb1/wiki/page-a (dataset-scoped), got %s", it.FileID)
	}
	if it.SlugKwd == nil || *it.SlugKwd != "page-a" {
		t.Fatalf("expected slug_kwd page-a, got %v", it.SlugKwd)
	}
	if it.PageTypeKwd == nil || *it.PageTypeKwd != "wiki" {
		t.Fatalf("expected page_type_kwd wiki, got %v", it.PageTypeKwd)
	}
	if it.Diff == nil || !strings.Contains(*it.Diff, "hello world, edited") {
		t.Fatalf("expected diff containing new content, got %v", it.Diff)
	}
	if it.ContentAfterStorage == nil || *it.ContentAfterStorage != "es" {
		t.Fatalf("expected content_after_storage es, got %v", it.ContentAfterStorage)
	}
	if it.ContentAfterLocation == nil || *it.ContentAfterLocation != "wiki/page-a" {
		t.Fatalf("expected content_after_location wiki/page-a, got %v", it.ContentAfterLocation)
	}
}

func TestRecordPageEdit_SecondEditLinksParent(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := context.Background()

	base := PageEditCommitInput{
		DatasetID: "kb1",
		DocID:     "wiki/page-a",
		Slug:      "page-a",
		PageType:  "wiki",
		Title:     "first",
		AuthorID:  "u1",
	}
	if _, err := svc.RecordPageEdit(ctx, base); err != nil {
		t.Fatalf("first RecordPageEdit: %v", err)
	}

	second := base
	second.Title = "second"
	second.OldContent = "hello"
	second.NewContent = "hello world"
	commit2, err := svc.RecordPageEdit(ctx, second)
	if err != nil {
		t.Fatalf("second RecordPageEdit: %v", err)
	}
	if commit2.ParentID == nil {
		t.Fatal("second edit should link to the first commit as parent")
	}
	var first entity.FileCommit
	if err := dao.DB.Where("title = ?", "first").First(&first).Error; err != nil {
		t.Fatalf("load first commit: %v", err)
	}
	if *commit2.ParentID != first.ID {
		t.Fatalf("parent id mismatch: got %s want %s", *commit2.ParentID, first.ID)
	}
}

func TestRecordPageEdit_IsolatesDatasets(t *testing.T) {
	newPageCommitTestDB(t)
	svc := NewFileCommitService()
	ctx := context.Background()

	mk := func(datasetID string) PageEditCommitInput {
		return PageEditCommitInput{
			DatasetID:  datasetID,
			DocID:      datasetID + "/wiki/page-a",
			Slug:       "page-a",
			PageType:   "wiki",
			Title:      datasetID + "-edit",
			AuthorID:   "u1",
			OldContent: "old",
			NewContent: "new",
		}
	}
	if _, err := svc.RecordPageEdit(ctx, mk("kb1")); err != nil {
		t.Fatalf("kb1 RecordPageEdit: %v", err)
	}
	if _, err := svc.RecordPageEdit(ctx, mk("kb2")); err != nil {
		t.Fatalf("kb2 RecordPageEdit: %v", err)
	}

	// A second edit in kb1 must parent to kb1's own first commit, not kb2's.
	kb1Again := mk("kb1")
	kb1Again.Title = "kb1-edit-2"
	kb1Again.OldContent = "old"
	kb1Again.NewContent = "newer"
	kb1Commit2, err := svc.RecordPageEdit(ctx, kb1Again)
	if err != nil {
		t.Fatalf("kb1 second RecordPageEdit: %v", err)
	}
	if kb1Commit2.ParentID == nil {
		t.Fatal("kb1 second edit should have a parent")
	}

	var kb1First, kb2First entity.FileCommit
	if err := dao.DB.Where("title = ?", "kb1-edit").First(&kb1First).Error; err != nil {
		t.Fatalf("load kb1 first commit: %v", err)
	}
	if err := dao.DB.Where("title = ?", "kb2-edit").First(&kb2First).Error; err != nil {
		t.Fatalf("load kb2 first commit: %v", err)
	}
	if *kb1Commit2.ParentID != kb1First.ID {
		t.Fatalf("kb1 parent mismatch: got %s want %s", *kb1Commit2.ParentID, kb1First.ID)
	}
	if *kb1Commit2.ParentID == kb2First.ID {
		t.Fatal("kb1 parent must not cross into kb2 history")
	}
}

func TestUnifiedDiff_EmptyWhenNoChange(t *testing.T) {
	if d := unifiedDiff("same", "same"); d != "" {
		t.Fatalf("expected empty diff for identical text, got %q", d)
	}
}

func TestUnifiedDiff_DetectsAddition(t *testing.T) {
	d := unifiedDiff("a\nb\nc\n", "a\nb\nc\nd\n")
	if !strings.Contains(d, "+d") {
		t.Fatalf("expected diff to contain added line, got %q", d)
	}
}

func TestUnifiedDiff_DetectsRemoval(t *testing.T) {
	d := unifiedDiff("a\nb\nc\n", "a\nc\n")
	if !strings.Contains(d, "-b") {
		t.Fatalf("expected diff to contain removed line, got %q", d)
	}
}

func TestUnifiedDiff_TruncatesLongDiff(t *testing.T) {
	oldLines := make([]string, 50)
	newLines := make([]string, 50)
	for i := range oldLines {
		oldLines[i] = "old-line"
		newLines[i] = "new-line"
	}
	d := unifiedDiff(strings.Join(oldLines, "\n"), strings.Join(newLines, "\n"))
	if !strings.Contains(d, "... ") || !strings.Contains(d, "lines omitted") {
		t.Fatalf("expected long diff to be truncated, got %q", d)
	}
}
