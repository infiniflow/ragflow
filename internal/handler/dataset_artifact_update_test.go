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

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/service"
	dataset "ragflow/internal/service/dataset"
	"ragflow/internal/service/file"
	"ragflow/internal/storage"
)

// fakeWikiPageEngine is a minimal DocEngine backing a single wiki page row so
// the artifact handler's Get/Update wiki-page flow runs without Elasticsearch.
type fakeWikiPageEngine struct {
	engine.DocEngine
	row     map[string]interface{}
	updates []map[string]interface{}
}

func (f *fakeWikiPageEngine) Close() error               { return nil }
func (f *fakeWikiPageEngine) Ping(context.Context) error { return nil }
func (f *fakeWikiPageEngine) GetType() string             { return "fake" }
func (f *fakeWikiPageEngine) DropChunkStore(context.Context, string, string) error {
	return nil
}
func (f *fakeWikiPageEngine) CreateChunkStore(context.Context, string, string, int, string, string) error {
	return nil
}

func (f *fakeWikiPageEngine) Search(_ context.Context, _ *types.SearchRequest) (*types.SearchResult, error) {
	row := make(map[string]interface{}, len(f.row))
	for k, v := range f.row {
		row[k] = v
	}
	return &types.SearchResult{Chunks: []map[string]interface{}{row}, Total: 1}, nil
}

func (f *fakeWikiPageEngine) GetChunk(_ context.Context, _, _ string, _ []string) (interface{}, error) {
	return f.row, nil
}

func (f *fakeWikiPageEngine) UpdateChunks(_ context.Context, _ map[string]interface{}, update map[string]interface{}, _ string, _ string) error {
	f.updates = append(f.updates, update)
	for k, v := range update {
		f.row[k] = v
	}
	return nil
}

// updateArtifactResponse mirrors the PUT /artifacts/<page_type>/<slug>
// envelope: the wiki page detail fields plus the audit commit id.
type updateArtifactResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Slug      string `json:"slug"`
		Title     string `json:"title"`
		ContentMd string `json:"content_md_rendered"`
		CommitID  string `json:"commit_id"`
	} `json:"data"`
}

// setupUpdateArtifactTest migrates the dataset-owner permission schema plus
// the file-commit tables, installs a fake doc engine and memory storage, and
// returns the handler, the backing fake engine, and the test database.
func setupUpdateArtifactTest(t *testing.T) (*DatasetArtifactHandler, *fakeWikiPageEngine, *gorm.DB) {
	t.Helper()
	db := setupArtifactGraphDB(t)
	if err := db.AutoMigrate(&entity.FileCommit{}, &entity.FileCommitItem{}); err != nil {
		t.Fatalf("migrate file commit schema: %v", err)
	}
	oldStorage := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(oldStorage) })

	fake := &fakeWikiPageEngine{row: map[string]interface{}{
		"id":                  "chunk-1",
		"type_kwd":            []interface{}{service.CompileKwdWikiPage},
		"page_type_kwd":       []interface{}{"wiki"},
		"slug_kwd":            []interface{}{"wiki/page-a"},
		"title_kwd":           "Page A",
		"content_with_weight": "old content",
		"available_int":       1,
	}}
	engine.SetDocEngine(fake)
	t.Cleanup(func() { engine.SetDocEngine(nil) })

	h := &DatasetArtifactHandler{
		svc:           service.NewDatasetArtifactService(),
		datasetSvc:    dataset.NewDatasetService(),
		fileCommitSvc: file.NewFileCommitService(),
	}
	return h, fake, db
}

// callUpdateArtifact runs PUT /datasets/{kb}/artifacts/wiki/page-a with the
// given body as user-1 and returns the parsed response.
func callUpdateArtifact(t *testing.T, h *DatasetArtifactHandler, kbID, body string) updateArtifactResponse {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/datasets/"+kbID+"/artifacts/wiki/page-a", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(w)
	c.Request = req.WithContext(permission.WithIdentity(req.Context(), "user-1", "user-1"))
	c.Set("user", &entity.User{ID: "user-1"})
	c.Params = gin.Params{
		{Key: "dataset_id", Value: kbID},
		{Key: "page_type", Value: "wiki"},
		{Key: "slug", Value: "page-a"},
	}
	h.UpdateArtifact(c)
	var resp updateArtifactResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, w.Body.String())
	}
	if resp.Code != 0 {
		t.Fatalf("unexpected code %d (%s)", resp.Code, resp.Message)
	}
	return resp
}

// TestUpdateArtifactReturnsCommitID pins the contract that a content-changing
// PUT reports the id of the audit commit it recorded, so API integrations can
// link an edit to its commit without a follow-up query.
func TestUpdateArtifactReturnsCommitID(t *testing.T) {
	const kbID = "11111111111141118111111111111111"
	h, fake, db := setupUpdateArtifactTest(t)
	insertArtifactGraphKB(t, db, kbID, "user-1")

	resp := callUpdateArtifact(t, h, kbID, `{"content_md": "new content", "title": "Page A"}`)
	if resp.Data.CommitID == "" {
		t.Fatal("expected commit_id in response for a content-changing edit")
	}
	var commit entity.FileCommit
	if err := db.Where("id = ?", resp.Data.CommitID).Take(&commit).Error; err != nil {
		t.Fatalf("load commit %s: %v", resp.Data.CommitID, err)
	}
	if commit.AuthorID != "user-1" {
		t.Fatalf("expected author user-1, got %s", commit.AuthorID)
	}
	if len(fake.updates) == 0 {
		t.Fatal("expected the engine to receive the content update")
	}

	// An edit that leaves the content unchanged records no commit; commit_id
	// must stay empty rather than echo the previous commit.
	resp = callUpdateArtifact(t, h, kbID, `{"content_md": "new content", "title": "Page A"}`)
	if resp.Data.CommitID != "" {
		t.Fatalf("expected empty commit_id for a no-change edit, got %s", resp.Data.CommitID)
	}
}
