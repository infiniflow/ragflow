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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
)

type fakeUploadStorage struct {
	objects map[string][]byte
}

func newFakeUploadStorage() *fakeUploadStorage {
	return &fakeUploadStorage{objects: map[string][]byte{}}
}

func (f *fakeUploadStorage) Health() bool                  { return true }
func (f *fakeUploadStorage) key(bucket, fnm string) string { return bucket + "/" + fnm }
func (f *fakeUploadStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	f.objects[f.key(bucket, fnm)] = append([]byte(nil), binary...)
	return nil
}
func (f *fakeUploadStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	v, ok := f.objects[f.key(bucket, fnm)]
	if !ok {
		return nil, errors.New("not found")
	}
	return append([]byte(nil), v...), nil
}
func (f *fakeUploadStorage) Remove(bucket, fnm string, tenantID ...string) error {
	delete(f.objects, f.key(bucket, fnm))
	return nil
}
func (f *fakeUploadStorage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	_, ok := f.objects[f.key(bucket, fnm)]
	return ok
}
func (f *fakeUploadStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	return "", nil
}
func (f *fakeUploadStorage) BucketExists(bucket string) bool  { return true }
func (f *fakeUploadStorage) RemoveBucket(bucket string) error { return nil }
func (f *fakeUploadStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool {
	v, ok := f.objects[f.key(srcBucket, srcPath)]
	if !ok {
		return false
	}
	f.objects[f.key(destBucket, destPath)] = append([]byte(nil), v...)
	return true
}
func (f *fakeUploadStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	if !f.Copy(srcBucket, srcPath, destBucket, destPath) {
		return false
	}
	delete(f.objects, f.key(srcBucket, srcPath))
	return true
}

type fakeChatDocEngine struct{}

func (fakeChatDocEngine) CreateChunkStore(context.Context, string, string, int, string) error {
	return nil
}
func (fakeChatDocEngine) InsertChunks(context.Context, []map[string]interface{}, string, string) ([]string, error) {
	return nil, nil
}
func (fakeChatDocEngine) UpdateChunks(context.Context, map[string]interface{}, map[string]interface{}, string, string) error {
	return nil
}
func (fakeChatDocEngine) DeleteChunks(context.Context, map[string]interface{}, string, string) (int64, error) {
	return 0, nil
}
func (fakeChatDocEngine) Search(context.Context, *types.SearchRequest) (*types.SearchResult, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetChunk(context.Context, string, string, []string) (interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) DropChunkStore(context.Context, string, string) error {
	return nil
}
func (fakeChatDocEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return false, nil
}
func (fakeChatDocEngine) CreateMetadataStore(context.Context, string) error {
	return nil
}
func (fakeChatDocEngine) InsertMetadata(context.Context, []map[string]interface{}, string) ([]string, error) {
	return nil, nil
}
func (fakeChatDocEngine) UpdateMetadata(context.Context, string, string, map[string]interface{}, string) error {
	return nil
}
func (fakeChatDocEngine) DeleteMetadata(context.Context, map[string]interface{}, string) (int64, error) {
	return 0, nil
}
func (fakeChatDocEngine) DeleteMetadataKeys(context.Context, string, string, []string, string) error {
	return nil
}
func (fakeChatDocEngine) DropMetadataStore(context.Context, string) error {
	return nil
}
func (fakeChatDocEngine) MetadataStoreExists(context.Context, string) (bool, error) {
	return false, nil
}
func (fakeChatDocEngine) SearchMetadata(context.Context, *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	return nil, nil
}
func (fakeChatDocEngine) IndexDocument(context.Context, string, string, interface{}) error {
	return nil
}
func (fakeChatDocEngine) DeleteDocument(context.Context, string, string) error {
	return nil
}
func (fakeChatDocEngine) BulkIndex(context.Context, string, []interface{}) (interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetFields([]map[string]interface{}, []string) map[string]map[string]interface{} {
	return nil
}
func (fakeChatDocEngine) GetAggregation([]map[string]interface{}, string) []map[string]interface{} {
	return nil
}
func (fakeChatDocEngine) GetHighlight([]map[string]interface{}, []string, string) map[string]string {
	return nil
}
func (fakeChatDocEngine) RunSQL(context.Context, string, string, []string, string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetChunkIDs([]map[string]interface{}) []string {
	return nil
}
func (fakeChatDocEngine) KNNScores(context.Context, []map[string]interface{}, []float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetScores(map[string]interface{}) map[string]float64 {
	return nil
}
func (fakeChatDocEngine) Ping(context.Context) error {
	return nil
}
func (fakeChatDocEngine) Close() error {
	return nil
}
func (fakeChatDocEngine) GetType() string {
	return "fake"
}
func (fakeChatDocEngine) FilterDocIdsByMetaPushdown(context.Context, []string, []map[string]interface{}, string) []string {
	return nil
}

type failingDeleteMetadataEngine struct {
	fakeChatDocEngine
	deleteErr    error
	updateCalled bool
}

type rerunDeleteDocEngine struct {
	fakeChatDocEngine
	deleteCalls int
	condition   map[string]interface{}
	indexName   string
	datasetID   string
}

func (e *rerunDeleteDocEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return true, nil
}

func (e *rerunDeleteDocEngine) DeleteChunks(_ context.Context, condition map[string]interface{}, indexName string, datasetID string) (int64, error) {
	e.deleteCalls++
	e.condition = condition
	e.indexName = indexName
	e.datasetID = datasetID
	return 3, nil
}

type metadataDocEngine struct {
	fakeChatDocEngine
	records map[string]map[string]interface{}
	docKBs  map[string]string
}

func newMetadataDocEngine(records map[string]map[string]interface{}, docKBs map[string]string) *metadataDocEngine {
	cp := make(map[string]map[string]interface{}, len(records))
	for id, meta := range records {
		dup := make(map[string]interface{}, len(meta))
		for k, v := range meta {
			dup[k] = v
		}
		cp[id] = dup
	}
	return &metadataDocEngine{records: cp, docKBs: docKBs}
}

func (m *metadataDocEngine) SearchMetadata(_ context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	var ids map[string]struct{}
	if rawIDs, ok := req.Filter["id"]; ok && rawIDs != nil {
		ids = make(map[string]struct{})
		switch typed := rawIDs.(type) {
		case []string:
			for _, id := range typed {
				ids[id] = struct{}{}
			}
		case []interface{}:
			for _, id := range typed {
				if s, ok := id.(string); ok {
					ids[s] = struct{}{}
				}
			}
		}
	}

	var kbFilter map[string]struct{}
	if rawKB, ok := req.Filter["kb_id"]; ok && rawKB != nil {
		kbFilter = make(map[string]struct{})
		switch typed := rawKB.(type) {
		case string:
			kbFilter[typed] = struct{}{}
		case []string:
			for _, kb := range typed {
				kbFilter[kb] = struct{}{}
			}
		case []interface{}:
			for _, kb := range typed {
				if s, ok := kb.(string); ok {
					kbFilter[s] = struct{}{}
				}
			}
		}
	}

	result := &types.SearchMetadataResult{MetadataRecords: []map[string]interface{}{}}
	for docID, meta := range m.records {
		if ids != nil {
			if _, ok := ids[docID]; !ok {
				continue
			}
		}
		kbID := m.docKBs[docID]
		if kbFilter != nil {
			if _, ok := kbFilter[kbID]; !ok {
				continue
			}
		}
		result.MetadataRecords = append(result.MetadataRecords, map[string]interface{}{
			"id":          docID,
			"kb_id":       kbID,
			"meta_fields": meta,
		})
	}
	return result, nil
}

func (m *metadataDocEngine) UpdateMetadata(_ context.Context, docID string, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	dup := make(map[string]interface{}, len(m.records[docID])+len(metaFields))
	for k, v := range m.records[docID] {
		dup[k] = v
	}
	for k, v := range metaFields {
		dup[k] = v
	}
	m.records[docID] = dup
	if _, ok := m.docKBs[docID]; !ok {
		m.docKBs[docID] = datasetID
	}
	return nil
}

func (m *metadataDocEngine) InsertMetadata(_ context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	for _, doc := range metadata {
		docID, _ := doc["id"].(string)
		kbID, _ := doc["kb_id"].(string)
		metaFields, _ := doc["meta_fields"].(map[string]interface{})
		if docID == "" || kbID == "" {
			continue
		}
		dup := make(map[string]interface{}, len(metaFields))
		for k, v := range metaFields {
			dup[k] = v
		}
		m.records[docID] = dup
		m.docKBs[docID] = kbID
	}
	return []string{}, nil
}

func (m *metadataDocEngine) DeleteMetadata(_ context.Context, condition map[string]interface{}, tenantID string) (int64, error) {
	docID, _ := condition["id"].(string)
	if docID == "" {
		return 0, nil
	}
	if _, ok := m.records[docID]; ok {
		delete(m.records, docID)
		return 1, nil
	}
	return 0, nil
}

func (m *metadataDocEngine) DeleteMetadataKeys(_ context.Context, docID string, datasetID string, keys []string, tenantID string) error {
	meta, ok := m.records[docID]
	if !ok {
		return nil
	}
	for _, key := range keys {
		delete(meta, key)
	}
	if len(meta) == 0 {
		delete(m.records, docID)
		return nil
	}
	m.records[docID] = meta
	if _, ok := m.docKBs[docID]; !ok {
		m.docKBs[docID] = datasetID
	}
	return nil
}

type staleSearchMetadataDocEngine struct {
	*metadataDocEngine
}

func (m *staleSearchMetadataDocEngine) SearchMetadata(context.Context, *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	return &types.SearchMetadataResult{MetadataRecords: []map[string]interface{}{}}, nil
}

func (f *failingDeleteMetadataEngine) DeleteMetadata(ctx context.Context, condition map[string]interface{}, tenantID string) (int64, error) {
	return 0, f.deleteErr
}

func (f *failingDeleteMetadataEngine) UpdateMetadata(ctx context.Context, docID string, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	f.updateCalled = true
	return nil
}

// setupServiceTestDB initializes an in-memory SQLite database for service tests.
func setupServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	// Migrate tables used by deleteDocumentFull + DeleteDocuments
	if err := db.AutoMigrate(
		&entity.Document{},
		&entity.Knowledgebase{},
		&entity.Task{},
		&entity.File2Document{},
		&entity.File{},
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

// pushServiceDB swaps dao.DB for the test and restores after.
func pushServiceDB(t *testing.T, testDB *gorm.DB) {
	t.Helper()
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() {
		dao.DB = orig
	})
}

func testDocumentService(t *testing.T) *DocumentService {
	t.Helper()
	// Use nil engine since we test DB cleanup only; engine ops are nil-guarded.
	return &DocumentService{
		documentDAO:      dao.NewDocumentDAO(),
		kbDAO:            dao.NewKnowledgebaseDAO(),
		taskDAO:          dao.NewTaskDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
		fileDAO:          dao.NewFileDAO(),
		docEngine:        nil,
		metadataSvc:      nil, // nil engine → metadata ops skipped
	}
}

func makeTestFileHeader(t *testing.T, field, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(int64(len(content) + 1024)); err != nil {
		t.Fatalf("parse multipart form: %v", err)
	}
	fhs := req.MultipartForm.File[field]
	if len(fhs) != 1 {
		t.Fatalf("expected 1 file header, got %d", len(fhs))
	}
	return fhs[0]
}

// sptr returns a pointer to the given string.
func sptr(s string) *string { return &s }

func insertTestKB(t *testing.T, id, tenantID string, docNum, tokenNum, chunkNum int64) {
	t.Helper()
	kb := &entity.Knowledgebase{
		ID:         id,
		TenantID:   tenantID,
		Name:       "test-kb",
		EmbdID:     "embd-1",
		CreatedBy:  "user-1",
		Permission: string(entity.TenantPermissionTeam),
		DocNum:     docNum,
		TokenNum:   tokenNum,
		ChunkNum:   chunkNum,
		Status:     sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert test kb: %v", err)
	}
}

func assertKBDocNum(t *testing.T, kbID string, want int64) {
	t.Helper()
	var got int64
	if err := dao.DB.Model(&entity.Knowledgebase{}).Select("doc_num").Where("id = ?", kbID).Scan(&got).Error; err != nil {
		t.Fatalf("get kb doc_num %s: %v", kbID, err)
	}
	if got != want {
		t.Fatalf("kb %s doc_num=%d, want %d", kbID, got, want)
	}
}

func insertTestDoc(t *testing.T, id, kbID string, tokenNum, chunkNum int64) {
	t.Helper()
	doc := &entity.Document{
		ID:           id,
		KbID:         kbID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		TokenNum:     tokenNum,
		ChunkNum:     chunkNum,
		Suffix:       ".txt",
		Status:       sptr("1"),
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("insert test doc: %v", err)
	}
}

func insertTestTask(t *testing.T, id, docID string) {
	t.Helper()
	task := &entity.Task{
		ID:    id,
		DocID: docID,
	}
	if err := dao.DB.Create(task).Error; err != nil {
		t.Fatalf("insert test task: %v", err)
	}
}

func insertTestFile2Document(t *testing.T, id, fileID, docID string) {
	t.Helper()
	f2d := &entity.File2Document{
		ID:         id,
		FileID:     &fileID,
		DocumentID: &docID,
	}
	if err := dao.DB.Create(f2d).Error; err != nil {
		t.Fatalf("insert test f2d: %v", err)
	}
}

func insertTestFile(t *testing.T, id, parentID, name string, location *string) {
	t.Helper()
	srcType := string(entity.FileSourceKnowledgebase)
	f := &entity.File{
		ID:         id,
		ParentID:   parentID,
		TenantID:   "tenant-1",
		CreatedBy:  "user-1",
		Name:       name,
		Location:   location,
		SourceType: srcType,
		Type:       "pdf",
	}
	if err := dao.DB.Create(f).Error; err != nil {
		t.Fatalf("insert test file: %v", err)
	}
}

func TestCreateDocumentIncrementsKBDocNum(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-create", "tenant-1", 0, 0, 0)

	svc := testDocumentService(t)
	doc, err := svc.CreateDocument(&CreateDocumentRequest{
		Name:      "created.txt",
		KbID:      "kb-create",
		ParserID:  "naive",
		CreatedBy: "tenant-1",
		Type:      "doc",
		Source:    "local",
	})
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	if doc == nil || doc.KbID != "kb-create" {
		t.Fatalf("unexpected doc: %+v", doc)
	}
	assertKBDocNum(t, "kb-create", 1)
}

func TestDeleteDocumentFull_Basic(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 3, 100, 50)
	insertTestDoc(t, "doc-1", "kb-1", 30, 10)
	insertTestTask(t, "task-1", "doc-1")

	svc := testDocumentService(t)

	err := svc.deleteDocumentFull("doc-1")
	if err != nil {
		t.Fatalf("deleteDocumentFull failed: %v", err)
	}

	// Verify document deleted
	_, err = dao.NewDocumentDAO().GetByID("doc-1")
	if err == nil {
		t.Fatal("document should be deleted but it still exists")
	}

	// Verify tasks deleted
	tasks, _ := dao.NewTaskDAO().GetAllTasks()
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}

	// Verify KB counters decremented
	kb, err := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if err != nil {
		t.Fatalf("kb not found: %v", err)
	}
	if kb.DocNum != 2 {
		t.Fatalf("doc_num: expected 2, got %d", kb.DocNum)
	}
	if kb.TokenNum != 70 {
		t.Fatalf("token_num: expected 70, got %d", kb.TokenNum)
	}
	if kb.ChunkNum != 40 {
		t.Fatalf("chunk_num: expected 40, got %d", kb.ChunkNum)
	}
}

func TestDeleteDocumentFull_NotFound(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	svc := testDocumentService(t)

	err := svc.deleteDocumentFull("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent document")
	}
}

func TestDeleteDocumentFull_CleansUpFile2Document(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDoc(t, "doc-1", "kb-1", 10, 5)
	loc := "path/to/blob"
	insertTestFile(t, "file-1", "kb-1", "test.pdf", &loc)
	insertTestFile2Document(t, "f2d-1", "file-1", "doc-1")

	svc := testDocumentService(t)

	err := svc.deleteDocumentFull("doc-1")
	if err != nil {
		t.Fatalf("deleteDocumentFull failed: %v", err)
	}

	// Verify f2d mapping deleted
	f2dDAO := dao.NewFile2DocumentDAO()
	mappings, _ := f2dDAO.GetByDocumentID("doc-1")
	if len(mappings) != 0 {
		t.Fatalf("expected 0 f2d mappings, got %d", len(mappings))
	}

	// Verify file record deleted (hard delete)
	files, _ := dao.NewFileDAO().GetByIDs([]string{"file-1"})
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestDeleteDocumentFull_SharedFilePreserved(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 2, 20, 10)
	insertTestDoc(t, "doc-1", "kb-1", 10, 5)
	insertTestDoc(t, "doc-2", "kb-1", 10, 5)
	loc := "shared/blob"
	insertTestFile(t, "file-shared", "kb-1", "shared.pdf", &loc)

	// Same file linked to TWO documents
	insertTestFile2Document(t, "f2d-1", "file-shared", "doc-1")
	insertTestFile2Document(t, "f2d-2", "file-shared", "doc-2")

	svc := testDocumentService(t)

	// Delete doc-1; file-shared should survive because doc-2 still references it
	err := svc.deleteDocumentFull("doc-1")
	if err != nil {
		t.Fatalf("deleteDocumentFull failed: %v", err)
	}

	// f2d mapping for doc-1 should be gone
	f2dDAO := dao.NewFile2DocumentDAO()
	mappings, _ := f2dDAO.GetByDocumentID("doc-1")
	if len(mappings) != 0 {
		t.Fatalf("expected 0 f2d mappings for doc-1, got %d", len(mappings))
	}

	// file record should still exist (doc-2 still references it)
	files, _ := dao.NewFileDAO().GetByIDs([]string{"file-shared"})
	if len(files) != 1 {
		t.Fatalf("expected 1 file record to survive, got %d", len(files))
	}

	// f2d mapping for doc-2 should still exist
	mappings, _ = f2dDAO.GetByDocumentID("doc-2")
	if len(mappings) != 1 {
		t.Fatalf("expected 1 f2d mapping for doc-2, got %d", len(mappings))
	}
}

func TestSelectUploadParser_MirrorsPython(t *testing.T) {
	tests := []struct {
		name         string
		docType      utility.FileType
		filename     string
		defaultValue string
		want         string
	}{
		{name: "visual", docType: utility.FileTypeVISUAL, filename: "img.png", defaultValue: "naive", want: "picture"},
		{name: "aural", docType: utility.FileTypeAURAL, filename: "audio.mp3", defaultValue: "naive", want: "audio"},
		{name: "presentation by ext", docType: utility.FileTypeDOC, filename: "deck.pptx", defaultValue: "naive", want: "presentation"},
		{name: "email by ext", docType: utility.FileTypeDOC, filename: "mail.eml", defaultValue: "naive", want: "email"},
		{name: "fallback default", docType: utility.FileTypeDOC, filename: "notes.txt", defaultValue: "manual", want: "manual"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectUploadParser(tt.docType, tt.filename, tt.defaultValue); got != tt.want {
				t.Fatalf("selectUploadParser(%q)=%q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestContentHashHex_MatchesPythonXXH128(t *testing.T) {
	tests := []struct {
		data []byte
		want string
	}{
		{data: []byte("abc"), want: "06b05ab6733a618578af5f94892f3950"},
		{data: []byte(""), want: "99aa06d3014798d86001c324468d497f"},
	}
	for _, tt := range tests {
		if got := contentHashHex(tt.data); got != tt.want {
			t.Fatalf("contentHashHex(%q)=%s, want %s", tt.data, got, tt.want)
		}
	}
}

func TestUploadLocalDocuments_MirrorsPythonCoreFields(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	mockStorage := newFakeUploadStorage()
	factory := storage.GetStorageFactory()
	origStorage := factory.GetStorage()
	factory.SetStorage(mockStorage)
	t.Cleanup(func() { factory.SetStorage(origStorage) })

	pipelineID := "pipe-1"
	kb := &entity.Knowledgebase{
		ID:         "kb-upload",
		TenantID:   "tenant-1",
		Name:       "kb-upload",
		ParserID:   "naive",
		PipelineID: &pipelineID,
		ParserConfig: entity.JSONMap{
			"existing": "value",
		},
		DocNum: 1,
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
	if err := dao.DB.Create(&entity.Document{
		ID:           "doc-existing",
		KbID:         kb.ID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		Name:         sptr("deck.pptx"),
		Status:       sptr("1"),
	}).Error; err != nil {
		t.Fatalf("insert existing doc: %v", err)
	}

	svc := testDocumentService(t)
	fh := makeTestFileHeader(t, "file", "deck.pptx", []byte("abc"))
	got, errs := svc.UploadLocalDocuments(kb, "user-1", []*multipart.FileHeader{fh}, "nested/path", map[string]interface{}{
		"table_column_mode": "assist",
	})
	if len(errs) != 0 {
		t.Fatalf("unexpected errs: %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 uploaded doc, got %d", len(got))
	}
	doc := got[0]
	if doc["name"] != "deck(1).pptx" {
		t.Fatalf("name=%v, want deck(1).pptx", doc["name"])
	}
	if doc["location"] != "nested/path/deck(1).pptx" {
		t.Fatalf("location=%v, want nested/path/deck(1).pptx", doc["location"])
	}
	if doc["parser_id"] != "presentation" {
		t.Fatalf("parser_id=%v, want presentation", doc["parser_id"])
	}
	if doc["content_hash"] != "06b05ab6733a618578af5f94892f3950" {
		t.Fatalf("content_hash=%v", doc["content_hash"])
	}
	cfg := doc["parser_config"].(map[string]interface{})
	if cfg["existing"] != "value" || cfg["table_column_mode"] != "assist" {
		t.Fatalf("parser_config=%v", cfg)
	}

	storedBlob, err := mockStorage.Get(kb.ID, "nested/path/deck(1).pptx")
	if err != nil {
		t.Fatalf("blob not stored: %v", err)
	}
	if string(storedBlob) != "abc" {
		t.Fatalf("stored blob=%q, want abc", storedBlob)
	}
	assertKBDocNum(t, kb.ID, 2)
}

func TestUploadEmptyDocument_CreatesVirtualDocumentAndFileLink(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	pipelineID := "pipe-2"
	kb := &entity.Knowledgebase{
		ID:         "kb-empty",
		TenantID:   "tenant-1",
		Name:       "kb-empty",
		ParserID:   "manual",
		PipelineID: &pipelineID,
		ParserConfig: entity.JSONMap{
			"foo": "bar",
		},
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}

	svc := testDocumentService(t)
	got, code, err := svc.UploadEmptyDocument(kb, "user-1", "draft.md")
	if err != nil {
		t.Fatalf("UploadEmptyDocument error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code=%v, want success", code)
	}
	if got["type"] != "virtual" || got["parser_id"] != "manual" || got["size"] != int64(0) {
		t.Fatalf("unexpected doc map: %v", got)
	}

	var docCount int64
	if err := dao.DB.Model(&entity.Document{}).Where("kb_id = ?", kb.ID).Count(&docCount).Error; err != nil {
		t.Fatalf("count docs: %v", err)
	}
	if docCount != 1 {
		t.Fatalf("doc count=%d, want 1", docCount)
	}
	var linkCount int64
	if err := dao.DB.Model(&entity.File2Document{}).Count(&linkCount).Error; err != nil {
		t.Fatalf("count links: %v", err)
	}
	if linkCount != 1 {
		t.Fatalf("link count=%d, want 1", linkCount)
	}
	assertKBDocNum(t, kb.ID, 1)
}

func insertUserTenantForAccessCheck(t *testing.T, userID, tenantID string) {
	t.Helper()
	// Insert user if not exists (email is NOT NULL, password is nullable pointer)
	var existingUser entity.User
	if err := dao.DB.Where("id = ?", userID).First(&existingUser).Error; err != nil {
		u := &entity.User{ID: userID, Nickname: "test-user", Email: userID + "@test.com", Password: sptr("x")}
		if err := dao.DB.Create(u).Error; err != nil {
			t.Fatalf("insert test user: %v", err)
		}
	}
	// Insert tenant if not exists (llm_id, embd_id, asr_id are NOT NULL)
	var existingTenant entity.Tenant
	if err := dao.DB.Where("id = ?", tenantID).First(&existingTenant).Error; err != nil {
		tn := &entity.Tenant{
			ID:     tenantID,
			LLMID:  "llm-default",
			EmbdID: "embd-default",
			ASRID:  "asr-default",
		}
		if err := dao.DB.Create(tn).Error; err != nil {
			t.Fatalf("insert test tenant: %v", err)
		}
	}
	// Insert user_tenant mapping if not exists
	var existingUT entity.UserTenant
	if err := dao.DB.Where("user_id = ? AND tenant_id = ?", userID, tenantID).First(&existingUT).Error; err != nil {
		ut := &entity.UserTenant{
			ID:       userID + "_" + tenantID,
			UserID:   userID,
			TenantID: tenantID,
			Role:     "admin",
		}
		if err := dao.DB.Create(ut).Error; err != nil {
			t.Fatalf("insert test user_tenant: %v", err)
		}
	}
}

func TestDeleteDocuments_DeleteAll(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")

	insertTestKB(t, "kb-1", "tenant-1", 3, 100, 50)
	insertTestDoc(t, "doc-1", "kb-1", 30, 10)
	insertTestDoc(t, "doc-2", "kb-1", 40, 20)
	insertTestDoc(t, "doc-3", "kb-1", 30, 20)

	svc := testDocumentService(t)

	deleted, err := svc.DeleteDocuments(nil, true, "kb-1", "user-1")
	if err != nil {
		t.Fatalf("DeleteDocuments failed: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("expected 3 deleted, got %d", deleted)
	}

	// KB counters: doc_num 3→0, token_num 100→0, chunk_num 50→0
	kb, _ := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if kb.DocNum != 0 {
		t.Fatalf("doc_num: expected 0, got %d", kb.DocNum)
	}
}

func TestDeleteDocuments_ByIDs(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")

	insertTestKB(t, "kb-1", "tenant-1", 3, 100, 50)
	insertTestDoc(t, "doc-1", "kb-1", 30, 10)
	insertTestDoc(t, "doc-2", "kb-1", 40, 20)
	insertTestDoc(t, "doc-3", "kb-1", 30, 20) // won't be deleted

	svc := testDocumentService(t)

	deleted, err := svc.DeleteDocuments([]string{"doc-1", "doc-2"}, false, "kb-1", "user-1")
	if err != nil {
		t.Fatalf("DeleteDocuments failed: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deleted, got %d", deleted)
	}

	// doc-3 should still exist
	_, err = dao.NewDocumentDAO().GetByID("doc-3")
	if err != nil {
		t.Fatal("doc-3 should not have been deleted")
	}
}

func TestDeleteDocuments_WrongDataset(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")
	insertUserTenantForAccessCheck(t, "user-1", "tenant-2")

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestKB(t, "kb-2", "tenant-2", 1, 10, 5)
	insertTestDoc(t, "doc-1", "kb-2", 10, 5) // belongs to kb-2, not kb-1

	svc := testDocumentService(t)

	_, err := svc.DeleteDocuments([]string{"doc-1"}, false, "kb-1", "user-1")
	if err == nil {
		t.Fatal("expected error for doc not belonging to dataset")
	}
}

func TestDeleteDocuments_NotAccessible(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)

	svc := testDocumentService(t)

	// user-1 has no user_tenant entry → accessible returns false
	_, err := svc.DeleteDocuments([]string{"doc-1"}, false, "kb-1", "user-1")
	if err == nil {
		t.Fatal("expected error for inaccessible dataset")
	}
}

func TestDeleteDocuments_EmptyIDs(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")
	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)

	svc := testDocumentService(t)

	// Empty ids, no deleteAll → returns 0, no error
	deleted, err := svc.DeleteDocuments([]string{}, false, "kb-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}
}

func TestDeleteDocuments_Deduplicate(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertUserTenantForAccessCheck(t, "user-1", "tenant-1")

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDoc(t, "doc-1", "kb-1", 10, 5)

	svc := testDocumentService(t)

	deleted, err := svc.DeleteDocuments([]string{"doc-1", "doc-1", "doc-1"}, false, "kb-1", "user-1")
	if err != nil {
		t.Fatalf("DeleteDocuments failed: %v", err)
	}
	// Dedup should result in only 1 delete
	if deleted != 1 {
		t.Fatalf("expected 1 deleted after dedup, got %d", deleted)
	}
}

// insertTestDocWithRun inserts a document with the given Run status for StopParseDocuments tests.
func insertTestDocWithRun(t *testing.T, id, kbID, run string, tokenNum, chunkNum int64) {
	t.Helper()
	doc := &entity.Document{
		ID:           id,
		KbID:         kbID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		TokenNum:     tokenNum,
		ChunkNum:     chunkNum,
		Suffix:       ".txt",
		Status:       sptr("1"),
		Run:          &run,
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("insert test doc: %v", err)
	}
}

// insertTestTaskWithProgress inserts a task with the given progress value.
func insertTestTaskWithProgress(t *testing.T, id, docID string, progress float64) {
	t.Helper()
	task := &entity.Task{
		ID:       id,
		DocID:    docID,
		Progress: progress,
	}
	if err := dao.DB.Create(task).Error; err != nil {
		t.Fatalf("insert test task: %v", err)
	}
}

func TestStopParseDocuments_Success(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusRunning), 10, 5)
	insertTestTask(t, "task-1", "doc-1")

	svc := testDocumentService(t)

	result, err := svc.StopParseDocuments("kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("StopParseDocuments failed: %v", err)
	}

	sc, ok := result["success_count"].(int)
	if !ok {
		t.Fatalf("success_count not found or wrong type: %v", result)
	}
	if sc != 1 {
		t.Fatalf("expected success_count=1, got %d", sc)
	}

	// Verify document run status updated to CANCEL
	doc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	if doc == nil || doc.Run == nil {
		t.Fatal("doc not found or run is nil")
	}
	if *doc.Run != string(entity.TaskStatusCancel) {
		t.Fatalf("expected run=%q, got %q", string(entity.TaskStatusCancel), *doc.Run)
	}
}

func TestStopParseDocuments_CancelStatus(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	// Doc is already in CANCEL state — should still be accepted
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusCancel), 10, 5)
	insertTestTask(t, "task-1", "doc-1")

	svc := testDocumentService(t)

	result, err := svc.StopParseDocuments("kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("StopParseDocuments failed: %v", err)
	}

	sc := result["success_count"].(int)
	if sc != 1 {
		t.Fatalf("expected success_count=1, got %d", sc)
	}
}

func TestStopParseDocuments_NotRunningOrCancel(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	// Doc with Run="0" (UNSTART) and no unfinished tasks → cannot cancel
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusUnstart), 10, 5)

	svc := testDocumentService(t)

	result, err := svc.StopParseDocuments("kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("StopParseDocuments failed: %v", err)
	}

	sc := result["success_count"].(int)
	if sc != 0 {
		t.Fatalf("expected success_count=0, got %d", sc)
	}
	errors, ok := result["errors"].([]string)
	if !ok || len(errors) == 0 {
		t.Fatal("expected errors in result")
	}
}

func TestStopParseDocuments_UnfinishedTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	// Doc with Run="0" but has an unfinished task (progress < 1) → can cancel
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusUnstart), 10, 5)
	insertTestTaskWithProgress(t, "task-1", "doc-1", 0.0)

	svc := testDocumentService(t)

	result, err := svc.StopParseDocuments("kb-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("StopParseDocuments failed: %v", err)
	}

	sc := result["success_count"].(int)
	if sc != 1 {
		t.Fatalf("expected success_count=1 (has unfinished task), got %d", sc)
	}
}

func TestStopParseDocuments_WrongDataset(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestKB(t, "kb-2", "tenant-1", 1, 10, 5)
	insertTestDocWithRun(t, "doc-1", "kb-2", string(entity.TaskStatusRunning), 10, 5)

	svc := testDocumentService(t)

	_, err := svc.StopParseDocuments("kb-1", []string{"doc-1"})
	if err == nil {
		t.Fatal("expected error for doc not belonging to dataset")
	}
}

func TestStopParseDocuments_NotFound(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)

	svc := testDocumentService(t)

	_, err := svc.StopParseDocuments("kb-1", []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent document IDs")
	}
}

func TestStopParseDocuments_EmptyIDs(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)

	svc := testDocumentService(t)

	_, err := svc.StopParseDocuments("kb-1", []string{})
	if err == nil {
		t.Fatal("expected error for empty doc IDs")
	}
}

func TestStopParseDocuments_Deduplicate(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusRunning), 10, 5)
	insertTestTask(t, "task-1", "doc-1")

	svc := testDocumentService(t)

	result, err := svc.StopParseDocuments("kb-1", []string{"doc-1", "doc-1", "doc-1"})
	if err != nil {
		t.Fatalf("StopParseDocuments failed: %v", err)
	}

	// Dedup should result in only 1 success
	sc := result["success_count"].(int)
	if sc != 1 {
		t.Fatalf("expected success_count=1 after dedup, got %d", sc)
	}
}

func TestDeleteDocument_DeligatesToFullCleanup(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 5, 2)
	insertTestDoc(t, "doc-1", "kb-1", 5, 2)

	svc := testDocumentService(t)

	// Public DeleteDocument should delegate to deleteDocumentFull
	err := svc.DeleteDocument("doc-1")
	if err != nil {
		t.Fatalf("DeleteDocument failed: %v", err)
	}

	_, err = dao.NewDocumentDAO().GetByID("doc-1")
	if err == nil {
		t.Fatal("document should be deleted")
	}
}

// --- Sub-method tests ---

func TestResolveDocAndKB_Success(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDoc(t, "doc-1", "kb-1", 10, 5)

	svc := testDocumentService(t)

	doc, kb, err := svc.resolveDocAndKB("doc-1")
	if err != nil {
		t.Fatalf("resolveDocAndKB: %v", err)
	}
	if doc.ID != "doc-1" {
		t.Fatalf("doc ID mismatch: %s", doc.ID)
	}
	if kb.ID != "kb-1" {
		t.Fatalf("kb ID mismatch: %s", kb.ID)
	}
	if kb.TenantID != "tenant-1" {
		t.Fatalf("tenant ID mismatch: %s", kb.TenantID)
	}
}

func TestResolveDocAndKB_DocNotFound(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	svc := testDocumentService(t)

	_, _, err := svc.resolveDocAndKB("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent doc")
	}
}

func TestResolveDocAndKB_KBNotFound(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	// Insert a doc with kb_id that has no KB row
	d := &entity.Document{
		ID: "orphan-doc", KbID: "no-such-kb", ParserID: "naive",
		ParserConfig: entity.JSONMap{}, Suffix: ".txt", Status: sptr("1"),
	}
	if err := dao.DB.Create(d).Error; err != nil {
		t.Fatalf("insert doc: %v", err)
	}

	svc := testDocumentService(t)

	_, _, err := svc.resolveDocAndKB("orphan-doc")
	if err == nil {
		t.Fatal("expected error for nonexistent KB")
	}
}

func TestDeleteDocRecordWithCounters_Success(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 3, 100, 50)
	insertTestDoc(t, "doc-1", "kb-1", 30, 10)

	doc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	svc := testDocumentService(t)

	err := svc.deleteDocRecordWithCounters(doc, "kb-1")
	if err != nil {
		t.Fatalf("deleteDocRecordWithCounters: %v", err)
	}

	// Doc gone
	_, err = dao.NewDocumentDAO().GetByID("doc-1")
	if err == nil {
		t.Fatal("document should be deleted")
	}

	// Counters decremented
	kb, _ := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if kb.DocNum != 2 {
		t.Fatalf("doc_num: expected 2, got %d", kb.DocNum)
	}
	if kb.TokenNum != 70 {
		t.Fatalf("token_num: expected 70, got %d", kb.TokenNum)
	}
	if kb.ChunkNum != 40 {
		t.Fatalf("chunk_num: expected 40, got %d", kb.ChunkNum)
	}
}

func TestDeleteDocRecordWithCounters_DocAlreadyDeleted(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDoc(t, "doc-1", "kb-1", 10, 5)

	doc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	svc := testDocumentService(t)

	// First delete: row removed, counters decremented
	if err := svc.deleteDocRecordWithCounters(doc, "kb-1"); err != nil {
		t.Fatalf("first delete: %v", err)
	}

	// Second delete: RowsAffected==0 → counters NOT decremented again
	if err := svc.deleteDocRecordWithCounters(doc, "kb-1"); err != nil {
		t.Fatalf("second delete should not error: %v", err)
	}

	// KB counters should be decremented exactly once: 1→0 for doc_num
	kb, _ := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if kb.DocNum != 0 {
		t.Fatalf("doc_num: expected 0 (decremented once), got %d", kb.DocNum)
	}
	if kb.TokenNum != 0 {
		t.Fatalf("token_num: expected 0, got %d", kb.TokenNum)
	}
	if kb.ChunkNum != 0 {
		t.Fatalf("chunk_num: expected 0, got %d", kb.ChunkNum)
	}
}

func TestDeleteDocRecordWithCounters_KBUpdateFailureRollsBackDocumentDelete(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestDoc(t, "doc-1", "missing-kb", 10, 5)

	doc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	svc := testDocumentService(t)

	if err := svc.deleteDocRecordWithCounters(doc, "missing-kb"); err == nil {
		t.Fatal("expected missing KB counter update to return an error")
	}

	if _, err := dao.NewDocumentDAO().GetByID("doc-1"); err != nil {
		t.Fatalf("expected document delete to roll back, got: %v", err)
	}
}

func TestCleanupFileReferences_NoMappings(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	svc := testDocumentService(t)
	// Should not panic with no f2d mappings
	svc.cleanupFileReferences("no-mappings")
}

func TestCleanupFileReferences_SingleFileDeleted(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	loc := "blob/path"
	insertTestFile(t, "file-1", "kb-1", "test.pdf", &loc)
	insertTestFile2Document(t, "f2d-1", "file-1", "doc-1")

	svc := testDocumentService(t)
	svc.cleanupFileReferences("doc-1")

	// f2d gone
	mappings, _ := dao.NewFile2DocumentDAO().GetByDocumentID("doc-1")
	if len(mappings) != 0 {
		t.Fatalf("expected 0 f2d after cleanup, got %d", len(mappings))
	}
	// file record gone
	files, _ := dao.NewFileDAO().GetByIDs([]string{"file-1"})
	if len(files) != 0 {
		t.Fatalf("expected 0 files after cleanup, got %d", len(files))
	}
}

func TestCleanupFileReferences_SharedFileSurvives(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	loc := "shared/blob"
	insertTestFile(t, "file-shared", "kb-1", "shared.pdf", &loc)
	insertTestFile2Document(t, "f2d-1", "file-shared", "doc-1")
	insertTestFile2Document(t, "f2d-2", "file-shared", "doc-2")

	svc := testDocumentService(t)
	svc.cleanupFileReferences("doc-1")

	// f2d for doc-1 gone
	mappings, _ := dao.NewFile2DocumentDAO().GetByDocumentID("doc-1")
	if len(mappings) != 0 {
		t.Fatalf("expected 0 f2d for doc-1, got %d", len(mappings))
	}
	// file record survives
	files, _ := dao.NewFileDAO().GetByIDs([]string{"file-shared"})
	if len(files) != 1 {
		t.Fatalf("expected 1 file record, got %d", len(files))
	}
	// f2d for doc-2 survives
	mappings, _ = dao.NewFile2DocumentDAO().GetByDocumentID("doc-2")
	if len(mappings) != 1 {
		t.Fatalf("expected 1 f2d for doc-2, got %d", len(mappings))
	}
}

func TestArtifactHelpers(t *testing.T) {
	// Test sanitizeArtifactFilename
	safe := sanitizeArtifactFilename("test@#file.txt")
	if safe != "test__file.txt" {
		t.Errorf("expected test__file.txt, got %s", safe)
	}

	// Test shouldForceArtifactAttachment
	if !shouldForceArtifactAttachment(".html", "text/html") {
		t.Error("expected true for .html")
	}
	if shouldForceArtifactAttachment(".txt", "text/plain") {
		t.Error("expected false for .txt")
	}
}

func TestGetDocumentArtifact_InvalidFilename(t *testing.T) {
	svc := testDocumentService(t)
	_, err := svc.GetDocumentArtifact("../test.txt", "user-1")
	if err != ErrArtifactInvalidFilename {
		t.Errorf("expected ErrArtifactInvalidFilename, got %v", err)
	}
}

func TestGetDocumentArtifact_InvalidFileType(t *testing.T) {
	svc := testDocumentService(t)
	_, err := svc.GetDocumentArtifact("test.exe", "user-1")
	if err != ErrArtifactInvalidFileType {
		t.Errorf("expected ErrArtifactInvalidFileType, got %v", err)
	}
}

func TestGetDocumentPreview_DocumentNotFound(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	svc := testDocumentService(t)

	_, err := svc.GetDocumentPreview("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent document")
	}
}

func TestDownloadDocument_MissingDocID(t *testing.T) {
	svc := testDocumentService(t)
	_, err := svc.DownloadDocument("ds-1", "")
	if err == nil {
		t.Error("expected error for missing docID")
	}
}

func TestDownloadDocument_WrongDataset(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 5, 2)
	insertTestDoc(t, "doc-1", "kb-1", 5, 2)
	svc := testDocumentService(t)

	_, err := svc.DownloadDocument("wrong-ds", "doc-1")
	if err == nil {
		t.Error("expected error for wrong dataset")
	}
}

func TestUpdateDatasetDocumentRejectsNonOwner(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	svc := testDocumentService(t)
	_, code, err := svc.UpdateDatasetDocument("tenant-2", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{}, map[string]bool{})
	if err == nil {
		t.Fatal("expected ownership error")
	}
	if code != common.CodeDataError {
		t.Fatalf("code = %v, want %v", code, common.CodeDataError)
	}
	if err.Error() != "You don't own the dataset." {
		t.Fatalf("err = %q", err.Error())
	}
}

func TestUpdateDatasetDocumentRejectsCounterMutation(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDoc(t, "doc-1", "kb-1", 10, 5)

	chunkCount := int64(6)
	svc := testDocumentService(t)
	_, code, err := svc.UpdateDatasetDocument("tenant-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
		ChunkCount: &chunkCount,
	}, map[string]bool{"chunk_count": true})
	if err == nil {
		t.Fatal("expected chunk_count mutation error")
	}
	if code != common.CodeDataError {
		t.Fatalf("code = %v, want %v", code, common.CodeDataError)
	}
	if err.Error() != "Can't change `chunk_count`." {
		t.Fatalf("err = %q", err.Error())
	}
}

func TestUpdateDatasetDocumentAllowsZeroCounterLikePythonTruthyCheck(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDoc(t, "doc-1", "kb-1", 10, 5)

	chunkCount := int64(0)
	svc := testDocumentService(t)
	_, code, err := svc.UpdateDatasetDocument("tenant-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
		ChunkCount: &chunkCount,
	}, map[string]bool{"chunk_count": true})
	if err != nil {
		t.Fatalf("UpdateDatasetDocument failed: code=%v err=%v", code, err)
	}
}

func TestUpdateDatasetDocumentRejectsUnsupportedParserIDForVisualDoc(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "image.png", 0, 0)
	if err := dao.DB.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("type", "visual").Error; err != nil {
		t.Fatalf("update doc type: %v", err)
	}

	parserID := "naive"
	svc := testDocumentService(t)
	_, code, err := svc.UpdateDatasetDocument("tenant-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
		ParserID: &parserID,
	}, map[string]bool{"parser_id": true})
	if err == nil {
		t.Fatal("expected parser_id visual error")
	}
	if code != common.CodeDataError {
		t.Fatalf("code = %v, want %v", code, common.CodeDataError)
	}
	if err.Error() != "Not supported yet!" {
		t.Fatalf("err = %q", err.Error())
	}
}

func TestUpdateDatasetDocumentRenameUpdatesDocumentAndFile(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "old.pdf", 0, 0)
	insertTestFile(t, "file-1", "folder-1", "old.pdf", sptr("old.pdf"))
	insertTestFile2Document(t, "f2d-1", "file-1", "doc-1")

	newName := "new.pdf"
	svc := testDocumentService(t)
	resp, code, err := svc.UpdateDatasetDocument("tenant-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
		Name: &newName,
	}, map[string]bool{"name": true})
	if err != nil {
		t.Fatalf("UpdateDatasetDocument failed: code=%v err=%v", code, err)
	}
	if resp == nil || resp.Name == nil || *resp.Name != newName {
		t.Fatalf("response name = %#v, want %q", resp, newName)
	}

	doc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	if doc.Name == nil || *doc.Name != newName {
		t.Fatalf("document name = %v, want %q", doc.Name, newName)
	}
	file, _ := dao.NewFileDAO().GetByID("file-1")
	if file.Name != newName {
		t.Fatalf("file name = %q, want %q", file.Name, newName)
	}
}

func TestUpdateDatasetDocumentChunkMethodResetsForReparse(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	chunkMethod := "manual"
	svc := testDocumentService(t)
	resp, code, err := svc.UpdateDatasetDocument("tenant-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
		ChunkMethod: &chunkMethod,
	}, map[string]bool{"chunk_method": true})
	if err != nil {
		t.Fatalf("UpdateDatasetDocument failed: code=%v err=%v", code, err)
	}
	if resp.ChunkMethod != chunkMethod || resp.Run != "UNSTART" || resp.TokenCount != 0 || resp.ChunkCount != 0 {
		t.Fatalf("response = %+v, want method=%s run=UNSTART counts=0", resp, chunkMethod)
	}

	doc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	if doc.ParserID != chunkMethod {
		t.Fatalf("parser_id = %q, want %q", doc.ParserID, chunkMethod)
	}
	if doc.TokenNum != 0 || doc.ChunkNum != 0 || doc.Progress != 0 {
		t.Fatalf("doc counters/progress = token:%d chunk:%d progress:%f, want zero", doc.TokenNum, doc.ChunkNum, doc.Progress)
	}
	kb, _ := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if kb.TokenNum != 0 || kb.ChunkNum != 0 {
		t.Fatalf("kb counters = token:%d chunk:%d, want zero", kb.TokenNum, kb.ChunkNum)
	}
}

func TestUpdateDatasetDocumentParserIDResetsForReparse(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	parserID := "manual"
	svc := testDocumentService(t)
	resp, code, err := svc.UpdateDatasetDocument("tenant-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
		ParserID: &parserID,
	}, map[string]bool{"parser_id": true})
	if err != nil {
		t.Fatalf("UpdateDatasetDocument failed: code=%v err=%v", code, err)
	}
	if resp.ChunkMethod != parserID || resp.Run != "UNSTART" || resp.TokenCount != 0 || resp.ChunkCount != 0 {
		t.Fatalf("response = %+v, want parser_id=%s run=UNSTART counts=0", resp, parserID)
	}

	doc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	if doc.ParserID != parserID {
		t.Fatalf("parser_id = %q, want %q", doc.ParserID, parserID)
	}
	if doc.TokenNum != 0 || doc.ChunkNum != 0 {
		t.Fatalf("doc counters = token:%d chunk:%d, want zero", doc.TokenNum, doc.ChunkNum)
	}
	kb, _ := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if kb.TokenNum != 0 || kb.ChunkNum != 0 {
		t.Fatalf("kb counters = token:%d chunk:%d, want zero", kb.TokenNum, kb.ChunkNum)
	}
}

func TestResetDocumentForReparseSkipsSecondCounterDecrement(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	staleDoc, err := dao.NewDocumentDAO().GetByID("doc-1")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}

	svc := testDocumentService(t)
	parserID := "manual"
	if err := svc.resetDocumentForReparse(staleDoc, "tenant-1", &parserID, nil); err != nil {
		t.Fatalf("first resetDocumentForReparse failed: %v", err)
	}
	if err := svc.resetDocumentForReparse(staleDoc, "tenant-1", &parserID, nil); err != nil {
		t.Fatalf("second resetDocumentForReparse failed: %v", err)
	}

	doc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	if doc.TokenNum != 0 || doc.ChunkNum != 0 {
		t.Fatalf("doc counters = token:%d chunk:%d, want zero", doc.TokenNum, doc.ChunkNum)
	}
	kb, _ := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if kb.TokenNum != 0 || kb.ChunkNum != 0 {
		t.Fatalf("kb counters = token:%d chunk:%d, want zero after duplicate reset", kb.TokenNum, kb.ChunkNum)
	}
}

func TestPrepareDocumentRerunWithDeleteClearsCountersTasksAndChunks(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusDone), 10, 5)
	insertTestTask(t, "task-1", "doc-1")

	doc, err := dao.NewDocumentDAO().GetByID("doc-1")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}

	engine := &rerunDeleteDocEngine{}
	svc := testDocumentService(t)
	svc.docEngine = engine

	if err := svc.prepareDocumentRerunWithDelete(doc, "tenant-1"); err != nil {
		t.Fatalf("prepareDocumentRerunWithDelete failed: %v", err)
	}

	updatedDoc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	if updatedDoc.TokenNum != 0 || updatedDoc.ChunkNum != 0 {
		t.Fatalf("doc counters = token:%d chunk:%d, want zero", updatedDoc.TokenNum, updatedDoc.ChunkNum)
	}
	kb, _ := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if kb.TokenNum != 0 || kb.ChunkNum != 0 {
		t.Fatalf("kb counters = token:%d chunk:%d, want zero", kb.TokenNum, kb.ChunkNum)
	}
	var taskCount int64
	if err := dao.DB.Model(&entity.Task{}).Where("doc_id = ?", "doc-1").Count(&taskCount).Error; err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("task count = %d, want zero", taskCount)
	}
	if engine.deleteCalls != 1 {
		t.Fatalf("deleteCalls = %d, want 1", engine.deleteCalls)
	}
	if engine.indexName != "ragflow_tenant-1" || engine.datasetID != "kb-1" || engine.condition["doc_id"] != "doc-1" {
		t.Fatalf("unexpected delete call: index=%s dataset=%s condition=%v", engine.indexName, engine.datasetID, engine.condition)
	}
}

func TestPrepareDocumentRerunWithDeleteIsIdempotentForStaleDocSnapshot(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusDone), 10, 5)

	staleDoc, err := dao.NewDocumentDAO().GetByID("doc-1")
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}

	svc := testDocumentService(t)
	if err := svc.prepareDocumentRerunWithDelete(staleDoc, "tenant-1"); err != nil {
		t.Fatalf("first prepareDocumentRerunWithDelete failed: %v", err)
	}
	if err := svc.prepareDocumentRerunWithDelete(staleDoc, "tenant-1"); err != nil {
		t.Fatalf("second prepareDocumentRerunWithDelete failed: %v", err)
	}

	doc, _ := dao.NewDocumentDAO().GetByID("doc-1")
	if doc.TokenNum != 0 || doc.ChunkNum != 0 {
		t.Fatalf("doc counters = token:%d chunk:%d, want zero", doc.TokenNum, doc.ChunkNum)
	}
	kb, _ := dao.NewKnowledgebaseDAO().GetByID("kb-1")
	if kb.TokenNum != 0 || kb.ChunkNum != 0 {
		t.Fatalf("kb counters = token:%d chunk:%d, want zero after duplicate prepare", kb.TokenNum, kb.ChunkNum)
	}
}

func TestUpdateDatasetDocumentPropagatesMetadataDeleteFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 0, 0)

	engine := &failingDeleteMetadataEngine{deleteErr: errors.New("delete failed")}
	svc := testDocumentService(t)
	svc.docEngine = engine
	svc.metadataSvc = &MetadataService{}

	_, code, err := svc.UpdateDatasetDocument("tenant-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
		MetaFields: map[string]any{"new": "value"},
	}, map[string]bool{"meta_fields": true})
	if err == nil {
		t.Fatal("expected metadata delete error")
	}
	if code != common.CodeDataError {
		t.Fatalf("code = %v, want %v", code, common.CodeDataError)
	}
	if err.Error() != "failed to delete document metadata: delete failed" {
		t.Fatalf("err = %q", err.Error())
	}
	if engine.updateCalled {
		t.Fatal("metadata update should not run after delete failure")
	}
}

func TestSetDocumentMetadataMergesMetadataRow(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 0, 0)

	engine := newMetadataDocEngine(map[string]map[string]interface{}{
		"doc-1": {
			"author": "alice",
			"year":   2025,
		},
	}, map[string]string{"doc-1": "kb-1"})
	svc := testDocumentService(t)
	svc.docEngine = engine
	svc.metadataSvc = &MetadataService{kbDAO: dao.NewKnowledgebaseDAO(), docEngine: engine}

	if err := svc.SetDocumentMetadata("doc-1", map[string]interface{}{"category": "tech", "year": 2026}); err != nil {
		t.Fatalf("SetDocumentMetadata failed: %v", err)
	}
	if got := engine.records["doc-1"]["author"]; got != "alice" {
		t.Fatalf("author = %#v, want alice", got)
	}
	if got := engine.records["doc-1"]["category"]; got != "tech" {
		t.Fatalf("category = %#v, want tech", got)
	}
	if got := engine.records["doc-1"]["year"]; got != 2026 {
		t.Fatalf("year = %#v, want 2026", got)
	}
	if got := engine.docKBs["doc-1"]; got != "kb-1" {
		t.Fatalf("kb_id = %q, want kb-1", got)
	}
}

func TestChunkImageStorageKeyUsesImgIDWithDatasetPrefix(t *testing.T) {
	key, ok := chunkImageStorageKey("kb-1", map[string]interface{}{
		"id":     "chunk-1",
		"img_id": "kb-1-image-001",
	})
	if !ok {
		t.Fatal("expected image storage key")
	}
	if key != "image-001" {
		t.Fatalf("key = %q, want %q", key, "image-001")
	}
}

func TestChunkImageStorageKeyHandlesHyphenatedDatasetID(t *testing.T) {
	key, ok := chunkImageStorageKey("dataset-abc-123", map[string]interface{}{
		"id":     "chunk-1",
		"img_id": "dataset-abc-123-page-1-image",
	})
	if !ok {
		t.Fatal("expected image storage key")
	}
	if key != "page-1-image" {
		t.Fatalf("key = %q, want %q", key, "page-1-image")
	}
}

func TestChunkImageStorageKeyFallsBackToChunkID(t *testing.T) {
	key, ok := chunkImageStorageKey("kb-1", map[string]interface{}{
		"_id": "chunk-fallback",
	})
	if !ok {
		t.Fatal("expected fallback storage key")
	}
	if key != "chunk-fallback" {
		t.Fatalf("key = %q, want %q", key, "chunk-fallback")
	}
}

func TestBatchUpdateDocumentMetadatasMatchesPythonSemantics(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 3, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc1.txt", 0, 0)
	insertNamedTestDoc(t, "doc-2", "kb-1", "doc2.txt", 0, 0)
	insertNamedTestDoc(t, "doc-3", "kb-1", "doc3.txt", 0, 0)

	engine := newMetadataDocEngine(map[string]map[string]interface{}{
		"doc-1": {"tags": []interface{}{"old", "keep"}, "author": "alice"},
		"doc-2": {"tags": []interface{}{"old"}, "author": "bob"},
	}, map[string]string{"doc-1": "kb-1", "doc-2": "kb-1", "doc-3": "kb-1"})

	svc := testDocumentService(t)
	svc.docEngine = engine
	svc.metadataSvc = &MetadataService{kbDAO: dao.NewKnowledgebaseDAO(), docEngine: engine}

	resp, code, err := svc.BatchUpdateDocumentMetadatas("kb-1", &DocumentMetadataSelector{
		DocumentIDs: []string{"doc-1", "doc-2", "doc-3"},
	}, []DocumentMetadataUpdate{
		{Key: "tags", Value: "new", Match: "old"},
		{Key: "category", Value: "paper"},
	}, []DocumentMetadataDelete{
		{Key: "author", Value: "alice"},
	})
	if err != nil {
		t.Fatalf("BatchUpdateDocumentMetadatas failed: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code = %v, want success", code)
	}
	if resp.Updated != 3 || resp.MatchedDocs != 3 {
		t.Fatalf("resp = %#v, want updated=3 matched=3", resp)
	}

	got1 := engine.records["doc-1"]
	if fmt.Sprintf("%v", got1["category"]) != "paper" {
		t.Fatalf("doc-1 category = %#v", got1["category"])
	}
	if _, ok := got1["author"]; ok {
		t.Fatalf("doc-1 author should be deleted: %#v", got1)
	}
	if got := got1["tags"].([]interface{}); len(got) != 2 || got[0] != "new" || got[1] != "keep" {
		t.Fatalf("doc-1 tags = %#v", got)
	}

	got2 := engine.records["doc-2"]
	if fmt.Sprintf("%v", got2["author"]) != "bob" {
		t.Fatalf("doc-2 author should be kept: %#v", got2["author"])
	}
	if got := got2["tags"].([]interface{}); len(got) != 1 || got[0] != "new" {
		t.Fatalf("doc-2 tags = %#v", got)
	}

	got3 := engine.records["doc-3"]
	if fmt.Sprintf("%v", got3["category"]) != "paper" {
		t.Fatalf("doc-3 category = %#v", got3)
	}
	if _, ok := got3["tags"]; ok {
		t.Fatalf("doc-3 tags should not be created by match-only update: %#v", got3)
	}
}

func TestBatchUpdateDocumentMetadatasDoesNotReplaceWhenCurrentSearchIsStale(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc1.txt", 0, 0)

	baseEngine := newMetadataDocEngine(map[string]map[string]interface{}{
		"doc-1": {"author": "alice"},
	}, map[string]string{"doc-1": "kb-1"})
	engine := &staleSearchMetadataDocEngine{metadataDocEngine: baseEngine}

	svc := testDocumentService(t)
	svc.docEngine = engine
	svc.metadataSvc = &MetadataService{kbDAO: dao.NewKnowledgebaseDAO(), docEngine: engine}

	resp, code, err := svc.BatchUpdateDocumentMetadatas("kb-1", &DocumentMetadataSelector{
		DocumentIDs: []string{"doc-1"},
	}, []DocumentMetadataUpdate{
		{Key: "category", Value: "paper"},
	}, nil)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("batch update failed: code=%v err=%v", code, err)
	}
	if resp.Updated != 1 || resp.MatchedDocs != 1 {
		t.Fatalf("resp = %#v, want updated=1 matched=1", resp)
	}

	got := baseEngine.records["doc-1"]
	if got["author"] != "alice" {
		t.Fatalf("author should be preserved, got metadata %#v", got)
	}
	if got["category"] != "paper" {
		t.Fatalf("category = %#v, want paper", got["category"])
	}
}

func TestBatchUpdateDocumentMetadatasDeletesEmptyMetadataAndNoOps(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 2, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc1.txt", 0, 0)
	insertNamedTestDoc(t, "doc-2", "kb-1", "doc2.txt", 0, 0)

	engine := newMetadataDocEngine(map[string]map[string]interface{}{
		"doc-1": {"status": "draft"},
		"doc-2": {"status": "done"},
	}, map[string]string{"doc-1": "kb-1", "doc-2": "kb-1"})

	svc := testDocumentService(t)
	svc.docEngine = engine
	svc.metadataSvc = &MetadataService{kbDAO: dao.NewKnowledgebaseDAO(), docEngine: engine}

	resp, code, err := svc.BatchUpdateDocumentMetadatas("kb-1", &DocumentMetadataSelector{
		DocumentIDs: []string{"doc-1", "doc-2"},
	}, nil, []DocumentMetadataDelete{{Key: "status", Value: "draft"}})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("delete batch failed: code=%v err=%v", code, err)
	}
	if resp.Updated != 1 || resp.MatchedDocs != 2 {
		t.Fatalf("resp = %#v, want updated=1 matched=2", resp)
	}
	if _, ok := engine.records["doc-1"]; ok {
		t.Fatalf("doc-1 metadata should be fully removed: %#v", engine.records["doc-1"])
	}
	if fmt.Sprintf("%v", engine.records["doc-2"]["status"]) != "done" {
		t.Fatalf("doc-2 metadata unexpectedly changed: %#v", engine.records["doc-2"])
	}
}

func TestBatchUpdateDocumentMetadatasNormalizesNumberValues(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc1.txt", 0, 0)

	engine := newMetadataDocEngine(map[string]map[string]interface{}{}, map[string]string{"doc-1": "kb-1"})

	svc := testDocumentService(t)
	svc.docEngine = engine
	svc.metadataSvc = &MetadataService{kbDAO: dao.NewKnowledgebaseDAO(), docEngine: engine}

	resp, code, err := svc.BatchUpdateDocumentMetadatas("kb-1", &DocumentMetadataSelector{
		DocumentIDs: []string{"doc-1"},
	}, []DocumentMetadataUpdate{
		{Key: "score", Value: "42", ValueType: "number"},
	}, nil)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("number batch failed: code=%v err=%v", code, err)
	}
	if resp.Updated != 1 || resp.MatchedDocs != 1 {
		t.Fatalf("resp = %#v, want updated=1 matched=1", resp)
	}

	got := engine.records["doc-1"]["score"]
	switch v := got.(type) {
	case int64:
		if v != 42 {
			t.Fatalf("score = %v, want 42", v)
		}
	case float64:
		if v != 42 {
			t.Fatalf("score = %v, want 42", v)
		}
	default:
		t.Fatalf("score type = %T, want numeric value", got)
	}
}

func TestBatchUpdateDocumentMetadatasRejectsMissingValue(t *testing.T) {
	svc := testDocumentService(t)
	resp, code, err := svc.BatchUpdateDocumentMetadatas("kb-1", &DocumentMetadataSelector{}, []DocumentMetadataUpdate{
		{Key: "status"},
	}, nil)
	if err == nil {
		t.Fatal("expected validation error for missing value")
	}
	if resp != nil {
		t.Fatalf("resp = %#v, want nil", resp)
	}
	if code != common.CodeDataError {
		t.Fatalf("code = %v, want data error", code)
	}
	if !strings.Contains(err.Error(), "Each update requires key and value.") {
		t.Fatalf("err = %v", err)
	}
}

func TestAggregateMetadataIgnoresNestedEmptyLists(t *testing.T) {
	summary := aggregateMetadata([]map[string]interface{}{
		{
			"id":    "doc-1",
			"kb_id": "kb-1",
			"meta_fields": map[string]interface{}{
				"score": []interface{}{[]interface{}{}, 7.0},
				"name":  "alice",
			},
		},
	})

	scoreField, ok := summary["score"].(map[string]interface{})
	if !ok {
		t.Fatalf("score summary missing: %#v", summary)
	}
	values, ok := scoreField["values"].([][2]interface{})
	if !ok {
		t.Fatalf("score values type = %T", scoreField["values"])
	}
	if len(values) != 1 || values[0][0] != "7" || values[0][1] != 1 {
		t.Fatalf("score values = %#v, want [[\"7\",1]]", values)
	}
}

func TestMergeFieldValuesKeepsNumericValues(t *testing.T) {
	got := mergeFieldValues(1.0, 2.0)
	if len(got) != 2 || got[0] != 1.0 || got[1] != 2.0 {
		t.Fatalf("mergeFieldValues = %#v, want [1 2]", got)
	}
}

func TestUpdateDatasetDocumentPipelineIDTakesPrecedenceOverChunkMethod(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertNamedTestDoc(t, "doc-1", "kb-1", "doc.txt", 10, 5)

	pipelineID := "1234567890abcdef1234567890abcdef"
	chunkMethod := "manual"
	svc := testDocumentService(t)
	resp, code, err := svc.UpdateDatasetDocument("tenant-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
		PipelineID:  &pipelineID,
		ChunkMethod: &chunkMethod,
	}, map[string]bool{"pipeline_id": true, "chunk_method": true})
	if err != nil {
		t.Fatalf("UpdateDatasetDocument failed: code=%v err=%v", code, err)
	}
	if resp.PipelineID == nil || *resp.PipelineID != pipelineID {
		t.Fatalf("pipeline_id = %v, want %q", resp.PipelineID, pipelineID)
	}
	if resp.ChunkMethod != "naive" {
		t.Fatalf("chunk_method = %q, want original naive", resp.ChunkMethod)
	}
}

func TestUpdateDatasetDocumentEnabledUpdatesStatus(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	enabled := 0
	svc := testDocumentService(t)
	resp, code, err := svc.UpdateDatasetDocument("tenant-1", "kb-1", "doc-1", &UpdateDatasetDocumentRequest{
		Enabled: &enabled,
	}, map[string]bool{"enabled": true})
	if err != nil {
		t.Fatalf("UpdateDatasetDocument failed: code=%v err=%v", code, err)
	}
	if resp.Status == nil || *resp.Status != "0" {
		t.Fatalf("status = %v, want 0", resp.Status)
	}
}

func insertNamedTestDoc(t *testing.T, id, kbID, name string, tokenNum, chunkNum int64) {
	t.Helper()
	doc := &entity.Document{
		ID:           id,
		KbID:         kbID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		TokenNum:     tokenNum,
		ChunkNum:     chunkNum,
		Progress:     0.75,
		Name:         sptr(name),
		Type:         "doc",
		SourceType:   "local",
		CreatedBy:    "tenant-1",
		Suffix:       filepath.Ext(name),
		Status:       sptr("1"),
		Run:          sptr(string(entity.TaskStatusDone)),
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("insert named test doc: %v", err)
	}
}

// TestGetDocumentArtifact_AuthGate mirrors PR #16169: the sandbox
// artifact download endpoint must be gated on the caller owning
// (or having team access to) an agent session whose `message`
// references the filename. Three cases:
//   - empty userID -> ErrArtifactNotFound
//   - filename referenced by another user's session -> ErrArtifactNotFound
//   - filename referenced by the caller's own session, with
//     accessible canvas -> no error (storage layer short-circuits
//     in this test because no real storage is wired)
//
// TestEscapeSQLLikePattern pins PR review round 5, Major #8:
// SQL LIKE wildcards (%, _, \) MUST be escaped before being
// interpolated into the auth-gate LIKE pattern, otherwise a
// caller can match a different referenced artifact's filename
// and bypass the per-filename authorization. The escape
// character is '\\' to match the ESCAPE clause in
// sandboxArtifactDialogIDsForUser.
func TestEscapeSQLLikePattern(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain.png", "plain.png"},
		// % is a wildcard; . is literal — the dot does not need escaping.
		{"%.png", "!%.png"},
		{"_underscore", "!_underscore"},
		// '!' is the escape character; double it inside the input.
		{"with!bang", "with!!bang"},
		// Compound: % and _ in one input.
		{"%_", "!%!_"},
		// Empty passthrough.
		{"", ""},
	}
	for _, c := range cases {
		if got := escapeSQLLikePattern(c.in); got != c.want {
			t.Errorf("escapeSQLLikePattern(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSandboxArtifactDialogIDsForUser_LikeWildcardEscaped pins PR review
// round 5, Major #8: filename wildcards (%, _) MUST be escaped
// before being interpolated into the LIKE auth-gate, otherwise a
// caller can submit a wildcard filename, pass the authorization
// check against a different referenced artifact, and then GET
// the requested object by its real name.
//
// We exercise the SQL LIKE behavior using a custom in-memory
// table with TEXT columns (SQLite's gorm AutoMigrate creates
// columns with NUMERIC affinity for `type:longtext`, which
// defeats LIKE; production uses MySQL where longtext is a real
// string type — so the test isolates the SQL escape behaviour
// rather than the column-type quirk).
func TestSandboxArtifactDialogIDsForUser_LikeWildcardEscaped(t *testing.T) {
	db := setupServiceTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	// Hand-rolled schema with TEXT columns so SQLite LIKE works
	// correctly. The production entity uses `type:longtext` which
	// SQLite gives NUMERIC affinity (LIKE then compares strings
	// numerically and never matches). The auth-gate query this
	// test pins operates over TEXT, so we recreate the schema
	// accordingly.
	if err := db.Exec(`CREATE TABLE sandbox_artifacts (
		user_id TEXT,
		dialog_id TEXT,
		message TEXT
	)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Exec(`INSERT INTO sandbox_artifacts (user_id, dialog_id, message) VALUES (?, ?, ?)`,
		"user-1", "agent-1",
		`[{"role":"assistant","content":"saved as documents/artifact/x.png"}]`).Error; err != nil {
		t.Fatalf("seed row: %v", err)
	}

	// Wildcard filename must NOT cross-match user-1's x.png.
	var wildcards int64
	db.Raw(`SELECT COUNT(*) FROM sandbox_artifacts WHERE message LIKE ? ESCAPE '!'`, "!%.png%").Scan(&wildcards)
	if wildcards != 0 {
		t.Errorf("wildcard filename must not match user-1's x.png; got count=%d", wildcards)
	}

	// Literal filename still matches for the owner.
	var literal int64
	db.Raw(`SELECT COUNT(*) FROM sandbox_artifacts WHERE message LIKE ? ESCAPE '!'`, "%x.png%").Scan(&literal)
	if literal != 1 {
		t.Errorf("literal filename for the owner must still match; got count=%d", literal)
	}
}

func TestGetDocumentArtifact_AuthGate(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(
		&entity.UserCanvas{},
		&entity.API4Conversation{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	// Seed a canvas owned by user-1.
	if err := db.Create(&entity.UserCanvas{
		ID:             "agent-1",
		UserID:         "user-1",
		Title:          sptr("Agent"),
		CanvasCategory: "agent_canvas",
	}).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}
	// Seed an API4Conversation whose message references the filename.
	if err := db.Create(&entity.API4Conversation{
		ID:       "sess-1",
		DialogID: "agent-1",
		UserID:   "user-1",
		Message:  json.RawMessage(`[{"role":"assistant","content":"saved as documents/artifact/result.png"}]`),
	}).Error; err != nil {
		t.Fatalf("seed conv: %v", err)
	}

	svc := testDocumentService(t)

	// Case 1: empty user -> not allowed.
	if _, err := svc.GetDocumentArtifact("result.png", ""); !errors.Is(err, ErrArtifactNotFound) {
		t.Errorf("empty user: want ErrArtifactNotFound, got %v", err)
	}

	// Case 2: another user without any session reference -> not allowed.
	if _, err := svc.GetDocumentArtifact("result.png", "user-2"); !errors.Is(err, ErrArtifactNotFound) {
		t.Errorf("unrelated user: want ErrArtifactNotFound, got %v", err)
	}

	// Case 3: another user who has their own (unrelated) session for a
	// different agent that does NOT mention the filename -> not allowed.
	if err := db.Create(&entity.UserCanvas{
		ID:             "agent-2",
		UserID:         "user-2",
		Title:          sptr("Other Agent"),
		CanvasCategory: "agent_canvas",
	}).Error; err != nil {
		t.Fatalf("seed canvas 2: %v", err)
	}
	if err := db.Create(&entity.API4Conversation{
		ID:       "sess-2",
		DialogID: "agent-2",
		UserID:   "user-2",
		Message:  json.RawMessage(`[{"role":"user","content":"hello"}]`),
	}).Error; err != nil {
		t.Fatalf("seed conv 2: %v", err)
	}
	if _, err := svc.GetDocumentArtifact("result.png", "user-2"); !errors.Is(err, ErrArtifactNotFound) {
		t.Errorf("user-2 with unrelated session: want ErrArtifactNotFound, got %v", err)
	}
}

func TestGetThumbnails_AlignsWithPythonFormatting(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	if err := db.AutoMigrate(&entity.Document{}, &entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)
	insertTestKB(t, "kb-2", "tenant-1", 0, 0, 0)
	insertTestKB(t, "kb-other", "tenant-other", 0, 0, 0)
	if err := db.Create(&entity.UserTenant{
		ID:        "user-1_tenant-1",
		UserID:    "user-1",
		TenantID:  "tenant-1",
		Role:      "owner",
		InvitedBy: "user-1",
		Status:    sptr("1"),
	}).Error; err != nil {
		t.Fatalf("seed user tenant: %v", err)
	}

	base64Thumb := "data:image/png;base64,AAAA"
	fileThumb := "thumb.png"
	otherThumb := "secret.png"
	if err := db.Create(&entity.Document{
		ID:           "doc-file",
		KbID:         "kb-1",
		Thumbnail:    &fileThumb,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         "pdf",
		CreatedBy:    "user-1",
		Suffix:       "png",
	}).Error; err != nil {
		t.Fatalf("seed file thumbnail doc: %v", err)
	}
	if err := db.Create(&entity.Document{
		ID:           "doc-base64",
		KbID:         "kb-2",
		Thumbnail:    &base64Thumb,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         "pdf",
		CreatedBy:    "user-1",
		Suffix:       "png",
	}).Error; err != nil {
		t.Fatalf("seed base64 thumbnail doc: %v", err)
	}
	if err := db.Create(&entity.Document{
		ID:           "doc-other",
		KbID:         "kb-other",
		Thumbnail:    &otherThumb,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		SourceType:   "local",
		Type:         "pdf",
		CreatedBy:    "user-other",
		Suffix:       "png",
	}).Error; err != nil {
		t.Fatalf("seed other tenant thumbnail doc: %v", err)
	}

	svc := testDocumentService(t)
	got, err := svc.GetThumbnails("user-1", []string{"doc-file", "doc-base64", "doc-other", "missing-doc"})
	if err != nil {
		t.Fatalf("GetThumbnails failed: %v", err)
	}

	if got["doc-file"] != "/api/v1/documents/images/kb-1-thumb.png" {
		t.Fatalf("unexpected file thumbnail: %q", got["doc-file"])
	}
	if got["doc-base64"] != base64Thumb {
		t.Fatalf("unexpected base64 thumbnail: %q", got["doc-base64"])
	}
	if _, ok := got["missing-doc"]; ok {
		t.Fatalf("did not expect missing doc in result: %#v", got)
	}
	if _, ok := got["doc-other"]; ok {
		t.Fatalf("did not expect other tenant doc in result: %#v", got)
	}
}
