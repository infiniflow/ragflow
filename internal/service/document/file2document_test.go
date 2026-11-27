package document

import (
	"mime/multipart"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/service/file"
)

func TestLinkedFilesRemainReadableAfterBrowsing(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	useFakeStorage(t)
	const tenantID = "user-1"
	insertTestKB(t, "dataset-a", tenantID, 0, 0, 0)
	insertTestKB(t, "dataset-b", tenantID, 0, 0, 0)
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "dataset-b").Update("name", "second-dataset").Error; err != nil {
		t.Fatal(err)
	}

	ctx := documentTestContextFor(t.Context(), tenantID)
	docs := testDocumentService(t)
	files := file.NewFileService(docs)
	folder, err := files.CreateFolder(ctx, "practice", "", file.FileTypeFolder)
	if err != nil {
		t.Fatal(err)
	}
	folderID := folder["id"].(string)
	const filename = "说明书.pdf"
	const blob = "%PDF-1.4 sample"
	uploads, err := files.UploadFile(ctx, folderID, []*multipart.FileHeader{
		makeTestFileHeader(t, "file", filename, []byte(blob)),
	}, 0)
	if err != nil || len(uploads) != 1 {
		t.Fatalf("upload to file management: uploads=%v, err=%v", uploads, err)
	}
	fileID := uploads[0]["id"].(string)
	linker := NewFile2DocumentService()
	// Run the same conversion as LinkToDatasets synchronously so both mappings
	// are committed before browsing triggers dataset folder initialization.
	if err := linker.convertFiles(ctx, []string{fileID}, []string{"dataset-a", "dataset-b"}, "add"); err != nil {
		t.Fatalf("link uploaded file to datasets: %v", err)
	}
	var linked []entity.Document
	if err := db.Where("kb_id IN ?", []string{"dataset-a", "dataset-b"}).Find(&linked).Error; err != nil || len(linked) != 2 {
		t.Fatalf("load both linked documents: count=%d, err=%v", len(linked), err)
	}

	kb, err := docs.kbDAO.GetByID(ctx, db, "dataset-a")
	if err != nil {
		t.Fatal(err)
	}
	direct, errs := docs.UploadLocalDocuments(ctx, kb, []*multipart.FileHeader{
		makeTestFileHeader(t, "file", filename, []byte("direct dataset upload")),
	}, "nested", nil)
	if len(errs) != 0 || len(direct) != 1 {
		t.Fatalf("upload directly to dataset: uploads=%v, errors=%v", direct, errs)
	}
	directID := direct[0]["id"].(string)
	directMappings, err := docs.file2DocumentDAO.GetByDocumentID(ctx, db, directID)
	if err != nil || len(directMappings) != 1 || directMappings[0].FileID == nil {
		t.Fatalf("load direct upload mapping: mappings=%v, err=%v", directMappings, err)
	}
	directFileID := *directMappings[0].FileID
	directFile, err := docs.fileDAO.GetByID(ctx, db, directFileID)
	if err != nil {
		t.Fatal(err)
	}
	// Folder initialization must still repair metadata for dataset-owned files.
	if err := db.Model(&entity.File{}).Where("id = ?", directFileID).
		Updates(map[string]interface{}{"parent_id": folderID, "user_id": "stale-owner"}).Error; err != nil {
		t.Fatal(err)
	}

	checkPreview := func(when, docID, want string) {
		t.Helper()
		preview, err := docs.GetDocumentPreview(ctx, docID)
		if err != nil {
			t.Errorf("%s: preview %s: %v", when, docID, err)
		} else if string(preview.Data) != want {
			t.Errorf("%s: preview %s = %q, want %q", when, docID, preview.Data, want)
		}
	}
	for _, doc := range linked {
		checkPreview("before browsing", doc.ID, blob)
	}
	checkPreview("before browsing", directID, "direct dataset upload")

	for range 2 {
		if _, err := files.ListFilesForTenant(ctx, tenantID, "", 1, 10, nil, ""); err != nil {
			t.Fatalf("browse file management root: %v", err)
		}
	}
	for _, doc := range linked {
		checkPreview("after browsing", doc.ID, blob)
		ref, err := component.ResolveDocumentStorage(ctx, db, doc.ID)
		if err != nil {
			t.Errorf("resolve ingestion input for %s: %v", doc.KbID, err)
		} else if ref.Bucket != folderID || ref.Path != filename {
			t.Errorf("ingestion input for %s = %s/%s, want %s/%s", doc.KbID, ref.Bucket, ref.Path, folderID, filename)
		}
	}
	addr, err := files.GetStorageAddress(ctx, fileID)
	if err != nil {
		t.Errorf("resolve linked file storage: %v", err)
	} else if addr.Bucket != folderID || addr.Name != filename {
		t.Errorf("linked file storage = %s/%s, want %s/%s", addr.Bucket, addr.Name, folderID, filename)
	}
	list, err := files.ListFilesForTenant(ctx, tenantID, folderID, 1, 10, nil, "")
	if err != nil || len(list.Files) != 1 || list.Files[0]["id"] != fileID {
		t.Errorf("list original folder after browsing: list=%+v, err=%v", list, err)
	}
	checkPreview("after browsing", directID, "direct dataset upload")
	repaired, err := docs.fileDAO.GetByID(ctx, db, directFileID)
	if err != nil {
		t.Fatal(err)
	}
	if repaired.ParentID != directFile.ParentID || repaired.UserID != directFile.UserID {
		t.Errorf("dataset file parent/owner = %s/%s, want %s/%s", repaired.ParentID, repaired.UserID, directFile.ParentID, directFile.UserID)
	}
}
