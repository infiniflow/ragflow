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
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
)

// sptr returns a pointer to the given string.
func sptr(s string) *string { return &s }

// setupServiceTestDB initializes an in-memory SQLite database for service tests.
func setupServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	if err = db.AutoMigrate(
		&entity.Document{},
		&entity.Knowledgebase{},
		&entity.Task{},
		&entity.IngestionTask{},
		&entity.IngestionTaskLog{},
		&entity.File2Document{},
		&entity.File{},
		&entity.User{},
		&entity.Tenant{},
		&entity.UserTenant{},
		&entity.API4Conversation{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

// pushServiceDB swaps dao.DB for the test and restores after.
func pushServiceDB(t *testing.T, testDB *gorm.DB) {
	t.Helper()
	orig := dao.DB
	dao.DB = testDB
	t.Cleanup(func() {
		dao.DB = orig
	})
}

// fakeChatDocEngine is a stub engine.DocEngine used by parent-package tests.
type fakeChatDocEngine struct{}

func (fakeChatDocEngine) CreateChunkStore(context.Context, string, string, int, string) error {
	return nil
}
func (fakeChatDocEngine) InsertChunks(context.Context, []map[string]interface{}, string, string) ([]string, error) {
	return nil, nil
}
func (fakeChatDocEngine) UpdateChunks(context.Context, map[string]interface{}, map[string]interface{}, string, string) error {
	return nil
}
func (fakeChatDocEngine) DeleteChunks(context.Context, map[string]interface{}, string, string) (int64, error) {
	return 0, nil
}
func (fakeChatDocEngine) Search(context.Context, *types.SearchRequest) (*types.SearchResult, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetChunk(context.Context, string, string, []string) (interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) DropChunkStore(context.Context, string, string) error { return nil }
func (fakeChatDocEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return false, nil
}
func (fakeChatDocEngine) CreateMetadataStore(context.Context, string) error { return nil }
func (fakeChatDocEngine) InsertMetadata(context.Context, []map[string]interface{}, string) ([]string, error) {
	return nil, nil
}
func (fakeChatDocEngine) UpdateMetadata(context.Context, string, string, map[string]interface{}, string) error {
	return nil
}
func (fakeChatDocEngine) DeleteMetadata(context.Context, map[string]interface{}, string) (int64, error) {
	return 0, nil
}
func (fakeChatDocEngine) DeleteMetadataKeys(context.Context, string, string, []string, string) error {
	return nil
}
func (fakeChatDocEngine) DropMetadataStore(context.Context, string) error { return nil }
func (fakeChatDocEngine) MetadataStoreExists(context.Context, string) (bool, error) {
	return false, nil
}
func (fakeChatDocEngine) SearchMetadata(context.Context, *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	return nil, nil
}
func (fakeChatDocEngine) IndexDocument(context.Context, string, string, interface{}) error {
	return nil
}
func (fakeChatDocEngine) DeleteDocument(context.Context, string, string) error { return nil }
func (fakeChatDocEngine) BulkIndex(context.Context, string, []interface{}) (interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetFields([]map[string]interface{}, []string) map[string]map[string]interface{} {
	return nil
}
func (fakeChatDocEngine) GetAggregation([]map[string]interface{}, string) []map[string]interface{} {
	return nil
}
func (fakeChatDocEngine) GetHighlight([]map[string]interface{}, []string, string) map[string]string {
	return nil
}
func (fakeChatDocEngine) RunSQL(context.Context, string, string, []string, string) ([]map[string]interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetChunkIDs([]map[string]interface{}) []string { return nil }
func (fakeChatDocEngine) KNNScores(context.Context, []map[string]interface{}, []float64, int) (map[string]interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetScores(map[string]interface{}) map[string]float64 { return nil }
func (fakeChatDocEngine) Ping(context.Context) error                          { return nil }
func (fakeChatDocEngine) Close() error                                        { return nil }
func (fakeChatDocEngine) CheckStatus() error                                  { return nil }
func (fakeChatDocEngine) FilterDocIdsByMetaPushdown(context.Context, []string, []map[string]interface{}, string) []string {
	return nil
}
func (fakeChatDocEngine) GetMessages(context.Context, string, int, int) ([]interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) GetType() string     { return "fake" }
func (fakeChatDocEngine) Init() error         { return nil }
func (fakeChatDocEngine) InitConsumer() error { return nil }
func (fakeChatDocEngine) ListMessages(context.Context, string, int, int) ([]interface{}, error) {
	return nil, nil
}
func (fakeChatDocEngine) PublishTask(map[string]interface{}) error { return nil }
func (fakeChatDocEngine) ShowMessageQueue() string                 { return "" }
func (fakeChatDocEngine) SupportsPageRank() bool                   { return false }

// insertTestKB inserts a test Knowledgebase row.
func insertTestKB(t *testing.T, id, tenantID string, docNum, tokenNum, chunkNum int64) {
	t.Helper()
	kb := &entity.Knowledgebase{
		ID:         id,
		TenantID:   tenantID,
		Name:       "test-kb",
		EmbdID:     "embd-1",
		CreatedBy:  "user-1",
		Permission: string(entity.TenantPermissionTeam),
		DocNum:     docNum,
		TokenNum:   tokenNum,
		ChunkNum:   chunkNum,
		Status:     sptr(string(entity.StatusValid)),
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert test kb: %v", err)
	}
}

// insertTestDoc inserts a test Document row.
func insertTestDoc(t *testing.T, id, kbID string, tokenNum, chunkNum int64) {
	t.Helper()
	doc := &entity.Document{
		ID:           id,
		KbID:         kbID,
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
		TokenNum:     tokenNum,
		ChunkNum:     chunkNum,
		Suffix:       ".txt",
		Status:       sptr("1"),
	}
	if err := dao.DB.Create(doc).Error; err != nil {
		t.Fatalf("insert test doc: %v", err)
	}
}

// insertTestIngestionTask inserts a test IngestionTask row with CREATED status.
func insertTestIngestionTask(t *testing.T, id, userID, docID, datasetID string) {
	insertTestIngestionTaskWithStatus(t, id, userID, docID, datasetID, common.CREATED)
}

// insertTestIngestionTaskWithStatus inserts a test IngestionTask with a specific status.
func insertTestIngestionTaskWithStatus(t *testing.T, id, userID, docID, datasetID, status string) {
	t.Helper()
	task := &entity.IngestionTask{
		ID:         id,
		UserID:     userID,
		DocumentID: docID,
		DatasetID:  datasetID,
		Status:     status,
	}
	if err := dao.DB.Create(task).Error; err != nil {
		t.Fatalf("insert test ingestion task: %v", err)
	}
}
