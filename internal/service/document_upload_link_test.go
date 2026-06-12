//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package service

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// uploadLinkTestService builds a DocumentService wired with the DAOs the upload
// path needs (including fileDAO, which testDocumentService omits).
func uploadLinkTestService() *DocumentService {
	return &DocumentService{
		documentDAO:      dao.NewDocumentDAO(),
		kbDAO:            dao.NewKnowledgebaseDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
		fileDAO:          dao.NewFileDAO(),
	}
}

func uploadLinkTestKB() *entity.Knowledgebase {
	return &entity.Knowledgebase{
		ID:           "kb-1",
		TenantID:     "tenant-1",
		Name:         "test-kb",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
	}
}

// TestUploadEmptyDocument_AppearsInDatasetList reproduces the reviewer's
// "can't upload any file" report: DocumentDAO.ListByKBID inner-joins
// file2document + file, so a document inserted without that linkage never
// shows up in the dataset's document list. UploadEmptyDocument must create the
// File row + file2document mapping (addFileFromKB) so the document is listed.
func TestUploadEmptyDocument_AppearsInDatasetList(t *testing.T) {
	db := setupServiceTestDB(t)
	// ListByKBID LEFT JOINs user_canvas; the table must exist for the query.
	if err := db.AutoMigrate(&entity.UserCanvas{}); err != nil {
		t.Fatalf("migrate user_canvas: %v", err)
	}
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)
	svc := uploadLinkTestService()
	kb := uploadLinkTestKB()

	data, code, err := svc.UploadEmptyDocument(kb, "tenant-1", "report.txt")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("UploadEmptyDocument failed: code=%d err=%v", code, err)
	}
	docID, _ := data["id"].(string)
	if docID == "" {
		t.Fatalf("expected a document id, got %v", data["id"])
	}

	// The document must be linked into the file manager.
	mappings, err := dao.NewFile2DocumentDAO().GetByDocumentID(docID)
	if err != nil {
		t.Fatalf("GetByDocumentID: %v", err)
	}
	if len(mappings) != 1 {
		t.Fatalf("expected exactly 1 file2document mapping, got %d", len(mappings))
	}
	files, err := dao.NewFileDAO().GetByIDs([]string{*mappings[0].FileID})
	if err != nil || len(files) != 1 {
		t.Fatalf("expected the linked file row to exist, got %d (err=%v)", len(files), err)
	}
	if files[0].Name != "report.txt" {
		t.Fatalf("linked file name = %q, want report.txt", files[0].Name)
	}

	// The regression check: the document is now visible in the dataset list.
	docs, total, err := dao.NewDocumentDAO().ListByKBID("kb-1", 0, 10)
	if err != nil {
		t.Fatalf("ListByKBID: %v", err)
	}
	if total != 1 || len(docs) != 1 {
		t.Fatalf("expected the uploaded document to be listed (total=1), got total=%d len=%d", total, len(docs))
	}
	if docs[0].Name == nil || *docs[0].Name != "report.txt" {
		t.Fatalf("listed document name = %v, want report.txt", docs[0].Name)
	}
}

// TestUploadEmptyDocument_ReusesDatasetFolders verifies the folder chain
// (root -> .knowledgebase -> <dataset>) is created once and reused across
// uploads rather than duplicated, mirroring Python new_a_file_from_kb dedupe.
func TestUploadEmptyDocument_ReusesDatasetFolders(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.UserCanvas{}); err != nil {
		t.Fatalf("migrate user_canvas: %v", err)
	}
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)
	svc := uploadLinkTestService()
	kb := uploadLinkTestKB()

	for _, name := range []string{"a.txt", "b.txt"} {
		if _, code, err := svc.UploadEmptyDocument(kb, "tenant-1", name); err != nil || code != common.CodeSuccess {
			t.Fatalf("upload %s failed: code=%d err=%v", name, code, err)
		}
	}

	// Exactly three folders: root, .knowledgebase, and the dataset folder.
	var folders int64
	if err := dao.DB.Model(&entity.File{}).Where("type = ?", "folder").Count(&folders).Error; err != nil {
		t.Fatalf("count folders: %v", err)
	}
	if folders != 3 {
		t.Fatalf("expected 3 reused folders across two uploads, got %d", folders)
	}

	// Both documents are listed.
	_, total, err := dao.NewDocumentDAO().ListByKBID("kb-1", 0, 10)
	if err != nil {
		t.Fatalf("ListByKBID: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 listed documents, got %d", total)
	}
}
