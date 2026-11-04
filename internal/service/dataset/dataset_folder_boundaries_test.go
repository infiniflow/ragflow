package dataset

import (
	"testing"

	"ragflow/internal/entity"
	filesvc "ragflow/internal/service/file"
)

func TestDatasetFolderRevisedBoundaries(t *testing.T) {
	t.Run("own_nested_files_allow_upload_and_rename", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		af := f.put(a, "before")
		sub, err := filesvc.NewFileService(f.docs).CreateFolder(f.ctx, "tenant-1", "nested", af.ParentID, "folder")
		f.must(err)
		subID := sub["id"].(string)
		f.must(f.db.Model(&entity.File{}).Where("id = ?", af.ID).Update("parent_id", subID).Error)
		after := f.put(a, "after")
		if after.ParentID != af.ParentID {
			t.Fatal("upload changed dataset folder")
		}
		f.must(f.rename(a, "beta"))
		if f.file(af.ParentID).Name != "beta" {
			t.Fatal("rename did not update dataset folder")
		}
	})
	t.Run("unbound_foreign_only_nested_file_blocks_mutation_and_survives_delete", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		af := f.put(a, "a")
		// Unbound data still requires one-time association validation.
		f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("folder_id", nil).Error)
		b := f.create("bravo")
		bf := f.put(b, "b")
		sub, err := filesvc.NewFileService(f.docs).CreateFolder(f.ctx, "tenant-1", "nested", af.ParentID, "folder")
		f.must(err)
		subID := sub["id"].(string)
		f.must(f.db.Model(&entity.File{}).Where("id = ?", bf.ID).Update("parent_id", subID).Error)
		if _, err := f.upload(a, "after"); err == nil {
			t.Fatal("foreign-only nested file did not block upload")
		}
		if err := f.rename(a, "beta"); err == nil {
			t.Fatal("foreign-only nested file did not block rename")
		}
		f.remove(a)
		for _, id := range []string{af.ParentID, subID, bf.ID} {
			if f.count(&entity.File{}, "id = ?", id) != 1 {
				t.Fatalf("deletion removed surviving directory/file %s", id)
			}
		}
		if f.count(&entity.Document{}, "kb_id = ?", b) != 1 {
			t.Fatal("deletion removed B document")
		}
	})
	t.Run("draft_before_first_dataset_upload_is_allowed", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		folder := f.empty(a)
		_, err := filesvc.NewFileService(f.docs).CreateFolder(f.ctx, "tenant-1", "draft", folder.ID, "virtual")
		f.must(err)
		file := f.put(a, "first")
		if file.ParentID != folder.ID {
			t.Fatal("first upload failed to reuse normal draft directory")
		}
	})
	t.Run("unbound_dangling_document_mapping_still_blocks_mutation", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		af := f.put(a, "a")
		f.must(f.db.Model(&entity.Knowledgebase{}).Where("id = ?", a).Update("folder_id", nil).Error)
		other := entity.File{ID: "dangling-file", ParentID: af.ParentID, TenantID: "tenant-1", CreatedBy: "tenant-1", Name: "dangling", Type: "pdf", SourceType: "knowledgebase"}
		f.must(f.db.Create(&other).Error)
		missing := "missing-doc"
		f.must(f.db.Create(&entity.File2Document{ID: "dangling-map", FileID: &other.ID, DocumentID: &missing}).Error)
		if _, err := f.upload(a, "after"); err == nil {
			t.Fatal("dangling mapping did not block upload")
		}
		if err := f.rename(a, "beta"); err == nil {
			t.Fatal("dangling mapping did not block rename")
		}
	})
}
