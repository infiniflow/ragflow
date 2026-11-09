package document

import (
	"testing"

	"ragflow/internal/entity"
)

func TestEnsureKBFolderUsesCurrentName(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "create"
		if existing {
			name = "reuse"
		}
		t.Run(name, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertTestKB(t, "kb-upload", "tenant-1", 0, 0, 0)
			if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-upload").Update("name", "Renamed").Error; err != nil {
				t.Fatal(err)
			}
			svc := testDocumentService(t)
			stale := &entity.Knowledgebase{ID: "kb-upload", TenantID: "tenant-1", Name: "Original"}
			var originalID string
			if existing {
				folder, err := svc.ensureKBFolder(t.Context(), stale, "tenant-1")
				if err != nil {
					t.Fatal(err)
				}
				originalID = folder.ID
			}
			folder, err := svc.ensureKBFolder(t.Context(), stale, "tenant-1")
			if err != nil {
				t.Fatal(err)
			}
			if folder.Name != "Renamed" || (existing && folder.ID != originalID) {
				t.Fatalf("unexpected folder: %+v", folder)
			}
			var count int64
			if err := db.Model(&entity.File{}).Where("name = ?", "Original").Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("old folders=%d err=%v", count, err)
			}
		})
	}
}

func TestEnsureKBFolderRollsBackCreatedParents(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-upload", "tenant-1", 0, 0, 0)
	if err := db.Exec(`CREATE TRIGGER reject_dataset_folder BEFORE INSERT ON file WHEN NEW.name = 'Rejected' BEGIN SELECT RAISE(ABORT, 'reject folder'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-upload").Update("name", "Rejected").Error; err != nil {
		t.Fatal(err)
	}
	kb := &entity.Knowledgebase{ID: "kb-upload", TenantID: "tenant-1", Name: "Rejected"}
	if _, err := testDocumentService(t).ensureKBFolder(t.Context(), kb, "tenant-1"); err == nil {
		t.Fatal("expected folder creation failure")
	}
	var count int64
	if err := db.Model(&entity.File{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial folders=%d err=%v", count, err)
	}
}
