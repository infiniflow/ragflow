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
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// setupUploadTestDB sets up SQLite in-memory DB for upload handler tests.
func setupUploadTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&entity.User{},
		&entity.UserCanvas{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

// fakeUploadFileService implements fileUploader for tests.
type fakeUploadFileService struct {
	uploaded []map[string]interface{}
	err      error
}

func (f *fakeUploadFileService) UploadFile(tenantID, parentID string, files []*multipart.FileHeader) ([]map[string]interface{}, error) {
	return f.uploaded, f.err
}

// TestUploadAgentFileHandler_Success verifies the happy path.
func TestUploadAgentFileHandler_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUploadTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	db.Create(&entity.User{ID: "user-1", Nickname: "test", Email: "test@test.com"})
	db.Create(&entity.UserCanvas{ID: "canvas-1", UserID: "user-1", Title: sp("Test Agent")})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := strings.NewReader("--boundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"test.txt\"\r\nContent-Type: text/plain\r\n\r\nhello world\r\n--boundary--")
	req := httptest.NewRequest("POST", "/api/v1/agents/canvas-1/upload", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}}

	svc := &fakeUploadFileService{
		uploaded: []map[string]interface{}{
			{"id": "file-1", "name": "test.txt"},
		},
	}
	h := &AgentHandler{
		agentService: service.NewAgentService(),
		fileService:  svc,
	}
	h.UploadAgentFile(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code != float64(common.CodeSuccess) {
		t.Fatalf("expected code 0, got %v: %v", code, resp["message"])
	}
}

// TestUploadAgentFileHandler_NoPermission verifies cross-user access is denied.
func TestUploadAgentFileHandler_NoPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUploadTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	db.Create(&entity.User{ID: "user-a", Nickname: "a", Email: "a@test.com"})
	db.Create(&entity.UserCanvas{ID: "canvas-b", UserID: "user-b", Title: sp("Not Yours")})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/agents/canvas-b/upload", nil)
	c.Set("user", &entity.User{ID: "user-a"})
	c.Set("user_id", "user-a")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-b"}}

	h := &AgentHandler{
		agentService: service.NewAgentService(),
		fileService:  &fakeUploadFileService{},
	}
	h.UploadAgentFile(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code != float64(common.CodeOperatingError) {
		t.Errorf("expected operating error %d, got %v", common.CodeOperatingError, code)
	}
}

// TestUploadAgentFileHandler_NoFiles verifies empty file list is rejected.
func TestUploadAgentFileHandler_NoFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := setupUploadTestDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	db.Create(&entity.User{ID: "user-1", Nickname: "test", Email: "test@test.com"})
	db.Create(&entity.UserCanvas{ID: "canvas-1", UserID: "user-1", Title: sp("Test Agent")})

	body := strings.NewReader("--boundary\r\nContent-Disposition: form-data; name=\"dummy\"\r\n\r\nvalue\r\n--boundary--")
	req := httptest.NewRequest("POST", "/api/v1/agents/canvas-1/upload", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "agent_id", Value: "canvas-1"}}

	h := &AgentHandler{
		agentService: service.NewAgentService(),
		fileService:  &fakeUploadFileService{},
	}
	h.UploadAgentFile(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code != float64(common.CodeArgumentError) {
		t.Errorf("expected argument error, got code %v", code)
	}
}

// sp returns a pointer to the given string.
func sp(s string) *string { return &s }