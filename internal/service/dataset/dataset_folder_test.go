package dataset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	documentsvc "ragflow/internal/service/document"
	"ragflow/internal/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type datasetFolderFixture struct {
	t    *testing.T
	db   *gorm.DB
	ctx  context.Context
	ds   *DatasetService
	docs *documentsvc.DocumentService
}

func newDatasetFolderFixture(t *testing.T) *datasetFolderFixture {
	db := setupDatasetUpdateTestDB(t).Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.FileCommit{}, &entity.FileCommitItem{}, &entity.SkillSpace{}); err != nil {
		t.Fatal(err)
	}
	insertCreateDatasetTenant(t, "tenant-1")
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(previous) })
	return &datasetFolderFixture{
		t:    t,
		db:   db,
		ctx:  withDatasetTestContext(t.Context(), "tenant-1", "tenant-1"),
		ds:   testDatasetCreateService(t),
		docs: documentsvc.NewDocumentService(),
	}
}

func (f *datasetFolderFixture) must(err error) {
	f.t.Helper()
	if err != nil {
		f.t.Fatal(err)
	}
}

func (f *datasetFolderFixture) kb(id string) *entity.Knowledgebase {
	f.t.Helper()
	kb, err := dao.NewKnowledgebaseDAO().GetByID(f.ctx, f.db, id)
	f.must(err)
	return kb
}

func (f *datasetFolderFixture) create(name string) string {
	f.t.Helper()
	r, c, e := f.ds.CreateDataset(f.ctx, &service.CreateDatasetRequest{Name: name, ParserID: sptr("general"), ParseType: iptr(1)})
	if e != nil || c != common.CodeSuccess {
		f.t.Fatalf("create: code=%v error=%v", c, e)
	}
	return r["id"].(string)
}

func (f *datasetFolderFixture) rename(id, name string) error {
	f.t.Helper()
	_, _, e := f.ds.UpdateDataset(f.ctx, id, "tenant-1", service.UpdateDatasetRequest{Name: &name})
	return e
}

func (f *datasetFolderFixture) upload(id, name string) (entity.File, error) {
	f.t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	p, e := w.CreateFormFile("file", name+".txt")
	f.must(e)
	_, e = p.Write([]byte(name))
	f.must(e)
	f.must(w.Close())
	r := httptest.NewRequest("POST", "/", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	f.must(r.ParseMultipartForm(1 << 20))
	defer r.MultipartForm.RemoveAll()
	results, errs := f.docs.UploadLocalDocuments(f.ctx, f.kb(id), "tenant-1", r.MultipartForm.File["file"], "", nil)
	if len(errs) > 0 {
		return entity.File{}, errors.New(errs[0])
	}
	if len(results) != 1 {
		f.t.Fatalf("upload returned %v", results)
	}
	var file entity.File
	f.must(f.db.Table("file").Select("file.*").Joins("JOIN file2document ON file2document.file_id=file.id").Where("file2document.document_id = ?", results[0]["id"]).Take(&file).Error)
	return file, nil
}

func (f *datasetFolderFixture) put(id, name string) entity.File {
	f.t.Helper()
	file, e := f.upload(id, name)
	f.must(e)
	return file
}

func (f *datasetFolderFixture) file(id string) entity.File {
	f.t.Helper()
	var file entity.File
	f.must(f.db.Where("id = ?", id).Take(&file).Error)
	return file
}

func (f *datasetFolderFixture) count(model any, condition string, args ...any) int64 {
	f.t.Helper()
	var n int64
	f.must(f.db.Model(model).Where(condition, args...).Count(&n).Error)
	return n
}

func (f *datasetFolderFixture) remove(id string) {
	f.t.Helper()
	_, c, e := f.ds.DeleteDatasets(f.ctx, []string{id}, false)
	if e != nil || c != common.CodeSuccess {
		f.t.Fatalf("delete: code=%v error=%v", c, e)
	}
}

func (f *datasetFolderFixture) empty(id string) entity.File {
	f.t.Helper()
	root, e := dao.NewFileDAO().GetRootFolder(f.ctx, f.db, "tenant-1")
	f.must(e)
	var parent entity.File
	e = f.db.Where("parent_id = ? AND name = ?", root.ID, ".knowledgebase").Take(&parent).Error
	if e == gorm.ErrRecordNotFound {
		parent = entity.File{ID: common.GenerateToken(), ParentID: root.ID, TenantID: "tenant-1", CreatedBy: "tenant-1", Name: ".knowledgebase", Type: "folder", SourceType: "knowledgebase"}
		f.must(f.db.Create(&parent).Error)
	} else {
		f.must(e)
	}
	folder := entity.File{ID: common.GenerateToken(), ParentID: parent.ID, TenantID: "tenant-1", CreatedBy: "tenant-1", Name: f.kb(id).Name, Type: "folder", SourceType: "knowledgebase"}
	f.must(f.db.Create(&folder).Error)
	return folder
}

func (f *datasetFolderFixture) reference(kind string, folder entity.File) {
	f.t.Helper()
	switch kind {
	case "commit":
		f.must(f.db.Create(&entity.FileCommit{ID: "history", FolderID: folder.ID, AuthorID: "tenant-1"}).Error)
	case "item":
		f.must(f.db.Create(&entity.FileCommitItem{ID: "item", CommitID: "foreign-history", FileID: folder.ID, Operation: "delete"}).Error)
	case "snapshot":
		tree, _ := json.Marshal(map[string]any{folder.ID: map[string]any{"parent_id": "old-parent"}})
		s := string(tree)
		f.must(f.db.Create(&entity.FileCommit{ID: "history", FolderID: "foreign-folder", AuthorID: "tenant-1", TreeState: &s}).Error)
	case "snapshot_parent":
		tree, _ := json.Marshal(map[string]any{"deleted-child": map[string]any{"parent_id": folder.ID}})
		s := string(tree)
		f.must(f.db.Create(&entity.FileCommit{ID: "history", FolderID: "foreign-folder", AuthorID: "tenant-1", TreeState: &s}).Error)
	case "mapping":
		f.must(f.db.Create(&entity.File2Document{ID: "direct-map", FileID: &folder.ID, DocumentID: sptr("old-doc")}).Error)
	case "skill":
		f.must(f.db.Create(&entity.SkillSpace{ID: "space", TenantID: "tenant-1", Name: "space", FolderID: folder.ID}).Error)
	}
}

func TestDatasetFolderLifecycle(t *testing.T) {
	t.Run("delete_preserves_directory_mapped_to_deleted_document", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		id := f.create("alpha")
		folder := f.empty(id)
		doc := &entity.Document{
			ID:           "linked-doc",
			KbID:         id,
			CreatedBy:    "tenant-1",
			Name:         sptr("linked"),
			Type:         "virtual",
			ParserID:     "general",
			ParserConfig: entity.JSONMap{},
		}
		f.must(f.db.Create(doc).Error)
		f.must(f.db.Create(&entity.File2Document{
			ID:         "folder-mapping",
			FileID:     &folder.ID,
			DocumentID: &doc.ID,
		}).Error)
		f.reference("commit", folder)

		f.remove(id)

		if f.count(&entity.Knowledgebase{}, "id = ?", id) != 0 ||
			f.count(&entity.Document{}, "id = ?", doc.ID) != 0 ||
			f.count(&entity.File2Document{}, "id = ?", "folder-mapping") != 0 {
			t.Fatal("dataset deletion retained its own records")
		}
		if f.count(&entity.File{}, "id = ?", folder.ID) != 1 ||
			f.count(&entity.FileCommit{}, "folder_id = ?", folder.ID) != 1 {
			t.Fatal("mapped-file cleanup removed a directory with retained history")
		}
	})
	t.Run("rename_preserves_folder_on_next_upload", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		before := f.put(a, "before")
		f.must(f.rename(a, "beta"))
		after := f.put(a, "after")
		if before.ParentID != after.ParentID || f.file(before.ParentID).Name != "beta" {
			t.Fatal("rename split dataset across folders")
		}
	})
	t.Run("old_name_reuse_does_not_share_folder", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		af := f.put(a, "a")
		f.must(f.rename(a, "beta"))
		b := f.create("alpha")
		bf := f.put(b, "b")
		if af.ParentID == bf.ParentID {
			t.Fatal("different datasets reused one folder")
		}
		f.remove(b)
		if f.count(&entity.File{}, "id = ?", af.ParentID) != 1 {
			t.Fatal("deleting B removed A folder")
		}
	})
	t.Run("rename_without_directory_is_read_only", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		f.must(f.rename(a, "beta"))
		if f.count(&entity.File{}, "1 = 1") != 0 {
			t.Fatal("rename created directories")
		}
	})
	t.Run("initialized_empty_folder_accepts_first_upload", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		root, e := dao.NewFileDAO().GetRootFolder(f.ctx, f.db, "tenant-1")
		f.must(e)
		f.must(dao.NewFileDAO().InitDatasetDocs(f.ctx, f.db, root.ID, "tenant-1", dao.NewFile2DocumentDAO()))
		var folder entity.File
		f.must(f.db.Where("name = ? AND type = ?", "alpha", "folder").Take(&folder).Error)
		file := f.put(a, "first")
		if file.ParentID != folder.ID {
			t.Fatal("first upload did not reuse initialized empty folder")
		}
	})
	t.Run("empty_folder_is_renamed", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		folder := f.empty(a)
		f.must(f.rename(a, "beta"))
		if f.file(folder.ID).Name != "beta" {
			t.Fatal("empty folder not renamed")
		}
	})
	t.Run("shared_historical_folder_blocks_upload", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		f.put(a, "a")
		f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("name", "beta").Error)
		b := f.create("alpha")
		_, e := f.upload(b, "b")
		if e == nil {
			t.Fatal("upload reused another dataset's historical folder")
		}
	})
	t.Run("delete_preserves_another_dataset_historical_folder", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		af := f.put(a, "a")
		f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("name", "beta").Error)
		b := f.create("alpha")
		f.remove(b)
		if f.count(&entity.File{}, "id = ?", af.ParentID) != 1 {
			t.Fatal("deleted another dataset folder")
		}
	})
	t.Run("delete_shared_folder_preserves_surviving_child", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		af := f.put(a, "a")
		b := f.create("bravo")
		bf := f.put(b, "b")
		f.must(f.db.Model(&entity.File{}).Where("id = ?", bf.ID).Update("parent_id", af.ParentID).Error)
		f.must(f.db.Delete(&entity.File{}, "id = ?", bf.ParentID).Error)
		f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("name", "beta").Error)
		f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", b).Update("name", "alpha").Error)
		f.remove(b)
		if f.count(&entity.File{}, "id = ?", af.ParentID) != 1 || f.count(&entity.File{}, "id = ?", af.ID) != 1 {
			t.Fatal("shared directory or surviving child removed")
		}
	})
	t.Run("delete_preserves_file_linked_to_other_dataset", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		af := f.put(a, "a")
		b := f.create("bravo")
		var doc entity.Document
		f.must(f.db.Where("kb_id = ?", a).Take(&doc).Error)
		doc.ID = "shared-doc"
		doc.KbID = b
		f.must(f.db.Create(&doc).Error)
		f.must(f.db.Create(&entity.File2Document{ID: "shared-map", FileID: &af.ID, DocumentID: &doc.ID}).Error)
		f.remove(a)
		if f.count(&entity.File{}, "id = ?", af.ID) != 1 || f.count(&entity.File{}, "id = ?", af.ParentID) != 1 {
			t.Fatal("shared File or parent was removed")
		}
	})
	t.Run("target_directory_collision_rolls_back", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		af := f.put(a, "a")
		target := entity.File{ID: "stale-target", ParentID: f.file(af.ParentID).ParentID, TenantID: "tenant-1", CreatedBy: "tenant-1", Name: "beta", Type: "folder", SourceType: "knowledgebase"}
		f.must(f.db.Create(&target).Error)
		if f.rename(a, "beta") == nil {
			t.Fatal("rename accepted stale target directory")
		}
		if f.kb(a).Name != "alpha" || f.file(af.ParentID).Name != "alpha" {
			t.Fatal("rename partially committed")
		}
	})
	t.Run("folder_update_error_rolls_back_dataset", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		af := f.put(a, "a")
		f.must(f.db.Exec("CREATE TRIGGER fail_folder_rename BEFORE UPDATE OF name ON file BEGIN SELECT RAISE(ABORT, 'folder update failure'); END").Error)
		if f.rename(a, "beta") == nil {
			t.Fatal("folder update failure did not reject rename")
		}
		if f.kb(a).Name != "alpha" || f.file(af.ParentID).Name != "alpha" {
			t.Fatal("failed rename changed stored names")
		}
	})
	t.Run("reserved_name_does_not_delete_shared_root", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create(".knowledgebase")
		af := f.put(a, "a")
		shared := f.file(af.ParentID).ParentID
		f.remove(a)
		if f.count(&entity.File{}, "id = ?", shared) != 1 {
			t.Fatal("dataset deletion removed .knowledgebase parent")
		}
	})
	for _, kind := range []string{"commit", "item", "snapshot", "snapshot_parent", "mapping", "skill"} {
		t.Run("referenced_empty_rejects_upload_"+kind, func(t *testing.T) {
			f := newDatasetFolderFixture(t)
			a := f.create("alpha")
			folder := f.empty(a)
			f.reference(kind, folder)
			if _, e := f.upload(a, "first"); e == nil {
				t.Fatal("referenced empty directory was claimed")
			}
			if f.count(&entity.Document{}, "kb_id = ?", a) != 0 {
				t.Fatal("rejected upload created a document")
			}
		})
		t.Run("referenced_empty_survives_delete_"+kind, func(t *testing.T) {
			f := newDatasetFolderFixture(t)
			a := f.create("alpha")
			folder := f.empty(a)
			f.reference(kind, folder)
			f.remove(a)
			if f.count(&entity.File{}, "id = ?", folder.ID) != 1 {
				t.Fatal("directory with retained association was deleted")
			}
		})
	}
	t.Run("unrelated_update_does_not_inspect_empty_history", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		folder := f.empty(a)
		f.reference("commit", folder)
		description := "updated"
		_, c, e := f.ds.UpdateDataset(f.ctx, a, "tenant-1", service.UpdateDatasetRequest{Name: sptr("alpha"), Description: &description})
		if e != nil || c != common.CodeSuccess {
			t.Fatalf("unchanged-name update rejected: %v", e)
		}
	})
	t.Run("same_name_nonfolder_is_not_used_as_directory", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		folder := f.empty(a)
		f.must(f.db.Model(&entity.File{}).Where("id = ?", folder.ID).Updates(map[string]any{"type": "pdf", "source_type": "local"}).Error)
		file, e := f.upload(a, "first")
		if e == nil && file.ParentID == folder.ID {
			t.Fatal("ordinary file used as parent directory")
		}
	})
}
