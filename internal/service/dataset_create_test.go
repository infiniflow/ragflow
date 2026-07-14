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

package service

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func setupDatasetCreateTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.Tenant{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	status := string(entity.StatusValid)
	if err := db.Create(&entity.Tenant{
		ID:        "tenant-1",
		LLMID:     "",
		EmbdID:    "",
		ParserIDs: "naive",
		Status:    &status,
	}).Error; err != nil {
		t.Fatalf("failed to create tenant: %v", err)
	}
	return db
}

func TestDatasetServiceCreateDatasetValidatesName(t *testing.T) {
	setupDatasetCreateTestDB(t)

	svc := NewDatasetService()
	_, code, err := svc.CreateDataset(&CreateDatasetRequest{Name: "   "}, "tenant-1")
	if err == nil {
		t.Fatal("expected name validation error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "`name` is required." {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceCreateDatasetRejectsDuplicateName(t *testing.T) {
	db := setupDatasetCreateTestDB(t)

	if err := db.Create(&entity.Knowledgebase{
		ID:        "kb-1",
		TenantID:  "tenant-1",
		Name:      "Existing",
		ParserID:  "naive",
		CreatedBy: "tenant-1",
		Status:    sptr(string(entity.StatusValid)),
	}).Error; err != nil {
		t.Fatalf("failed to create existing kb: %v", err)
	}

	svc := NewDatasetService()
	_, code, err := svc.CreateDataset(&CreateDatasetRequest{Name: "Existing"}, "tenant-1")
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceCreateDatasetExtNameValidation(t *testing.T) {
	setupDatasetCreateTestDB(t)

	svc := NewDatasetService()
	_, code, err := svc.CreateDataset(&CreateDatasetRequest{Name: "valid", Ext: map[string]interface{}{"name": "   "}}, "tenant-1")
	if err == nil {
		t.Fatal("expected ext name validation error")
	}
	if code != common.CodeDataError {
		t.Fatalf("expected data error code, got %d", code)
	}
	if err.Error() != "`name` is required." {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDatasetServiceCreateDatasetRejectsInvalidEmbeddingModel(t *testing.T) {
	setupDatasetCreateTestDB(t)

	cases := []struct {
		name            string
		embeddingModel  string
		expectedMessage string
	}{
		{"empty", "", "Embedding model identifier must follow <model_name>@<provider> format"},
		{"whitespace", " ", "Embedding model identifier must follow <model_name>@<provider> format"},
		{"missing_at", "BAAI/bge-small-en-v1.5Builtin", "Embedding model identifier must follow <model_name>@<provider> format"},
		{"empty_model_name", "@Builtin", "Both model_name and provider must be non-empty strings"},
		{"empty_provider", "BAAI/bge-small-en-v1.5@", "Both model_name and provider must be non-empty strings"},
		{"whitespace_model_name", " @Builtin", "Both model_name and provider must be non-empty strings"},
		{"whitespace_provider", "BAAI/bge-small-en-v1.5@ ", "Both model_name and provider must be non-empty strings"},
	}

	svc := NewDatasetService()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, code, err := svc.CreateDataset(&CreateDatasetRequest{
				Name:           "ds-embd-" + tc.name,
				EmbeddingModel: &tc.embeddingModel,
			}, "tenant-1")
			if err == nil {
				t.Fatal("expected embedding model validation error")
			}
			if code != common.CodeDataError {
				t.Fatalf("expected data error code, got %d", code)
			}
			if err.Error() != tc.expectedMessage {
				t.Fatalf("unexpected error: got %q, want %q", err.Error(), tc.expectedMessage)
			}
		})
	}
}
