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
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
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
	if err := db.Model(&entity.Tenant{}).Where("id = ?", "user-1").Update("status", "1").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	if _, err := factory.ResolveInfo(ctx, entity.ModelTypeChat, "platform-model"); err == nil || errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("database error was hidden as a permission denial: %v", err)
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
	if shared.request == nil || len(shared.request.KbIDs) != 1 || shared.request.KbIDs[0] != "shared-empty" || !shared.request.IncludeUnavailable || !shared.request.RequireComplete {
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

func TestPlatformModelListWithoutTenantProvider(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	if err := db.AutoMigrate(&entity.Tenant{}, &entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	status := "1"
	rows := []any{
		&entity.Tenant{ID: "user-1", Status: &status},
		&entity.UserTenant{ID: "owner", UserID: "user-1", TenantID: "user-1", Role: "owner", InvitedBy: "user-1", Status: &status},
		&entity.SystemSettings{Name: entity.PlatformProviderLockSetting, Source: "system", DataType: "string", Value: "ready"},
		&entity.TenantModelProvider{ID: "platform-provider", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "platform-instance", ProviderID: "platform-provider", InstanceName: "shared", APIKey: "never-return-this-secret", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "platform-model", ProviderID: "platform-provider", InstanceID: "platform-instance", ModelName: "gpt-test", ModelType: int(entity.ModelTypeChat), Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := serviceTestContext(t, "user-1", "user-1")
	models, code, err := NewModelProviderService().ListModelsForUserInTenant(ctx, "user-1", "chat")
	if err != nil || code != common.CodeSuccess || len(models) != 1 {
		t.Fatalf("list = %+v, code %v, err %v", models, code, err)
	}
	if models[0]["model_id"] != "platform-model" || models[0]["source"] != "platform" {
		t.Fatalf("unexpected platform model: %+v", models[0])
	}
	if _, ok := models[0]["api_key"]; ok {
		t.Fatal("model list contains credential field")
	}
	if err := db.Model(&entity.TenantModel{}).Where("id = ?", "platform-model").Update("extra", `{"model_types":["chat","doc_parse"]}`).Error; err != nil {
		t.Fatal(err)
	}
	models, code, err = NewModelProviderService().ListModelsForUserInTenant(ctx, "user-1", "doc_parse")
	if err != nil || code != common.CodeSuccess || len(models) != 1 {
		t.Fatalf("doc_parse list = %+v, %v, %v", models, code, err)
	}
	kinds, ok := models[0]["model_type"].([]string)
	if !ok || !slices.Contains(kinds, "doc_parse") {
		t.Fatalf("doc_parse missing: %v", kinds)
	}
	catalog := dao.GetModelProviderManager().FindProvider("OpenAI")
	var catalogChat string
	for _, item := range catalog.Models {
		if slices.Contains(item.ModelTypes, "chat") {
			catalogChat = item.Name
			break
		}
	}
	if catalogChat == "" {
		t.Fatal("missing catalog chat model")
	}
	if err := db.Model(&entity.TenantModel{}).Where("id = ?", "platform-model").Updates(map[string]interface{}{"model_name": catalogChat, "model_type": 0, "extra": `{"model_types":["doc_parse"]}`}).Error; err != nil {
		t.Fatal(err)
	}
	models, code, err = NewModelProviderService().ListModelsForUserInTenant(ctx, "user-1", "chat")
	if err != nil || code != common.CodeSuccess || len(models) != 0 {
		t.Fatalf("doc_parse appeared as chat: %+v %v %v", models, code, err)
	}
}

func TestCreatePlatformInstanceUsesOnlyPlatformProvider(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	for _, provider := range []*entity.TenantModelProvider{
		{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		{ID: "tenant", TenantID: "tenant", UserID: "owner", ProviderName: "OpenAI"},
	} {
		if err := db.Create(provider).Error; err != nil {
			t.Fatal(err)
		}
	}
	req := CreateProviderInstanceRequest{InstanceName: "shared", APIKey: json.RawMessage(`"secret"`), ModelInfo: []CreateInstanceModelInfo{{ModelName: "chat-model", ModelTypes: []string{"chat"}, MaxTokens: 1024}}}
	service := NewModelProviderService()
	if _, err := service.CreatePlatformInstance(t.Context(), db, "tenant", req); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("tenant provider accepted: %v", err)
	}
	instance, err := service.CreatePlatformInstance(t.Context(), db, "platform", req)
	if err != nil {
		t.Fatal(err)
	}
	if instance.ProviderID != "platform" || instance.APIKey != "secret" {
		t.Fatalf("instance = %+v", instance)
	}
	duplicate := req
	duplicate.InstanceName = "SHARED"
	if _, err := service.CreatePlatformInstance(t.Context(), db, "platform", duplicate); !errors.Is(err, ErrPlatformInstanceConflict) {
		t.Fatalf("duplicate instance: %v", err)
	}
	details, err := PlatformInstanceDetails(t.Context(), db, "platform", instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(details)
	if err != nil || strings.Contains(string(encoded), "secret") || details["api_key_configured"] != true {
		t.Fatalf("unsafe instance details: %s, %v", encoded, err)
	}
	if _, err := PlatformInstanceDetails(t.Context(), db, "tenant", instance.ID); err == nil {
		t.Fatal("cross-scope instance accepted")
	}
	var models []entity.TenantModel
	if err := db.Where("instance_id = ?", instance.ID).Find(&models).Error; err != nil || len(models) != 1 || models[0].ModelName != "chat-model" {
		t.Fatalf("models = %+v, err %v", models, err)
	}
}

func TestListDefaultModelsPlatformFallbackAndStaleSelection(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedModelProviderServiceScope(t, db)
	if err := db.AutoMigrate(&entity.Tenant{}, &entity.SystemSettings{}); err != nil {
		t.Fatal(err)
	}
	status := "1"
	rows := []any{
		&entity.Tenant{ID: "user-1", Status: &status},
		&entity.SystemSettings{Name: entity.PlatformProviderLockSetting, Source: "system", DataType: "string", Value: "ready"},
		&entity.TenantModelProvider{ID: "platform-provider", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "platform-instance", ProviderID: "platform-provider", InstanceName: "shared", APIKey: "secret", Status: "active", Extra: "{}"},
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
	ctx := serviceTestContext(t, "user-1", "user-1")
	service := NewUserTenantService()
	models, err := service.ListDefaultModels(ctx)
	if err != nil || len(models) != 1 || models[0].Source != "platform" || models[0].ModelID != "platform-model" || !models[0].Enable {
		t.Fatalf("platform fallback: %+v, %v", models, err)
	}
	code, err := service.SetModelSettings(ctx, &SetModelSettingsRequest{DefaultModel: &DefaultModelSelection{ModelType: "chat", ModelID: "platform-model"}})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("select platform model: code=%v err=%v", code, err)
	}
	models, err = service.ListDefaultModels(ctx)
	if err != nil || len(models) != 1 || models[0].Source != "user" || models[0].ModelID != "platform-model" {
		t.Fatalf("explicit selection: %+v, %v", models, err)
	}
	code, err = service.SetModelSettings(ctx, &SetModelSettingsRequest{DefaultModel: &DefaultModelSelection{ModelType: "chat"}})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("clear selection: code=%v err=%v", code, err)
	}
	models, err = service.ListDefaultModels(ctx)
	if err != nil || len(models) != 1 || models[0].Source != "platform" || models[0].ModelID != "platform-model" {
		t.Fatalf("restored fallback: %+v, %v", models, err)
	}
	if err := db.Model(&entity.UserTenant{}).Where("id = ?", "user-tenant-1").Update("tenant_llm_id", "missing").Error; err != nil {
		t.Fatal(err)
	}
	models, err = service.ListDefaultModels(ctx)
	if err != nil || len(models) != 1 || models[0].Source != "user" || models[0].ModelID != "missing" || models[0].Enable {
		t.Fatalf("stale explicit selection: %+v, %v", models, err)
	}
	if err := db.Migrator().DropTable(&entity.TenantModel{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListDefaultModels(ctx); err == nil {
		t.Fatal("database failure was treated as a stale selection")
	}
}

func TestUpdatePlatformInstancePreservesCredentialsAndModels(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	rows := []any{
		&entity.TenantModelProvider{ID: "provider", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "instance", ProviderID: "provider", InstanceName: "shared", APIKey: `{"api_key":"old-secret","region":"old"}`, Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "model", ProviderID: "provider", InstanceID: "instance", ModelName: "old-model", ModelType: int(entity.ModelTypeChat), Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	models := []CreateInstanceModelInfo{}
	req := UpdatePlatformInstanceRequest{ModelInfo: &models, APIKey: json.RawMessage(`{"region":"new"}`)}
	svc := NewModelProviderService()
	if err := svc.UpdatePlatformInstance(t.Context(), db, "provider", "instance", req); err != nil {
		t.Fatal(err)
	}
	var instance entity.TenantModelInstance
	if err := db.Take(&instance, "id = ?", "instance").Error; err != nil {
		t.Fatal(err)
	}
	var key map[string]interface{}
	if err := json.Unmarshal([]byte(instance.APIKey), &key); err != nil || key["api_key"] != "old-secret" || key["region"] != "new" {
		t.Fatalf("credentials = %v, %v", key, err)
	}
	var count int64
	if err := db.Model(&entity.TenantModel{}).Where("id = ?", "model").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("model removed: count=%d err=%v", count, err)
	}
	if err := svc.UpdatePlatformInstance(t.Context(), db, "other", "instance", req); err == nil {
		t.Fatal("cross-provider update accepted")
	}
}

func TestPlatformModelsAreScopedAndRedacted(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelProvider{ID: "tenant", TenantID: "tenant", UserID: "owner", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "instance", ProviderID: "platform", InstanceName: "shared", APIKey: "secret", Status: "active", Extra: "{}"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := NewModelProviderService()
	id, err := svc.AddPlatformModel(t.Context(), db, "platform", "instance", AddModelRequest{ModelName: "model", ModelTypes: []string{"chat"}, MaxTokens: 1024, Extra: map[string]interface{}{"api_key": "sensitive", "thinking": true, "nested": []interface{}{map[string]interface{}{"access_token": "nested-sensitive"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddPlatformModel(t.Context(), db, "platform", "instance", AddModelRequest{ModelName: "model", ModelTypes: []string{"chat"}}); !errors.Is(err, ErrPlatformModelConflict) {
		t.Fatalf("duplicate model error: %v", err)
	}
	models, err := PlatformModels(t.Context(), db, "platform", "instance")
	if err != nil || len(models) != 1 || models[0]["model_id"] != id {
		t.Fatalf("models = %+v, err %v", models, err)
	}
	if models[0]["max_tokens"] != 1024 {
		t.Fatalf("max_tokens lost: %+v", models[0])
	}

	encoded, err := json.Marshal(models)
	if err != nil || strings.Contains(string(encoded), "sensitive") || strings.Contains(string(encoded), "secret") {
		t.Fatalf("unsafe model list: %s, %v", encoded, err)
	}
	inactive := "inactive"
	if err := UpdatePlatformModel(t.Context(), db, "platform", "instance", id, UpdatePlatformModelRequest{Status: &inactive}); err != nil {
		t.Fatal(err)
	}
	var stored entity.TenantModel
	if err := db.Take(&stored, "id = ?", id).Error; err != nil || stored.Status != "inactive" {
		t.Fatalf("updated model: %+v, %v", stored, err)
	}
	if err := UpdatePlatformModel(t.Context(), db, "tenant", "instance", id, UpdatePlatformModelRequest{Status: &inactive}); err == nil {
		t.Fatal("cross-scope model update accepted")
	}
}

func TestResolvePlatformProviderConfigRequiresMatchingInstance(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelProvider{ID: "other", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "Other"},
		&entity.TenantModelInstance{ID: "instance", ProviderID: "platform", InstanceName: "shared", APIKey: "saved-secret", Status: "active", Extra: `{"base_url":"https://example.test","region":"us"}`},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	req := CheckConnectionRequest{InstanceID: "instance", UseSavedCredentials: true}
	provider, config, err := ResolvePlatformProviderConfig(t.Context(), db, "platform", req)
	if err != nil || provider.ID != "platform" || config.APIKey != "saved-secret" || config.Region != "us" {
		t.Fatalf("saved config = %+v, %v", config, err)
	}
	if _, _, err := ResolvePlatformProviderConfig(t.Context(), db, "other", req); err == nil {
		t.Fatal("accepted instance from another provider")
	}
	req.APIKey = json.RawMessage(`"submitted"`)
	if _, _, err := ResolvePlatformProviderConfig(t.Context(), db, "platform", req); err == nil {
		t.Fatal("accepted submitted and saved credentials together")
	}
}

func TestCheckPlatformConnectionOnlyUpdatesMatchingInstance(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "instance", ProviderID: "platform", InstanceName: "first", APIKey: "saved-secret", Status: "active", Extra: "{}"},
		&entity.TenantModelInstance{ID: "other-instance", ProviderID: "platform", InstanceName: "second", APIKey: "other-secret", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "model", ProviderID: "platform", InstanceID: "instance", ModelName: "gpt-test", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "skipped-model", ProviderID: "platform", InstanceID: "instance", ModelName: "gpt-skipped", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: `{"verify":"unknown"}`},
		&entity.TenantModel{ID: "other-model", ProviderID: "platform", InstanceID: "other-instance", ModelName: "gpt-test", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: "{}"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	provider := dao.GetModelProviderManager().FindProvider("OpenAI")
	if provider == nil {
		t.Fatal("OpenAI provider missing")
	}
	probe := &remoteModelProbeDriver{DummyModel: modelModule.NewDummyModel(nil, modelModule.URLSuffix{})}
	original := provider.ModelDriver
	provider.ModelDriver = probe
	t.Cleanup(func() { provider.ModelDriver = original })
	req := CheckConnectionRequest{InstanceID: "instance", UseSavedCredentials: true, ModelInfo: []CheckConnectionModelInfo{{ModelName: "gpt-test", ModelTypes: []string{"doc_parse"}}, {ModelName: "gpt-skipped", ModelTypes: []string{"doc_parse"}}}}
	if err := CheckPlatformConnection(t.Context(), db, "platform", req); err != nil {
		t.Fatal(err)
	}
	if probe.checkConfig == nil || probe.checkConfig.ApiKey == nil || *probe.checkConfig.ApiKey != "saved-secret" {
		t.Fatalf("probe config = %+v", probe.checkConfig)
	}
	var model, other entity.TenantModel
	if err := db.Take(&model, "id = ?", "model").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Take(&other, "id = ?", "other-model").Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(model.Extra, "verify") || other.Extra != "{}" {
		t.Fatalf("verification affected wrong models: %s / %s", model.Extra, other.Extra)
	}
	var skipped entity.TenantModel
	if err := db.Take(&skipped, "id = ?", "skipped-model").Error; err != nil {
		t.Fatal(err)
	}
	if skipped.Extra != `{"verify":"success"}` || probe.checkCalls != 2 {
		t.Fatalf("specified model not tested: %s; calls=%d", skipped.Extra, probe.checkCalls)
	}
	if err := db.Model(&entity.TenantModel{}).Where("id = ?", "model").Update("extra", "{}").Error; err != nil {
		t.Fatal(err)
	}
	originalCatalog := provider.Models
	provider.Models = []*modelModule.Model{{Name: "gpt-test", ModelTypes: []string{"doc_parse"}}}
	t.Cleanup(func() { provider.Models = originalCatalog })
	req.ModelInfo = nil
	if err := CheckPlatformConnection(t.Context(), db, "platform", req); err != nil {
		t.Fatal(err)
	}
	if err := db.Take(&model, "id = ?", "model").Error; err != nil {
		t.Fatal(err)
	}
	if model.Extra != "{}" {
		t.Fatalf("catalog-only verification changed model: %s", model.Extra)
	}
}

type partialConnectionProbe struct{ *modelModule.DummyModel }

func (d *partialConnectionProbe) ChatWithMessages(_ context.Context, name string, _ []modelModule.Message, _ *modelModule.APIConfig, _ *modelModule.ChatConfig, _ *common.ModelUsage) (*modelModule.ChatResponse, error) {
	if name == "bad" {
		return nil, errors.New("invalid model")
	}
	answer := "ok"
	return &modelModule.ChatResponse{Answer: &answer}, nil
}

func TestPlatformConnectionRejectsPartialModelSuccess(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	if err := db.Create(&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"}).Error; err != nil {
		t.Fatal(err)
	}
	provider := dao.GetModelProviderManager().FindProvider("OpenAI")
	original := provider.ModelDriver
	provider.ModelDriver = &partialConnectionProbe{DummyModel: modelModule.NewDummyModel(nil, modelModule.URLSuffix{})}
	t.Cleanup(func() { provider.ModelDriver = original })
	req := CheckConnectionRequest{APIKey: json.RawMessage(`"key"`), ModelInfo: []CheckConnectionModelInfo{{ModelName: "good", ModelTypes: []string{"chat"}}, {ModelName: "bad", ModelTypes: []string{"chat"}}}}
	if err := CheckPlatformConnection(t.Context(), db, "platform", req); err == nil {
		t.Fatal("partial verification reported success")
	}
}

func TestPlatformModelTypesValidated(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "instance", ProviderID: "platform", InstanceName: "test", Status: "active", Extra: "{}"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := NewModelProviderService()
	id, err := svc.AddPlatformModel(t.Context(), db, "platform", "instance", AddModelRequest{ModelName: "custom", ModelTypes: []string{" CHAT ", "doc_parse"}})
	if err != nil {
		t.Fatal(err)
	}
	invalid := []string{"bogus"}
	if err := UpdatePlatformModel(t.Context(), db, "platform", "instance", id, UpdatePlatformModelRequest{ModelTypes: &invalid}); !errors.Is(err, ErrInvalidPlatformModelType) {
		t.Fatalf("invalid type: %v", err)
	}
	var model entity.TenantModel
	if err := db.Take(&model, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if model.ModelType != int(entity.ModelTypeChat) {
		t.Fatalf("type = %d", model.ModelType)
	}
}

func TestPlatformPDFParserRequiresOCRType(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	for _, name := range []string{"SoMark", "OpenDataLoader", "OpenAI"} {
		if err := db.Create(&entity.TenantModelProvider{ID: name, TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: name}).Error; err != nil {
			t.Fatal(err)
		}
		err := validatePlatformParserTypes(t.Context(), db, name, []string{"doc_parse"})
		if (err != nil) != (name != "OpenAI") {
			t.Fatalf("provider %s: %v", name, err)
		}
		if err := validatePlatformParserTypes(t.Context(), db, name, []string{"doc_parse", "ocr"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlatformInstanceDetailsHidesURLCredentials(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "instance", ProviderID: "platform", InstanceName: "test", Status: "active", Extra: `{"base_url":"https://user:secret@example.test/v1?api_key=secret"}`},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	details, err := PlatformInstanceDetails(t.Context(), db, "platform", "instance")
	if err != nil {
		t.Fatal(err)
	}
	if details["base_url"] != "" || details["base_url_configured"] != true {
		t.Fatalf("unexpected URL details: %v", details)
	}
	encoded, err := json.Marshal(details)
	if err != nil || strings.Contains(string(encoded), "secret") {
		t.Fatalf("unsafe instance details: %s, %v", encoded, err)
	}
}

func TestPlatformInstanceDetailsNamesHiddenAddresses(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "MinerU"},
		&entity.TenantModelInstance{ID: "instance", ProviderID: "platform", InstanceName: "test", Status: "active", APIKey: `{"mineru_api_token":"secret","mineru_server_url":"","opendataloader_apiserver":"https://example.test/?token=secret","mineru_backend":"vlm-http-client"}`, Extra: `{}`},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	details, err := PlatformInstanceDetails(t.Context(), db, "platform", "instance")
	if err != nil {
		t.Fatal(err)
	}
	// A value saved empty is not hidden; only a withheld address is named.
	hidden, _ := details["api_key_config_hidden"].([]string)
	if len(hidden) != 1 || hidden[0] != "opendataloader_apiserver" {
		t.Fatalf("hidden = %v", details["api_key_config_hidden"])
	}
	encoded, err := json.Marshal(details)
	if err != nil || strings.Contains(string(encoded), "secret") {
		t.Fatalf("unsafe details: %s, %v", encoded, err)
	}
}

func TestPlatformSafeExtraHidesURLCredentials(t *testing.T) {
	extra := platformSafeExtra(map[string]interface{}{"paddleocr_api_url": "https://example.test/?token=secret", "nested": map[string]interface{}{"endpoint": "https://u:secret@example.test"}, "name": "safe", "opendataloader_apiserver": "example.test/path?token=hidden", "mineru_apiserver": "bad%zz?token=hidden"})
	encoded, err := json.Marshal(extra)
	if err != nil || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "hidden") || extra["name"] != "safe" {
		t.Fatalf("unsafe extra: %s, %v", encoded, err)
	}
}

func TestPlatformDocParseTypePersists(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "instance", ProviderID: "platform", InstanceName: "test", Status: "active", Extra: "{}"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	id, err := NewModelProviderService().AddPlatformModel(t.Context(), db, "platform", "instance", AddModelRequest{ModelName: "custom", ModelTypes: []string{"doc_parse"}})
	if err != nil {
		t.Fatal(err)
	}
	items, err := PlatformModels(t.Context(), db, "platform", "instance")
	if err != nil || len(items) != 1 {
		t.Fatalf("items: %v, %v", items, err)
	}
	types, ok := items[0]["model_type"].([]string)
	if !ok || len(types) != 1 || types[0] != "doc_parse" {
		t.Fatalf("types: %v", items[0]["model_type"])
	}
	var model entity.TenantModel
	if err := db.Take(&model, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	info, err := modelInfoWithTenantExtra(nil, &model, true)
	if err != nil || !info.ModelTypeMap["doc_parse"] {
		t.Fatalf("model metadata: %+v, %v", info, err)
	}
}

func TestExplicitModelTypesOverrideCatalogDocParse(t *testing.T) {
	catalog := &modelModule.Model{Name: "model", ModelTypes: []string{"doc_parse"}, ModelTypeMap: map[string]bool{"doc_parse": true}}
	model := &entity.TenantModel{ModelName: "model", ModelType: int(entity.ModelTypeOCR), Extra: `{"model_types":["ocr"]}`}
	info, err := modelInfoWithTenantExtra(catalog, model, true)
	if err != nil || info.ModelTypeMap["doc_parse"] {
		t.Fatalf("stale capability: %+v, %v", info, err)
	}
}

func TestTenantModelTypesPreserveCatalogDocParse(t *testing.T) {
	catalog := &modelModule.Model{Name: "model", ModelTypes: []string{"doc_parse"}, ModelTypeMap: map[string]bool{"doc_parse": true}}
	model := &entity.TenantModel{ModelName: "model", ModelType: int(entity.ModelTypeOCR), Extra: `{"model_types":["ocr"]}`}
	info, err := modelInfoWithTenantExtra(catalog, model, false)
	if err != nil || !info.ModelTypeMap["doc_parse"] {
		t.Fatalf("tenant catalog capability: %+v, %v", info, err)
	}
}

func TestPlatformCatalogModelRetainsDocParse(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	provider := &entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "302.AI"}
	instance := &entity.TenantModelInstance{ID: "instance", ProviderID: "platform", InstanceName: "test", Status: "active", Extra: "{}"}
	for _, row := range []any{provider, instance} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	catalog := dao.GetModelProviderManager().FindProvider("302.AI")
	var selected *modelModule.Model
	for _, model := range catalog.Models {
		if slices.Contains(model.ModelTypes, "doc_parse") {
			selected = model
			break
		}
	}
	if selected == nil {
		t.Fatal("catalog has no doc_parse model")
	}
	svc := NewModelProviderService()
	if err := svc.addModelToInstance(t.Context(), db, provider, instance, CreateInstanceModelInfo{ModelName: selected.Name, ModelTypes: selected.ModelTypes}); err != nil {
		t.Fatal(err)
	}
	items, err := PlatformModels(t.Context(), db, provider.ID, instance.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("models %v: %v", items, err)
	}
	kinds, ok := items[0]["model_type"].([]string)
	if !ok || !slices.Contains(kinds, "doc_parse") {
		t.Fatalf("missing doc_parse: %v", items[0])
	}
}

func TestTenantConnectionCannotUsePlatformInstance(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedModelProviderServiceScope(t, db)
	for _, row := range []any{
		// The platform provider was created by the same user who now calls the tenant API.
		&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "user-1", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "platform-instance", ProviderID: "platform", InstanceName: "shared", APIKey: "platform-secret", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "platform-model", ProviderID: "platform", InstanceID: "platform-instance", ModelName: "gpt-test", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: "{}"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	provider := dao.GetModelProviderManager().FindProvider("OpenAI")
	probe := &remoteModelProbeDriver{DummyModel: modelModule.NewDummyModel(nil, modelModule.URLSuffix{})}
	original := provider.ModelDriver
	provider.ModelDriver = probe
	t.Cleanup(func() { provider.ModelDriver = original })

	ctx := serviceTestContext(t, "user-1", "user-1")
	req := CheckConnectionRequest{InstanceID: "platform-instance", UseSavedCredentials: true, BaseURL: "https://attacker.example", ModelInfo: []CheckConnectionModelInfo{{ModelName: "gpt-test", ModelTypes: []string{"chat"}}}}
	code, err := NewModelProviderService().CheckConnection(ctx, "OpenAI", req)
	if err == nil || code != common.CodeNotFound {
		t.Fatalf("platform instance reached through tenant API: code=%d err=%v", code, err)
	}
	if probe.checkCalls != 0 || probe.checkConfig != nil {
		t.Fatal("platform credentials were sent")
	}
	var model entity.TenantModel
	if err := db.Take(&model, "id = ?", "platform-model").Error; err != nil {
		t.Fatal(err)
	}
	if model.Extra != "{}" {
		t.Fatalf("platform model changed: %s", model.Extra)
	}
	if err := permission.NewDatabaseChecker(db).CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindModelInstance, ID: "platform-instance"}, permission.OperationUpdate); err == nil {
		t.Fatal("creator gained tenant owner rights over a platform instance")
	}
}

func TestPlatformModelTypesIgnoreExtra(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "instance", ProviderID: "platform", InstanceName: "test", Status: "active", Extra: "{}"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := NewModelProviderService()
	id, err := svc.AddPlatformModel(t.Context(), db, "platform", "instance", AddModelRequest{ModelName: "custom", ModelTypes: []string{"chat"}, Extra: map[string]interface{}{"model_types": []string{"doc_parse"}}})
	if err != nil {
		t.Fatal(err)
	}
	assertTypes := func() {
		var row entity.TenantModel
		if err := db.Take(&row, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if row.ModelType != int(entity.ModelTypeChat) || strings.Contains(strings.ToLower(row.Extra), "doc_parse") {
			t.Fatalf("unexpected model types: %+v", row)
		}
	}
	assertTypes()
	for _, extra := range []map[string]interface{}{{"model_types": []string{"doc_parse"}}, {"model_types": "chat"}, {"Model_Types": []string{"doc_parse"}}} {
		if err := UpdatePlatformModel(t.Context(), db, "platform", "instance", id, UpdatePlatformModelRequest{Extra: extra}); err != nil {
			t.Fatal(err)
		}
		assertTypes()
	}
	update := UpdatePlatformInstanceRequest{ModelInfo: &[]CreateInstanceModelInfo{{ModelName: "custom", Extra: map[string]interface{}{"model_types": []string{"doc_parse"}}}}}
	if err := svc.UpdatePlatformInstance(t.Context(), db, "platform", "instance", update); err != nil {
		t.Fatal(err)
	}
	untyped := UpdatePlatformInstanceRequest{ModelInfo: &[]CreateInstanceModelInfo{{ModelName: "untyped"}}}
	if err := svc.UpdatePlatformInstance(t.Context(), db, "platform", "instance", untyped); !errors.Is(err, ErrInvalidPlatformModelType) {
		t.Fatalf("model without a type was created: %v", err)
	}
	items, err := PlatformModels(t.Context(), db, "platform", "instance")
	if err != nil || len(items) != 1 {
		t.Fatalf("models = %v, %v", items, err)
	}
	if types := items[0]["model_type"].([]string); len(types) != 1 || types[0] != "chat" {
		t.Fatalf("extra changed model types: %v", types)
	}
	var stored entity.TenantModel
	if err := db.Take(&stored, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(stored.Extra), "doc_parse") {
		t.Fatalf("override persisted: %s", stored.Extra)
	}
}

func TestCreatePlatformInstanceAddsOnlyListedModels(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	if err := db.Create(&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"}).Error; err != nil {
		t.Fatal(err)
	}
	instance, err := NewModelProviderService().CreatePlatformInstance(t.Context(), db, "platform", CreateProviderInstanceRequest{InstanceName: "empty", APIKey: json.RawMessage(`"secret"`), BaseURL: "https://api.openai.com/v1"})
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&entity.TenantModel{}).Where("instance_id = ?", instance.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("catalog models were published implicitly: %d, %v", count, err)
	}
}

func TestPlatformInstanceInputValidation(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "platform", TenantID: entity.PlatformTenantID, UserID: "admin", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "instance", ProviderID: "platform", InstanceName: "saved", Status: "active", Extra: `{"base_url":"https://private.example/v1","region":"eu"}`},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := NewModelProviderService()
	for _, name := range []string{"", "   "} {
		req := CreateProviderInstanceRequest{InstanceName: "named", ModelInfo: []CreateInstanceModelInfo{{ModelName: name, ModelTypes: []string{"chat"}}}}
		if _, err := svc.CreatePlatformInstance(t.Context(), db, "platform", req); !errors.Is(err, ErrPlatformModelNameRequired) {
			t.Fatalf("model name %q: %v", name, err)
		}
	}
	update := UpdatePlatformInstanceRequest{BaseURL: "   ", Region: " ", ModelInfo: &[]CreateInstanceModelInfo{}}
	if err := svc.UpdatePlatformInstance(t.Context(), db, "platform", "instance", update); err != nil {
		t.Fatal(err)
	}
	var instance entity.TenantModelInstance
	if err := db.Take(&instance, "id = ?", "instance").Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(instance.Extra, "https://private.example/v1") || !strings.Contains(instance.Extra, `"eu"`) {
		t.Fatalf("blank values replaced saved settings: %s", instance.Extra)
	}
}
