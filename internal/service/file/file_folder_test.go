package file

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"

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

func TestFileService_CreateFolderDedupesDuplicateName(t *testing.T) {
	setupFolderTestDB(t)

	parent := &entity.File{ID: "pf1", ParentID: "pf1", TenantID: "tenant-1", Name: "root", Type: FileTypeFolder}
	if err := dao.DB.Create(parent).Error; err != nil {
		t.Fatalf("seed parent folder: %v", err)
	}

	svc := testFileService()
	ctx := context.Background()

	created, err := svc.CreateFolder(ctx, "tenant-1", "notes", "pf1", FileTypeFolder)
	if err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}
	if got := created["name"]; got != "notes" {
		t.Fatalf("first create name = %v, want notes", got)
	}

	// A second create with the same name in the same folder appends a counter
	// instead of failing.
	created, err = svc.CreateFolder(ctx, "tenant-1", "notes", "pf1", FileTypeFolder)
	if err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}
	if got := created["name"]; got != "notes(1)" {
		t.Fatalf("duplicate create name = %v, want notes(1)", got)
	}

	// Virtual files keep the extension at the end of the generated name.
	if _, err = svc.CreateFolder(ctx, "tenant-1", "report.pdf", "pf1", FileTypeVirtual); err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}
	created, err = svc.CreateFolder(ctx, "tenant-1", "report.pdf", "pf1", FileTypeVirtual)
	if err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}
	if got := created["name"]; got != "report(1).pdf" {
		t.Fatalf("duplicate create name = %v, want report(1).pdf", got)
	}

	// Folders are not files: a dotted folder name keeps its dot at the end.
	if got := firstCreatedFolderName(t, svc, "v1.2", FileTypeFolder); got != "v1.2" {
		t.Fatalf("first folder name = %q, want %q", got, "v1.2")
	}
	if got := firstCreatedFolderName(t, svc, "v1.2", FileTypeFolder); got != "v1.2(1)" {
		t.Fatalf("duplicate folder name = %q, want %q", got, "v1.2(1)")
	}
}

// firstCreatedFolderName creates one entry and returns its resulting name.
func firstCreatedFolderName(t *testing.T, svc *FileService, name, fileType string) string {
	t.Helper()
	created, err := svc.CreateFolder(context.Background(), "tenant-1", name, "pf1", fileType)
	if err != nil {
		t.Fatalf("CreateFolder(%q) failed: %v", name, err)
	}
	return created["name"].(string)
}

func TestFileService_MoveFiles_RejectsSlashInNewName(t *testing.T) {
	db := setupFolderTestDB(t)

	folder := &entity.File{ID: "f1", ParentID: "pf1", TenantID: "tenant1", Name: "old", Type: FileTypeFolder}
	if err := db.Create(folder).Error; err != nil {
		t.Fatalf("seed folder: %v", err)
	}

	svc := testFileService()
	err := svc.MoveFiles(context.Background(), "tenant1", []string{"f1"}, "", "a/b")
	if err == nil || !strings.Contains(err.Error(), `cannot contain "/"`) {
		t.Fatalf("MoveFiles rename error = %v, want slash validation error", err)
	}
}

func TestFileService_MoveFilesRejectsDuplicateName(t *testing.T) {
	setupFolderTestDB(t)

	if err := dao.DB.Create(&entity.File{ID: "t1", ParentID: "pf1", TenantID: "tenant-1", Name: "target", Type: FileTypeFolder}).Error; err != nil {
		t.Fatalf("seed target folder: %v", err)
	}
	if err := dao.DB.Create(&entity.File{ID: "s1", ParentID: "pf1", TenantID: "tenant-1", Name: "source", Type: FileTypeFolder}).Error; err != nil {
		t.Fatalf("seed source folder: %v", err)
	}

	svc := testFileService()
	ctx := context.Background()

	err := svc.MoveFiles(ctx, "tenant-1", []string{"s1"}, "", "TARGET")
	if err == nil || !strings.Contains(err.Error(), "duplicated file name") {
		t.Fatalf("rename to case-variant duplicate error = %v, want duplicate error", err)
	}

	if err = svc.MoveFiles(ctx, "tenant-1", []string{"s1"}, "", "SOURCE"); err != nil {
		t.Fatalf("case-only rename failed: %v", err)
	}
}

// Moving a file into another folder must check the destination namespace even
// when the requested name only differs from the source name by case.
func TestFileService_MoveFilesRejectsCaseVariantDuplicateInDestinationFolder(t *testing.T) {
	setupFolderTestDB(t)

	for _, f := range []*entity.File{
		{ID: "pf1", ParentID: "pf1", TenantID: "tenant-1", Name: "root", Type: FileTypeFolder},
		{ID: "d1", ParentID: "pf1", TenantID: "tenant-1", Name: "dest", Type: FileTypeFolder},
		{ID: "s1", ParentID: "pf1", TenantID: "tenant-1", Name: "report.txt", Type: "pdf"},
		{ID: "existing", ParentID: "d1", TenantID: "tenant-1", Name: "report.txt", Type: "pdf"},
	} {
		if err := dao.DB.Create(f).Error; err != nil {
			t.Fatalf("seed %s: %v", f.ID, err)
		}
	}

	svc := testFileService()
	err := svc.MoveFiles(context.Background(), "tenant-1", []string{"s1"}, "d1", "REPORT.TXT")
	if err == nil || !strings.Contains(err.Error(), "duplicated file name") {
		t.Fatalf("move+case-variant rename error = %v, want duplicate error", err)
	}
}

// A plain move (no new_name) must detect a case-variant duplicate in the
// destination folder too, not only the rename path.
func TestFileService_MoveFilesRejectsCaseVariantDuplicateWithoutRename(t *testing.T) {
	setupFolderTestDB(t)

	for _, f := range []*entity.File{
		{ID: "pf1", ParentID: "pf1", TenantID: "tenant-1", Name: "root", Type: FileTypeFolder},
		{ID: "d1", ParentID: "pf1", TenantID: "tenant-1", Name: "dest", Type: FileTypeFolder},
		{ID: "s1", ParentID: "pf1", TenantID: "tenant-1", Name: "report.txt", Type: "pdf"},
		{ID: "existing", ParentID: "d1", TenantID: "tenant-1", Name: "REPORT.TXT", Type: "pdf"},
	} {
		if err := dao.DB.Create(f).Error; err != nil {
			t.Fatalf("seed %s: %v", f.ID, err)
		}
	}

	svc := testFileService()
	err := svc.MoveFiles(context.Background(), "tenant-1", []string{"s1"}, "d1", "")
	if err == nil || !strings.Contains(err.Error(), "Duplicated file name") {
		t.Fatalf("plain move into case-variant duplicate error = %v, want duplicate error", err)
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
		&entity.Knowledgebase{},
		&entity.UserTenant{},
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

func TestFileService_ListFilesMissingParentReturnsNotFound(t *testing.T) {
	setupFolderTestDB(t)

	root := &entity.File{ID: "root-1", ParentID: "root-1", TenantID: "tenant-1", Name: "/", Type: FileTypeFolder}
	if err := dao.DB.Create(root).Error; err != nil {
		t.Fatalf("seed root: %v", err)
	}

	svc := testFileService()
	_, err := svc.ListFiles(context.Background(), "tenant-1", "missing-folder", 1, 10, nil, "")
	var coded *common.CodedError
	if !errors.As(err, &coded) || coded.Code != common.CodeNotFound {
		t.Fatalf("ListFiles error = %v, want CodeNotFound CodedError", err)
	}
}

func TestFileService_AncestryMissingIDReturnsNotFound(t *testing.T) {
	setupFolderTestDB(t)

	svc := testFileService()
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"GetParentFolder":     func() error { _, err := svc.GetParentFolder(ctx, "user-1", "missing-file"); return err },
		"GetAllParentFolders": func() error { _, err := svc.GetAllParentFolders(ctx, "user-1", "missing-file"); return err },
	} {
		var coded *common.CodedError
		if err := call(); !errors.As(err, &coded) || coded.Code != common.CodeNotFound {
			t.Fatalf("%s error = %v, want CodeNotFound CodedError", name, err)
		}
	}
}

func TestFileService_DatabaseErrorIsNotReportedAsNotFound(t *testing.T) {
	db := setupFolderTestDB(t)

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get underlying sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sql db: %v", err)
	}

	svc := testFileService()
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"ListFiles":           func() error { _, err := svc.ListFiles(ctx, "tenant-1", "any", 1, 10, nil, ""); return err },
		"GetParentFolder":     func() error { _, err := svc.GetParentFolder(ctx, "user-1", "any"); return err },
		"GetAllParentFolders": func() error { _, err := svc.GetAllParentFolders(ctx, "user-1", "any"); return err },
	} {
		coded := (*common.CodedError)(nil)
		if err := call(); err == nil {
			t.Fatalf("%s: expected a database error, got nil", name)
		} else if errors.As(err, &coded) {
			t.Fatalf("%s error = %v, must not be reported as CodeNotFound", name, err)
		}
	}
}

func TestFileService_AncestryRejectsUnauthorized(t *testing.T) {
	setupFolderTestDB(t)
	insertFolderTestFile(t, "file-1", "folder-1", "file.pdf")
	svc := testFileService()
	for _, call := range []func() error{
		func() error { _, err := svc.GetParentFolder(t.Context(), "user-2", "file-1"); return err },
		func() error { _, err := svc.GetAllParentFolders(t.Context(), "user-2", "file-1"); return err },
	} {
		if err := call(); !errors.Is(err, permission.ErrPermissionDenied) {
			t.Fatalf("error = %v, want permission denied", err)
		}
	}
}

func TestFileService_GetFileContentRejectsInaccessibleFile(t *testing.T) {
	setupFolderTestDB(t)
	insertFolderTestFile(t, "file-1", "folder-1", "private.pdf")

	_, err := testFileService().GetFileContent(t.Context(), "user-2", "file-1")
	if !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("GetFileContent error = %v, want permission denied", err)
	}
}

func TestFileService_ListFilesRejectsInaccessibleParent(t *testing.T) {
	setupFolderTestDB(t)
	foreignFolder := &entity.File{ID: "foreign-root", ParentID: "foreign-root", TenantID: "user-2", Name: "/", Type: FileTypeFolder}
	if err := dao.DB.Create(foreignFolder).Error; err != nil {
		t.Fatalf("seed foreign folder: %v", err)
	}

	_, err := testFileService().ListFiles(t.Context(), "user-1", foreignFolder.ID, 1, 15, nil, "")
	if !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("ListFiles error = %v, want permission denied", err)
	}
}

func TestFileService_CreateFolderRejectsInaccessibleParent(t *testing.T) {
	setupFolderTestDB(t)
	foreignFolder := &entity.File{ID: "foreign-root", ParentID: "foreign-root", TenantID: "user-2", Name: "/", Type: FileTypeFolder}
	if err := dao.DB.Create(foreignFolder).Error; err != nil {
		t.Fatalf("seed foreign folder: %v", err)
	}

	_, err := testFileService().CreateFolder(t.Context(), "user-1", "child", foreignFolder.ID, FileTypeFolder)
	if !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("CreateFolder error = %v, want permission denied", err)
	}
	children, err := dao.NewFileDAO().ListByParentID(t.Context(), dao.DB, foreignFolder.ID)
	if err != nil {
		t.Fatalf("list foreign folder children: %v", err)
	}
	if len(children) != 0 {
		t.Fatalf("created %d children under inaccessible folder, want none", len(children))
	}
}

func TestFileService_MoveFilesRejectsInaccessibleSource(t *testing.T) {
	db := setupFolderTestDB(t)
	foreignFile := &entity.File{ID: "foreign-file", ParentID: "foreign-root", TenantID: "user-2", Name: "private.pdf", Type: "pdf"}
	if err := db.Create(foreignFile).Error; err != nil {
		t.Fatalf("seed foreign file: %v", err)
	}

	err := testFileService().MoveFiles(t.Context(), "user-1", []string{foreignFile.ID}, "", "renamed.pdf")
	if !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("MoveFiles error = %v, want permission denied", err)
	}
	stored, err := dao.NewFileDAO().GetByID(t.Context(), db, foreignFile.ID)
	if err != nil {
		t.Fatalf("get foreign file: %v", err)
	}
	if stored.Name != "private.pdf" {
		t.Fatalf("foreign file name = %q after denied move, want unchanged", stored.Name)
	}
}

func TestFileService_MoveFilesRejectsInaccessibleDestination(t *testing.T) {
	db := setupFolderTestDB(t)
	insertFolderTestFile(t, "file-1", "root", "report.pdf")
	foreignFolder := &entity.File{ID: "foreign-folder", ParentID: "foreign-root", TenantID: "user-2", Name: "private", Type: FileTypeFolder}
	if err := db.Create(foreignFolder).Error; err != nil {
		t.Fatalf("seed foreign folder: %v", err)
	}

	err := testFileService().MoveFiles(t.Context(), "user-1", []string{"file-1"}, foreignFolder.ID, "")
	if !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("MoveFiles error = %v, want permission denied", err)
	}
	stored, err := dao.NewFileDAO().GetByID(t.Context(), db, "file-1")
	if err != nil {
		t.Fatalf("get source file: %v", err)
	}
	if stored.ParentID != "root" {
		t.Fatalf("source parent = %q after denied move, want root", stored.ParentID)
	}
}

func TestFileService_UploadRejectsInaccessibleParent(t *testing.T) {
	db := setupFolderTestDB(t)
	foreignFolder := &entity.File{ID: "foreign-folder", ParentID: "foreign-root", TenantID: "user-2", Name: "private", Type: FileTypeFolder}
	if err := db.Create(foreignFolder).Error; err != nil {
		t.Fatalf("seed foreign folder: %v", err)
	}

	_, err := testFileService().UploadFile(t.Context(), "user-1", foreignFolder.ID, nil, 1024)
	if !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("UploadFile error = %v, want permission denied", err)
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
	if err := svc.MoveFiles(ctx, "tenant-1", []string{"file-1"}, "", "new.pdf"); err != nil {
		t.Fatalf("MoveFiles failed: %v", err)
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
	insertFolderTestDocument(t, "doc-1", "kb-1", "old.pdf")
	insertFolderTestFile2Document(t, "f2d-1", "file-1", "doc-1")

	// Keep the link lookup available for the permission preflight, but fail the
	// linked document update to exercise rename error propagation.
	if err := db.Exec(`CREATE TRIGGER fail_document_rename BEFORE UPDATE OF name ON document
		BEGIN SELECT RAISE(FAIL, 'rename disabled'); END;`).Error; err != nil {
		t.Fatalf("create document rename trigger: %v", err)
	}

	svc := testFileService()
	if err := svc.renameLinkedDocuments(t.Context(), "file-1", "new.pdf"); err == nil {
		t.Fatal("renameLinkedDocuments returned nil error on update failure")
	}

	err := svc.MoveFiles(t.Context(), "tenant-1", []string{"file-1"}, "", "new.pdf")
	if err == nil {
		t.Fatal("MoveFiles succeeded despite file2document lookup failure")
	}
	if !strings.Contains(err.Error(), "Document rename") {
		t.Fatalf("MoveFiles error = %v, want it to mention %q", err, "Document rename")
	}
}

// TestMoveEntryRecursiveRenameUpdatesAllLinkedDocuments verifies that renaming while
// moving into the same parent folder (no storage move) must also propagate the new
// name to every linked document.
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

	if err = svc.moveEntryRecursive(ctx, "tenant-1", srcFile, destFolder, "new.pdf"); err != nil {
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

// seedMoveHierarchyDB seeds R (root) > D (destination) and R > A > B > C so
// tests can select an entry together with one of its descendants.
func seedMoveHierarchyDB(t *testing.T) {
	t.Helper()
	setupFolderTestDB(t)
	for _, f := range []*entity.File{
		{ID: "R", ParentID: "R", TenantID: "tenant-1", Name: "/", Type: FileTypeFolder},
		{ID: "D", ParentID: "R", TenantID: "tenant-1", Name: "dest", Type: FileTypeFolder},
		{ID: "A", ParentID: "R", TenantID: "tenant-1", Name: "A", Type: FileTypeFolder},
		{ID: "B", ParentID: "A", TenantID: "tenant-1", Name: "B", Type: FileTypeFolder},
		{ID: "C", ParentID: "B", TenantID: "tenant-1", Name: "C", Type: FileTypeFolder},
	} {
		if err := dao.DB.Create(f).Error; err != nil {
			t.Fatalf("seed %s: %v", f.ID, err)
		}
	}
}

func requireParent(t *testing.T, id, wantParent string) {
	t.Helper()
	f, err := dao.NewFileDAO().GetByID(context.Background(), dao.DB, id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	if f.ParentID != wantParent {
		t.Fatalf("%s parent = %q, want %q", id, f.ParentID, wantParent)
	}
}

func countChildren(t *testing.T, parentID string) int64 {
	t.Helper()
	var count int64
	if err := dao.DB.Model(&entity.File{}).Where("parent_id = ?", parentID).Count(&count).Error; err != nil {
		t.Fatalf("count children of %s: %v", parentID, err)
	}
	return count
}

func childByName(t *testing.T, parentID, name string) *entity.File {
	t.Helper()
	var f entity.File
	if err := dao.DB.Where("parent_id = ? AND name = ?", parentID, name).First(&f).Error; err != nil {
		t.Fatalf("find %q under %s: %v", name, parentID, err)
	}
	return &f
}

// Selecting a folder together with its direct subfolder must be rejected before
// any mutation: the ancestor's recursive move already relocates and deletes the
// subfolder, so processing it again would recreate an empty duplicate.
func TestMoveFilesRejectsFolderAndSubfolder(t *testing.T) {
	seedMoveHierarchyDB(t)

	svc := testFileService()
	err := svc.MoveFiles(context.Background(), "tenant-1", []string{"A", "B"}, "D", "")
	if err == nil || !strings.Contains(err.Error(), "descendant") {
		t.Fatalf("move folder+subfolder error = %v, want descendant rejection", err)
	}

	requireParent(t, "A", "R")
	requireParent(t, "B", "A")
	if got := countChildren(t, "D"); got != 0 {
		t.Fatalf("destination children = %d, want 0 (nothing should move)", got)
	}
}

// A grandchild (any depth) in the selection is rejected too.
func TestMoveFilesRejectsFolderAndGrandchild(t *testing.T) {
	seedMoveHierarchyDB(t)

	svc := testFileService()
	err := svc.MoveFiles(context.Background(), "tenant-1", []string{"A", "C"}, "D", "")
	if err == nil || !strings.Contains(err.Error(), "descendant") {
		t.Fatalf("move folder+grandchild error = %v, want descendant rejection", err)
	}

	requireParent(t, "A", "R")
	requireParent(t, "C", "B")
	if got := countChildren(t, "D"); got != 0 {
		t.Fatalf("destination children = %d, want 0 (nothing should move)", got)
	}
}

// A file contained in a selected folder counts as a descendant relationship.
func TestMoveFilesRejectsFolderAndContainedFile(t *testing.T) {
	seedMoveHierarchyDB(t)
	insertFolderTestFile(t, "f1", "A", "inside.pdf")

	svc := testFileService()
	err := svc.MoveFiles(context.Background(), "tenant-1", []string{"A", "f1"}, "D", "")
	if err == nil || !strings.Contains(err.Error(), "descendant") {
		t.Fatalf("move folder+contained file error = %v, want descendant rejection", err)
	}

	requireParent(t, "A", "R")
	requireParent(t, "f1", "A")
	if got := countChildren(t, "D"); got != 0 {
		t.Fatalf("destination children = %d, want 0 (nothing should move)", got)
	}
}

// The rejection must not depend on the order of src_file_ids.
func TestMoveFilesRejectsNestedSourcesInEitherOrder(t *testing.T) {
	seedMoveHierarchyDB(t)

	svc := testFileService()
	for _, src := range [][]string{{"A", "B"}, {"B", "A"}} {
		err := svc.MoveFiles(context.Background(), "tenant-1", src, "D", "")
		if err == nil || !strings.Contains(err.Error(), "descendant") {
			t.Fatalf("move %v error = %v, want descendant rejection", src, err)
		}
	}

	requireParent(t, "A", "R")
	requireParent(t, "B", "A")
	if got := countChildren(t, "D"); got != 0 {
		t.Fatalf("destination children = %d, want 0 (nothing should move)", got)
	}
}

// Unrelated entries (no ancestor/descendant relationship) must still move.
func TestMoveFilesAllowsUnrelatedEntries(t *testing.T) {
	setupFolderTestDB(t)
	for _, f := range []*entity.File{
		{ID: "R", ParentID: "R", TenantID: "tenant-1", Name: "/", Type: FileTypeFolder},
		{ID: "D", ParentID: "R", TenantID: "tenant-1", Name: "dest", Type: FileTypeFolder},
		{ID: "A", ParentID: "R", TenantID: "tenant-1", Name: "A", Type: FileTypeFolder},
		{ID: "X", ParentID: "R", TenantID: "tenant-1", Name: "X", Type: FileTypeFolder},
	} {
		if err := dao.DB.Create(f).Error; err != nil {
			t.Fatalf("seed %s: %v", f.ID, err)
		}
	}

	svc := testFileService()
	if err := svc.MoveFiles(context.Background(), "tenant-1", []string{"A", "X"}, "D", ""); err != nil {
		t.Fatalf("move unrelated entries failed: %v", err)
	}
	if got := countChildren(t, "D"); got != 2 {
		t.Fatalf("destination children = %d, want 2", got)
	}
	childByName(t, "D", "A")
	childByName(t, "D", "X")
}

// A single folder still moves with its whole subtree; the new check must not
// block the legitimate case it was designed around.
func TestMoveFilesSingleFolderWithSubtreeStillMoves(t *testing.T) {
	seedMoveHierarchyDB(t)

	svc := testFileService()
	if err := svc.MoveFiles(context.Background(), "tenant-1", []string{"A"}, "D", ""); err != nil {
		t.Fatalf("move single folder failed: %v", err)
	}

	movedA := childByName(t, "D", "A")
	childByName(t, movedA.ID, "B")
}
