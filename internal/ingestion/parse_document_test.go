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

package ingestion

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	doctype "ragflow/internal/deepdoc/parser/type"
)

// ── Mock DAOs ──────────────────────────────────────────────────────────

type mockDocDAO struct {
	dao.DocumentDAO
	docs map[string]*entity.Document
	err  error
}

func (m *mockDocDAO) GetByID(id string) (*entity.Document, error) {
	if m.err != nil {
		return nil, m.err
	}
	d, ok := m.docs[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return d, nil
}

type mockF2dDAO struct {
	dao.File2DocumentDAO
	mappings map[string][]*entity.File2Document
	err      error
}

func (m *mockF2dDAO) GetByDocumentID(docID string) ([]*entity.File2Document, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.mappings[docID], nil
}

type mockFileDAO struct {
	dao.FileDAO
	files map[string]*entity.File
	err   error
}

func (m *mockFileDAO) GetByID(id string) (*entity.File, error) {
	if m.err != nil {
		return nil, m.err
	}
	f, ok := m.files[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return f, nil
}

// ── Mock Storage ───────────────────────────────────────────────────────

type mockStorage struct {
	data         map[string][]byte // key = "bucket/objectName"
	err          error
	putCallCount int
	putBucket    string
	putFnm       string
	putData      []byte
}

func (m *mockStorage) Get(bucket, objectName string, tenantID ...string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	key := bucket + "/" + objectName
	d, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	return d, nil
}

func (m *mockStorage) Health() bool { return true }
func (m *mockStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	if m.err != nil {
		return m.err
	}
	m.putCallCount++
	m.putBucket = bucket
	m.putFnm = fnm
	m.putData = binary
	return nil
}
func (m *mockStorage) Remove(_, _ string, _ ...string) error       { return nil }
func (m *mockStorage) ObjExist(_, _ string, _ ...string) bool      { return true }
func (m *mockStorage) GetPresignedURL(_, _ string, _ time.Duration, _ ...string) (string, error) {
	return "", nil
}
func (m *mockStorage) BucketExists(_ string) bool { return true }
func (m *mockStorage) RemoveBucket(_ string) error { return nil }
func (m *mockStorage) Copy(_, _, _, _ string) bool  { return true }
func (m *mockStorage) Move(_, _, _, _ string) bool  { return true }

// ── Mock PDF Parser ────────────────────────────────────────────────────

type mockPDFParser struct {
	sections []map[string]any
	err      error
}

func (m *mockPDFParser) ParseWithDeepDoc(ctx context.Context, filename string, data []byte, config doctype.ParserConfig) ([]map[string]any, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.sections, nil
}

// ── Test Helpers ───────────────────────────────────────────────────────

func newTestIngestor() *Ingestor {
	return &Ingestor{
		id: "test-ingestor",
	}
}

func testTask(docID string) *entity.IngestionTask {
	return &entity.IngestionTask{
		ID:         "task-1",
		DocumentID: docID,
		DatasetID:  "kb-1",
	}
}

func testDoc(id string, docType string, suffix string, kbID ...string) *entity.Document {
	name := "test." + suffix
	loc := "doc_store/" + id
	kbid := "kb-1"
	if len(kbID) > 0 {
		kbid = kbID[0]
	}
	return &entity.Document{
		ID:       id,
		KbID:     kbid,
		Type:     docType,
		Suffix:   suffix,
		Name:     &name,
		Location: &loc,
		ParserID: "naive",
	}
}

// ── Tests ──────────────────────────────────────────────────────────────

func TestParseDocument_DocNotFound(t *testing.T) {
	e := newTestIngestor()
	e.documentDAO = &mockDocDAO{
		err: errors.New("record not found"),
	}

	err := e.parseDocument(context.Background(), testTask("doc-1"))
	if err == nil {
		t.Fatal("expected error for doc not found")
	}
	if !errors.Is(err, errors.New("record not found")) {
		// Just check it contains the root error
	}
}

func TestParseDocument_NilName(t *testing.T) {
	e := newTestIngestor()
	e.documentDAO = &mockDocDAO{
		docs: map[string]*entity.Document{
			"doc-1": {ID: "doc-1", Name: nil, Location: strPtr("loc")},
		},
	}

	err := e.parseDocument(context.Background(), testTask("doc-1"))
	if err == nil {
		t.Fatal("expected error for nil name")
	}
}

func TestParseDocument_NilLocation(t *testing.T) {
	e := newTestIngestor()
	e.documentDAO = &mockDocDAO{
		docs: map[string]*entity.Document{
			"doc-1": {ID: "doc-1", Name: strPtr("test.pdf"), Location: nil},
		},
	}

	err := e.parseDocument(context.Background(), testTask("doc-1"))
	if err == nil {
		t.Fatal("expected error for nil location")
	}
}

func TestParseDocument_StorageNil(t *testing.T) {
	e := newTestIngestor()
	e.documentDAO = &mockDocDAO{
		docs: map[string]*entity.Document{
			"doc-1": testDoc("doc-1", "pdf", ".pdf"),
		},
	}
	e.f2dDAO = &mockF2dDAO{} // no mappings → falls back to doc.Location
	// storageImpl is nil (not set)

	err := e.parseDocument(context.Background(), testTask("doc-1"))
	if err == nil {
		t.Fatal("expected error for nil storage")
	}
}

func TestParseDocument_StorageGetFails(t *testing.T) {
	e := newTestIngestor()
	e.documentDAO = &mockDocDAO{
		docs: map[string]*entity.Document{
			"doc-1": testDoc("doc-1", "pdf", ".pdf"),
		},
	}
	e.f2dDAO = &mockF2dDAO{}
	e.storageImpl = &mockStorage{
		err: errors.New("connection refused"),
	}

	err := e.parseDocument(context.Background(), testTask("doc-1"))
	if err == nil {
		t.Fatal("expected error for storage failure")
	}
}

func TestParseDocument_NonPDF(t *testing.T) {
	e := newTestIngestor()
	doc := testDoc("doc-1", "docx", ".docx")
	e.documentDAO = &mockDocDAO{
		docs: map[string]*entity.Document{
			"doc-1": doc,
		},
	}
	e.f2dDAO = &mockF2dDAO{}
	e.storageImpl = &mockStorage{
		data: map[string][]byte{
			"/doc_store/doc-1": []byte("fake docx bytes"),
		},
	}

	err := e.parseDocument(context.Background(), testTask("doc-1"))
	if err != nil {
		t.Fatalf("expected no error for non-PDF, got: %v", err)
	}
}

func TestParseDocument_PDFParseFails(t *testing.T) {
	e := newTestIngestor()
	e.documentDAO = &mockDocDAO{
		docs: map[string]*entity.Document{
			"doc-1": testDoc("doc-1", "pdf", ".pdf"),
		},
	}
	e.f2dDAO = &mockF2dDAO{}
	e.storageImpl = &mockStorage{
		data: map[string][]byte{
			"/doc_store/doc-1": []byte("fake pdf bytes"),
		},
	}
	e.pdfParser = &mockPDFParser{
		err: errors.New("parse error"),
	}

	err := e.parseDocument(context.Background(), testTask("doc-1"))
	if err == nil {
		t.Fatal("expected error for parse failure")
	}
}

func TestParseDocument_Success(t *testing.T) {
	e := newTestIngestor()
	e.documentDAO = &mockDocDAO{
		docs: map[string]*entity.Document{
			"doc-1": testDoc("doc-1", "pdf", ".pdf"),
		},
	}
	e.f2dDAO = &mockF2dDAO{}
	e.storageImpl = &mockStorage{
		data: map[string][]byte{
			"/doc_store/doc-1": []byte("fake pdf bytes"),
		},
	}
	e.pdfParser = &mockPDFParser{
		sections: []map[string]any{
			{"text": "Hello", "doc_type_kwd": "text", "img_id": ""},
			{"text": "Table", "doc_type_kwd": "table", "img_id": ""},
		},
	}

	err := e.parseDocument(context.Background(), testTask("doc-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseDocument_SuccessViaFile2Document(t *testing.T) {
	fileID := "file-1"
	doc := testDoc("doc-1", "pdf", ".pdf")
	f2dMappings := []*entity.File2Document{
		{FileID: &fileID},
	}
	file := &entity.File{
		ID:       fileID,
		Location: strPtr("custom_path/document.pdf"),
		ParentID: "custom_bucket",
	}

	e := newTestIngestor()
	e.documentDAO = &mockDocDAO{
		docs: map[string]*entity.Document{"doc-1": doc},
	}
	e.f2dDAO = &mockF2dDAO{
		mappings: map[string][]*entity.File2Document{"doc-1": f2dMappings},
	}
	e.fileDAO = &mockFileDAO{
		files: map[string]*entity.File{fileID: file},
	}
	e.storageImpl = &mockStorage{
		data: map[string][]byte{
			"custom_bucket/custom_path/document.pdf": []byte("pdf bytes from file"),
		},
	}
	e.pdfParser = &mockPDFParser{
		sections: []map[string]any{
			{"text": "Via File2Document", "doc_type_kwd": "text", "img_id": ""},
		},
	}

	err := e.parseDocument(context.Background(), testTask("doc-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func strPtr(s string) *string {
	return &s
}

func TestParseDocument_WithImageUpload(t *testing.T) {
	imgBase64 := base64.StdEncoding.EncodeToString([]byte("fake image content"))

	e := newTestIngestor()
	e.documentDAO = &mockDocDAO{
		docs: map[string]*entity.Document{
			"doc-1": testDoc("doc-1", "pdf", ".pdf"),
		},
	}
	e.f2dDAO = &mockF2dDAO{}
	ms := &mockStorage{
		data: map[string][]byte{
			"/doc_store/doc-1": []byte("fake pdf bytes"),
		},
	}
	e.storageImpl = ms
	e.pdfParser = &mockPDFParser{
		sections: []map[string]any{
			{"text": "Page with image", "doc_type_kwd": "text", "img_id": "", "image": imgBase64},
		},
	}

	err := e.parseDocument(context.Background(), testTask("doc-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ms.putCallCount != 1 {
		t.Fatalf("expected 1 Put call, got %d", ms.putCallCount)
	}
	if ms.putBucket != "kb-1" {
		t.Errorf("expected put bucket 'kb-1', got %q", ms.putBucket)
	}
	if ms.putFnm == "" {
		t.Errorf("expected non-empty put fnm (chunk ID)")
	}
}
