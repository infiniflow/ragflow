package dataset

import (
	"encoding/json"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	documentsvc "ragflow/internal/service/document"

	"gorm.io/gorm"
)

func assertBoundFolder(t *testing.T, f *datasetFolderFixture, id, folderID string) {
	t.Helper()
	kb := f.kb(id)
	if kb.FolderID == nil || *kb.FolderID != folderID {
		t.Fatalf("dataset %s binding=%v, expected %s", id, kb.FolderID, folderID)
	}
}
func waitReplacedLinks(t *testing.T, f *datasetFolderFixture, a, b, fileID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for f.count(&entity.Document{}, "kb_id = ?", a) != 0 || f.count(&entity.File2Document{}, "file_id = ?", fileID) != boolCount(b != "") {
		if time.Now().After(deadline) {
			t.Fatal("replace/unlink did not complete")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if b != "" && f.count(&entity.Document{}, "kb_id = ?", b) != 1 {
		t.Fatal("replacement target document missing")
	}
}
func boolCount(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func TestStableDatasetFolderBinding(t *testing.T) {
	t.Run("new_upload_records_binding", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		if f.kb(a).FolderID != nil {
			t.Fatal("create eagerly bound a nonexistent folder")
		}
		file := f.put(a, "first")
		assertBoundFolder(t, f, a, file.ParentID)
	})
	for _, legacy := range []bool{false, true} {
		for _, unlink := range []bool{false, true} {
			name := "bound"
			if legacy {
				name = "legacy"
			}
			mode := "replace"
			if unlink {
				mode = "unlink"
			}
			t.Run(name+"_"+mode+"_keeps_original_dataset_working", func(t *testing.T) {
				f := newDatasetFolderFixture(t)
				a := f.create("alpha")
				file := f.put(a, "before")
				if legacy {
					f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("folder_id", nil).Error)
				}
				var targets []string
				b := ""
				if !unlink {
					b = f.create("bravo")
					targets = []string{b}
				}
				f.must(documentsvc.NewFile2DocumentService().LinkToDatasets(f.ctx, "tenant-1", &documentsvc.LinkToDatasetsRequest{FileIDs: []string{file.ID}, KbIDs: targets}, "replace"))
				waitReplacedLinks(t, f, a, b, file.ID)
				assertBoundFolder(t, f, a, file.ParentID)
				if f.file(file.ID).ParentID != file.ParentID {
					t.Fatal("replace relocated source File")
				}
				after := f.put(a, "after")
				if after.ParentID != file.ParentID {
					t.Fatal("upload switched directory")
				}
				f.must(f.rename(a, "beta"))
				if f.file(file.ParentID).Name != "beta" {
					t.Fatal("rename did not update bound folder")
				}
				if b != "" {
					f.remove(a)
					if f.count(&entity.File{}, "id = ?", file.ID) != 1 || f.count(&entity.File{}, "id = ?", file.ParentID) != 1 || f.count(&entity.Document{}, "kb_id = ?", b) != 1 {
						t.Fatal("deleting A damaged B's remaining File/directory/document")
					}
				}
			})
		}
	}
	t.Run("binding_failure_rolls_back_new_directories", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		f.must(f.db.Exec("CREATE TRIGGER reject_binding BEFORE UPDATE OF folder_id ON knowledgebase BEGIN SELECT RAISE(ABORT, 'binding failure'); END").Error)
		if _, err := f.upload(a, "first"); err == nil {
			t.Fatal("binding failure accepted")
		}
		if f.count(&entity.File{}, "1 = 1") != 0 || f.count(&entity.Document{}, "kb_id = ?", a) != 0 || f.kb(a).FolderID != nil {
			t.Fatal("binding failure left partial records")
		}
	})
	t.Run("rename_failure_rolls_back_lazy_binding", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		folder := f.empty(a)
		f.must(f.db.Exec("CREATE TRIGGER reject_rename BEFORE UPDATE OF name ON file BEGIN SELECT RAISE(ABORT, 'rename failure'); END").Error)
		if f.rename(a, "beta") == nil {
			t.Fatal("rename failure accepted")
		}
		if f.kb(a).FolderID != nil || f.kb(a).Name != "alpha" || f.file(folder.ID).Name != "alpha" {
			t.Fatal("rename failure committed binding or name")
		}
	})
	t.Run("missing_bound_folder_does_not_adopt_same_name", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		file := f.put(a, "first")
		original := f.file(file.ParentID)
		f.must(f.db.Delete(&entity.File{}, "id = ?", original.ID).Error)
		other := original
		other.ID = "replacement-folder"
		f.must(f.db.Create(&other).Error)
		if _, err := f.upload(a, "after"); err == nil {
			t.Fatal("missing binding silently adopted a same-name directory")
		}
		assertBoundFolder(t, f, a, original.ID)
		if f.count(&entity.Document{}, "kb_id = ?", a) != 1 {
			t.Fatal("failed upload created a document")
		}
	})
	t.Run("stale_kb_snapshot_uses_live_binding", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		snapshot := f.kb(a)
		file := f.put(a, "first")
		f.must(f.db.Model(&entity.File{}).Where("id = ?", file.ParentID).Update("name", "manual-folder-name").Error)
		result, code, err := f.docs.UploadEmptyDocument(f.ctx, snapshot, "tenant-1", "after")
		if err != nil || code != common.CodeSuccess {
			t.Fatalf("stale snapshot upload failed: %v", err)
		}
		var linked entity.File
		f.must(f.db.Table("file").Select("file.*").Joins("JOIN file2document ON file2document.file_id = file.id").Where("file2document.document_id = ?", result["id"]).Take(&linked).Error)
		if linked.ParentID != file.ParentID {
			t.Fatal("stale snapshot switched bound folder")
		}
	})
	t.Run("bound_empty_history_accepts_upload_and_rename", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		file := f.put(a, "first")
		folder := f.file(file.ParentID)
		f.reference("commit", folder)
		var doc entity.Document
		f.must(f.db.Where("kb_id = ?", a).Take(&doc).Error)
		n, err := f.docs.DeleteDocuments(f.ctx, []string{doc.ID}, false, a, "tenant-1")
		f.must(err)
		if n != 1 || f.count(&entity.File{}, "parent_id = ?", folder.ID) != 0 {
			t.Fatal("delete did not empty folder")
		}
		assertBoundFolder(t, f, a, folder.ID)
		after := f.put(a, "after")
		if after.ParentID != folder.ID {
			t.Fatal("history-bearing bound folder not reused")
		}
		f.must(f.rename(a, "beta"))
		if f.count(&entity.FileCommit{}, "folder_id = ?", folder.ID) != 1 {
			t.Fatal("rename lost existing history")
		}
	})
	t.Run("another_dataset_cannot_claim_bound_empty_folder", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		file := f.put(a, "first")
		f.must(f.db.Where("file_id = ?", file.ID).Delete(&entity.File2Document{}).Error)
		f.must(f.db.Where("kb_id = ?", a).Delete(&entity.Document{}).Error)
		f.must(f.db.Delete(&entity.File{}, "id = ?", file.ID).Error)
		f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("name", "beta").Error)
		b := f.create("alpha")
		if _, err := f.upload(b, "first"); err == nil {
			t.Fatal("B claimed A's bound empty folder")
		}
		assertBoundFolder(t, f, a, file.ParentID)
		if f.kb(b).FolderID != nil {
			t.Fatal("B acquired conflicting binding")
		}
	})
	t.Run("bound_directory_accepts_changed_descendant_mappings", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		af := f.put(a, "a")
		b := f.create("bravo")
		bf := f.put(b, "b")
		f.must(f.db.Model(&entity.File{}).Where("id = ?", bf.ID).Update("parent_id", af.ParentID).Error)
		f.put(a, "after")
		f.must(f.rename(a, "beta"))
		f.remove(a)
		if f.count(&entity.File{}, "id = ?", bf.ID) != 1 || f.count(&entity.File{}, "id = ?", af.ParentID) != 1 {
			t.Fatal("stable directory owner cleanup removed foreign child")
		}
	})
	t.Run("binding_is_not_part_of_public_json", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		f.put(a, "first")
		encoded, err := json.Marshal(f.kb(a))
		f.must(err)
		var data map[string]any
		f.must(json.Unmarshal(encoded, &data))
		if _, ok := data["folder_id"]; ok {
			t.Fatal("entity JSON exposes internal binding")
		}
		result, code, err := f.ds.GetDataset(f.ctx, a, "tenant-1")
		if err != nil || code != common.CodeSuccess {
			t.Fatalf("dataset response failed: %v", err)
		}
		if _, ok := result["folder_id"]; ok {
			t.Fatal("dataset API exposes internal binding")
		}
	})
	t.Run("legacy_binding_preparation_does_not_create_directory", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		f.must(dao.NewFileDAO().BindExistingDatasetFolder(f.ctx, f.db, f.kb(a)))
		if f.count(&entity.File{}, "1 = 1") != 0 || f.kb(a).FolderID != nil {
			t.Fatal("preparation created or bound absent directory")
		}
	})
	t.Run("bound_folder_survives_caller_transaction_rollback", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		err := f.db.Transaction(func(tx *gorm.DB) error {
			_, err := dao.NewFileDAO().EnsureDatasetFolder(f.ctx, tx, f.kb(a))
			if err != nil {
				return err
			}
			return gorm.ErrInvalidTransaction
		})
		if err != gorm.ErrInvalidTransaction {
			t.Fatalf("unexpected error %v", err)
		}
		if f.count(&entity.File{}, "1 = 1") != 0 || f.kb(a).FolderID != nil {
			t.Fatal("caller rollback retained directory binding")
		}
	})
	t.Run("read_only_lookup_does_not_bind_legacy_directory", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		folder := f.empty(a)
		got, err := dao.NewFileDAO().GetDatasetFolder(f.ctx, f.db, f.kb(a))
		f.must(err)
		if got == nil || got.ID != folder.ID || f.kb(a).FolderID != nil {
			t.Fatal("read-only lookup changed legacy binding")
		}
	})
	t.Run("legacy_ambiguous_directory_is_not_force_bound", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		af := f.put(a, "a")
		f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("folder_id", nil).Error)
		b := f.create("bravo")
		bf := f.put(b, "b")
		f.must(f.db.Model(&entity.File{}).Where("id = ?", bf.ID).Update("parent_id", af.ParentID).Error)
		if err := dao.NewFileDAO().BindExistingDatasetFolder(f.ctx, f.db, f.kb(a)); err == nil {
			t.Fatal("ambiguous legacy directory was force-bound")
		}
		if f.kb(a).FolderID != nil {
			t.Fatal("ambiguous legacy directory received binding")
		}
	})
	t.Run("unrelated_updates_keep_binding", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		file := f.put(a, "first")
		description := "updated"
		_, code, err := f.ds.UpdateDataset(f.ctx, a, "tenant-1", service.UpdateDatasetRequest{Description: &description})
		if err != nil || code != common.CodeSuccess {
			t.Fatalf("update failed: %v", err)
		}
		assertBoundFolder(t, f, a, file.ParentID)
	})
}

func TestStableDatasetFolderDeletionAndSnapshots(t *testing.T) {
	t.Run("legacy_document_delete_prepares_binding_before_unlink", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		file := f.put(a, "before")
		f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("folder_id", nil).Error)
		b := f.create("bravo")
		f.must(documentsvc.NewFile2DocumentService().LinkToDatasets(f.ctx, "tenant-1", &documentsvc.LinkToDatasetsRequest{FileIDs: []string{file.ID}, KbIDs: []string{b}}, "add"))
		deadline := time.Now().Add(3 * time.Second)
		for f.count(&entity.Document{}, "kb_id = ?", b) != 1 || f.count(&entity.File2Document{}, "file_id = ?", file.ID) != 2 {
			if time.Now().After(deadline) {
				t.Fatal("link did not complete")
			}
			time.Sleep(10 * time.Millisecond)
		}
		var doc entity.Document
		f.must(f.db.Where("kb_id = ?", a).Take(&doc).Error)
		n, err := f.docs.DeleteDocuments(f.ctx, []string{doc.ID}, false, a, "tenant-1")
		f.must(err)
		if n != 1 {
			t.Fatal("document deletion failed")
		}
		assertBoundFolder(t, f, a, file.ParentID)
		f.put(a, "after")
		f.must(f.rename(a, "beta"))
	})
	t.Run("legacy_last_document_with_history_can_be_replaced", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		file := f.put(a, "before")
		f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("folder_id", nil).Error)
		f.reference("commit", f.file(file.ParentID))
		var doc entity.Document
		f.must(f.db.Where("kb_id = ?", a).Take(&doc).Error)
		n, err := f.docs.DeleteDocuments(f.ctx, []string{doc.ID}, false, a, "tenant-1")
		f.must(err)
		if n != 1 {
			t.Fatal("document deletion failed")
		}
		assertBoundFolder(t, f, a, file.ParentID)
		f.put(a, "after")
		f.must(f.rename(a, "beta"))
	})
	t.Run("unbound_snapshot_uses_live_name_after_rename", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		snapshot := f.kb(a)
		f.must(f.rename(a, "beta"))
		result, code, err := f.docs.UploadEmptyDocument(f.ctx, snapshot, "tenant-1", "after")
		if err != nil || code != common.CodeSuccess {
			t.Fatalf("upload failed %v", err)
		}
		var file entity.File
		f.must(f.db.Table("file").Select("file.*").Joins("JOIN file2document ON file2document.file_id = file.id").Where("file2document.document_id = ?", result["id"]).Take(&file).Error)
		if f.file(file.ParentID).Name != "beta" {
			t.Fatalf("stale snapshot created folder %q for live beta", f.file(file.ParentID).Name)
		}
		if snapshot.Name != "alpha" {
			t.Fatal("DAO mutated caller snapshot")
		}
	})
	t.Run("missing_bound_folder_does_not_block_dataset_delete", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		file := f.put(a, "before")
		folder := f.file(file.ParentID)
		f.must(f.db.Delete(&entity.File{}, "id = ?", folder.ID).Error)
		replacement := folder
		replacement.ID = "replacement"
		f.must(f.db.Create(&replacement).Error)
		f.remove(a)
		if f.count(&entity.Knowledgebase{}, "id = ?", a) != 0 || f.count(&entity.File{}, "id = ?", replacement.ID) != 1 {
			t.Fatal("delete did not preserve unrelated replacement directory")
		}
	})
	t.Run("legacy_multi_file_replace_binds_once_before_removal", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		var ids []string
		folderID := ""
		for _, name := range []string{"one", "two", "three"} {
			file := f.put(a, name)
			ids = append(ids, file.ID)
			folderID = file.ParentID
		}
		f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("folder_id", nil).Error)
		b := f.create("bravo")
		f.must(documentsvc.NewFile2DocumentService().LinkToDatasets(f.ctx, "tenant-1", &documentsvc.LinkToDatasetsRequest{FileIDs: ids, KbIDs: []string{b}}, "replace"))
		deadline := time.Now().Add(3 * time.Second)
		for f.count(&entity.Document{}, "kb_id = ?", a) != 0 || f.count(&entity.Document{}, "kb_id = ?", b) != 3 || f.count(&entity.File2Document{}, "file_id IN ?", ids) != 3 {
			if time.Now().After(deadline) {
				t.Fatal("multi-file replacement failed")
			}
			time.Sleep(10 * time.Millisecond)
		}
		assertBoundFolder(t, f, a, folderID)
		f.put(a, "after")
		f.must(f.rename(a, "beta"))
	})
}

func TestStableDatasetFolderAmbiguousDeletion(t *testing.T) {
	f := newDatasetFolderFixture(t)
	a := f.create("alpha")
	af := f.put(a, "a")
	b := f.create("bravo")
	bf := f.put(b, "b")
	f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("folder_id", nil).Error)
	f.must(f.db.Model(&entity.File{}).Where("id = ?", bf.ID).Update("parent_id", af.ParentID).Error)
	var doc entity.Document
	f.must(f.db.Where("kb_id = ?", a).Take(&doc).Error)
	n, err := f.docs.DeleteDocuments(f.ctx, []string{doc.ID}, false, a, "tenant-1")
	f.must(err)
	if n != 1 || f.count(&entity.Document{}, "id = ?", doc.ID) != 0 {
		t.Fatal("ambiguous legacy directory blocked ordinary document deletion")
	}
	if f.kb(a).FolderID != nil {
		t.Fatal("ambiguous directory was force-bound")
	}
	if f.count(&entity.File{}, "id = ?", bf.ID) != 1 || f.count(&entity.File{}, "id = ?", af.ParentID) != 1 || f.count(&entity.Document{}, "kb_id = ?", b) != 1 {
		t.Fatal("ordinary document deletion damaged B or shared directory")
	}
}

func TestStableDatasetFolderBindingFailurePreservesDocument(t *testing.T) {
	f := newDatasetFolderFixture(t)
	a := f.create("alpha")
	file := f.put(a, "before")
	f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("folder_id", nil).Error)
	f.must(f.db.Exec("CREATE TRIGGER reject_binding BEFORE UPDATE OF folder_id ON knowledgebase BEGIN SELECT RAISE(ABORT, 'binding failure'); END").Error)
	var doc entity.Document
	f.must(f.db.Where("kb_id = ?", a).Take(&doc).Error)
	n, err := f.docs.DeleteDocuments(f.ctx, []string{doc.ID}, false, a, "tenant-1")
	f.must(err)
	if n != 0 || f.count(&entity.Document{}, "id = ?", doc.ID) != 1 || f.count(&entity.File2Document{}, "document_id = ?", doc.ID) != 1 || f.count(&entity.File{}, "id = ?", file.ID) != 1 {
		t.Fatal("binding database failure allowed partial document deletion")
	}
}
