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
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// --- fake-service upload stubs (struct fields live in document_test.go) ---

func (f *fakeDocumentService) UploadLocalDocuments(kb *entity.Knowledgebase, tenantID string, files []*multipart.FileHeader, parentPath string, parserConfigOverride map[string]interface{}) ([]map[string]interface{}, []string) {
	return f.uploadLocalData, f.uploadLocalErrs
}

func (f *fakeDocumentService) UploadEmptyDocument(kb *entity.Knowledgebase, tenantID, name string) (map[string]interface{}, common.ErrorCode, error) {
	return f.uploadEmptyData, f.uploadEmptyCode, f.uploadEmptyErr
}

func (f *fakeDocumentService) UploadWebDocument(kb *entity.Knowledgebase, tenantID, name, url string) (map[string]interface{}, common.ErrorCode, error) {
	return f.uploadWebData, f.uploadWebCode, f.uploadWebErr
}

// uploadHandler wires a fake document service to a real dataset service backed
// by the in-memory access DB (kb "ds-1" owned by user-1).
func uploadHandler(t *testing.T, fake *fakeDocumentService) *DocumentHandler {
	t.Helper()
	db := setupHandlerAccessDB(t)
	// A second dataset owned by another user, to exercise the no-permission path.
	db.Create(&entity.Knowledgebase{
		ID: "ds-other", TenantID: "tenant-2", Name: "other-kb", EmbdID: "embd-1",
		CreatedBy: "user-2", Permission: string(entity.TenantPermissionMe),
		Status: sptr(string(entity.StatusValid)),
	})
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })
	return &DocumentHandler{documentService: fake, datasetService: service.NewDatasetService()}
}

func uploadCtx(t *testing.T, datasetID, query string, contentType string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	path := "/api/v1/datasets/" + datasetID + "/documents"
	if query != "" {
		path += "?" + query
	}
	c.Request = httptest.NewRequest("POST", path, bytes.NewReader(body))
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "dataset_id", Value: datasetID}}
	return c, w
}

func multipartFile(field, filename string, content []byte) ([]byte, string) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile(field, filename)
	_, _ = fw.Write(content)
	_ = mw.Close()
	return buf.Bytes(), mw.FormDataContentType()
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	return body
}

func TestUploadDocuments_UnknownType_ReturnsArgumentError(t *testing.T) {
	h := uploadHandler(t, &fakeDocumentService{})
	c, w := uploadCtx(t, "ds-1", "type=bogus", "", nil)
	h.UploadDocuments(c)

	body := decodeBody(t, w)
	if body["code"] != float64(common.CodeArgumentError) {
		t.Errorf("code=%v want %d", body["code"], common.CodeArgumentError)
	}
	if msg, _ := body["message"].(string); !strings.Contains(msg, `must be one of "local", "web", or "empty"`) {
		t.Errorf("message=%q", msg)
	}
}

func TestUploadDocuments_DatasetNotFound(t *testing.T) {
	h := uploadHandler(t, &fakeDocumentService{})
	c, w := uploadCtx(t, "no-such-ds", "", "", nil)
	h.UploadDocuments(c)

	body := decodeBody(t, w)
	if body["code"] != float64(common.CodeDataError) {
		t.Errorf("code=%v want %d", body["code"], common.CodeDataError)
	}
	if msg, _ := body["message"].(string); !strings.Contains(msg, "Can't find the dataset with ID") {
		t.Errorf("message=%q", msg)
	}
}

func TestUploadDocuments_NoPermission(t *testing.T) {
	h := uploadHandler(t, &fakeDocumentService{})
	// ds-other exists but is owned by user-2 → existence passes, permission fails.
	c, w := uploadCtx(t, "ds-other", "", "", nil)
	h.UploadDocuments(c)

	body := decodeBody(t, w)
	if body["code"] != float64(common.CodeAuthenticationError) {
		t.Errorf("code=%v want %d (existence-before-permission)", body["code"], common.CodeAuthenticationError)
	}
}

func TestUploadDocuments_Local_NoFilePart(t *testing.T) {
	h := uploadHandler(t, &fakeDocumentService{})
	body, ct := multipartFile("notfile", "x.txt", []byte("hi"))
	c, w := uploadCtx(t, "ds-1", "", ct, body)
	h.UploadDocuments(c)

	resp := decodeBody(t, w)
	if resp["code"] != float64(common.CodeArgumentError) {
		t.Errorf("code=%v want %d", resp["code"], common.CodeArgumentError)
	}
	if msg, _ := resp["message"].(string); msg != "No file part!" {
		t.Errorf("message=%q want 'No file part!'", msg)
	}
}

func TestUploadDocuments_Local_Success_MappedKeys(t *testing.T) {
	fake := &fakeDocumentService{
		uploadLocalData: []map[string]interface{}{
			{"id": "doc-1", "name": "a.txt", "kb_id": "ds-1", "parser_id": "naive", "chunk_num": int64(0), "token_num": int64(0), "type": "doc", "size": int64(2)},
		},
	}
	h := uploadHandler(t, fake)
	body, ct := multipartFile("file", "a.txt", []byte("hi"))
	c, w := uploadCtx(t, "ds-1", "", ct, body)
	h.UploadDocuments(c)

	resp := decodeBody(t, w)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("code=%v want 0 body=%s", resp["code"], w.Body.String())
	}
	arr, ok := resp["data"].([]interface{})
	if !ok || len(arr) != 1 {
		t.Fatalf("data is not a 1-element array: %v", resp["data"])
	}
	d := arr[0].(map[string]interface{})
	if d["dataset_id"] != "ds-1" || d["chunk_method"] != "naive" || d["run"] != "UNSTART" {
		t.Errorf("mapped keys wrong: %v", d)
	}
	for _, raw := range []string{"chunk_num", "kb_id", "token_num", "parser_id"} {
		if _, present := d[raw]; present {
			t.Errorf("raw key %q should not be present in mapped response", raw)
		}
	}
	if _, ok := d["chunk_count"]; !ok {
		t.Errorf("chunk_count missing")
	}
}

func TestUploadDocuments_Local_ServiceError(t *testing.T) {
	fake := &fakeDocumentService{uploadLocalErrs: []string{"a.txt: boom"}}
	h := uploadHandler(t, fake)
	body, ct := multipartFile("file", "a.txt", []byte("hi"))
	c, w := uploadCtx(t, "ds-1", "", ct, body)
	h.UploadDocuments(c)

	resp := decodeBody(t, w)
	if resp["code"] != float64(common.CodeServerError) {
		t.Errorf("code=%v want %d", resp["code"], common.CodeServerError)
	}
}

func TestUploadDocuments_Empty_BlankName(t *testing.T) {
	h := uploadHandler(t, &fakeDocumentService{})
	c, w := uploadCtx(t, "ds-1", "type=empty", "application/json", []byte(`{}`))
	h.UploadDocuments(c)

	resp := decodeBody(t, w)
	if resp["code"] != float64(common.CodeArgumentError) {
		t.Errorf("code=%v want %d", resp["code"], common.CodeArgumentError)
	}
	if msg, _ := resp["message"].(string); msg != "File name can't be empty." {
		t.Errorf("message=%q", msg)
	}
}

func TestUploadDocuments_Empty_Success(t *testing.T) {
	fake := &fakeDocumentService{
		uploadEmptyData: map[string]interface{}{"id": "doc-9", "name": "v", "kb_id": "ds-1", "parser_id": "naive", "type": "virtual", "size": int64(0), "chunk_num": int64(0), "token_num": int64(0)},
		uploadEmptyCode: common.CodeSuccess,
	}
	h := uploadHandler(t, fake)
	c, w := uploadCtx(t, "ds-1", "type=empty", "application/json", []byte(`{"name":"v"}`))
	h.UploadDocuments(c)

	resp := decodeBody(t, w)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("code=%v want 0 body=%s", resp["code"], w.Body.String())
	}
	d, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data is not an object: %v", resp["data"])
	}
	if d["type"] != "virtual" || d["dataset_id"] != "ds-1" || d["chunk_method"] != "naive" || d["run"] != "UNSTART" {
		t.Errorf("empty doc mapped keys wrong: %v", d)
	}
}

func TestUploadDocuments_Web_MissingName(t *testing.T) {
	h := uploadHandler(t, &fakeDocumentService{})
	body, ct := multipartFile("url", "ignored", []byte("https://example.com"))
	c, w := uploadCtx(t, "ds-1", "type=web", ct, body)
	h.UploadDocuments(c)

	resp := decodeBody(t, w)
	if resp["code"] != float64(common.CodeArgumentError) {
		t.Errorf("code=%v want %d", resp["code"], common.CodeArgumentError)
	}
	if msg, _ := resp["message"].(string); !strings.Contains(msg, `Lack of "name"`) {
		t.Errorf("message=%q", msg)
	}
}
