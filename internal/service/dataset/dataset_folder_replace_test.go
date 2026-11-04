package dataset

import (
	"ragflow/internal/entity"
	documentsvc "ragflow/internal/service/document"
	"testing"
	"time"
)

func TestDatasetFolderAfterReplacingFileLinks(t *testing.T) {
	f := newDatasetFolderFixture(t)
	a := f.create("alpha")
	file := f.put(a, "before")
	b := f.create("bravo")
	var original entity.Document
	f.must(f.db.Where("kb_id = ?", a).Take(&original).Error)
	f.must(documentsvc.NewFile2DocumentService().LinkToDatasets(f.ctx, "tenant-1", &documentsvc.LinkToDatasetsRequest{FileIDs: []string{file.ID}, KbIDs: []string{b}}, "replace"))
	deadline := time.Now().Add(3 * time.Second)
	for f.count(&entity.Document{}, "kb_id = ?", a) != 0 || f.count(&entity.Document{}, "kb_id = ?", b) != 1 || f.count(&entity.File2Document{}, "file_id = ?", file.ID) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("normal replace did not complete")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if f.file(file.ID).ParentID != file.ParentID {
		t.Fatal("replace unexpectedly moved source File")
	}
	if _, err := f.upload(a, "after-replace"); err != nil {
		t.Errorf("normal replace blocked original dataset upload: %v", err)
	}
	if err := f.rename(a, "renamed"); err != nil {
		t.Errorf("normal replace blocked original dataset rename: %v", err)
	}
}
