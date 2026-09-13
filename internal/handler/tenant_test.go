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
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/dao"
	dataset "ragflow/internal/service/dataset"
)

func writeChunksFile(t *testing.T, kbID string) string {
	t.Helper()
	payload := `{"index_name":"ragflow_anything","knowledgebase_id":"` + kbID + `","chunks":[{"doc_id":"d1","content_with_weight":"x"}]}`
	f := filepath.Join(t.TempDir(), "chunks.json")
	if err := os.WriteFile(f, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestInsertChunksFromFile_ForeignDatasetRejected(t *testing.T) {
	db := setupHandlerAccessDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	h := NewTenantHandler(nil, nil, dataset.NewDatasetService())

	// ds-1 belongs to tenant-1/user-1; user-2 must not insert chunks into it,
	// whatever index/table name the payload claims.
	c, w := ginContextAsUser("POST", "/api/v1/tenant/dev_insert_chunks_from_file",
		`{"file_path": "`+writeChunksFile(t, "ds-1")+`"}`, "user-2")
	h.InsertChunksFromFile(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for foreign user, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInsertChunksFromFile_MissingKnowledgebaseID(t *testing.T) {
	db := setupHandlerAccessDB(t)
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	h := NewTenantHandler(nil, nil, dataset.NewDatasetService())

	c, w := ginContextAsUser("POST", "/api/v1/tenant/dev_insert_chunks_from_file",
		`{"file_path": "`+writeChunksFile(t, "")+`"}`, "user-1")
	h.InsertChunksFromFile(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing knowledgebase_id, got %d: %s", w.Code, w.Body.String())
	}
}
