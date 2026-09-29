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

package dataset

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestDeleteDatasetRemovesTemporaryFiles(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Dataset")
	if err := db.Exec("INSERT INTO document (id, kb_id, name, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", "doc-1", "kb-1", "Document", "general", "{}", "pdf", "tenant-1", "pdf", "tracked").Error; err != nil {
		t.Fatal(err)
	}
	store := storage.NewMemoryStorage()
	if err := store.Put(t.Context(), "kb-1", "tracked", []byte("content")); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(t.Context(), "kb-1", "temporary", []byte("content")); err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })
	core, logs := observer.New(zapcore.InfoLevel)
	previousLogger := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() { common.Logger = previousLogger })

	kb, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), db, "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	service := testDatasetUpdateService(t)
	if err := service.deleteDataset(t.Context(), "tenant-1", kb); err != nil {
		t.Fatalf("delete dataset with temporary file: %v", err)
	}
	if store.ObjExist(t.Context(), "kb-1", "temporary") {
		t.Fatal("dataset deletion retained a temporary object")
	}
	if store.ObjExist(t.Context(), "kb-1", "tracked") {
		t.Fatal("dataset deletion retained a document object")
	}
	if store.BucketExists(t.Context(), "kb-1") {
		t.Fatal("dataset deletion retained the bucket")
	}
	var count int64
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("dataset retained after deletion: count=%d, err=%v", count, err)
	}
	if entries := logs.FilterMessage("Removed dataset document object").All(); len(entries) != 1 || entries[0].ContextMap()["document"] != "Document (doc-1)" || entries[0].ContextMap()["dataset"] != "Dataset (kb-1)" {
		t.Errorf("document deletion logs = %v", entries)
	}
	if entries := logs.FilterMessage("Deleted dataset").All(); len(entries) != 1 || entries[0].ContextMap()["dataset"] != "Dataset (kb-1)" {
		t.Errorf("dataset deletion logs = %v", entries)
	}
}

func TestDeleteDatasetContinuesWhenStorageIsMissing(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Dataset")
	if err := db.Exec("INSERT INTO document (id, kb_id, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "doc-1", "kb-1", "general", "{}", "pdf", "tenant-1", "pdf", "missing").Error; err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(storage.NewMemoryStorage())
	t.Cleanup(func() { factory.SetStorage(previous) })
	kb, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), db, "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := testDatasetUpdateService(t).deleteDataset(t.Context(), "tenant-1", kb); err != nil {
		t.Fatalf("delete dataset with missing storage: %v", err)
	}
	var count int64
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("dataset retained after missing storage: count=%d, err=%v", count, err)
	}
}
