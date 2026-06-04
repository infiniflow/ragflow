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

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// fakeDocumentService implements documentServiceIface for handler tests.
type fakeDocumentService struct {
	deleted int
	err     error
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
	return nil, nil
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
