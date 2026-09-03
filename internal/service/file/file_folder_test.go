package file

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestFileService_CreateFolder_RejectsSlashInName(t *testing.T) {
	svc := testFileService()
	for _, name := range []string{"/", "a/b", "dir/sub/"} {
		_, err := svc.CreateFolder(context.Background(), "tenant1", name, "pf1", FileTypeFolder)
		if err == nil || !strings.Contains(err.Error(), `cannot contain "/"`) {
			t.Fatalf("CreateFolder(%q) error = %v, want slash validation error", name, err)
		}
	}
}

func TestFileService_MoveFiles_RejectsSlashInNewName(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Keep a single connection so the :memory: database is shared across
	// goroutines (sqlite :memory: is otherwise per-connection).
	if sqlDB, serr := db.DB(); serr == nil {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
	}
	if err := db.AutoMigrate(&entity.File{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	old := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = old })

	folder := &entity.File{ID: "f1", ParentID: "pf1", TenantID: "tenant1", Name: "old", Type: FileTypeFolder}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("seed folder: %v", err)
	}

	svc := testFileService()
	ok, msg := svc.MoveFiles(context.Background(), "tenant1", []string{"f1"}, "", "a/b")
	if ok || !strings.Contains(msg, `cannot contain "/"`) {
		t.Fatalf("MoveFiles rename = %v, %q, want slash validation error", ok, msg)
	}
}

// setupFolderTestDB initializes an in-memory SQLite database for file folder tests.
func setupFolderTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err = db.AutoMigrate(
		&entity.File{},
		&entity.File2Document{},
		&entity.Document{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() {
		dao.DB = orig
	})
	return db
}

func insertFolderTestFile(t *testing.T, id, parentID, name string) {
	t.Helper()
	f := &entity.File{
		ID:        id,
		ParentID:  parentID,
		TenantID:  "tenant-1",
		CreatedBy: "user-1",
		Name:      name,
		Location:  sptr(name),
		Type:      "pdf",
	}
	if err := dao.DB.Create(f).Error; err != nil {
		t.Fatalf("insert test file: %v", err)
	}
}

func TestFileService_AncestryRejectsUnauthorized(t *testing.T) {
	setupFolderTestDB(t)
	insertFolderTestFile(t, "file-1", "folder-1", "file.pdf")
	svc := testFileService()
	svc.checkFilePerm = func(context.Context, *dao.FileDAO, *entity.File, string) bool { return false }

	for _, call := range []func() error{
		func() error { _, err := svc.GetParentFolder(t.Context(), "user-2", "file-1"); return err },
		func() error { _, err := svc.GetAllParentFolders(t.Context(), "user-2", "file-1"); return err },
	} {
		if err := call(); !errors.Is(err, ErrNoAuthorization) {
			t.Fatalf("error = %v, want ErrNoAuthorization", err)
		}
	}
}

func insertFolderTestDocument(t *testing.T, id, kbID, name string) {
	t.Helper()
	doc := &entity.Document{
		ID:           id,
		KbID:         kbID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		CreatedBy:    "user-1",
		Name:         sptr(name),
		Location:     sptr(name),
		Type:         "pdf",
		Suffix:       "pdf",
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("insert test document: %v", err)
	}
}

func insertFolderTestFile2Document(t *testing.T, id, fileID, docID string) {
	t.Helper()
	f2d := &entity.File2Document{
		ID:         id,
		FileID:     &fileID,
		DocumentID: &docID,
	}
	if err := dao.DB.Create(f2d).Error; err != nil {
		t.Fatalf("insert test f2d: %v", err)
	}
}

// Renaming a file linked to multiple datasets must propagate the new name to
// every linked document, not just the first one.
func TestMoveFilesRenameUpdatesAllLinkedDocuments(t *testing.T) {
	db := setupFolderTestDB(t)
	insertFolderTestFile(t, "file-1", "folder-1", "old.pdf")
	insertFolderTestDocument(t, "doc-1", "kb-1", "old.pdf")
	insertFolderTestDocument(t, "doc-2", "kb-2", "old.pdf")
	insertFolderTestFile2Document(t, "f2d-1", "file-1", "doc-1")
	insertFolderTestFile2Document(t, "f2d-2", "file-1", "doc-2")

	svc := testFileService()
	ctx := t.Context()
	ok, msg := svc.MoveFiles(ctx, "user-1", []string{"file-1"}, "", "new.pdf")
	if !ok {
		t.Fatalf("MoveFiles failed: %s", msg)
	}

	file, err := dao.NewFileDAO().GetByID(ctx, db, "file-1")
	if err != nil {
		t.Fatalf("get file: %v", err)
	}
	if file.Name != "new.pdf" {
		t.Fatalf("file name = %q, want %q", file.Name, "new.pdf")
	}

	documentDAO := dao.NewDocumentDAO()
	for _, docID := range []string{"doc-1", "doc-2"} {
		doc, err := documentDAO.GetByID(ctx, db, docID)
		if err != nil {
			t.Fatalf("get %s: %v", docID, err)
		}
		if doc.Name == nil || *doc.Name != "new.pdf" {
			t.Fatalf("%s name = %v, want %q", docID, doc.Name, "new.pdf")
		}
	}
}

// A failing file2document lookup must surface as an error instead of being
// silently treated as "no links", so a rename never reports success while
// linked documents keep stale names.
func TestRenameLinkedDocumentsLookupErrorPropagates(t *testing.T) {
	db := setupFolderTestDB(t)
	insertFolderTestFile(t, "file-1", "folder-1", "old.pdf")

	// Force the link lookup to fail by dropping its table.
	if err := db.Migrator().DropTable(&entity.File2Document{}); err != nil {
		t.Fatalf("drop file2document table: %v", err)
	}

	svc := testFileService()
	if err := svc.renameLinkedDocuments(t.Context(), "file-1", "new.pdf"); err == nil {
		t.Fatal("renameLinkedDocuments returned nil error on lookup failure")
	}

	ok, msg := svc.MoveFiles(t.Context(), "user-1", []string{"file-1"}, "", "new.pdf")
	if ok {
		t.Fatal("MoveFiles succeeded despite file2document lookup failure")
	}
	if !strings.Contains(msg, "Document rename") {
		t.Fatalf("MoveFiles message = %q, want it to mention %q", msg, "Document rename")
	}
}

// Renaming while moving into the same parent folder (no storage move) must
// also propagate the new name to every linked document.
func TestMoveEntryRecursiveRenameUpdatesAllLinkedDocuments(t *testing.T) {
	db := setupFolderTestDB(t)
	insertFolderTestFile(t, "file-1", "folder-1", "old.pdf")
	insertFolderTestDocument(t, "doc-1", "kb-1", "old.pdf")
	insertFolderTestDocument(t, "doc-2", "kb-2", "old.pdf")
	insertFolderTestFile2Document(t, "f2d-1", "file-1", "doc-1")
	insertFolderTestFile2Document(t, "f2d-2", "file-1", "doc-2")

	svc := testFileService()
	ctx := t.Context()
	destFolder, err := dao.NewFileDAO().GetByID(ctx, db, "folder-1")
	if err != nil {
		// folder-1 does not exist as a row; construct the minimal folder entity
		// with the same parent id so no storage move happens.
		destFolder = &entity.File{ID: "folder-1", Type: FileTypeFolder}
	}
	srcFile, err := dao.NewFileDAO().GetByID(ctx, db, "file-1")
	if err != nil {
		t.Fatalf("get file: %v", err)
	}

	if err = svc.moveEntryRecursive(ctx, srcFile, destFolder, "new.pdf"); err != nil {
		t.Fatalf("moveEntryRecursive failed: %v", err)
	}

	documentDAO := dao.NewDocumentDAO()
	for _, docID := range []string{"doc-1", "doc-2"} {
		doc, err := documentDAO.GetByID(ctx, db, docID)
		if err != nil {
			t.Fatalf("get %s: %v", docID, err)
		}
		if doc.Name == nil || *doc.Name != "new.pdf" {
			t.Fatalf("%s name = %v, want %q", docID, doc.Name, "new.pdf")
		}
	}
}
