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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"
)

// mockFileCommitSvc implements FileCommitServiceInterface for testing
type mockFileCommitSvc struct {
	createCommitFn                func(folderID, authorID, message string, changes []entity.FileChange) (*entity.FileCommit, error)
	listCommitsFn                 func(folderID string, page, pageSize int, orderBy string, desc bool) ([]*entity.FileCommit, int64, error)
	getCommitFn                   func(commitID string) (*entity.FileCommit, error)
	listCommitFilesFn             func(commitID string) ([]*entity.FileCommitItem, error)
	diffCommitsFn                 func(fromID, toID string) ([]entity.DiffEntry, error)
	getUncommittedChangesFn       func(folderID string) ([]entity.DiffEntry, error)
	getCommitTreeFn               func(commitID string) (map[string]interface{}, error)
	getCommitFileContentFn        func(folderID, commitID, fileID string) ([]byte, error)
	getFileVersionHistoryFn       func(fileID string) ([]entity.VersionEntry, error)
}

func (m *mockFileCommitSvc) CreateCommit(folderID, authorID, message string, changes []entity.FileChange) (*entity.FileCommit, error) {
	if m.createCommitFn != nil {
		return m.createCommitFn(folderID, authorID, message, changes)
	}
	return &entity.FileCommit{
		ID:        "commit-1",
		FolderID:  folderID,
		Message:   message,
		AuthorID:  authorID,
		FileCount: len(changes),
	}, nil
}

func (m *mockFileCommitSvc) ListCommits(folderID string, page, pageSize int, orderBy string, desc bool) ([]*entity.FileCommit, int64, error) {
	if m.listCommitsFn != nil {
		return m.listCommitsFn(folderID, page, pageSize, orderBy, desc)
	}
	now := int64(1718200000000)
	return []*entity.FileCommit{
		{ID: "c2", FolderID: folderID, Message: "second", AuthorID: "u1", FileCount: 1, BaseModel: entity.BaseModel{CreateTime: &now}},
		{ID: "c1", FolderID: folderID, Message: "first", AuthorID: "u1", FileCount: 2, BaseModel: entity.BaseModel{CreateTime: &now}},
	}, 2, nil
}

func (m *mockFileCommitSvc) GetCommit(commitID string) (*entity.FileCommit, error) {
	if m.getCommitFn != nil {
		return m.getCommitFn(commitID)
	}
	return &entity.FileCommit{ID: commitID, FolderID: "folder-1", Message: "test commit", AuthorID: "u1", FileCount: 1}, nil
}

func (m *mockFileCommitSvc) ListCommitFiles(commitID string) ([]*entity.FileCommitItem, error) {
	if m.listCommitFilesFn != nil {
		return m.listCommitFilesFn(commitID)
	}
	return []*entity.FileCommitItem{
		{ID: "i1", CommitID: commitID, FileID: "f1", Operation: "add"},
	}, nil
}

func (m *mockFileCommitSvc) DiffCommits(fromID, toID string) ([]entity.DiffEntry, error) {
	if m.diffCommitsFn != nil {
		return m.diffCommitsFn(fromID, toID)
	}
	return []entity.DiffEntry{
		{FileID: "f1", FileName: "file.txt", Operation: "modify"},
	}, nil
}

func (m *mockFileCommitSvc) GetUncommittedChanges(folderID string) ([]entity.DiffEntry, error) {
	if m.getUncommittedChangesFn != nil {
		return m.getUncommittedChangesFn(folderID)
	}
	return []entity.DiffEntry{
		{FileID: "f1", FileName: "new.txt", Operation: "add"},
	}, nil
}

func (m *mockFileCommitSvc) GetCommitTree(commitID string) (map[string]interface{}, error) {
	if m.getCommitTreeFn != nil {
		return m.getCommitTreeFn(commitID)
	}
	return map[string]interface{}{
		"f1": map[string]interface{}{"name": "file.txt", "hash": "abc123", "size": 100, "status": "1"},
	}, nil
}

func (m *mockFileCommitSvc) GetCommitFileContent(folderID, commitID, fileID string) ([]byte, error) {
	if m.getCommitFileContentFn != nil {
		return m.getCommitFileContentFn(folderID, commitID, fileID)
	}
	return []byte("file content"), nil
}

func (m *mockFileCommitSvc) GetFileVersionHistory(fileID string) ([]entity.VersionEntry, error) {
	if m.getFileVersionHistoryFn != nil {
		return m.getFileVersionHistoryFn(fileID)
	}
	now := int64(1718200000000)
	return []entity.VersionEntry{
		{CommitID: "c2", Operation: "modify", Hash: "def456", CreateTime: &now, Message: "updated"},
		{CommitID: "c1", Operation: "add", Hash: "abc123", CreateTime: &now, Message: "initial"},
	}, nil
}

func setupFileCommitTest(userID string) (*gin.Engine, *mockFileCommitSvc) {
	mock := &mockFileCommitSvc{}
	h := &FileCommitHandler{commitService: mock}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: userID})
	})
	r.POST("/api/v1/folders/:folder_id/commits", h.CreateCommit)
	r.GET("/api/v1/folders/:folder_id/commits", h.ListCommits)
	r.GET("/api/v1/folders/:folder_id/commits/:commit_id", h.GetCommit)
	r.GET("/api/v1/folders/:folder_id/commits/:commit_id/files", h.ListCommitFiles)
	r.GET("/api/v1/folders/:folder_id/commits/diff", h.DiffCommits)
	r.GET("/api/v1/folders/:folder_id/changes", h.GetUncommittedChanges)
	r.GET("/api/v1/folders/:folder_id/commits/:commit_id/tree", h.GetCommitTree)
	r.GET("/api/v1/folders/:folder_id/commits/:commit_id/files/:file_id/content", h.GetCommitFileContent)
	r.GET("/api/v1/files/:id/versions", h.GetFileVersionHistory)
	return r, mock
}

func setupFileCommitTestNoAuth() *gin.Engine {
	h := &FileCommitHandler{}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/folders/:folder_id/commits", h.CreateCommit)
	return r
}

// ── Tests ────────────────────────────────────────────────────────────────

func TestFileCommit_CreateCommit_Success(t *testing.T) {
	r, mock := setupFileCommitTest("user-1")
	mock.createCommitFn = func(folderID, authorID, message string, changes []entity.FileChange) (*entity.FileCommit, error) {
		if folderID != "folder-1" {
			t.Errorf("expected folder-1, got %s", folderID)
		}
		if authorID != "user-1" {
			t.Errorf("expected user-1, got %s", authorID)
		}
		if message != "initial commit" {
			t.Errorf("expected 'initial commit', got %s", message)
		}
		if len(changes) != 1 || changes[0].FileID != "f1" {
			t.Errorf("unexpected changes: %+v", changes)
		}
		now := int64(1718200000000)
		return &entity.FileCommit{
			ID: "commit-1", FolderID: folderID, Message: message,
			AuthorID: authorID, FileCount: len(changes),
			BaseModel: entity.BaseModel{CreateTime: &now},
		}, nil
	}

	body := `{"message": "initial commit", "files": [{"file_id": "f1", "file_name": "test.txt", "operation": "add", "content": "hello"}]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/folders/folder-1/commits", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected code 0, got %v", resp["code"])
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be object, got %T", resp["data"])
	}
	if data["message"] != "initial commit" {
		t.Errorf("expected 'initial commit', got %v", data["message"])
	}
}

func TestFileCommit_CreateCommit_NoAuth(t *testing.T) {
	r := setupFileCommitTestNoAuth()
	body := `{"message": "test", "files": [{"file_id": "f1", "file_name": "t.txt", "operation": "add"}]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/folders/folder-1/commits", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// No auth middleware → code 401
	if code, _ := resp["code"].(float64); code != float64(common.CodeUnauthorized) {
		t.Errorf("expected unauthorized, got code %v", code)
	}
}

func TestFileCommit_CreateCommit_InvalidJSON(t *testing.T) {
	r, _ := setupFileCommitTest("user-1")
	body := `{invalid json`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/folders/folder-1/commits", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if code, _ := resp["code"].(float64); code != float64(common.CodeBadRequest) {
		t.Errorf("expected bad request, got code %v", code)
	}
}

func TestFileCommit_ListCommits_Success(t *testing.T) {
	r, mock := setupFileCommitTest("user-1")
	mock.listCommitsFn = func(folderID string, page, pageSize int, orderBy string, desc bool) ([]*entity.FileCommit, int64, error) {
		if folderID != "folder-1" {
			t.Errorf("expected folder-1, got %s", folderID)
		}
		return []*entity.FileCommit{
			{ID: "c2", FolderID: folderID, Message: "second", AuthorID: "u1", FileCount: 1},
			{ID: "c1", FolderID: folderID, Message: "first", AuthorID: "u1", FileCount: 2},
		}, 2, nil
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/folders/folder-1/commits?page=1&page_size=10", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected code 0, got %v", resp["code"])
	}
	data, _ := resp["data"].(map[string]interface{})
	if total, _ := data["total"].(float64); total != 2 {
		t.Errorf("expected total 2, got %v", total)
	}
}

func TestFileCommit_GetCommit_Success(t *testing.T) {
	r, _ := setupFileCommitTest("user-1")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/folders/folder-1/commits/commit-1", nil)
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected success, got code %v: %s", resp["code"], resp["message"])
	}
	data, _ := resp["data"].(map[string]interface{})
	if data["id"] != "commit-1" {
		t.Errorf("expected commit-1, got %v", data["id"])
	}
}

func TestFileCommit_GetCommit_NotFound(t *testing.T) {
	r, mock := setupFileCommitTest("user-1")
	mock.getCommitFn = func(commitID string) (*entity.FileCommit, error) {
		return nil, common.ErrNotFound
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/folders/folder-1/commits/missing", nil)
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if code, _ := resp["code"].(float64); code != float64(common.CodeNotFound) {
		t.Errorf("expected 404, got code %v", code)
	}
}

func TestFileCommit_ListCommitFiles_Success(t *testing.T) {
	r, _ := setupFileCommitTest("user-1")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/folders/folder-1/commits/commit-1/files", nil)
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected success, got code %v", resp["code"])
	}
}

func TestFileCommit_DiffCommits_Success(t *testing.T) {
	r, _ := setupFileCommitTest("user-1")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/folders/folder-1/commits/diff?from=c1&to=c2", nil)
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected success, got code %v", resp["code"])
	}
}

func TestFileCommit_DiffCommits_MissingParams(t *testing.T) {
	r, _ := setupFileCommitTest("user-1")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/folders/folder-1/commits/diff", nil)
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if code, _ := resp["code"].(float64); code != float64(common.CodeParamError) {
		t.Errorf("expected param error, got code %v", code)
	}
}

func TestFileCommit_GetUncommittedChanges_Success(t *testing.T) {
	r, _ := setupFileCommitTest("user-1")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/folders/folder-1/changes", nil)
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected success, got code %v", resp["code"])
	}
}

func TestFileCommit_GetCommitTree_Success(t *testing.T) {
	r, _ := setupFileCommitTest("user-1")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/folders/folder-1/commits/commit-1/tree", nil)
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected success, got code %v", resp["code"])
	}
}

func TestFileCommit_GetCommitFileContent_Success(t *testing.T) {
	r, mock := setupFileCommitTest("user-1")
	mock.getCommitFileContentFn = func(folderID, commitID, fileID string) ([]byte, error) {
		return []byte("hello world"), nil
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/folders/folder-1/commits/commit-1/files/f1/content", nil)
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected success, got code %v", resp["code"])
	}
	data, _ := resp["data"].(map[string]interface{})
	if content, _ := data["content"].(string); content != "hello world" {
		t.Errorf("expected 'hello world', got %q", content)
	}
}

func TestFileCommit_GetFileVersionHistory_Success(t *testing.T) {
	r, _ := setupFileCommitTest("user-1")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/files/f1/versions", nil)
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected success, got code %v", resp["code"])
	}
}
