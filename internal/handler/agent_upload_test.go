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
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// uploadFakes is a paired pair of fakes used by the upload-handler
// tests. The canvasLoader fake grants/denies access per case; the
// fileService fake records the call and returns a canned descriptor.
type uploadFakes struct {
	loader  *fakeCanvasLoader
	fileSvc *fakeAgentFileService
}

func (u *uploadFakes) setUploadResult(list []map[string]interface{}) {
	u.fileSvc.uploadList = list
}

// makeUploadCtx builds a Gin context with a multipart body containing
// `n` files under the "file" field. Each file's body is the
// decimal-string representation of its 1-based index.
func makeUploadCtx(t *testing.T, n int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	for i := 1; i <= n; i++ {
		name := "f" + strconv.Itoa(i) + ".txt"
		fw, err := mw.CreateFormFile("file", name)
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := fw.Write([]byte(name)); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	mw.Close()

	req := httptest.NewRequest("POST", "/api/v1/agents/c1/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	c.Request = req
	c.Params = gin.Params{{Key: "canvas_id", Value: "c1"}}
	c.Set("user", &entity.User{ID: "u-1"})
	return c, w
}

// makeUploadCtxNoFile builds a Gin context with a multipart body that
// has no "file" field (used for the missing-file test).
func makeUploadCtxNoFile(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	// "other" field, not "file"
	fw, _ := mw.CreateFormFile("other", "o.txt")
	fw.Write([]byte("o"))
	mw.Close()
	req := httptest.NewRequest("POST", "/api/v1/agents/c1/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	c.Request = req
	c.Params = gin.Params{{Key: "canvas_id", Value: "c1"}}
	c.Set("user", &entity.User{ID: "u-1"})
	return c, w
}

// TestUploadAgentFile_SingleFile pins the 1-file → single-dict
// response shape (python agent_api.py:775-779).
func TestUploadAgentFile_SingleFile(t *testing.T) {
	loader := &fakeCanvasLoader{canvas: &entity.UserCanvas{ID: "c1"}}
	fu := &uploadFakes{
		loader:  loader,
		fileSvc: &fakeAgentFileService{},
	}
	fu.setUploadResult([]map[string]interface{}{{"id": "upload-1", "name": "f1.txt"}})
	h := &AgentHandler{loader: fu.loader, fileService: fu.fileSvc}

	c, w := makeUploadCtx(t, 1)
	h.UploadAgentFile(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	// Single-file path returns data as a single dict, NOT a list.
	var env struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if env.Code != 0 {
		t.Errorf("code = %d, want 0", env.Code)
	}
	if env.Data["id"] != "upload-1" {
		t.Errorf("data.id = %v, want upload-1", env.Data["id"])
	}
}

// TestUploadAgentFile_MultiFile pins the >1-file → list-of-dicts
// response shape (python agent_api.py:780-783).
func TestUploadAgentFile_MultiFile(t *testing.T) {
	loader := &fakeCanvasLoader{canvas: &entity.UserCanvas{ID: "c1"}}
	fu := &uploadFakes{loader: loader, fileSvc: &fakeAgentFileService{}}
	fu.setUploadResult([]map[string]interface{}{
		{"id": "u1", "name": "f1.txt"},
		{"id": "u2", "name": "f2.txt"},
		{"id": "u3", "name": "f3.txt"},
	})
	h := &AgentHandler{loader: fu.loader, fileService: fu.fileSvc}

	c, w := makeUploadCtx(t, 3)
	h.UploadAgentFile(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Code int                      `json:"code"`
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if env.Code != 0 {
		t.Errorf("code = %d, want 0", env.Code)
	}
	if len(env.Data) != 3 {
		t.Errorf("data has %d items, want 3", len(env.Data))
	}
}

// TestUploadAgentFile_MissingFileField verifies that a multipart body
// without a "file" field is rejected with a 101 envelope.
func TestUploadAgentFile_MissingFileField(t *testing.T) {
	loader := &fakeCanvasLoader{canvas: &entity.UserCanvas{ID: "c1"}}
	h := &AgentHandler{loader: loader, fileService: &fakeAgentFileService{}}

	c, w := makeUploadCtxNoFile(t)
	h.UploadAgentFile(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != 101 { // CodeArgumentError
		t.Errorf("code = %d, want 101; msg=%q", code, msg)
	}
	if !strings.Contains(msg, "file") {
		t.Errorf("msg = %q, want mention of 'file'", msg)
	}
}

// TestUploadAgentFile_CannotAccessCanvas verifies that an
// ErrUserCanvasNotFound from the loader short-circuits with a 103
// envelope carrying the python permission-failure message
// (matches @_require_canvas_access_async at agent_api.py:78,89).
func TestUploadAgentFile_CannotAccessCanvas(t *testing.T) {
	loader := &fakeCanvasLoader{err: dao.ErrUserCanvasNotFound}
	h := &AgentHandler{loader: loader, fileService: &fakeAgentFileService{}}

	c, w := makeUploadCtx(t, 1)
	h.UploadAgentFile(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != 103 { // CodeOperatingError
		t.Errorf("code = %d, want 103; msg=%q", code, msg)
	}
	want := "Make sure you have permission to access the agent."
	if msg != want {
		t.Errorf("msg = %q, want %q", msg, want)
	}
}

// TestUploadAgentFile_URLImport pins the ?url= import path (python
// agent_api.py:775-779). When ?url= is set AND the body has exactly
// one `file` field, the handler delegates to FileService.UploadFromURL
// and returns its single-dict result.
func TestUploadAgentFile_URLImport(t *testing.T) {
	loader := &fakeCanvasLoader{canvas: &entity.UserCanvas{ID: "c1"}}
	fu := &uploadFakes{loader: loader, fileSvc: &fakeAgentFileService{}}
	fu.fileSvc.urlUpload = map[string]interface{}{"id": "url-1", "name": "remote.bin"}
	h := &AgentHandler{loader: fu.loader, fileService: fu.fileSvc}

	c, w := makeUploadCtx(t, 1)
	c.Request.URL.RawQuery = "url=https%3A%2F%2Fexample.com%2Ffile.bin"
	h.UploadAgentFile(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if env.Code != 0 {
		t.Errorf("code = %d, want 0", env.Code)
	}
	if env.Data["id"] != "url-1" {
		t.Errorf("data.id = %v, want url-1", env.Data["id"])
	}
}

// TestUploadAgentFile_URLImport_IgnoredForMultiFile pins that
// ?url= is silently ignored when the body has >1 files, matching
// python's behaviour at agent_api.py:780-783 (the multi-file branch
// never reads url). The request flows into the normal UploadInfos
// path and returns a list of dicts.
func TestUploadAgentFile_URLImport_IgnoredForMultiFile(t *testing.T) {
	loader := &fakeCanvasLoader{canvas: &entity.UserCanvas{ID: "c1"}}
	fu := &uploadFakes{loader: loader, fileSvc: &fakeAgentFileService{}}
	fu.fileSvc.urlUpload = map[string]interface{}{"id": "should-not-appear"}
	fu.setUploadResult([]map[string]interface{}{
		{"id": "u1", "name": "f1.txt"},
		{"id": "u2", "name": "f2.txt"},
		{"id": "u3", "name": "f3.txt"},
	})
	h := &AgentHandler{loader: fu.loader, fileService: fu.fileSvc}

	c, w := makeUploadCtx(t, 3)
	c.Request.URL.RawQuery = "url=https%3A%2F%2Fexample.com%2Ffile.bin"
	h.UploadAgentFile(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Code int                      `json:"code"`
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	if env.Code != 0 {
		t.Errorf("code = %d, want 0", env.Code)
	}
	if len(env.Data) != 3 {
		t.Errorf("data has %d items, want 3 (url was ignored)", len(env.Data))
	}
	// Sanity check: the urlUpload map should NOT have been consumed.
	// We don't read it back, but the test setup proves the
	// handler didn't dispatch to UploadFromURL (otherwise it would
	// have returned the single urlUpload dict, not the 3-element
	// list from uploadList).
}

// TestUploadAgentFile_URLImport_LoaderError pins that a failed URL
// fetch surfaces as a 500 envelope (matches python's
// `server_error_response(exc)` on line 784).
func TestUploadAgentFile_URLImport_LoaderError(t *testing.T) {
	loader := &fakeCanvasLoader{canvas: &entity.UserCanvas{ID: "c1"}}
	fu := &uploadFakes{loader: loader, fileSvc: &fakeAgentFileService{urlUploadErr: errors.New("ssrf guard tripped")}}
	h := &AgentHandler{loader: fu.loader, fileService: fu.fileSvc}

	c, w := makeUploadCtx(t, 1)
	c.Request.URL.RawQuery = "url=https%3A%2F%2Finternal.example.com%2Fsecret"
	h.UploadAgentFile(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != 500 { // CodeServerError
		t.Errorf("code = %d, want 500; msg=%q", code, msg)
	}
	if !strings.Contains(msg, "ssrf guard tripped") {
		t.Errorf("msg = %q, want contains 'ssrf guard tripped'", msg)
	}
}

// TestUploadAgentFile_LoaderError verifies that a non-ErrUserCanvasNotFound
// service error surfaces as a 500 envelope with the error string.
func TestUploadAgentFile_LoaderError(t *testing.T) {
	loader := &fakeCanvasLoader{err: context.DeadlineExceeded}
	h := &AgentHandler{loader: loader, fileService: &fakeAgentFileService{}}

	c, w := makeUploadCtx(t, 1)
	h.UploadAgentFile(c)

	code, msg := errBody(t, w.Body.Bytes())
	if code != 500 { // CodeServerError
		t.Errorf("code = %d, want 500; msg=%q", code, msg)
	}
	if !strings.Contains(msg, "deadline") {
		t.Errorf("msg = %q, want contains 'deadline'", msg)
	}
}
