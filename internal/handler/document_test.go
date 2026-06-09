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

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// fakeDocumentService implements documentServiceIface for handler tests.
type fakeDocumentService struct {
	deleted         int
	err             error
	stopResult      map[string]interface{}
	stopErr         error
	metadataSummary map[string]interface{}
	metadataErr     error
	metadataKBID    string
	metadataDocIDs  []string
}

func (f *fakeDocumentService) GetDocumentArtifact(filename string) (*service.ArtifactResponse, error) {
	if filename == "error.txt" {
		return nil, service.ErrArtifactNotFound
	}
	if filename == "unexpected.txt" {
		return nil, fmt.Errorf("unexpected error")
	}
	return &service.ArtifactResponse{
		Data:            []byte("artifact content"),
		ContentType:     "text/plain",
		SafeFilename:    "safe.txt",
		ForceAttachment: false,
	}, nil
}
func (f *fakeDocumentService) GetDocumentPreview(docID string) (*service.DocumentPreview, error) {
	if docID == "not-found" {
		return nil, fmt.Errorf("not found")
	}
	return &service.DocumentPreview{
		Data:        []byte("preview content"),
		ContentType: "text/plain",
		FileName:    "preview.txt",
	}, nil
}
func (f *fakeDocumentService) DownloadDocument(datasetID, docID string) (*service.DownloadDocumentResp, error) {
	if docID == "not-found" {
		return nil, fmt.Errorf("not found")
	}
	return &service.DownloadDocumentResp{
		Data:        []byte("document data"),
		ContentType: "application/pdf",
		FileName:    "doc.pdf",
	}, nil
}
func (f *fakeDocumentService) CreateDocument(req *service.CreateDocumentRequest) (*entity.Document, error) {
	return nil, nil
}
func (f *fakeDocumentService) GetDocumentByID(id string) (*service.DocumentResponse, error) {
	return nil, nil
}
func (f *fakeDocumentService) UpdateDocument(id string, req *service.UpdateDocumentRequest) error {
	return nil
}
func (f *fakeDocumentService) DeleteDocument(id string) error {
	return nil
}
func (f *fakeDocumentService) DeleteDocuments(ids []string, deleteAll bool, datasetID, userID string) (int, error) {
	return f.deleted, f.err
}
func (f *fakeDocumentService) ParseDocuments(datasetID, userID string, docIDs []string) ([]*service.ParseDocumentResponse, error) {
	return nil, nil
}
func (f *fakeDocumentService) StopParseDocuments(datasetID string, docIDs []string) (map[string]interface{}, error) {
	return f.stopResult, f.stopErr
}
func (f *fakeDocumentService) ListDocuments(page, pageSize int) ([]*service.DocumentResponse, int64, error) {
	return nil, 0, nil
}
func (f *fakeDocumentService) ListDocumentsByDatasetID(kbID string, page, pageSize int) ([]*entity.DocumentListItem, int64, error) {
	return nil, 0, nil
}
func (f *fakeDocumentService) GetThumbnail(docID string) (*service.ThumbnailResponse, error) {
	return nil, nil
}
func (f *fakeDocumentService) GetDocumentImage(imageID string) ([]byte, error) {
	return nil, nil
}
func (f *fakeDocumentService) GetDocumentsByAuthorID(authorID, page, pageSize int) ([]*service.DocumentResponse, int64, error) {
	return nil, 0, nil
}
func (f *fakeDocumentService) GetMetadataSummary(kbID string, docIDs []string) (map[string]interface{}, error) {
	f.metadataKBID = kbID
	f.metadataDocIDs = docIDs
	return f.metadataSummary, f.metadataErr
}
func (f *fakeDocumentService) SetDocumentMetadata(docID string, meta map[string]interface{}) error {
	return nil
}
func (f *fakeDocumentService) DeleteDocumentMetadata(docID string, keys []string) error {
	return nil
}
func (f *fakeDocumentService) DeleteDocumentAllMetadata(docID string) error {
	return nil
}
func (f *fakeDocumentService) GetDocumentMetadataByID(docID string) (map[string]interface{}, error) {
	return nil, nil
}

func setupGinContextWithUser(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")
	return c, w
}

func TestDeleteDocumentsHandler_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{deleted: 3}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("DELETE", "/api/v1/datasets/ds-1/documents", `{"ids": ["doc-1", "doc-2", "doc-3"]}`)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.DeleteDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code 0, got %v", resp["code"])
	}
	data := resp["data"].(map[string]interface{})
	if data["deleted"] != float64(3) {
		t.Fatalf("expected deleted=3, got %v", data["deleted"])
	}
}

func TestDeleteDocumentsHandler_DeleteAll(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{deleted: 5}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("DELETE", "/api/v1/datasets/ds-1/documents", `{"delete_all": true}`)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.DeleteDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteDocumentsHandler_MutuallyExclusive(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("DELETE", "/api/v1/datasets/ds-1/documents", `{"ids": ["doc-1"], "delete_all": true}`)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.DeleteDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == float64(common.CodeSuccess) {
		t.Fatal("expected error for mutually exclusive ids+delete_all")
	}
}

func TestDeleteDocumentsHandler_NoIDsNoDeleteAll(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("DELETE", "/api/v1/datasets/ds-1/documents", `{}`)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.DeleteDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == float64(common.CodeSuccess) {
		t.Fatal("expected error for no ids and no delete_all")
	}
}

func TestDeleteDocumentsHandler_ServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{err: fmt.Errorf("permission denied")}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("DELETE", "/api/v1/datasets/ds-1/documents", `{"ids": ["doc-1"]}`)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.DeleteDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == float64(common.CodeSuccess) {
		t.Fatal("expected error code")
	}
}

func TestDeleteDocumentsHandler_MissingDatasetID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("DELETE", "/api/v1/datasets//documents", `{"ids": ["doc-1"]}`)

	h.DeleteDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == float64(common.CodeSuccess) {
		t.Fatal("expected error for missing dataset_id")
	}
}

func TestStopParseDocumentsHandler_EmptyDocIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("POST", "/api/v1/datasets/ds-1/documents/stop", `{"document_ids": []}`)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.StopParseDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == float64(common.CodeSuccess) {
		t.Fatal("expected error for empty document_ids")
	}
}

func TestStopParseDocumentsHandler_BadJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("POST", "/api/v1/datasets/ds-1/documents/stop", `not json`)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.StopParseDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == float64(common.CodeSuccess) {
		t.Fatal("expected error for bad JSON body")
	}
}

// setupHandlerAccessDB sets up SQLite in-memory DB for handler tests that need
// datasetService.Accessible to work.
func setupHandlerAccessDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.Knowledgebase{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	// Insert user
	db.Create(&entity.User{ID: "user-1", Nickname: "test", Email: "test@test.com", Password: sptr("x")})
	// Insert tenant
	db.Create(&entity.Tenant{ID: "tenant-1", LLMID: "llm-1", EmbdID: "embd-1", ASRID: "asr-1"})
	// Insert user_tenant mapping
	db.Create(&entity.UserTenant{ID: "ut-1", UserID: "user-1", TenantID: "tenant-1", Role: "admin"})
	// Insert knowledgebase
	db.Create(&entity.Knowledgebase{
		ID: "ds-1", TenantID: "tenant-1", Name: "test-kb", EmbdID: "embd-1",
		CreatedBy: "user-1", Permission: string(entity.TenantPermissionTeam),
		Status: sptr(string(entity.StatusValid)),
	})

	return db
}

// sptr returns a pointer to the given string (copy of service test helper).
func sptr(s string) *string { return &s }

func TestStopParseDocumentsHandler_Success(t *testing.T) {
	db := setupHandlerAccessDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{
		stopResult: map[string]interface{}{"success_count": 1},
	}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("POST", "/api/v1/datasets/ds-1/documents/stop", `{"document_ids": ["doc-1"]}`)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.StopParseDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code 0, got %v: %v", resp["code"], resp)
	}
	data := resp["data"].(map[string]interface{})
	if data["success_count"] != float64(1) {
		t.Fatalf("expected success_count=1, got %v", data["success_count"])
	}
}

func TestStopParseDocumentsHandler_ServiceError(t *testing.T) {
	db := setupHandlerAccessDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{
		stopErr: fmt.Errorf("internal failure"),
	}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("POST", "/api/v1/datasets/ds-1/documents/stop", `{"document_ids": ["doc-1"]}`)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.StopParseDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == float64(common.CodeSuccess) {
		t.Fatal("expected error code for service error")
	}
}

func TestStopParseDocumentsHandler_NotAccessible(t *testing.T) {
	db := setupHandlerAccessDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("POST", "/api/v1/datasets/ds-1/documents/stop", `{"document_ids": ["doc-1"]}`)
	// Use a user that doesn't have access to ds-1
	c.Set("user_id", "other-user")
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.StopParseDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == float64(common.CodeSuccess) {
		t.Fatal("expected error for no authorization")
	}
}

func TestMetadataSummaryByDataset_Success(t *testing.T) {
	db := setupHandlerAccessDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{
		metadataSummary: map[string]interface{}{
			"author": map[string]interface{}{
				"type": "string",
				"values": []interface{}{
					[]interface{}{"alice", 2},
				},
			},
		},
	}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("GET", "/api/v1/datasets/ds-1/metadata/summary?doc_ids=doc-1,doc-2", "")
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}

	h.MetadataSummaryByDataset(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if fake.metadataKBID != "ds-1" {
		t.Fatalf("expected kbID ds-1, got %q", fake.metadataKBID)
	}
	if len(fake.metadataDocIDs) != 2 || fake.metadataDocIDs[0] != "doc-1" || fake.metadataDocIDs[1] != "doc-2" {
		t.Fatalf("unexpected docIDs: %#v", fake.metadataDocIDs)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code 0, got %v: %v", resp["code"], resp)
	}
	data := resp["data"].(map[string]interface{})
	summary := data["summary"].(map[string]interface{})
	author := summary["author"].(map[string]interface{})
	if author["type"] != "string" {
		t.Fatalf("expected author type string, got %v", author["type"])
	}
}

func TestGetDocumentArtifact_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DocumentHandler{
		documentService: &fakeDocumentService{},
	}
	c, w := setupGinContextWithUser("GET", "/api/v1/documents/artifact/test.txt", "")
	c.Params = gin.Params{{Key: "filename", Value: "test.txt"}}

	h.GetDocumentArtifact(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "text/plain" {
		t.Fatalf("unexpected content type: %s", w.Header().Get("Content-Type"))
	}
	if w.Body.String() != "artifact content" {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestGetDocumentArtifact_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DocumentHandler{
		documentService: &fakeDocumentService{},
	}
	c, w := setupGinContextWithUser("GET", "/api/v1/documents/artifact/error.txt", "")
	c.Params = gin.Params{{Key: "filename", Value: "error.txt"}}

	h.GetDocumentArtifact(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeDataError) {
		t.Fatalf("expected code %d, got %v", common.CodeDataError, resp["code"])
	}
}

func TestGetDocumentArtifact_UnexpectedError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DocumentHandler{
		documentService: &fakeDocumentService{},
	}
	c, w := setupGinContextWithUser("GET", "/api/v1/documents/artifact/unexpected.txt", "")
	c.Params = gin.Params{{Key: "filename", Value: "unexpected.txt"}}

	h.GetDocumentArtifact(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeExceptionError) {
		t.Fatalf("expected code %d, got %v", common.CodeExceptionError, resp["code"])
	}
}

func TestGetDocumentPreview_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DocumentHandler{
		documentService: &fakeDocumentService{},
	}
	c, w := setupGinContextWithUser("GET", "/api/v1/documents/doc-1/preview", "")
	c.Params = gin.Params{{Key: "id", Value: "doc-1"}}

	h.GetDocumentPreview(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "text/plain" {
		t.Fatalf("unexpected content type: %s", w.Header().Get("Content-Type"))
	}
	if w.Body.String() != "preview content" {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestGetDocumentPreview_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DocumentHandler{
		documentService: &fakeDocumentService{},
	}
	c, w := setupGinContextWithUser("GET", "/api/v1/documents/not-found/preview", "")
	c.Params = gin.Params{{Key: "id", Value: "not-found"}}

	h.GetDocumentPreview(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeDataError) {
		t.Fatalf("expected code %d, got %v", common.CodeDataError, resp["code"])
	}
}

func TestDownloadDocument_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DocumentHandler{
		documentService: &fakeDocumentService{},
	}
	c, w := setupGinContextWithUser("GET", "/api/v1/datasets/ds-1/documents/doc-1", "")
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}, {Key: "document_id", Value: "doc-1"}}

	h.DownloadDocument(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/pdf" {
		t.Fatalf("unexpected content type: %s", w.Header().Get("Content-Type"))
	}
	if w.Body.String() != "document data" {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestDownloadDocument_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &DocumentHandler{
		documentService: &fakeDocumentService{},
	}
	c, w := setupGinContextWithUser("GET", "/api/v1/datasets/ds-1/documents/not-found", "")
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}, {Key: "document_id", Value: "not-found"}}

	h.DownloadDocument(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeDataError) {
		t.Fatalf("expected code %d, got %v", common.CodeDataError, resp["code"])
	}
}
