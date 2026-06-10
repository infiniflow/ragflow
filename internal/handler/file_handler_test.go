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

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// setupLinkToDatasetsDB creates an in-memory SQLite DB with the tables needed
// for LinkToDatasets service validation.
func setupLinkToDatasetsDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&entity.File{},
		&entity.Knowledgebase{},
		&entity.UserTenant{},
		&entity.File2Document{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

// setupLinkToDatasetsHandler builds a FileHandler with File2DocumentService and
// swaps dao.DB to the given test DB.
func setupLinkToDatasetsHandler(t *testing.T, db *gorm.DB) *FileHandler {
	t.Helper()
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	return &FileHandler{
		fileService:          service.NewFileService(),
		file2DocumentService: service.NewFile2DocumentService(),
	}
}

func linkToDatasetsCtx(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
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

func TestLinkToDatasets_MissingBothArgs(t *testing.T) {
	db := setupLinkToDatasetsDB(t)
	h := setupLinkToDatasetsHandler(t, db)

	c, w := linkToDatasetsCtx(http.MethodPost, "/api/v1/files/link-to-datasets", `{}`)
	h.LinkToDatasets(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeArgumentError) {
		t.Fatalf("expected code %d (argument error), got %v: %v", common.CodeArgumentError, resp["code"], resp["message"])
	}
}

func TestLinkToDatasets_MissingKbIDs(t *testing.T) {
	db := setupLinkToDatasetsDB(t)
	h := setupLinkToDatasetsHandler(t, db)

	c, w := linkToDatasetsCtx(http.MethodPost, "/api/v1/files/link-to-datasets", `{"file_ids":["f1"]}`)
	h.LinkToDatasets(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeArgumentError) {
		t.Fatalf("expected code %d (missing kb_ids), got %v", common.CodeArgumentError, resp["code"])
	}
}

func TestLinkToDatasets_FileNotFound(t *testing.T) {
	db := setupLinkToDatasetsDB(t)
	h := setupLinkToDatasetsHandler(t, db)

	// file-404 does not exist in the DB
	c, w := linkToDatasetsCtx(http.MethodPost, "/api/v1/files/link-to-datasets",
		`{"file_ids":["file-404"],"kb_ids":["kb-1"]}`)
	h.LinkToDatasets(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeDataError) {
		t.Fatalf("expected code %d (file not found → data error), got %v", common.CodeDataError, resp["code"])
	}
	if resp["message"] != service.ErrLinkFileNotFound.Error() {
		t.Fatalf("expected %q, got %v", service.ErrLinkFileNotFound.Error(), resp["message"])
	}
}

func TestLinkToDatasets_DatasetNotFound(t *testing.T) {
	db := setupLinkToDatasetsDB(t)
	h := setupLinkToDatasetsHandler(t, db)

	loc := "loc.pdf"
	db.Create(&entity.File{ID: "file-1", TenantID: "user-1", ParentID: "root", Name: "file.pdf", Type: "pdf", Location: &loc})

	// kb-404 does not exist in the DB
	c, w := linkToDatasetsCtx(http.MethodPost, "/api/v1/files/link-to-datasets",
		`{"file_ids":["file-1"],"kb_ids":["kb-404"]}`)
	h.LinkToDatasets(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeDataError) {
		t.Fatalf("expected code %d (dataset not found → data error), got %v", common.CodeDataError, resp["code"])
	}
	if resp["message"] != service.ErrLinkDatasetNotFound.Error() {
		t.Fatalf("expected %q, got %v", service.ErrLinkDatasetNotFound.Error(), resp["message"])
	}
}

func TestLinkToDatasets_Success(t *testing.T) {
	db := setupLinkToDatasetsDB(t)
	h := setupLinkToDatasetsHandler(t, db)

	// file owned by "user-1" (TenantID == userID → passes checkFileTeamPermission)
	loc := "loc.pdf"
	db.Create(&entity.File{ID: "file-1", TenantID: "user-1", ParentID: "root", Name: "file.pdf", Type: "pdf", Location: &loc})

	// KB owned by "user-1" (TenantID == userID → passes checkKBTeamPermission)
	db.Create(&entity.Knowledgebase{
		ID:        "kb-1",
		TenantID:  "user-1",
		Name:      "Test KB",
		EmbdID:    "embd-1",
		CreatedBy: "user-1",
		Status:    sptr(string(entity.StatusValid)),
	})

	c, w := linkToDatasetsCtx(http.MethodPost, "/api/v1/files/link-to-datasets",
		`{"file_ids":["file-1"],"kb_ids":["kb-1"]}`)
	h.LinkToDatasets(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code %d, got %v: %v", common.CodeSuccess, resp["code"], resp["message"])
	}
	if resp["data"] != true {
		t.Fatalf("expected data true, got %v", resp["data"])
	}
}
