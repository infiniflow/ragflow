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
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/entity"
)

// fakeAgentFileService satisfies the agentFileService interface declared
// in agent.go. It returns canned bytes / errors for DownloadAgentFile so
// download-handler tests can run without a real FileService or MinIO.
// UploadInfos returns whatever is in `uploadList` so the upload tests
// can assert the response payload shape. UploadFromURL returns whatever
// is in `urlUpload` (or `urlUploadErr`) for the ?url= import path.
type fakeAgentFileService struct {
	blob         []byte
	err          error
	uploadList   []map[string]interface{}
	uploadErr    error
	urlUpload    map[string]interface{}
	urlUploadErr error
}

func (f *fakeAgentFileService) UploadInfos(_ string, _ []*multipart.FileHeader) ([]map[string]interface{}, error) {
	if f.uploadErr != nil {
		return nil, f.uploadErr
	}
	return f.uploadList, nil
}
func (f *fakeAgentFileService) UploadFromURL(_ string, _ string) (map[string]interface{}, error) {
	if f.urlUploadErr != nil {
		return nil, f.urlUploadErr
	}
	return f.urlUpload, nil
}
func (f *fakeAgentFileService) DownloadAgentFile(_ string, _ string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.blob, nil
}

// downloadCtx builds a Gin context for the /agents/download endpoint with
// optional id query parameter and a stubbed authenticated user.
func downloadCtx(t *testing.T, id string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	url := "/api/v1/agents/download"
	if id != "" {
		url += "?id=" + id
	}
	c.Request = httptest.NewRequest("GET", url, nil)
	c.Set("user", &entity.User{ID: "user-1"})
	return c, w
}

// TestDownloadAgentFile_HappyPath verifies that a known id returns the
// raw bytes with application/octet-stream content type and a sane
// Content-Disposition header.
func TestDownloadAgentFile_HappyPath(t *testing.T) {
	h := &AgentHandler{fileService: &fakeAgentFileService{blob: []byte("hello world")}}

	c, w := downloadCtx(t, "file-123")
	h.DownloadAgentFile(c)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != "hello world" {
		t.Errorf("body = %q, want %q", w.Body.String(), "hello world")
	}
	if got := w.Header().Get("Content-Disposition"); !strings.Contains(got, "file-123") {
		t.Errorf("Content-Disposition = %q, want filename=file-123", got)
	}
	if got := w.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", got)
	}
}

// TestDownloadAgentFile_MissingID verifies that an empty id is rejected
// with a 102-shaped envelope.
func TestDownloadAgentFile_MissingID(t *testing.T) {
	h := &AgentHandler{fileService: &fakeAgentFileService{}}

	c, w := downloadCtx(t, "") // no id
	h.DownloadAgentFile(c)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200 (envelope returned in body)", w.Code)
	}
	code, _ := errBody(t, w.Body.Bytes())
	if code != 101 { // CodeArgumentError
		t.Errorf("code = %d, want 101", code)
	}
}

// TestDownloadAgentFile_LoaderError verifies that a storage error
// bubbles up as a 500-class envelope.
func TestDownloadAgentFile_LoaderError(t *testing.T) {
	h := &AgentHandler{fileService: &fakeAgentFileService{err: errors.New("minio down")}}

	c, w := downloadCtx(t, "file-1")
	h.DownloadAgentFile(c)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200 (envelope in body)", w.Code)
	}
	code, msg := errBody(t, w.Body.Bytes())
	if code != 500 { // CodeServerError
		t.Errorf("code = %d, want 500", code)
	}
	if !strings.Contains(msg, "minio down") {
		t.Errorf("msg = %q, want contains 'minio down'", msg)
	}
}
