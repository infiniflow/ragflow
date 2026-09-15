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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// mockFileCommitSvc implements FileCommitServiceInterface for testing
type mockFileCommitSvc struct {
	createCommitFn          func(ctx context.Context, folderID, authorID, message string, changes []entity.FileChange) (*entity.FileCommit, error)
	listCommitsFn           func(ctx context.Context, folderID string, page, pageSize int, orderBy string, desc bool) ([]*entity.FileCommit, int64, error)
	getCommitFn             func(ctx context.Context, commitID string) (*entity.FileCommit, error)
	listCommitFilesFn       func(ctx context.Context, commitID string) ([]*entity.FileCommitItem, error)
	diffCommitsFn           func(ctx context.Context, fromID, toID string) ([]entity.DiffEntry, error)
	getUncommittedChangesFn func(ctx context.Context, folderID string) ([]entity.DiffEntry, error)
	getCommitTreeFn         func(ctx context.Context, commitID string) (map[string]interface{}, error)
	getCommitFileContentFn  func(ctx context.Context, folderID, commitID, fileID string) ([]byte, error)
	getFileVersionHistoryFn func(ctx context.Context, fileID string) ([]entity.VersionEntry, error)
	listPageCommitsFn       func(ctx context.Context, datasetID, pageType, slug string, page, pageSize int) ([]*entity.WikiPageCommit, int64, error)
}

func (m *mockFileCommitSvc) CreateCommit(ctx context.Context, folderID, authorID, message string, changes []entity.FileChange) (*entity.FileCommit, error) {
	if m.createCommitFn != nil {
		return m.createCommitFn(ctx, folderID, authorID, message, changes)
	}
	return &entity.FileCommit{
		ID:        "commit-1",
		FolderID:  folderID,
		Message:   message,
		AuthorID:  authorID,
		FileCount: len(changes),
	}, nil
}

func (m *mockFileCommitSvc) ListCommits(ctx context.Context, folderID string, page, pageSize int, orderBy string, desc bool) ([]*entity.FileCommit, int64, error) {
	if m.listCommitsFn != nil {
		return m.listCommitsFn(ctx, folderID, page, pageSize, orderBy, desc)
	}
	now := int64(1718200000000)
	return []*entity.FileCommit{
		{ID: "c2", FolderID: folderID, Message: "second", AuthorID: "u1", FileCount: 1, BaseModel: entity.BaseModel{CreateTime: &now}},
		{ID: "c1", FolderID: folderID, Message: "first", AuthorID: "u1", FileCount: 2, BaseModel: entity.BaseModel{CreateTime: &now}},
	}, 2, nil
}

func (m *mockFileCommitSvc) GetCommit(ctx context.Context, commitID string) (*entity.FileCommit, error) {
	if m.getCommitFn != nil {
		return m.getCommitFn(ctx, commitID)
	}
	return &entity.FileCommit{ID: commitID, FolderID: "folder-1", Message: "test commit", AuthorID: "u1", FileCount: 1}, nil
}

func (m *mockFileCommitSvc) ListCommitFiles(ctx context.Context, commitID string) ([]*entity.FileCommitItem, error) {
	if m.listCommitFilesFn != nil {
		return m.listCommitFilesFn(ctx, commitID)
	}
	return []*entity.FileCommitItem{
		{ID: "i1", CommitID: commitID, FileID: "f1", Operation: "add"},
	}, nil
}

func (m *mockFileCommitSvc) DiffCommits(ctx context.Context, fromID, toID string) ([]entity.DiffEntry, error) {
	if m.diffCommitsFn != nil {
		return m.diffCommitsFn(ctx, fromID, toID)
	}
	return []entity.DiffEntry{
		{FileID: "f1", FileName: "file.txt", Operation: "modify"},
	}, nil
}

func (m *mockFileCommitSvc) GetUncommittedChanges(ctx context.Context, folderID string) ([]entity.DiffEntry, error) {
	if m.getUncommittedChangesFn != nil {
		return m.getUncommittedChangesFn(ctx, folderID)
	}
	return []entity.DiffEntry{
		{FileID: "f1", FileName: "new.txt", Operation: "add"},
	}, nil
}

func (m *mockFileCommitSvc) GetCommitTree(ctx context.Context, commitID string) (map[string]interface{}, error) {
	if m.getCommitTreeFn != nil {
		return m.getCommitTreeFn(ctx, commitID)
	}
	return map[string]interface{}{
		"f1": map[string]interface{}{"name": "file.txt", "hash": "abc123", "size": 100, "status": "1"},
	}, nil
}

func (m *mockFileCommitSvc) GetCommitFileContent(ctx context.Context, folderID, commitID, fileID string) ([]byte, error) {
	if m.getCommitFileContentFn != nil {
		return m.getCommitFileContentFn(ctx, folderID, commitID, fileID)
	}
	return []byte("file content"), nil
}

func (m *mockFileCommitSvc) GetFileVersionHistory(ctx context.Context, fileID string) ([]entity.VersionEntry, error) {
	if m.getFileVersionHistoryFn != nil {
		return m.getFileVersionHistoryFn(ctx, fileID)
	}
	now := int64(1718200000000)
	return []entity.VersionEntry{
		{CommitID: "c2", Operation: "modify", Hash: "def456", CreateTime: &now, Message: "updated"},
		{CommitID: "c1", Operation: "add", Hash: "abc123", CreateTime: &now, Message: "initial"},
	}, nil
}

func (m *mockFileCommitSvc) ListPageCommits(ctx context.Context, datasetID, pageType, slug string, page, pageSize int) ([]*entity.WikiPageCommit, int64, error) {
	if m.listPageCommitsFn != nil {
		return m.listPageCommitsFn(ctx, datasetID, pageType, slug, page, pageSize)
	}
	return []*entity.WikiPageCommit{}, 0, nil
}

func setupFileCommitTest(userID string) (*gin.Engine, *mockFileCommitSvc) {
	r, mock, _ := setupFileCommitTestWithHandler(userID)
	return r, mock
}

func setupFileCommitTestWithHandler(userID string) (*gin.Engine, *mockFileCommitSvc, *FileCommitHandler) {
	mock := &mockFileCommitSvc{}
	h := NewFileCommitHandler(mock)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		panic(err)
	}
	if err := db.AutoMigrate(&entity.File{}); err != nil {
		panic(err)
	}
	dao.DB = db
	// Seed the workspace folder and file the tests reference, owned by the
	// calling user so the tenant authorization passes for them and fails for
	// anyone else. Panic on seed failures so a broken fixture reports its
	// real cause instead of surfacing as a spurious CodeNotFound.
	seed := func(f *entity.File) {
		if err := db.Create(f).Error; err != nil {
			panic(err)
		}
	}
	seed(&entity.File{ID: "folder-1", TenantID: userID, Name: "workspace", Type: "folder"})
	seed(&entity.File{ID: "f1", TenantID: userID, Name: "test.txt", Type: "file"})
	// The mirrored /datasets/{dataset_id}/commits test route maps dataset_id
	// straight onto folder_id, so seed kb-1 as a folder the user owns to pass
	// the folder authorization main added.
	seed(&entity.File{ID: "kb-1", TenantID: userID, Name: "kb-1", Type: "folder"})

	gin.SetMode(gin.TestMode)
	r := fileCommitRouter(h, userID)
	return r, mock, h
}

func fileCommitRouter(h *FileCommitHandler, userID string) *gin.Engine {
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
	// Mirrors the CommitFolderResolver middleware used on the production
	// /datasets/{dataset_id}/commits route: dataset_id resolves to folder_id.
	r.GET("/api/v1/datasets/:dataset_id/commits", func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "folder_id", Value: c.Param("dataset_id")})
		c.Next()
	}, h.ListCommits)
	return r
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
	mock.createCommitFn = func(ctx context.Context, folderID, authorID, message string, changes []entity.FileChange) (*entity.FileCommit, error) {
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
	mock.listCommitsFn = func(ctx context.Context, folderID string, page, pageSize int, orderBy string, desc bool) ([]*entity.FileCommit, int64, error) {
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

func TestFileCommit_ListCommits_PageSlugFilter(t *testing.T) {
	r, mock := setupFileCommitTest("user-1")
	var gotPageType, gotSlug string
	mock.listPageCommitsFn = func(ctx context.Context, datasetID, pageType, slug string, page, pageSize int) ([]*entity.WikiPageCommit, int64, error) {
		if datasetID != "kb-1" {
			t.Errorf("expected dataset kb-1, got %s", datasetID)
		}
		gotPageType, gotSlug = pageType, slug
		return []*entity.WikiPageCommit{
			{ID: "c2", Title: "second edit", UserID: "user-1", UserNickname: "Tester"},
			{ID: "c1", Title: "first edit", UserID: "user-1", UserNickname: "Tester"},
		}, 2, nil
	}

	// The frontend sends the full "<page_type>/<slug>" form, exactly like the
	// Python API contract (?slug=<page_type>/<name>).
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/datasets/kb-1/commits?slug=topic%2Ffireworks+display", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotPageType != "topic" {
		t.Errorf("expected page type topic, got %q", gotPageType)
	}
	if gotSlug != "fireworks display" {
		t.Errorf("expected bare slug \"fireworks display\", got %q", gotSlug)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Total   int `json:"total"`
			Commits []struct {
				ID           string `json:"id"`
				Title        string `json:"title"`
				UserID       string `json:"user_id"`
				UserNickname string `json:"user_nickname"`
			} `json:"commits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Data.Total != 2 || len(resp.Data.Commits) != 2 {
		t.Fatalf("expected 2 commits, got total=%d len=%d", resp.Data.Total, len(resp.Data.Commits))
	}
	if resp.Data.Commits[0].Title != "second edit" || resp.Data.Commits[0].UserNickname != "Tester" {
		t.Errorf("unexpected first commit: %+v", resp.Data.Commits[0])
	}
}

func TestFileCommit_ListCommits_PageSlugWithExplicitPageType(t *testing.T) {
	r, mock := setupFileCommitTest("user-1")
	var gotPageType, gotSlug string
	mock.listPageCommitsFn = func(ctx context.Context, datasetID, pageType, slug string, page, pageSize int) ([]*entity.WikiPageCommit, int64, error) {
		gotPageType, gotSlug = pageType, slug
		return []*entity.WikiPageCommit{}, 0, nil
	}

	// Explicit page_type plus a bare slug must also work.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/datasets/kb-1/commits?page_type=topic&slug=fireworks+display", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotPageType != "topic" || gotSlug != "fireworks display" {
		t.Errorf("expected topic / fireworks display, got %q / %q", gotPageType, gotSlug)
	}
}

func TestFileCommit_ListCommits_PageSlugNestedTopicPath(t *testing.T) {
	r, mock := setupFileCommitTest("user-1")
	var gotPageType, gotSlug string
	mock.listPageCommitsFn = func(ctx context.Context, datasetID, pageType, slug string, page, pageSize int) ([]*entity.WikiPageCommit, int64, error) {
		gotPageType, gotSlug = pageType, slug
		return []*entity.WikiPageCommit{}, 0, nil
	}

	// Nested page paths (PUT /artifacts/topic/People/Writers) arrive as
	// ?slug=topic/People/Writers: only the first segment is the page type and
	// the remainder stays part of the slug, mirroring UpdateArtifact's split
	// when it derives the page file key on the write side.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/datasets/kb-1/commits?slug=topic%2FPeople%2FWriters", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotPageType != "topic" || gotSlug != "People/Writers" {
		t.Errorf("expected topic / People/Writers, got %q / %q", gotPageType, gotSlug)
	}

	// With an explicit page_type the matching prefix is trimmed once and the
	// nested remainder is preserved verbatim.
	gotPageType, gotSlug = "", ""
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/datasets/kb-1/commits?page_type=topic&slug=topic%2FPeople%2FWriters", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotPageType != "topic" || gotSlug != "People/Writers" {
		t.Errorf("expected topic / People/Writers, got %q / %q", gotPageType, gotSlug)
	}
}

func TestFileCommit_ListCommits_PageSlugWithoutDatasetScope(t *testing.T) {
	r, _ := setupFileCommitTest("user-1")

	// ?slug= scopes the history to a dataset's page key; on a folder route
	// there is no dataset scope to resolve it against.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/folders/folder-1/commits?slug=topic%2Fname", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if code := decodeCode(t, w); code != float64(common.CodeArgumentError) {
		t.Errorf("expected code %d, got %v", common.CodeArgumentError, code)
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
	mock.getCommitFn = func(ctx context.Context, commitID string) (*entity.FileCommit, error) {
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
	mock.getCommitFileContentFn = func(ctx context.Context, folderID, commitID, fileID string) ([]byte, error) {
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

// ── Tenant authorization tests ───────────────────────────────────────────

func decodeCode(t *testing.T, w *httptest.ResponseRecorder) float64 {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	code, _ := resp["code"].(float64)
	return code
}

func TestFileCommit_ForeignFolderRejected(t *testing.T) {
	// folder-1 is seeded for user-1; user-2 must not read or write it.
	_, _, h := setupFileCommitTestWithHandler("user-1")
	foreign := fileCommitRouter(h, "user-2")

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/v1/folders/folder-1/commits", ""},
		{"GET", "/api/v1/folders/folder-1/changes", ""},
		{"GET", "/api/v1/folders/folder-1/commits/commit-1", ""},
		{"GET", "/api/v1/folders/folder-1/commits/commit-1/files", ""},
		{"GET", "/api/v1/folders/folder-1/commits/commit-1/tree", ""},
		{"GET", "/api/v1/folders/folder-1/commits/commit-1/files/f1/content", ""},
		{"GET", "/api/v1/folders/folder-1/commits/diff?from=c1&to=c2", ""},
		{"POST", "/api/v1/folders/folder-1/commits", `{"message": "x", "files": [{"file_id": "f1", "file_name": "t", "operation": "add", "content": "c"}]}`},
		{"GET", "/api/v1/files/f1/versions", ""},
	}

	for _, e := range endpoints {
		w := httptest.NewRecorder()
		var req *http.Request
		if e.body != "" {
			req, _ = http.NewRequest(e.method, e.path, strings.NewReader(e.body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req, _ = http.NewRequest(e.method, e.path, nil)
		}
		foreign.ServeHTTP(w, req)
		if code := decodeCode(t, w); code != float64(common.CodeNotFound) {
			t.Errorf("%s %s: expected code %d for foreign user, got %v: %s", e.method, e.path, common.CodeNotFound, code, w.Body.String())
		}
	}
}

func TestFileCommit_MissingFolderRejected(t *testing.T) {
	r, _ := setupFileCommitTest("user-1")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/folders/no-such-folder/commits", nil)
	r.ServeHTTP(w, req)
	if code := decodeCode(t, w); code != float64(common.CodeNotFound) {
		t.Errorf("expected code %d for unknown folder, got %v", common.CodeNotFound, code)
	}
}

func TestFileCommit_OwnFolderStillWorks(t *testing.T) {
	r, _ := setupFileCommitTest("user-1")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/folders/folder-1/commits", nil)
	r.ServeHTTP(w, req)
	if code := decodeCode(t, w); code != float64(common.CodeSuccess) {
		t.Errorf("expected code %d for owner, got %v: %s", common.CodeSuccess, code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/files/f1/versions", nil)
	r.ServeHTTP(w, req)
	if code := decodeCode(t, w); code != float64(common.CodeSuccess) {
		t.Errorf("expected code %d for owner version history, got %v: %s", common.CodeSuccess, code, w.Body.String())
	}
}
