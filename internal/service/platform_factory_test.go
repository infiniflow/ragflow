// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/dao"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func TestPlatformModelDefaultAndAuthorization(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedModelProviderServiceScope(t, db)
	if err := db.AutoMigrate(&entity.Tenant{}, &entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	status := "1"
	rows := []any{
		&entity.SystemSettings{Name: entity.PlatformProviderLockSetting, Source: "system", DataType: "string", Value: "ready"},
		&entity.Tenant{ID: "user-1", Status: &status},
		&entity.TenantModelProvider{ID: "platform-provider", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "platform-instance", ProviderID: "platform-provider", InstanceName: "default", APIKey: "secret", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "platform-model", ProviderID: "platform-provider", InstanceID: "platform-instance", ModelName: "gpt-test", ModelType: int(entity.ModelTypeChat), Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := (dao.PlatformModelSettings{}).SetDefault(t.Context(), db, "chat", "platform-model"); err != nil {
		t.Fatal(err)
	}
	factory := NewModelFactory()
	ctx := serviceTestContext(t, "user-1", "user-1")
	if err := db.Where("name = ?", entity.PlatformProviderLockSetting).Delete(&entity.SystemSettings{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := factory.ResolveInfo(ctx, entity.ModelTypeChat, "platform-model"); err == nil {
		t.Fatal("uninitialized platform accepted")
	}
	if err := db.Create(&entity.SystemSettings{Name: entity.PlatformProviderLockSetting, Source: "system", DataType: "string", Value: "ready"}).Error; err != nil {
		t.Fatal(err)
	}
	info, err := factory.ResolveDefaultInfo(ctx, entity.ModelTypeChat)
	if err != nil || info.ID != "platform-model" {
		t.Fatalf("default = %+v, %v", info, err)
	}
	if err := db.Model(&entity.UserTenant{}).Where("id = ?", "user-tenant-1").Update("tenant_llm_id", "model-1").Error; err != nil {
		t.Fatal(err)
	}
	info, err = factory.ResolveDefaultInfo(ctx, entity.ModelTypeChat)
	if err != nil || info.ID != "model-1" {
		t.Fatalf("explicit default = %+v, %v", info, err)
	}
	if err := db.Model(&entity.UserTenant{}).Where("id = ?", "user-tenant-1").Update("tenant_llm_id", "missing").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := factory.ResolveDefaultInfo(ctx, entity.ModelTypeChat); err == nil || errors.Is(err, ErrNoDefaultModel) {
		t.Fatalf("invalid selection must fail: %v", err)
	}
	taskCtx := permission.WithTenantID(t.Context(), "user-1")
	if _, err := factory.ResolveInfo(taskCtx, entity.ModelTypeChat, "platform-model"); err == nil {
		t.Fatal("untrusted task used platform model")
	}
	taskCtx = permission.WithTrustedModelTenant(taskCtx, "user-1")
	if _, err := factory.ResolveInfo(taskCtx, entity.ModelTypeChat, "platform-model"); err != nil {
		t.Fatalf("trusted task: %v", err)
	}
	if err := db.Model(&entity.TenantModel{}).Where("id = ?", "platform-model").Update("status", "inactive").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := factory.ResolveInfo(taskCtx, entity.ModelTypeChat, "platform-model"); err == nil {
		t.Fatal("inactive model was accepted")
	}
	if err := db.Model(&entity.TenantModel{}).Where("id = ?", "platform-model").Update("status", "active").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&entity.Tenant{}).Where("id = ?", "user-1").Update("status", "0").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := factory.ResolveInfo(taskCtx, entity.ModelTypeChat, "platform-model"); err == nil {
		t.Fatal("inactive tenant was accepted")
	}
}

type existingPlatformEmbeddingStore struct{ fakeChatDocEngine }

func (existingPlatformEmbeddingStore) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return true, nil
}

type sharedEmbeddingStore struct {
	fakeChatDocEngine
	request *enginetypes.SearchRequest
	total   int64
}

func (s *sharedEmbeddingStore) ChunkStoreExists(context.Context, string, string) (bool, error) {
	return true, nil
}

func (s *sharedEmbeddingStore) Search(_ context.Context, req *enginetypes.SearchRequest) (*enginetypes.SearchResult, error) {
	s.request = req
	return &enginetypes.SearchResult{Total: s.total}, nil
}

type failingRefreshEmbeddingStore struct{ sharedEmbeddingStore }

func (s *failingRefreshEmbeddingStore) RefreshChunkStore(context.Context, string, string) error {
	return errors.New("refresh failed")
}

func TestDatasetEmbeddingIndexCheckFailsClosedOnRefreshError(t *testing.T) {
	store := &failingRefreshEmbeddingStore{}
	kb := &entity.Knowledgebase{ID: "kb", TenantID: "tenant"}
	if err := CheckDatasetEmbeddingIndexEmpty(t.Context(), store, kb); err == nil {
		t.Fatal("expected refresh failure to prevent an empty-index decision")
	}
	if store.request != nil {
		t.Fatal("searched after refresh failed")
	}
}

func TestBindDatasetEmbeddingPinsModelAndProtectsExistingIndex(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedModelProviderServiceScope(t, db)
	if err := db.AutoMigrate(&entity.Tenant{}, &entity.SystemSettings{}, &entity.Knowledgebase{}); err != nil {
		t.Fatal(err)
	}
	status := "1"
	rows := []any{
		&entity.SystemSettings{Name: entity.PlatformProviderLockSetting, Source: "system", DataType: "string", Value: "ready"},
		&entity.Tenant{ID: "user-1", Status: &status},
		&entity.TenantModelProvider{ID: "platform-provider", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "platform-instance", ProviderID: "platform-provider", InstanceName: "default", APIKey: "secret", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "platform-model", ProviderID: "platform-provider", InstanceID: "platform-instance", ModelName: "text-embedding-3-small", ModelType: int(entity.ModelTypeEmbedding), Status: "active"},
		&entity.Knowledgebase{ID: "empty", TenantID: "user-1", Name: "empty", Status: &status},
		&entity.Knowledgebase{ID: "existing", TenantID: "user-1", Name: "existing", Status: &status},
		&entity.Knowledgebase{ID: "shared-empty", TenantID: "user-1", Name: "shared-empty", Status: &status},
		&entity.Knowledgebase{ID: "shared-populated", TenantID: "user-1", Name: "shared-populated", Status: &status},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := (dao.PlatformModelSettings{}).SetDefault(t.Context(), db, "embedding", "platform-model"); err != nil {
		t.Fatal(err)
	}
	ctx := serviceTestContext(t, "user-1", "user-1")
	ref, err := BindDatasetEmbedding(ctx, db, fakeChatDocEngine{}, "empty", "user-1")
	if err != nil || ref != "platform-model" {
		t.Fatalf("bind = %q, %v", ref, err)
	}
	kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, "empty")
	if err != nil || kb.EmbeddingModelReference() != "platform-model" || kb.EmbdID != "platform-model" {
		t.Fatalf("dataset = %+v, %v", kb, err)
	}
	if err := (dao.PlatformModelSettings{}).SetDefault(ctx, db, "embedding", ""); err != nil {
		t.Fatal(err)
	}
	ref, err = BindDatasetEmbedding(ctx, db, fakeChatDocEngine{}, "empty", "user-1")
	if err != nil || ref != "platform-model" {
		t.Fatalf("pinned = %q, %v", ref, err)
	}
	if err := (dao.PlatformModelSettings{}).SetDefault(ctx, db, "embedding", "platform-model"); err != nil {
		t.Fatal(err)
	}
	shared := &sharedEmbeddingStore{}
	if ref, err := BindDatasetEmbedding(ctx, db, shared, "shared-empty", "user-1"); err != nil || ref != "platform-model" {
		t.Fatalf("empty dataset in shared index = %q, %v", ref, err)
	}
	if shared.request == nil || len(shared.request.KbIDs) != 1 || shared.request.KbIDs[0] != "shared-empty" || !shared.request.IncludeUnavailable {
		t.Fatalf("search did not scope all chunks to dataset: %+v", shared.request)
	}
	shared.total = 1
	if _, err := BindDatasetEmbedding(ctx, db, shared, "shared-populated", "user-1"); err == nil {
		t.Fatal("populated dataset in shared index was bound automatically")
	}
	if _, err := BindDatasetEmbedding(ctx, db, existingPlatformEmbeddingStore{}, "existing", "user-1"); err == nil {
		t.Fatal("existing index was bound automatically")
	}
}

func TestStableDatasetEmbeddingRejectsStaleVectorWrite(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	if err := db.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatal(err)
	}
	status := "1"
	modelID := "new-model"
	if err := db.Create(&entity.Knowledgebase{ID: "kb", TenantID: "tenant", Name: "dataset", Status: &status, EmbdID: modelID, TenantEmbdID: &modelID}).Error; err != nil {
		t.Fatal(err)
	}
	wrote := false
	write := func() error { wrote = true; return nil }
	if err := WithStableDatasetEmbedding(t.Context(), db, "kb", "tenant", "old-model", write); err == nil || wrote {
		t.Fatalf("stale model write accepted: err=%v wrote=%v", err, wrote)
	}
	if err := WithStableDatasetEmbedding(t.Context(), db, "kb", "tenant", modelID, write); err != nil || !wrote {
		t.Fatalf("current model write rejected: err=%v wrote=%v", err, wrote)
	}
}
