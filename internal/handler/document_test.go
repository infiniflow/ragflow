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
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
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
	uploadLocalData []map[string]interface{}
	uploadLocalErrs []string
	uploadLocalKB   *entity.Knowledgebase
	uploadLocalPath string
	uploadOverride  map[string]interface{}
	ingestCode      common.ErrorCode
	ingestErr       error
	ingestUserID    string
	ingestReq       *service.IngestDocumentRequest
}

func (f *fakeDocumentService) Ingest(userID string, req *service.IngestDocumentRequest) (common.ErrorCode, error) {
	f.ingestUserID = userID
	f.ingestReq = req
	if f.ingestCode != 0 || f.ingestErr != nil {
		return f.ingestCode, f.ingestErr
	}
	return common.CodeSuccess, nil
}

const uploadTestDatasetID = "123e4567-e89b-12d3-a456-426614174000"

func (f *fakeDocumentService) UpdateDatasetDocument(userID, datasetID, documentID string, req *service.UpdateDatasetDocumentRequest, present map[string]bool) (*service.UpdateDatasetDocumentResponse, common.ErrorCode, error) {
	return nil, common.CodeSuccess, nil
}
func (f *fakeDocumentService) BatchUpdateDocumentMetadatas(datasetID string, selector *service.DocumentMetadataSelector, updates []service.DocumentMetadataUpdate, deletes []service.DocumentMetadataDelete) (*service.BatchUpdateDocumentMetadatasResponse, common.ErrorCode, error) {
	return nil, common.CodeSuccess, nil
}
func (f *fakeDocumentService) UploadDocumentInfos(userID string, files []*multipart.FileHeader) ([]map[string]interface{}, common.ErrorCode, error) {
	return nil, common.CodeSuccess, nil
}
func (f *fakeDocumentService) UploadDocumentInfoByURL(userID, rawURL string) (map[string]interface{}, common.ErrorCode, error) {
	return nil, common.CodeSuccess, nil
}

func (f *fakeDocumentService) GetDocumentArtifact(filename, _ string) (*service.ArtifactResponse, error) {
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
func (f *fakeDocumentService) BatchUpdateDocumentStatus(userID, datasetID, status string, documentIDs []string) (map[string]interface{}, common.ErrorCode, error) {
	return map[string]interface{}{}, common.CodeSuccess, nil
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
func (f *fakeDocumentService) UploadLocalDocuments(kb *entity.Knowledgebase, tenantID string, files []*multipart.FileHeader, parentPath string, parserConfigOverride map[string]interface{}) ([]map[string]interface{}, []string) {
	f.uploadLocalKB = kb
	f.uploadLocalPath = parentPath
	f.uploadOverride = parserConfigOverride
	return f.uploadLocalData, f.uploadLocalErrs
}
func (f *fakeDocumentService) UploadWebDocument(kb *entity.Knowledgebase, tenantID, name, url string) (map[string]interface{}, common.ErrorCode, error) {
	return nil, common.CodeServerError, fmt.Errorf("not implemented")
}
func (f *fakeDocumentService) UploadEmptyDocument(kb *entity.Knowledgebase, tenantID, name string) (map[string]interface{}, common.ErrorCode, error) {
	return nil, common.CodeServerError, fmt.Errorf("not implemented")
}

func (f *fakeDocumentService) ListIngestionTasks(userID string, datasetID *string, page, pageSize int) ([]*entity.IngestionTask, error) {
	return nil, nil
}
func (f *fakeDocumentService) IngestDocuments(datasetID, userID string, docIDs []string) ([]*service.ParseDocumentResponse, error) {
	return nil, nil
}
func (f *fakeDocumentService) StopIngestionTasks(tasks []string, userID string) ([]*entity.IngestionTask, error) {
	return nil, nil
}
func (f *fakeDocumentService) RemoveIngestionTasks(tasks []string, userID string) ([]map[string]string, error) {
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

func setupUploadHandlerDB(t *testing.T, role string) *gorm.DB {
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
	if err := db.Create(&entity.User{ID: "user-1", Nickname: "test", Email: "test@test.com", Password: sptr("x")}).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := db.Create(&entity.Tenant{ID: "tenant-1", LLMID: "llm-1", EmbdID: "embd-1", ASRID: "asr-1", Status: sptr(string(entity.StatusValid))}).Error; err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	if err := db.Create(&entity.UserTenant{ID: "ut-1", UserID: "user-1", TenantID: "tenant-1", Role: role, Status: sptr(string(entity.StatusValid))}).Error; err != nil {
		t.Fatalf("insert user_tenant: %v", err)
	}
	pipelineID := "pipe-1"
	if err := db.Create(&entity.Knowledgebase{
		ID:           "123e4567e89b12d3a456426614174000",
		TenantID:     "tenant-1",
		Name:         "kb-upload",
		EmbdID:       "embd-1",
		CreatedBy:    "user-1",
		Permission:   string(entity.TenantPermissionTeam),
		ParserID:     "naive",
		PipelineID:   &pipelineID,
		ParserConfig: entity.JSONMap{"base": "cfg"},
		Status:       sptr(string(entity.StatusValid)),
	}).Error; err != nil {
		t.Fatalf("insert knowledgebase: %v", err)
	}
	return db
}

func setupUploadContext(t *testing.T, path string, fields map[string]string, fileName string, fileContent []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatalf("write field %s: %v", k, err)
		}
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(fileContent); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "dataset_id", Value: uploadTestDatasetID}}
	return c, w
}

func setupDocumentIngestRoute(userID string, svc *fakeDocumentService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &DocumentHandler{
		documentService: svc,
		datasetService:  service.NewDatasetService(),
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: userID})
		c.Set("user_id", userID)
	})
	r.POST("/api/v1/documents/ingest", h.Ingest)
	return r
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

func TestUploadDocumentsHandler_LocalUsesFullKBAndIgnoresBadParserConfig(t *testing.T) {
	db := setupUploadHandlerDB(t, "normal")
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	fake := &fakeDocumentService{
		uploadLocalData: []map[string]interface{}{
			{"id": "doc-1", "kb_id": "ds-1", "parser_id": "naive", "chunk_num": int64(0), "token_num": int64(0), "name": "a.txt"},
		},
	}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupUploadContext(t, "/api/v1/datasets/ds-1/documents?type=local", map[string]string{
		"parent_path":   "nested/path",
		"parser_config": "{bad json",
	}, "a.txt", []byte("abc"))

	h.UploadDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if fake.uploadLocalKB == nil {
		t.Fatalf("UploadLocalDocuments was not called, response=%s", w.Body.String())
	}
	if fake.uploadLocalKB.TenantID != "tenant-1" || fake.uploadLocalKB.Name != "kb-upload" || fake.uploadLocalKB.ParserID != "naive" {
		t.Fatalf("incomplete kb passed to service: %+v", fake.uploadLocalKB)
	}
	if fake.uploadLocalPath != "nested/path" {
		t.Fatalf("parent path=%q, want nested/path", fake.uploadLocalPath)
	}
	if fake.uploadOverride != nil {
		t.Fatalf("bad parser_config should be ignored, got %v", fake.uploadOverride)
	}
}

func TestUploadDocumentsHandler_LocalReturnsPartialSuccess(t *testing.T) {
	db := setupUploadHandlerDB(t, "normal")
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	fake := &fakeDocumentService{
		uploadLocalData: []map[string]interface{}{
			{"id": "doc-1", "kb_id": "ds-1", "parser_id": "naive", "chunk_num": int64(0), "token_num": int64(0), "name": "ok.txt"},
		},
		uploadLocalErrs: []string{"bad.exe: This type of file has not been supported yet!"},
	}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupUploadContext(t, "/api/v1/datasets/ds-1/documents?type=local", nil, "ok.txt", []byte("abc"))
	h.UploadDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected success for partial upload, got %v", resp)
	}
	data := resp["data"].(map[string]interface{})
	if len(data["documents"].([]interface{})) != 1 {
		t.Fatalf("expected one successful document, got %v", data["documents"])
	}
	if len(data["errors"].([]interface{})) != 1 {
		t.Fatalf("expected one file error, got %v", data["errors"])
	}
}

func TestUploadDocumentsHandler_DeniesNonNormalTeamRole(t *testing.T) {
	db := setupUploadHandlerDB(t, "admin")
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	fake := &fakeDocumentService{}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupUploadContext(t, "/api/v1/datasets/ds-1/documents?type=local", nil, "a.txt", []byte("abc"))
	h.UploadDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["code"] == float64(common.CodeSuccess) {
		t.Fatalf("expected authorization error, got %v", resp)
	}
	if fake.uploadLocalKB != nil {
		t.Fatal("service should not be called on denied upload")
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

func TestDocumentHandlerIngestMatchesPythonResponseShape(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("POST", "/api/v1/documents/ingest", `{"doc_ids":["doc-1"],"run":"1"}`)
	h.Ingest(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected top-level code 0, got %v", resp["code"])
	}
	if resp["data"] != true {
		t.Fatalf("expected top-level data=true, got %#v", resp["data"])
	}
	if _, ok := resp["data"].(map[string]interface{}); ok {
		t.Fatalf("response must not nest code/message under data: %#v", resp["data"])
	}
	if fake.ingestUserID != "user-1" {
		t.Fatalf("expected user-1, got %q", fake.ingestUserID)
	}
	if fake.ingestReq == nil || len(fake.ingestReq.DocIDs) != 1 || fake.ingestReq.DocIDs[0] != "doc-1" {
		t.Fatalf("unexpected ingest request: %#v", fake.ingestReq)
	}
}

func TestDocumentIngestRoutePassesPythonBodyToService(t *testing.T) {
	fake := &fakeDocumentService{}
	r := setupDocumentIngestRoute("user-1", fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/documents/ingest", strings.NewReader(`{"doc_ids":["doc-1","doc-2"],"run":1,"delete":true,"apply_kb":true}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != float64(common.CodeSuccess) || resp["data"] != true {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
	if fake.ingestUserID != "user-1" {
		t.Fatalf("userID = %q, want user-1", fake.ingestUserID)
	}
	if fake.ingestReq == nil {
		t.Fatal("service did not receive ingest request")
	}
	if len(fake.ingestReq.DocIDs) != 2 || fake.ingestReq.DocIDs[0] != "doc-1" || fake.ingestReq.DocIDs[1] != "doc-2" {
		t.Fatalf("doc_ids = %#v, want [doc-1 doc-2]", fake.ingestReq.DocIDs)
	}
	if fmt.Sprint(fake.ingestReq.Run) != "1" {
		t.Fatalf("run = %#v, want 1", fake.ingestReq.Run)
	}
	if !fake.ingestReq.Delete {
		t.Fatal("delete = false, want true")
	}
	if !fake.ingestReq.ApplyKB {
		t.Fatal("apply_kb = false, want true")
	}
}

func TestDocumentHandlerIngestPropagatesServiceErrorCode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fake := &fakeDocumentService{
		ingestCode: common.CodeAuthenticationError,
		ingestErr:  fmt.Errorf("No authorization."),
	}
	h := &DocumentHandler{
		documentService: fake,
		datasetService:  service.NewDatasetService(),
	}

	c, w := setupGinContextWithUser("POST", "/api/v1/documents/ingest", `{"doc_ids":["doc-1"],"run":"1"}`)
	h.Ingest(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != float64(common.CodeAuthenticationError) {
		t.Fatalf("expected auth error code, got %v", resp["code"])
	}
	if resp["message"] != "No authorization." {
		t.Fatalf("unexpected message: %v", resp["message"])
	}
	if resp["data"] != nil {
		t.Fatalf("expected nil data, got %#v", resp["data"])
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
