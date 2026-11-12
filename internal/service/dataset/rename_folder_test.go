package dataset

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	"ragflow/internal/service/document"
	filesvc "ragflow/internal/service/file"
	"ragflow/internal/storage"
)

func insertRenameTestFolder(t *testing.T, id, parentID, tenantID, name, source string) {
	t.Helper()
	if err := dao.DB.Create(&entity.File{
		ID: id, ParentID: parentID, TenantID: tenantID, CreatedBy: tenantID,
		Name: name, Type: "folder", SourceType: source,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestDatasetRenameFolder(t *testing.T) {
	for _, tc := range []struct {
		name         string
		root         bool
		kbRoot       bool
		original     bool
		target       bool
		targetScope  string
		targetType   string
		originalType string
		kbRootType   string
		newName      string
		wantCode     common.ErrorCode
	}{
		{name: "rename existing folder", root: true, kbRoot: true, original: true, newName: "Renamed"},
		{name: "case only rename", root: true, kbRoot: true, original: true, newName: "ORIGINAL"},
		{name: "same name", root: true, kbRoot: true, original: true, newName: "Original"},
		{name: "target occupied", root: true, kbRoot: true, original: true, target: true, newName: "Renamed", wantCode: common.CodeDataError},
		{name: "target occupied without original", root: true, kbRoot: true, target: true, newName: "Renamed", wantCode: common.CodeDataError},
		{name: "target occupied by virtual file", root: true, kbRoot: true, original: true, target: true, targetType: "virtual", newName: "Renamed", wantCode: common.CodeDataError},
		{name: "virtual file collision without original", root: true, kbRoot: true, target: true, targetType: "virtual", newName: "Renamed", wantCode: common.CodeDataError},
		{name: "virtual file collision ignores case", root: true, kbRoot: true, original: true, target: true, targetType: "virtual", newName: "RENAMED", wantCode: common.CodeDataError},
		{name: "no original folder", root: true, kbRoot: true, newName: "Renamed"},
		{name: "no knowledgebase root", root: true, newName: "Renamed"},
		{name: "no root", newName: "Renamed"},
		{name: "original name is a virtual file", root: true, kbRoot: true, original: true, originalType: "virtual", newName: "Renamed", wantCode: common.CodeDataError},
		{name: "knowledgebase root is a virtual file", root: true, kbRoot: true, kbRootType: "virtual", newName: "Renamed", wantCode: common.CodeDataError},
		{name: "same name in another tenant", root: true, kbRoot: true, original: true, target: true, targetScope: "tenant", newName: "Renamed"},
		{name: "same name under another parent", root: true, kbRoot: true, original: true, target: true, targetScope: "parent", newName: "Renamed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupDatasetUpdateTestDB(t)
			pushServiceDB(t, db)
			insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
			if tc.root {
				insertRenameTestFolder(t, "root", "root", "tenant-1", "/", "")
			}
			if tc.kbRoot {
				insertRenameTestFolder(t, "kb-root", "root", "tenant-1", ".knowledgebase", "knowledgebase")
			}
			if tc.original {
				insertRenameTestFolder(t, "original", "kb-root", "tenant-1", "Original", "knowledgebase")
				// Matching names outside the owned folder must remain untouched.
				insertRenameTestFolder(t, "unrelated-original", "root", "tenant-1", "Original", "knowledgebase")
			}
			for id, fileType := range map[string]string{"original": tc.originalType, "kb-root": tc.kbRootType} {
				if fileType != "" {
					if err := db.Model(&entity.File{}).Where("id = ?", id).Update("type", fileType).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			if tc.target {
				parentID, tenantID := "kb-root", "tenant-1"
				if tc.targetScope == "tenant" {
					tenantID = "tenant-2"
				}
				if tc.targetScope == "parent" {
					parentID = "root"
				}
				if tc.targetType == "virtual" {
					ctx := withDatasetTestContext(t.Context(), "tenant-1", tenantID)
					if _, err := filesvc.NewFileService(nil).CreateFolder(ctx, "Renamed", parentID, "virtual"); err != nil {
						t.Fatalf("create virtual file: %v", err)
					}
				} else {
					insertRenameTestFolder(t, "target", parentID, tenantID, "Renamed", "")
				}
			}
			var before int64
			if err := db.Model(&entity.File{}).Count(&before).Error; err != nil {
				t.Fatal(err)
			}
			ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
			_, code, err := testDatasetUpdateService(t).UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{Name: &tc.newName})
			if code != tc.wantCode || (err != nil) != (tc.wantCode != common.CodeSuccess) {
				t.Fatalf("rename: code=%v err=%v; want code=%v", code, err, tc.wantCode)
			}
			wantName := tc.newName
			if tc.wantCode != common.CodeSuccess {
				wantName = "Original"
			}
			kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, "kb-1")
			if err != nil || kb.Name != wantName {
				t.Fatalf("knowledgebase=%+v err=%v; want name %s", kb, err, wantName)
			}
			if tc.original {
				var folder entity.File
				if err := db.Where("id = ?", "original").Take(&folder).Error; err != nil || folder.Name != wantName || folder.ParentID != "kb-root" {
					t.Fatalf("folder=%+v err=%v; want name %s with unchanged id and parent", folder, err, wantName)
				}
				folder = entity.File{}
				if err := db.Where("id = ?", "unrelated-original").Take(&folder).Error; err != nil || folder.Name != "Original" {
					t.Fatalf("unrelated folder changed: %+v err=%v", folder, err)
				}
			}
			var after int64
			if err := db.Model(&entity.File{}).Count(&after).Error; err != nil || after != before {
				t.Fatalf("rename created/deleted files: before=%d after=%d err=%v", before, after, err)
			}
		})
	}
}

func TestDatasetRenameFolderRollsBack(t *testing.T) {
	for _, table := range []string{"knowledgebase", "file"} {
		t.Run(table+" update fails", func(t *testing.T) {
			db := setupDatasetUpdateTestDB(t)
			pushServiceDB(t, db)
			insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Original")
			insertRenameTestFolder(t, "root", "root", "tenant-1", "/", "")
			insertRenameTestFolder(t, "kb-root", "root", "tenant-1", ".knowledgebase", "knowledgebase")
			insertRenameTestFolder(t, "original", "kb-root", "tenant-1", "Original", "knowledgebase")
			if err := db.Exec("CREATE TRIGGER reject_rename BEFORE UPDATE OF name ON " + table + " WHEN NEW.name = 'Renamed' BEGIN SELECT RAISE(ABORT, 'rename rejected'); END").Error; err != nil {
				t.Fatal(err)
			}
			name := "Renamed"
			ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
			_, code, err := testDatasetUpdateService(t).UpdateDataset(ctx, "kb-1", service.UpdateDatasetRequest{Name: &name})
			if err == nil || code != common.CodeServerError {
				t.Fatalf("rename: code=%v err=%v; want server error", code, err)
			}
			if !strings.Contains(err.Error(), "rename rejected") {
				t.Fatalf("unexpected public database error: %v", err)
			}
			kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, "kb-1")
			if err != nil || kb.Name != "Original" {
				t.Fatalf("knowledgebase did not roll back: %+v err=%v", kb, err)
			}
			var folder entity.File
			if err := db.Where("id = ?", "original").Take(&folder).Error; err != nil || folder.Name != "Original" {
				t.Fatalf("folder did not roll back: %+v err=%v", folder, err)
			}
		})
	}
}

func TestDatasetRenameFolderSurvivesDeletingReusedName(t *testing.T) {
	for _, tc := range []struct {
		name          string
		manualRoot    bool
		manualDataset bool
	}{
		{name: "managed folders"},
		{name: "user created dataset folder", manualDataset: true},
		{name: "user created knowledgebase root", manualRoot: true},
		{name: "both user created", manualRoot: true, manualDataset: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupDatasetUpdateTestDB(t)
			pushServiceDB(t, db)
			insertCreateDatasetTenant(t, "tenant-1")
			ctx := withDatasetTestContext(t.Context(), "tenant-1", "tenant-1")
			factory := storage.GetStorageFactory()
			previous := factory.GetStorage()
			store := storage.NewMemoryStorage()
			factory.SetStorage(store)
			t.Cleanup(func() { factory.SetStorage(previous) })
			datasets := testDatasetCreateService(t)
			documents := document.NewDocumentService()
			var manualFolderID, kbRootID string
			if tc.manualRoot || tc.manualDataset {
				root, err := dao.NewFileDAO().GetRootFolder(ctx, db, "tenant-1")
				if err != nil {
					t.Fatal(err)
				}
				if tc.manualRoot {
					created, err := filesvc.NewFileService(nil).CreateFolder(ctx, ".knowledgebase", root.ID, "folder")
					if err != nil {
						t.Fatal(err)
					}
					kbRootID = created["id"].(string)
				} else {
					kbRootID = "kb-root"
					insertRenameTestFolder(t, kbRootID, root.ID, "tenant-1", ".knowledgebase", "knowledgebase")
				}
				if tc.manualDataset {
					created, err := filesvc.NewFileService(nil).CreateFolder(ctx, "Original", kbRootID, "folder")
					if err != nil {
						t.Fatal(err)
					}
					manualFolderID = created["id"].(string)
				}
			}
			create := func(name string) *entity.Knowledgebase {
				t.Helper()
				result, code, err := datasets.CreateDataset(ctx, &service.CreateDatasetRequest{Name: name, ParserID: sptr("general"), ParseType: iptr(1)})
				if err != nil || code != common.CodeSuccess {
					t.Fatalf("create %s: code=%v err=%v", name, code, err)
				}
				kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, result["id"].(string))
				if err != nil {
					t.Fatal(err)
				}
				return kb
			}
			upload := func(kb *entity.Knowledgebase, name string) (string, entity.File) {
				t.Helper()
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				part, err := writer.CreateFormFile("file", name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = part.Write([]byte("content for " + name)); err != nil {
					t.Fatal(err)
				}
				if err = writer.Close(); err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest("POST", "/", &body)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				if err = req.ParseMultipartForm(1 << 20); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = req.MultipartForm.RemoveAll() })
				results, errs := documents.UploadLocalDocuments(ctx, kb, req.MultipartForm.File["file"], "", nil)
				if len(errs) != 0 || len(results) != 1 {
					t.Fatalf("upload %s: results=%v errors=%v", name, results, errs)
				}
				id := results[0]["id"].(string)
				var file entity.File
				if err = db.Table("file").Select("file.*").Joins("JOIN file2document ON file2document.file_id = file.id").Where("file2document.document_id = ?", id).Take(&file).Error; err != nil {
					t.Fatal(err)
				}
				return id, file
			}
			a := create("Original")
			oldDoc, oldFile := upload(a, "a-old.txt")
			if tc.manualDataset && oldFile.ParentID != manualFolderID {
				t.Fatal("upload did not reuse user-created dataset folder")
			}
			name := "Renamed"
			if _, code, err := datasets.UpdateDataset(ctx, a.ID, service.UpdateDatasetRequest{Name: &name}); err != nil || code != common.CodeSuccess {
				t.Fatalf("rename: code=%v err=%v", code, err)
			}
			var err error
			a, err = dao.NewKnowledgebaseDAO().GetByID(ctx, db, a.ID)
			if err != nil {
				t.Fatal(err)
			}
			newDoc, newFile := upload(a, "a-new.txt")
			if newFile.ParentID != oldFile.ParentID {
				t.Fatalf("A upload changed folders after rename: old=%s new=%s", oldFile.ParentID, newFile.ParentID)
			}
			b := create("Original")
			bDoc, bFile := upload(b, "b.txt")
			if bFile.ParentID == oldFile.ParentID {
				t.Fatal("B reused A's folder")
			}
			if _, code, err := datasets.DeleteDatasets(ctx, []string{b.ID}, false); err != nil || code != common.CodeSuccess {
				t.Fatalf("delete B: code=%v err=%v", code, err)
			}
			var folder entity.File
			if err := db.Where("id = ?", oldFile.ParentID).Take(&folder).Error; err != nil || folder.Name != "Renamed" {
				t.Fatalf("A folder missing or incorrectly named: %+v err=%v", folder, err)
			}
			if tc.manualDataset && folder.SourceType != "" {
				t.Fatalf("rename changed folder source: %q", folder.SourceType)
			}
			if kbRootID != "" && folder.ParentID != kbRootID {
				t.Fatal("upload did not reuse the expected knowledgebase root")
			}
			for _, check := range []struct {
				model interface{}
				id    string
				want  int64
			}{
				{&entity.Knowledgebase{}, a.ID, 1}, {&entity.Document{}, oldDoc, 1}, {&entity.Document{}, newDoc, 1},
				{&entity.File{}, oldFile.ID, 1}, {&entity.File{}, newFile.ID, 1},
				{&entity.Knowledgebase{}, b.ID, 0}, {&entity.Document{}, bDoc, 0}, {&entity.File{}, bFile.ID, 0}, {&entity.File{}, bFile.ParentID, 0},
			} {
				var count int64
				if err := db.Model(check.model).Where("id = ?", check.id).Count(&count).Error; err != nil || count != check.want {
					t.Fatalf("%T id=%s: count=%d want=%d err=%v", check.model, check.id, count, check.want, err)
				}
			}
			for _, name := range []string{"a-old.txt", "a-new.txt"} {
				blob, err := store.Get(ctx, a.ID, name)
				if err != nil || string(blob) != "content for "+name {
					t.Fatalf("A content changed: name=%s blob=%q err=%v", name, blob, err)
				}
			}
			if store.BucketExists(ctx, b.ID) {
				t.Fatal("B storage bucket survived deletion")
			}
		})
	}
}
