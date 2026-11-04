package dataset

import (
	"context"
	"fmt"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	documentsvc "ragflow/internal/service/document"
	filesvc "ragflow/internal/service/file"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDatasetFolderNormalOperations(t *testing.T) {
	t.Run("linking_file_to_second_dataset_keeps_uploads_working", func(t *testing.T) {
		f := newDatasetFolderFixture(t)
		a := f.create("alpha")
		af := f.put(a, "before")
		b := f.create("bravo")
		err := documentsvc.NewFile2DocumentService().LinkToDatasets(f.ctx, "tenant-1", &documentsvc.LinkToDatasetsRequest{FileIDs: []string{af.ID}, KbIDs: []string{b}}, "add")
		f.must(err)
		deadline := time.Now().Add(3 * time.Second)
		for f.count(&entity.Document{}, "kb_id = ?", b) != 1 || f.count(&entity.File2Document{}, "file_id = ?", af.ID) != 2 {
			if time.Now().After(deadline) {
				t.Fatal("normal file-linking operation did not complete")
			}
			time.Sleep(10 * time.Millisecond)
		}
		for _, id := range []string{a, b} {
			if _, err := f.upload(id, "after-link"); err != nil {
				t.Errorf("normal file link blocked upload to %s: %v", f.kb(id).Name, err)
			}
		}
		if err := f.rename(a, "renamed"); err != nil {
			t.Errorf("normal file link blocked rename: %v", err)
		}
	})
	for _, kind := range []string{"folder", "virtual"} {
		t.Run("file_manager_child_keeps_upload_working_"+kind, func(t *testing.T) {
			f := newDatasetFolderFixture(t)
			a := f.create("alpha")
			af := f.put(a, "before")
			_, err := filesvc.NewFileService(f.docs).CreateFolder(f.ctx, "tenant-1", "workspace-child", af.ParentID, kind)
			f.must(err)
			if _, err := f.upload(a, "after-file-manager-create"); err != nil {
				t.Errorf("normal %s creation blocked upload: %v", kind, err)
			}
			if err := f.rename(a, "renamed"); err != nil {
				t.Errorf("normal %s creation blocked rename: %v", kind, err)
			}
		})
	}
}

type folderQueryCounter struct {
	logger.Interface
	queries int
}

func (q *folderQueryCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	q.queries++
	q.Interface.Trace(ctx, begin, fc, err)
}
func TestDatasetFolderLookupQueryGrowth(t *testing.T) {
	for _, n := range []int{1, 100, 1000} {
		t.Run(fmt.Sprintf("children_%d", n), func(t *testing.T) {
			f := newDatasetFolderFixture(t)
			a := f.create("alpha")
			af := f.put(a, "before")
			var doc entity.Document
			f.must(f.db.Where("kb_id = ?", a).Take(&doc).Error)
			for i := 1; i < n; i++ {
				copyDoc := doc
				copyDoc.ID = fmt.Sprintf("extra-doc-%d", i)
				f.must(f.db.Create(&copyDoc).Error)
				file := af
				file.ID = fmt.Sprintf("extra-file-%d", i)
				f.must(f.db.Create(&file).Error)
				f.must(f.db.Create(&entity.File2Document{ID: fmt.Sprintf("extra-map-%d", i), FileID: &file.ID, DocumentID: &copyDoc.ID}).Error)
			}
			counter := &folderQueryCounter{Interface: logger.Default.LogMode(logger.Silent)}
			original := dao.DB
			dao.DB = f.db.Session(&gorm.Session{Logger: counter})
			defer func() { dao.DB = original }()
			result, code, err := f.docs.UploadEmptyDocument(f.ctx, f.kb(a), "tenant-1", "after-count")
			if err != nil {
				t.Fatalf("upload failed: code=%v result=%v error=%v", code, result, err)
			}
			t.Logf("real upload with %d existing children executed %d SQL statements", n, counter.queries)
			if counter.queries > 24 {
				t.Fatalf("upload queried existing files individually: %d SQL statements for %d children", counter.queries, n)
			}
		})
	}
}
