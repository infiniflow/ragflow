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
	"context"
	"errors"
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
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
	if err := service.deleteDataset(t.Context(), kb); err != nil {
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
	if err := testDatasetUpdateService(t).deleteDataset(t.Context(), kb); err != nil {
		t.Fatalf("delete dataset with missing storage: %v", err)
	}
	var count int64
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("dataset retained after missing storage: count=%d, err=%v", count, err)
	}
}

// sharedBucketStorage models a deployment that keeps every dataset in one
// physical bucket, as OSS does with OSS_BUCKET set: object operations are
// scoped to a per-dataset prefix, and the bucket itself cannot be removed.
type sharedBucketStorage struct {
	storage.Storage
}

func (s sharedBucketStorage) RemoveBucket(_ context.Context, bucket string) error {
	return fmt.Errorf("cannot remove logical OSS bucket %s: %w", bucket, storage.ErrSharedBucketRemoval)
}

// readOnlyBucketStorage models a removal that genuinely failed.
type readOnlyBucketStorage struct {
	storage.Storage
}

func (r readOnlyBucketStorage) RemoveBucket(_ context.Context, _ string) error {
	return errors.New("storage service unavailable")
}

// TestDeleteDatasetKeepsASharedBucket pins the shared-bucket contract: the
// dataset and its own objects go, the bucket every dataset shares stays, and
// the deletion still completes instead of returning after the sources are
// already gone (#22513).
func TestDeleteDatasetKeepsASharedBucket(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Dataset")
	if err := db.Exec("INSERT INTO document (id, kb_id, name, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", "doc-1", "kb-1", "Document", "general", "{}", "pdf", "tenant-1", "pdf", "tracked").Error; err != nil {
		t.Fatal(err)
	}
	store := sharedBucketStorage{Storage: storage.NewMemoryStorage()}
	if err := store.Put(t.Context(), "kb-1", "tracked", []byte("content")); err != nil {
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
	if err := testDatasetUpdateService(t).deleteDataset(t.Context(), kb); err != nil {
		t.Fatalf("delete dataset in a shared bucket: %v", err)
	}
	if store.ObjExist(t.Context(), "kb-1", "tracked") {
		t.Fatal("dataset deletion retained a document object")
	}
	if !store.BucketExists(t.Context(), "kb-1") {
		t.Fatal("dataset deletion removed the bucket every dataset shares")
	}
	var count int64
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("dataset retained after shared-bucket deletion: count=%d, err=%v", count, err)
	}
	if entries := logs.FilterMessage("Kept shared dataset bucket").All(); len(entries) != 1 || entries[0].ContextMap()["dataset"] != "Dataset (kb-1)" {
		t.Errorf("shared bucket logs = %v", entries)
	}
}

// TestDeleteDatasetReportsABucketRemovalFailure keeps the tolerance narrow: a
// removal that failed for any other reason still stops the deletion before the
// dataset rows go.
func TestDeleteDatasetReportsABucketRemovalFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Dataset")
	if err := db.Exec("INSERT INTO document (id, kb_id, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "doc-1", "kb-1", "general", "{}", "pdf", "tenant-1", "pdf", "tracked").Error; err != nil {
		t.Fatal(err)
	}
	store := readOnlyBucketStorage{Storage: storage.NewMemoryStorage()}
	if err := store.Put(t.Context(), "kb-1", "tracked", []byte("content")); err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })

	kb, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), db, "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := testDatasetUpdateService(t).deleteDataset(t.Context(), kb); err == nil {
		t.Fatal("delete dataset ignored a bucket removal failure")
	}
	var count int64
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("failed deletion removed the dataset row: count=%d, err=%v", count, err)
	}
}

// fakeImageEngine answers the chunk-image lookup with the chunks of one document,
// one page at a time, the way the real engine pages a search.
type fakeImageEngine struct {
	engine.DocEngine
	chunks             map[string][]map[string]interface{}
	err                error
	total              int64
	offsets            *[]int
	includeUnavailable *bool
}

func (f fakeImageEngine) DeleteChunks(context.Context, map[string]interface{}, string, string) (int64, error) {
	return 0, nil
}

func (f fakeImageEngine) DeleteMetadata(context.Context, map[string]interface{}, string) (int64, error) {
	return 0, nil
}

func (f fakeImageEngine) Search(_ context.Context, req *enginetypes.SearchRequest) (*enginetypes.SearchResult, error) {
	if f.offsets != nil {
		*f.offsets = append(*f.offsets, req.Offset)
	}
	if f.includeUnavailable != nil {
		*f.includeUnavailable = req.IncludeUnavailable
	}
	if f.err != nil {
		return nil, f.err
	}
	docID, _ := req.Filter["doc_id"].(string)
	all := f.chunks[docID]
	start := req.Offset
	if start > len(all) {
		start = len(all)
	}
	end := start + req.Limit
	if end > len(all) {
		end = len(all)
	}
	total := int64(len(all))
	if f.total > 0 {
		total = f.total
	}
	return &enginetypes.SearchResult{Chunks: all[start:end], Total: total}, nil
}

// TestDeleteDatasetRemovesChunkImagesFromASharedBucket covers the objects a kept
// bucket would otherwise hold forever: chunk images are stored under the chunk id
// and nothing else lists them, so they are read from the chunk index first and
// removed once the bucket is known to stay (#22513).
func TestDeleteDatasetRemovesChunkImagesFromASharedBucket(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Dataset")
	if err := db.Exec("INSERT INTO document (id, kb_id, name, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", "doc-1", "kb-1", "Document", "general", "{}", "pdf", "tenant-1", "pdf", "tracked").Error; err != nil {
		t.Fatal(err)
	}
	store := sharedBucketStorage{Storage: storage.NewMemoryStorage()}
	for _, object := range []string{"tracked", "chunk-9"} {
		if err := store.Put(t.Context(), "kb-1", object, []byte("content")); err != nil {
			t.Fatal(err)
		}
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })

	kb, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), db, "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	includeUnavailable := false
	service := testDatasetUpdateService(t)
	service.docEngine = fakeImageEngine{
		chunks:             map[string][]map[string]interface{}{"doc-1": {{"id": "chunk-9", "img_id": "kb-1-chunk-9"}}},
		includeUnavailable: &includeUnavailable,
	}
	if err := service.deleteDataset(t.Context(), kb); err != nil {
		t.Fatalf("delete dataset with a chunk image: %v", err)
	}
	if !includeUnavailable {
		t.Fatal("image lookup hid disabled chunks, whose images would stay behind")
	}
	if store.ObjExist(t.Context(), "kb-1", "chunk-9") {
		t.Fatal("dataset deletion retained a chunk image in the shared bucket")
	}
	if !store.BucketExists(t.Context(), "kb-1") {
		t.Fatal("dataset deletion removed the bucket every dataset shares")
	}
}

// TestDeleteDatasetReadsImageKeysBeforeDeletingAnything pins the order: when the
// chunk index cannot be read, the dataset keeps every object it had.
func TestDeleteDatasetReadsImageKeysBeforeDeletingAnything(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Dataset")
	if err := db.Exec("INSERT INTO document (id, kb_id, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "doc-1", "kb-1", "general", "{}", "pdf", "tenant-1", "pdf", "tracked").Error; err != nil {
		t.Fatal(err)
	}
	store := sharedBucketStorage{Storage: storage.NewMemoryStorage()}
	if err := store.Put(t.Context(), "kb-1", "tracked", []byte("content")); err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })

	kb, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), db, "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	service := testDatasetUpdateService(t)
	service.docEngine = fakeImageEngine{err: errors.New("chunk index unavailable")}
	if err := service.deleteDataset(t.Context(), kb); err == nil {
		t.Fatal("delete dataset ignored a chunk image lookup failure")
	}
	if !store.ObjExist(t.Context(), "kb-1", "tracked") {
		t.Fatal("document object was removed before its chunk images could be named")
	}
	var count int64
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("failed deletion removed the dataset row: count=%d, err=%v", count, err)
	}
}

// TestDeleteDatasetStopsImageLookupAtTheResultWindow pins the page budget: a
// document with more chunks than the backend will paginate keeps deleting, and no
// page is requested whose from + size leaves the window (#22513).
func TestDeleteDatasetStopsImageLookupAtTheResultWindow(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertDatasetUpdateKB(t, "kb-1", "tenant-1", "Dataset")
	if err := db.Exec("INSERT INTO document (id, kb_id, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "doc-1", "kb-1", "general", "{}", "pdf", "tenant-1", "pdf", "tracked").Error; err != nil {
		t.Fatal(err)
	}
	store := sharedBucketStorage{Storage: storage.NewMemoryStorage()}
	if err := store.Put(t.Context(), "kb-1", "tracked", []byte("content")); err != nil {
		t.Fatal(err)
	}
	factory := storage.GetStorageFactory()
	previous := factory.GetStorage()
	factory.SetStorage(store)
	t.Cleanup(func() { factory.SetStorage(previous) })

	kb, err := dao.NewKnowledgebaseDAO().GetByID(t.Context(), db, "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	offsets := make([]int, 0, 12)
	service := testDatasetUpdateService(t)
	service.docEngine = fakeImageEngine{
		chunks:  map[string][]map[string]interface{}{"doc-1": {{"id": "chunk-9"}}},
		total:   100_000,
		offsets: &offsets,
	}
	if err := service.deleteDataset(t.Context(), kb); err != nil {
		t.Fatalf("delete dataset with more chunks than the result window: %v", err)
	}
	for _, offset := range offsets {
		if offset+imageLookupPageSize > common.MAX_RESULT_WINDOW {
			t.Fatalf("asked for offset %d, whose page leaves the result window: %v", offset, offsets)
		}
	}
	var count int64
	if err := db.Model(&entity.Knowledgebase{}).Where("id = ?", "kb-1").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("dataset retained: count=%d, err=%v", count, err)
	}
}
