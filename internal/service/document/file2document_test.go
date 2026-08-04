package document

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

// linkTestFixture bundles a KB plus a helper to create source files for
// convertFiles tests.
type linkTestFixture struct {
	kbID     string
	tenantID string
	create   func(t *testing.T, name string) *entity.File
}

func setupLinkTest(t *testing.T) *linkTestFixture {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	fx := &linkTestFixture{
		kbID:     utility.GenerateUUID(),
		tenantID: utility.GenerateUUID(),
	}
	kb := &entity.Knowledgebase{
		ID:           fx.kbID,
		Name:         "kb",
		TenantID:     fx.tenantID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
	}
	if err := db.Create(kb).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}
	fx.create = func(t *testing.T, name string) *entity.File {
		t.Helper()
		f := &entity.File{
			ID:       utility.GenerateUUID(),
			ParentID: utility.GenerateUUID(),
			TenantID: fx.tenantID,
			Name:     name,
			Type:     "pdf",
			Size:     100,
		}
		if err := db.Create(f).Error; err != nil {
			t.Fatalf("create file: %v", err)
		}
		return f
	}
	return fx
}

func docNamesInKB(t *testing.T, kbID string) []string {
	t.Helper()
	names, err := dao.NewDocumentDAO().ListNamesByKbID(context.Background(), dao.DB, kbID)
	if err != nil {
		t.Fatalf("list doc names: %v", err)
	}
	return names
}

func assertNoDuplicateNames(t *testing.T, names []string) {
	t.Helper()
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if seen[n] {
			t.Fatalf("duplicate document name in KB: %q (all: %v)", n, names)
		}
		seen[n] = true
	}
}

// Linking the same file twice in add mode must not create a second document.
func TestConvertFilesAddModeSkipsAlreadyLinkedKB(t *testing.T) {
	fx := setupLinkTest(t)
	ctx := context.Background()
	svc := NewFile2DocumentService()
	f := fx.create(t, "laws.pdf")

	for i := 0; i < 2; i++ {
		if err := svc.convertFiles(ctx, []string{f.ID}, []string{fx.kbID}, "user1", "add"); err != nil {
			t.Fatalf("convert %d: %v", i, err)
		}
	}

	names := docNamesInKB(t, fx.kbID)
	if len(names) != 1 || names[0] != "laws.pdf" {
		t.Fatalf("expected a single laws.pdf document, got %v", names)
	}
}

// Linking two different files sharing a name must auto-rename the second
// document instead of creating duplicate names in the KB.
func TestConvertFilesAddModeRenamesNameCollision(t *testing.T) {
	fx := setupLinkTest(t)
	ctx := context.Background()
	svc := NewFile2DocumentService()
	f1 := fx.create(t, "laws.pdf")
	f2 := fx.create(t, "laws.pdf")

	if err := svc.convertFiles(ctx, []string{f1.ID}, []string{fx.kbID}, "user1", "add"); err != nil {
		t.Fatalf("first convert: %v", err)
	}
	if err := svc.convertFiles(ctx, []string{f2.ID}, []string{fx.kbID}, "user1", "add"); err != nil {
		t.Fatalf("second convert: %v", err)
	}

	names := docNamesInKB(t, fx.kbID)
	if len(names) != 2 {
		t.Fatalf("expected 2 documents, got %v", names)
	}
	assertNoDuplicateNames(t, names)
}

// Re-linking in replace mode removes the previous document, leaving exactly
// one document with the original name.
func TestConvertFilesReplaceModeKeepsSingleDocument(t *testing.T) {
	fx := setupLinkTest(t)
	ctx := context.Background()
	svc := NewFile2DocumentService()
	f := fx.create(t, "laws.pdf")

	for i := 0; i < 2; i++ {
		if err := svc.convertFiles(ctx, []string{f.ID}, []string{fx.kbID}, "user1", "replace"); err != nil {
			t.Fatalf("convert %d: %v", i, err)
		}
	}

	names := docNamesInKB(t, fx.kbID)
	if len(names) != 1 || names[0] != "laws.pdf" {
		t.Fatalf("expected a single laws.pdf document, got %v", names)
	}
}

// Concurrent link requests (the endpoint is fire-and-forget) must not race the
// name check and insert duplicate document names into the same KB.
func TestConvertFilesConcurrentLinksNoDuplicateNames(t *testing.T) {
	fx := setupLinkTest(t)
	ctx := context.Background()
	svc := NewFile2DocumentService()

	const n = 8
	files := make([]*entity.File, 0, n)
	for i := 0; i < n; i++ {
		files = append(files, fx.create(t, "laws.pdf"))
	}

	var wg sync.WaitGroup
	errs := make(chan error, n)
	for _, f := range files {
		wg.Add(1)
		go func(fileID string) {
			defer wg.Done()
			if err := svc.convertFiles(ctx, []string{fileID}, []string{fx.kbID}, "user1", "add"); err != nil {
				errs <- fmt.Errorf("convert %s: %w", fileID, err)
			}
		}(f.ID)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	names := docNamesInKB(t, fx.kbID)
	if len(names) != n {
		t.Fatalf("expected %d documents, got %v", n, names)
	}
	assertNoDuplicateNames(t, names)
}
