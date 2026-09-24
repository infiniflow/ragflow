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

package admin

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	"ragflow/internal/storage"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
)

type deletionEngine struct {
	engine.DocEngine
	dropped []string
	deleted []string
}

func (e *deletionEngine) GetType() string { return string(engine.EngineElasticsearch) }
func (e *deletionEngine) DropChunkStore(_ context.Context, name, _ string) error {
	e.dropped = append(e.dropped, name)
	return nil
}
func (e *deletionEngine) DropMetadataStore(_ context.Context, tenantID string) error {
	e.dropped = append(e.dropped, "metadata:"+tenantID)
	return nil
}
func (e *deletionEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return true, nil
}
func (e *deletionEngine) MetadataStoreExists(context.Context, string) (bool, error) {
	return true, nil
}
func (e *deletionEngine) DeleteChunks(_ context.Context, condition map[string]interface{}, _, _ string) (int64, error) {
	for key, value := range condition {
		e.deleted = append(e.deleted, fmt.Sprintf("%s:%v", key, value))
	}
	return 1, nil
}
func (e *deletionEngine) DeleteMetadata(_ context.Context, condition map[string]interface{}, _ string) (int64, error) {
	for key, value := range condition {
		e.deleted = append(e.deleted, fmt.Sprintf("metadata:%s:%v", key, value))
	}
	return 1, nil
}

type deletionStorage struct {
	storage.Storage
	buckets      []string
	emptyBuckets []string
	files        []string
	err          error
	bucketErr    error
	missing      bool
	checkErr     error
}

func (s *deletionStorage) RemoveBucket(_ context.Context, bucket string) error {
	if s.bucketErr != nil {
		return s.bucketErr
	}
	s.buckets = append(s.buckets, bucket)
	return nil
}
func (s *deletionStorage) RemoveEmptyBucket(_ context.Context, bucket string) error {
	if s.bucketErr != nil {
		return s.bucketErr
	}
	s.emptyBuckets = append(s.emptyBuckets, bucket)
	return nil
}
func (s *deletionStorage) Remove(_ context.Context, bucket, name string, _ ...string) error {
	if s.err != nil {
		return s.err
	}
	s.files = append(s.files, bucket+"/"+name)
	return nil
}
func (s *deletionStorage) ObjectExists(_ context.Context, bucket, name string) (bool, error) {
	if s.checkErr != nil {
		return false, s.checkErr
	}
	return !s.missing, nil
}
func (s *deletionStorage) BucketExistsWithError(_ context.Context, bucket string) (bool, error) {
	if s.checkErr != nil {
		return false, s.checkErr
	}
	return !s.missing, nil
}

func setupUserDeletionDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.SetupTestDB(t,
		&entity.User{}, &entity.Tenant{}, &entity.UserTenant{},
		&entity.Knowledgebase{}, &entity.Document{}, &entity.Task{}, &entity.File{}, &entity.File2Document{},
		&entity.Chat{}, &entity.ChatSession{}, &entity.ConversationMessage{}, &entity.ConversationReference{},
		&entity.API4Conversation{}, &entity.API4ConversationMessage{}, &entity.API4ConversationReference{},
		&entity.ChatChannel{}, &entity.UserCanvas{}, &entity.UserCanvasVersion{},
		&entity.Search{}, &entity.Memory{}, &entity.MemoryTask{}, &entity.MCPServer{},
		&entity.SkillSearchConfig{},
		&entity.CompilationTemplate{}, &entity.CompilationTemplateGroup{},
		&entity.EvaluationDataset{}, &entity.EvaluationCase{}, &entity.EvaluationRun{}, &entity.EvaluationResult{},
		&entity.Connector{}, &entity.Connector2Kb{}, &entity.SyncLogs{},
		&entity.KnowledgeCompileDataset{}, &entity.WikiDocumentDirty{}, &entity.PipelineOperationLog{},
		&entity.IngestionTask{}, &entity.IngestionTaskLog{}, &entity.FileCommit{}, &entity.FileCommitItem{},
		&entity.TenantModelProvider{}, &entity.TenantModelInstance{}, &entity.TenantModel{},
		&entity.TenantModelGroup{}, &entity.TenantModelGroupMapping{},
		&entity.TenantLLM{}, &entity.TenantLangfuse{}, &entity.APIToken{}, &entity.InvitationCode{},
	)
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	return db
}

func TestDeleteUserRemovesOwnedDataAndJoinedDocuments(t *testing.T) {
	db := setupUserDeletionDB(t)
	insert := func(query string, args ...interface{}) {
		t.Helper()
		if err := db.Exec(query, args...).Error; err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
	}
	insert("INSERT INTO user (id, email, nickname, is_active, is_authenticated, is_anonymous) VALUES (?, ?, ?, ?, ?, ?)", "user-1", "user@example.com", "User", "0", "1", "0")
	insert("INSERT INTO user (id, email, nickname, is_active, is_authenticated, is_anonymous) VALUES (?, ?, ?, ?, ?, ?)", "user-2", "other@example.com", "Other", "1", "1", "0")
	insert("INSERT INTO tenant (id, llm_id, embd_id, asr_id, img2txt_id, rerank_id, parser_ids) VALUES (?, ?, ?, ?, ?, ?, ?)", "user-1", "llm", "embd", "asr", "img", "rerank", "general")
	insert("INSERT INTO user_tenant (id, user_id, tenant_id, role, invited_by) VALUES (?, ?, ?, ?, ?)", "owner", "user-1", "user-1", "owner", "user-1")
	insert("INSERT INTO user_tenant (id, user_id, tenant_id, role, invited_by) VALUES (?, ?, ?, ?, ?)", "joined", "user-1", "team", "normal", "user-2")
	insert("INSERT INTO knowledgebase (id, tenant_id, name, embd_id, permission, created_by, parser_id, doc_num, token_num, chunk_num) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", "own-kb", "user-1", "Own", "embd", "me", "user-1", "general", 1, 2, 3)
	insert("INSERT INTO knowledgebase (id, tenant_id, name, embd_id, permission, created_by, parser_id, doc_num, token_num, chunk_num) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", "team-kb", "team", "Team", "embd", "team", "user-2", "general", 2, 5, 7)
	insert("INSERT INTO document (id, kb_id, name, parser_id, parser_config, type, created_by, suffix, location, token_num, chunk_num) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", "own-doc", "own-kb", "Owned document", "general", "{}", "pdf", "user-1", "pdf", "own.pdf", 2, 3)
	insert("INSERT INTO document (id, kb_id, name, parser_id, parser_config, type, created_by, suffix, location, token_num, chunk_num) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", "joined-doc", "team-kb", "Joined document", "general", "{}", "pdf", "user-1", "pdf", "joined.pdf", 2, 3)
	insert("INSERT INTO document (id, kb_id, parser_id, parser_config, type, created_by, suffix, token_num, chunk_num) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", "other-doc", "team-kb", "general", "{}", "pdf", "user-2", "pdf", 3, 4)
	insert("INSERT INTO file (id, parent_id, tenant_id, created_by, name, location, type) VALUES (?, ?, ?, ?, ?, ?, ?)", "own-file", "own-folder", "user-1", "user-1", "own.pdf", "own.pdf", "file")
	insert("INSERT INTO file (id, parent_id, tenant_id, created_by, name, type) VALUES (?, ?, ?, ?, ?, ?)", "own-folder", "own-folder", "user-1", "user-1", "/", "folder")
	insert("INSERT INTO file (id, parent_id, tenant_id, created_by, name, location, type, source_type) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "kb-file", "kb-folder", "user-1", "user-1", "own.pdf", "own.pdf", "pdf", "knowledgebase")
	insert("INSERT INTO file (id, parent_id, tenant_id, created_by, name, location, type) VALUES (?, ?, ?, ?, ?, ?, ?)", "joined-file", "team-folder", "team", "user-1", "joined.pdf", "joined.pdf", "file")
	insert("INSERT INTO file (id, parent_id, tenant_id, created_by, name, location, type) VALUES (?, ?, ?, ?, ?, ?, ?)", "other-file", "team-folder", "team", "user-2", "other.pdf", "other.pdf", "file")
	insert("INSERT INTO file2document (id, file_id, document_id) VALUES (?, ?, ?)", "own-link", "own-file", "own-doc")
	insert("INSERT INTO file2document (id, file_id, document_id) VALUES (?, ?, ?)", "joined-link", "joined-file", "joined-doc")
	insert("INSERT INTO file2document (id, file_id, document_id) VALUES (?, ?, ?)", "shared-link", "other-file", "joined-doc")
	insert("INSERT INTO file2document (id, file_id, document_id) VALUES (?, ?, ?)", "other-link", "other-file", "other-doc")
	insert("INSERT INTO search (id, tenant_id, name, created_by, search_config) VALUES (?, ?, ?, ?, ?)", "own-search", "user-1", "Own", "user-1", "{}")
	insert("INSERT INTO search (id, tenant_id, name, created_by, search_config) VALUES (?, ?, ?, ?, ?)", "joined-search", "team", "Joined", "user-1", "{}")
	insert("INSERT INTO search (id, tenant_id, name, created_by, search_config) VALUES (?, ?, ?, ?, ?)", "other-search", "team", "Other", "user-2", "{}")
	insert("INSERT INTO dialog (id, tenant_id, llm_id, llm_setting, prompt_config, do_refer, kb_ids) VALUES (?, ?, ?, ?, ?, ?, ?)", "own-chat", "user-1", "llm", "{}", "{}", "1", "[]")
	insert("INSERT INTO dialog (id, tenant_id, llm_id, llm_setting, prompt_config, do_refer, kb_ids) VALUES (?, ?, ?, ?, ?, ?, ?)", "team-chat", "team", "llm", "{}", "{}", "1", "[]")
	insert("INSERT INTO conversation (id, dialog_id, user_id) VALUES (?, ?, ?)", "own-conversation", "own-chat", "user-1")
	insert("INSERT INTO conversation (id, dialog_id, user_id) VALUES (?, ?, ?)", "joined-conversation", "team-chat", "user-1")
	insert("INSERT INTO conversation (id, dialog_id, user_id) VALUES (?, ?, ?)", "other-conversation", "team-chat", "user-2")
	insert("INSERT INTO conversation_message (conversation_id, position, metadata) VALUES (?, ?, ?)", "own-conversation", 0, "{}")
	insert("INSERT INTO conversation_reference (conversation_id, position, reference) VALUES (?, ?, ?)", "own-conversation", 0, "{}")
	insert("INSERT INTO api_4_conversation (id, dialog_id, user_id) VALUES (?, ?, ?)", "own-api-conversation", "own-chat", "user-1")
	insert("INSERT INTO api_4_conversation_message (conversation_id, position, metadata) VALUES (?, ?, ?)", "own-api-conversation", 0, "{}")
	insert("INSERT INTO api_4_conversation_reference (conversation_id, position, reference) VALUES (?, ?, ?)", "own-api-conversation", 0, "{}")
	insert("INSERT INTO user_canvas (id, user_id, tags, canvas_category) VALUES (?, ?, ?, ?)", "agent", "user-1", "", "agent_canvas")
	insert("INSERT INTO user_canvas_version (id, user_canvas_id) VALUES (?, ?)", "agent-version", "agent")
	insert("INSERT INTO api_4_conversation (id, dialog_id, user_id) VALUES (?, ?, ?)", "agent-conversation", "agent", "external-user")
	insert("INSERT INTO chat_channel (id, tenant_id, name, channel, config) VALUES (?, ?, ?, ?, ?)", "channel", "user-1", "Channel", "telegram", "{}")
	insert("INSERT INTO memory (id, tenant_id, name, embd_id, llm_id, permissions, forgetting_policy) VALUES (?, ?, ?, ?, ?, ?, ?)", "memory", "user-1", "Memory", "embd", "llm", "me", "FIFO")
	insert("INSERT INTO tenant_model_provider (id, provider_name, tenant_id) VALUES (?, ?, ?)", "provider", "custom", "user-1")
	insert("INSERT INTO tenant_model_instance (id, instance_name, provider_id, api_key) VALUES (?, ?, ?, ?)", "instance", "custom", "provider", "secret")
	insert("INSERT INTO tenant_model (id, model_name, provider_id, instance_id, model_type) VALUES (?, ?, ?, ?, ?)", "model", "model", "provider", "instance", 1)
	insert("INSERT INTO tenant_model_group (id, group_type, strategy) VALUES (?, ?, ?)", "group", "chat", "weighted")
	insert("INSERT INTO tenant_model_group_mapping (group_id, provider_id, instance_id, model_id) VALUES (?, ?, ?, ?)", "group", "provider", "instance", "model")
	insert("INSERT INTO file_commit (id, folder_id, message, author_id, file_count) VALUES (?, ?, ?, ?, ?)", "own-commit", "own-kb", "Own page edit", "user-2", 1)
	insert("INSERT INTO file_commit_item (id, commit_id, file_id, operation) VALUES (?, ?, ?, ?)", "own-commit-item", "own-commit", "own-page", "add")
	insert("INSERT INTO file_commit (id, folder_id, message, author_id, file_count) VALUES (?, ?, ?, ?, ?)", "other-commit", "team-kb", "Other page edit", "user-2", 1)
	insert("INSERT INTO file_commit_item (id, commit_id, file_id, operation) VALUES (?, ?, ?, ?)", "other-commit-item", "other-commit", "other-page", "add")

	docEngine := &deletionEngine{}
	store := &deletionStorage{}
	core, logs := observer.New(zapcore.InfoLevel)
	oldLogger := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() { common.Logger = oldLogger })
	service := NewService()
	service.deleteEngine = docEngine
	service.deleteStorage = store
	result, err := service.DeleteUser(t.Context(), "user@example.com")
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if result.UserCount != 1 || result.TenantCount != 1 {
		t.Fatalf("unexpected deletion result: %+v", result)
	}
	for _, item := range [][2]string{
		{"knowledgebase", "own-kb"}, {"document", "joined-doc"}, {"file", "own-file"}, {"file", "joined-file"},
		{"search", "own-search"}, {"search", "joined-search"}, {"dialog", "own-chat"},
		{"conversation", "own-conversation"}, {"conversation", "joined-conversation"}, {"api_4_conversation", "own-api-conversation"}, {"api_4_conversation", "agent-conversation"},
		{"user_canvas", "agent"}, {"user_canvas_version", "agent-version"}, {"chat_channel", "channel"},
		{"memory", "memory"}, {"tenant_model_provider", "provider"}, {"tenant_model_instance", "instance"},
		{"tenant_model", "model"}, {"tenant_model_group", "group"}, {"file_commit", "own-commit"},
		{"file_commit_item", "own-commit-item"},
	} {
		var count int64
		if err := db.Table(item[0]).Where("id = ?", item[1]).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s retained deleted resource %s", item[0], item[1])
		}
	}
	for _, table := range []string{"conversation_message", "conversation_reference", "api_4_conversation_message", "api_4_conversation_reference", "file2document", "tenant_model_group_mapping"} {
		var count int64
		want := int64(0)
		if table == "file2document" {
			want = 1
		}
		if err := db.Table(table).Count(&count).Error; err != nil || count != want {
			t.Fatalf("dependent rows remain in %s: count=%d, err=%v", table, count, err)
		}
	}
	for _, tableID := range [][2]string{{"document", "other-doc"}, {"file", "other-file"}, {"search", "other-search"}, {"knowledgebase", "team-kb"}, {"dialog", "team-chat"}, {"conversation", "other-conversation"}, {"file_commit", "other-commit"}, {"file_commit_item", "other-commit-item"}} {
		var count int64
		if err := db.Table(tableID[0]).Where("id = ?", tableID[1]).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("shared resource %s/%s missing: count=%d, err=%v", tableID[0], tableID[1], count, err)
		}
	}
	var team entity.Knowledgebase
	if err := db.First(&team, "id = ?", "team-kb").Error; err != nil || team.DocNum != 1 || team.TokenNum != 3 || team.ChunkNum != 4 {
		t.Fatalf("joined dataset counters = %+v, err=%v", team, err)
	}
	if !slices.Equal(store.buckets, []string{"own-kb", "user-1-downloads"}) || !slices.Equal(store.emptyBuckets, []string{"own-folder"}) || !slices.Equal(store.files, []string{"own-kb/own.pdf", "team-kb/joined.pdf", "own-folder/own.pdf", "team-folder/joined.pdf"}) {
		t.Fatalf("storage cleanup = buckets %v, empty buckets %v, files %v", store.buckets, store.emptyBuckets, store.files)
	}
	if !slices.Contains(docEngine.dropped, "ragflow_user-1") || !slices.Contains(docEngine.dropped, "memory_user-1") || !slices.Contains(docEngine.deleted, "doc_id:[joined-doc]") || !slices.Contains(docEngine.deleted, "metadata:id:[joined-doc]") {
		t.Fatalf("index cleanup = dropped %v, deleted %v", docEngine.dropped, docEngine.deleted)
	}
	for _, check := range []struct {
		message string
		field   string
		want    string
	}{
		{"Removed document object", "document", "Joined document (joined-doc)"},
		{"Removed document object", "dataset", "Team (team-kb)"},
		{"Removed file object", "file", "own.pdf (own-file)"},
		{"Removed empty folder bucket", "folder", "/ (own-folder)"},
	} {
		matched := false
		for _, entry := range logs.FilterMessage(check.message).All() {
			if entry.ContextMap()[check.field] == check.want {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("missing %s log with %s=%q", check.message, check.field, check.want)
		}
	}
}

func TestDeleteUserKeepsUserWhenExternalCleanupFails(t *testing.T) {
	db := setupUserDeletionDB(t)
	if err := db.Exec("INSERT INTO user (id, email, nickname, is_active, is_authenticated, is_anonymous) VALUES (?, ?, ?, ?, ?, ?)", "user-1", "user@example.com", "User", "0", "1", "0").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO knowledgebase (id, tenant_id, name, embd_id, permission, created_by, parser_id) VALUES (?, ?, ?, ?, ?, ?, ?)", "dataset", "team", "Dataset", "embd", "me", "user-1", "general").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO document (id, kb_id, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "doc", "dataset", "general", "{}", "pdf", "user-1", "pdf", "doc.pdf").Error; err != nil {
		t.Fatal(err)
	}
	service := NewService()
	service.deleteEngine = &deletionEngine{}
	service.deleteStorage = &deletionStorage{err: errors.New("storage unavailable")}
	if _, err := service.DeleteUser(t.Context(), "user@example.com"); err == nil || !strings.Contains(err.Error(), "storage unavailable") {
		t.Fatalf("DeleteUser error = %v", err)
	}
	var count int64
	if err := db.Model(&entity.User{}).Where("id = ?", "user-1").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("user was deleted after external failure: count=%d, err=%v", count, err)
	}
}

func TestDeleteUserKeepsUserWhenDatasetBucketRemovalFails(t *testing.T) {
	db := setupUserDeletionDB(t)
	if err := db.Exec("INSERT INTO user (id, email, nickname, is_active, is_authenticated, is_anonymous) VALUES (?, ?, ?, ?, ?, ?)", "user-1", "user@example.com", "User", "0", "1", "0").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO knowledgebase (id, tenant_id, name, embd_id, permission, created_by, parser_id) VALUES (?, ?, ?, ?, ?, ?, ?)", "dataset", "team", "Dataset", "embd", "me", "user-1", "general").Error; err != nil {
		t.Fatal(err)
	}
	service := NewService()
	service.deleteEngine = &deletionEngine{}
	service.deleteStorage = &deletionStorage{bucketErr: errors.New("storage unavailable")}
	if _, err := service.DeleteUser(t.Context(), "user@example.com"); err == nil || !strings.Contains(err.Error(), "remove dataset bucket Dataset (dataset): storage unavailable") {
		t.Fatalf("DeleteUser error = %v", err)
	}
	var count int64
	if err := db.Model(&entity.User{}).Where("id = ?", "user-1").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("user deleted after bucket failure: count=%d, err=%v", count, err)
	}
}

func TestDeleteUserContinuesWhenStorageItemsAreMissing(t *testing.T) {
	db := setupUserDeletionDB(t)
	if err := db.Exec("INSERT INTO user (id, email, nickname, is_active, is_authenticated, is_anonymous) VALUES (?, ?, ?, ?, ?, ?)", "user-1", "user@example.com", "User", "0", "1", "0").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO knowledgebase (id, tenant_id, name, embd_id, permission, created_by, parser_id) VALUES (?, ?, ?, ?, ?, ?, ?)", "dataset", "team", "Dataset", "embd", "me", "user-1", "general").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO document (id, kb_id, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "doc", "dataset", "general", "{}", "pdf", "user-1", "pdf", "doc.pdf").Error; err != nil {
		t.Fatal(err)
	}
	store := &deletionStorage{missing: true}
	service := NewService()
	service.deleteEngine = &deletionEngine{}
	service.deleteStorage = store
	if _, err := service.DeleteUser(t.Context(), "user@example.com"); err != nil {
		t.Fatalf("DeleteUser with missing storage items: %v", err)
	}
	if len(store.files) != 0 || len(store.buckets) != 0 {
		t.Fatalf("storage deletion attempted for missing items: files=%v buckets=%v", store.files, store.buckets)
	}
}

func TestDeleteUserStopsOnStorageCheckError(t *testing.T) {
	db := setupUserDeletionDB(t)
	if err := db.Exec("INSERT INTO user (id, email, nickname, is_active, is_authenticated, is_anonymous) VALUES (?, ?, ?, ?, ?, ?)", "user-1", "user@example.com", "User", "0", "1", "0").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO knowledgebase (id, tenant_id, name, embd_id, permission, created_by, parser_id) VALUES (?, ?, ?, ?, ?, ?, ?)", "dataset", "team", "Dataset", "embd", "me", "user-1", "general").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO document (id, kb_id, parser_id, parser_config, type, created_by, suffix, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "doc", "dataset", "general", "{}", "pdf", "user-1", "pdf", "doc.pdf").Error; err != nil {
		t.Fatal(err)
	}
	service := NewService()
	service.deleteEngine = &deletionEngine{}
	service.deleteStorage = &deletionStorage{checkErr: errors.New("storage unavailable")}
	if _, err := service.DeleteUser(t.Context(), "user@example.com"); err == nil || !strings.Contains(err.Error(), "storage unavailable") {
		t.Fatalf("DeleteUser error = %v", err)
	}
	var count int64
	if err := db.Model(&entity.User{}).Where("id = ?", "user-1").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("user deleted after storage check failed: count=%d, err=%v", count, err)
	}
}

func TestDeleteUserRollsBackDatabaseCleanup(t *testing.T) {
	db := setupUserDeletionDB(t)
	if err := db.Exec("INSERT INTO user (id, email, nickname, is_active, is_authenticated, is_anonymous) VALUES (?, ?, ?, ?, ?, ?)", "user-1", "user@example.com", "User", "0", "1", "0").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO user_canvas (id, user_id, tags, canvas_category) VALUES (?, ?, ?, ?)", "agent", "user-1", "", "agent_canvas").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO user_canvas_version (id, user_canvas_id) VALUES (?, ?)", "version", "agent").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&entity.Search{}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService().DeleteUser(t.Context(), "user@example.com"); err == nil {
		t.Fatal("expected SQL cleanup failure")
	}
	for _, item := range [][2]string{{"user", "user-1"}, {"user_canvas", "agent"}, {"user_canvas_version", "version"}} {
		var count int64
		if err := db.Table(item[0]).Where("id = ?", item[1]).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("transaction did not preserve %s/%s: count=%d, err=%v", item[0], item[1], count, err)
		}
	}
}
