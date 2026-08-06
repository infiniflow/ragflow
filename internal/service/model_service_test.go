package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

type remoteModelProbeDriver struct {
	*modelModule.DummyModel
	remoteModels []modelModule.ListModelResponse
	embedCalls   int
}

func (d *remoteModelProbeDriver) ListModels(context.Context, *modelModule.APIConfig) ([]modelModule.ListModelResponse, error) {
	return d.remoteModels, nil
}

func (d *remoteModelProbeDriver) Embed(context.Context, *string, []string, *modelModule.APIConfig, *modelModule.EmbeddingConfig, *common.ModelUsage) ([]modelModule.EmbeddingData, error) {
	d.embedCalls++
	return nil, nil
}

func TestValidateEmbeddingModel(t *testing.T) {
	maxDimension := 2048
	maxBatchSize := 128

	tests := []struct {
		name               string
		model              *modelModule.Model
		requestedDimension int
		requestedBatchSize int
		wantErr            string
	}{
		{
			name:               "rejects nil model",
			requestedDimension: 1024,
			requestedBatchSize: 16,
			wantErr:            "embedding model is nil",
		},
		{
			name:               "rejects zero dimension",
			model:              &modelModule.Model{},
			requestedDimension: 0,
			requestedBatchSize: 1,
			wantErr:            "input dimension <= 0",
		},
		{
			name:               "rejects negative dimension",
			model:              &modelModule.Model{},
			requestedDimension: -1,
			requestedBatchSize: 1,
			wantErr:            "input dimension <= 0",
		},
		{
			name:               "rejects zero batch size",
			model:              &modelModule.Model{},
			requestedDimension: 1024,
			requestedBatchSize: 0,
			wantErr:            "input batch size <= 0",
		},
		{
			name:               "rejects negative batch size",
			model:              &modelModule.Model{},
			requestedDimension: 1024,
			requestedBatchSize: -1,
			wantErr:            "input batch size <= 0",
		},
		{
			name:               "rejects missing max dimension",
			model:              &modelModule.Model{MaxBatchSize: &maxBatchSize},
			requestedDimension: 1024,
			requestedBatchSize: 1,
			wantErr:            "max dimension is nil",
		},
		{
			name:               "rejects missing max batch size",
			model:              &modelModule.Model{MaxDimension: &maxDimension},
			requestedDimension: 1024,
			requestedBatchSize: 1,
			wantErr:            "max batch size is nil",
		},
		{
			name:               "allows dimension listed in explicit options",
			model:              &modelModule.Model{Name: "embedding-3", MaxDimension: &maxDimension, MaxBatchSize: &maxBatchSize, Dimensions: []int{256, 512, 1024, 2048}},
			requestedDimension: 1024,
			requestedBatchSize: 128,
		},
		{
			name:               "rejects dimension not listed in explicit options",
			model:              &modelModule.Model{Name: "embedding-3", MaxDimension: &maxDimension, MaxBatchSize: &maxBatchSize, Dimensions: []int{256, 512, 1024, 2048}},
			requestedDimension: 1536,
			requestedBatchSize: 128,
			wantErr:            "supported dimensions",
		},
		{
			name:               "allows custom dimension within max dimension",
			model:              &modelModule.Model{Name: "flex-embedding", MaxDimension: &maxDimension, MaxBatchSize: &maxBatchSize},
			requestedDimension: 1536,
			requestedBatchSize: 1,
		},
		{
			name:               "rejects custom dimension above max dimension",
			model:              &modelModule.Model{Name: "flex-embedding", MaxDimension: &maxDimension, MaxBatchSize: &maxBatchSize},
			requestedDimension: 4096,
			requestedBatchSize: 1,
			wantErr:            "max dimension",
		},
		{
			name:               "allows batch at model limit",
			model:              &modelModule.Model{Name: "embedding-3", MaxDimension: &maxDimension, MaxBatchSize: &maxBatchSize},
			requestedDimension: 1024,
			requestedBatchSize: 128,
		},
		{
			name:               "rejects batch above model limit",
			model:              &modelModule.Model{Name: "embedding-3", MaxDimension: &maxDimension, MaxBatchSize: &maxBatchSize},
			requestedDimension: 1024,
			requestedBatchSize: 129,
			wantErr:            "max batch size",
		},
		{
			name:               "rejects batch when model limit is unspecified",
			model:              &modelModule.Model{Name: "custom-embedding", MaxDimension: &maxDimension},
			requestedDimension: 1024,
			requestedBatchSize: 10000,
			wantErr:            "max batch size is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEmbeddingModel(tt.model, tt.requestedDimension, tt.requestedBatchSize)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateEmbeddingModel() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateEmbeddingModel() expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateEmbeddingModel() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyProviderModelValidatesRemoteEmbeddingMetadata(t *testing.T) {
	maxDimension := 1024
	maxBatchSize := 0
	driver := &remoteModelProbeDriver{
		DummyModel: modelModule.NewDummyModel(nil, modelModule.URLSuffix{}),
		remoteModels: []modelModule.ListModelResponse{{
			Name:         "remote-embedding",
			ModelTypes:   []string{"embedding"},
			MaxDimension: &maxDimension,
			MaxBatchSize: &maxBatchSize,
		}},
	}

	result, err := verifyProviderModel(context.Background(), driver, nil, &modelModule.APIConfig{}, nil)
	if err == nil {
		t.Fatal("verifyProviderModel() error = nil, want validation error")
	}
	if result["remote-embedding"] != entity.ModelVerifyFail {
		t.Fatalf("verification result = %#v, want remote model failure", result)
	}
	if driver.embedCalls != 0 {
		t.Fatalf("Embed calls = %d, want 0 after metadata validation failure", driver.embedCalls)
	}
}

func TestModelInfoWithTenantExtraAppliesEmbeddingConstraints(t *testing.T) {
	factoryMaxDimension := 2048
	factoryBatchSize := 128
	modelInfo := &modelModule.Model{
		Name:         "embedding-3",
		MaxDimension: &factoryMaxDimension,
		MaxBatchSize: &factoryBatchSize,
		Dimensions:   []int{1024, 2048},
		ModelTypes:   []string{"embedding"},
		ModelTypeMap: map[string]bool{"embedding": true},
	}
	modelEntity := &entity.TenantModel{
		Extra: `{"max_dimension":768,"max_batch_size":16,"dimensions":[384,768],"model_types":["embedding"]}`,
	}

	merged, err := modelInfoWithTenantExtra(modelInfo, modelEntity)
	if err != nil {
		t.Fatalf("modelInfoWithTenantExtra() error = %v", err)
	}
	if merged == modelInfo {
		t.Fatalf("modelInfoWithTenantExtra() returned original model pointer")
	}
	if merged.MaxDimension == nil || *merged.MaxDimension != 768 {
		t.Fatalf("MaxDimension = %v, want 768", merged.MaxDimension)
	}
	if merged.MaxBatchSize == nil || *merged.MaxBatchSize != 16 {
		t.Fatalf("MaxBatchSize = %v, want 16", merged.MaxBatchSize)
	}
	if len(merged.Dimensions) != 2 || merged.Dimensions[0] != 384 || merged.Dimensions[1] != 768 {
		t.Fatalf("Dimensions = %v, want [384 768]", merged.Dimensions)
	}
	if validationErr := validateEmbeddingModel(merged, 1024, 16); validationErr == nil || !strings.Contains(validationErr.Error(), "supported dimensions") {
		t.Fatalf("validateEmbeddingModel() error = %v, want supported dimensions error", validationErr)
	}
	if validationErr := validateEmbeddingModel(merged, 768, 16); validationErr != nil {
		t.Fatalf("validateEmbeddingModel() error = %v", validationErr)
	}
	if validationErr := validateEmbeddingModel(merged, 768, 17); validationErr == nil || !strings.Contains(validationErr.Error(), "max batch size") {
		t.Fatalf("validateEmbeddingModel() error = %v, want max batch size error", validationErr)
	}
	if modelInfo.MaxDimension == nil || *modelInfo.MaxDimension != factoryMaxDimension {
		t.Fatalf("factory MaxDimension was mutated: %v", modelInfo.MaxDimension)
	}
	if modelInfo.MaxBatchSize == nil || *modelInfo.MaxBatchSize != factoryBatchSize {
		t.Fatalf("factory MaxBatchSize was mutated: %v", modelInfo.MaxBatchSize)
	}
	if len(modelInfo.Dimensions) != 2 || modelInfo.Dimensions[0] != 1024 || modelInfo.Dimensions[1] != 2048 {
		t.Fatalf("factory Dimensions were mutated: %v", modelInfo.Dimensions)
	}
}

func setupModelProviderServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.UserTenant{},
		&entity.TenantModelProvider{},
		&entity.TenantModelInstance{},
		&entity.TenantModel{},
	); err != nil {
		t.Fatalf("failed to migrate model service tables: %v", err)
	}
	return db
}

func useModelProviderServiceTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })
}

func seedModelProviderServiceScope(t *testing.T, db *gorm.DB) {
	t.Helper()
	activeStatus := "1"
	rows := []interface{}{
		&entity.UserTenant{ID: "user-tenant-1", UserID: "user-1", TenantID: "tenant-1", Role: "owner", InvitedBy: "user-1", Status: &activeStatus},
		&entity.TenantModelProvider{ID: "provider-1", TenantID: "tenant-1", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "instance-1", ProviderID: "provider-1", InstanceName: "default", APIKey: "sk-test", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "model-1", ProviderID: "provider-1", InstanceID: "instance-1", ModelName: "gpt-test", ModelType: int(entity.ModelTypeChat), Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("failed to seed %T: %v", row, err)
		}
	}
}

func TestModelProviderServiceAlterModelStatusByID(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedModelProviderServiceScope(t, db)

	ctx := t.Context()
	code, err := NewModelProviderService().AlterModel(ctx, "OpenAI", "default", "", "user-1", "model-1", map[string]interface{}{"status": "inactive"})
	if err != nil {
		t.Fatalf("AlterModel() error = %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code = %v, want %v", code, common.CodeSuccess)
	}

	var got entity.TenantModel
	if err := db.Where("id = ?", "model-1").First(&got).Error; err != nil {
		t.Fatalf("failed to reload tenant model: %v", err)
	}
	if got.Status != "inactive" {
		t.Fatalf("status = %q, want inactive", got.Status)
	}
}

func TestModelProviderServiceGetModelConfigByID(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedModelProviderServiceScope(t, db)

	ctx := t.Context()
	driver, modelName, apiConfig, _, err := NewModelProviderService().GetModelConfigByID(ctx, "user-1", entity.ModelTypeChat, "model-1")
	if err != nil {
		t.Fatalf("GetModelConfigByID() error = %v", err)
	}
	if driver == nil {
		t.Fatal("GetModelConfigByID() returned nil driver")
	}
	if modelName != "gpt-test" {
		t.Fatalf("modelName = %q, want %q", modelName, "gpt-test")
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey != "sk-test" {
		t.Fatalf("apiConfig.ApiKey = %v, want %q", apiConfig.ApiKey, "sk-test")
	}
}

func TestModelProviderServiceResolveModelContextLength(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	// Seed a tenant chat model that maps to a real factory-catalog model
	// (Anthropic / claude-opus-4-8 has content_length=1000000, max_output=128000).
	activeStatus := "1"
	rows := []interface{}{
		&entity.UserTenant{ID: "user-tenant-cl", UserID: "user-1", TenantID: "tenant-cl", Role: "owner", InvitedBy: "user-1", Status: &activeStatus},
		&entity.TenantModelProvider{ID: "provider-anthropic", TenantID: "tenant-cl", ProviderName: "Anthropic"},
		&entity.TenantModelInstance{ID: "instance-anthropic", ProviderID: "provider-anthropic", InstanceName: "default", APIKey: "sk-anthropic", Status: "active", Extra: "{}"},
		&entity.TenantModel{ID: "model-claude", ProviderID: "provider-anthropic", InstanceID: "instance-anthropic", ModelName: "claude-opus-4-8", ModelType: int(entity.ModelTypeChat), Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("failed to seed %T: %v", row, err)
		}
	}

	svc := NewModelProviderService()
	ctx := t.Context()

	// UUID path: resolves content_length (context window) from the factory
	// catalog, NOT max_output.
	got, err := svc.ResolveModelContextLength(ctx, "user-1", "model-claude")
	if err != nil {
		t.Fatalf("ResolveModelContextLength(uuid) error = %v", err)
	}
	if got != 1000000 {
		t.Fatalf("uuid content_length = %d, want 1000000 (must be the context window, not max_output=128000)", got)
	}

	// Composite "model@instance@provider" path resolves the same value.
	got2, err := svc.ResolveModelContextLength(ctx, "user-1", "claude-opus-4-8@default@Anthropic")
	if err != nil {
		t.Fatalf("ResolveModelContextLength(composite) error = %v", err)
	}
	if got2 != 1000000 {
		t.Fatalf("composite content_length = %d, want 1000000", got2)
	}
}

func TestModelProviderServiceResolveModelContextLengthUnknownModel(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)

	// A model that does not exist in the factory catalog resolves to 0 so the
	// caller falls back to its default context length instead of failing.
	got, err := NewModelProviderService().ResolveModelContextLength(
		t.Context(), "user-1", "gpt-no-such-model@default@OpenAI")
	if err != nil {
		t.Fatalf("ResolveModelContextLength(unknown) error = %v", err)
	}
	if got != 0 {
		t.Fatalf("unknown model content_length = %d, want 0", got)
	}
}

func TestModelProviderServiceAlterModelRejectsInvalidStatus(t *testing.T) {
	ctx := t.Context()
	code, err := NewModelProviderService().AlterModel(ctx, "OpenAI", "default", "", "user-1", "model-1", map[string]interface{}{"status": "disabled"})
	if err == nil {
		t.Fatalf("AlterModel() error = nil, want invalid status error")
	}
	if code != common.CodeBadRequest {
		t.Fatalf("code = %v, want %v", code, common.CodeBadRequest)
	}
	if !strings.Contains(err.Error(), "status must be") {
		t.Fatalf("error = %v, want status validation message", err)
	}
}

func TestModelProviderServiceAlterModelRejectsMissingModelSelector(t *testing.T) {
	ctx := t.Context()
	code, err := NewModelProviderService().AlterModel(ctx, "OpenAI", "default", "", "user-1", "", map[string]interface{}{"status": "active"})
	if err == nil {
		t.Fatalf("AlterModel() error = nil, want missing model selector error")
	}
	if code != common.CodeBadRequest {
		t.Fatalf("code = %v, want %v", code, common.CodeBadRequest)
	}
	if !strings.Contains(err.Error(), "model name or model ID is required") {
		t.Fatalf("error = %v, want missing model selector message", err)
	}
}

func TestModelProviderServiceAlterModelRejectsWrongScopedModelID(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedModelProviderServiceScope(t, db)
	if err := db.Create(&entity.TenantModelInstance{ID: "instance-2", ProviderID: "provider-1", InstanceName: "other", APIKey: "sk-test", Status: "active", Extra: "{}"}).Error; err != nil {
		t.Fatalf("failed to seed second instance: %v", err)
	}

	ctx := t.Context()
	code, err := NewModelProviderService().AlterModel(ctx, "OpenAI", "other", "", "user-1", "model-1", map[string]interface{}{"status": "inactive"})
	if err == nil {
		t.Fatalf("AlterModel() error = nil, want not found error")
	}
	if code != common.CodeNotFound {
		t.Fatalf("code = %v, want %v", code, common.CodeNotFound)
	}
}

func TestReconcileNvidiaInstanceModelsAddsUpdatesAndDeletes(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	provider := &entity.TenantModelProvider{ID: "provider-nvidia", TenantID: "tenant-1", ProviderName: "NVIDIA"}
	instance := &entity.TenantModelInstance{ID: "instance-nvidia", ProviderID: provider.ID, InstanceName: "default", APIKey: "nvapi-test", Status: "active", Extra: "{}"}
	rows := []interface{}{
		provider,
		instance,
		&entity.TenantModel{
			ID:         "keep-id",
			ProviderID: provider.ID,
			InstanceID: instance.ID,
			ModelName:  "nvidia/keep",
			ModelType:  int(entity.ModelTypeChat),
			Status:     "inactive",
			Extra:      `{"max_tokens":4096,"verify":"success","custom":"preserved"}`,
		},
		&entity.TenantModel{
			ID:         "stale-id",
			ProviderID: provider.ID,
			InstanceID: instance.ID,
			ModelName:  "nvidia/stale",
			ModelType:  int(entity.ModelTypeChat),
			Status:     "active",
			Extra:      `{}`,
		},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}

	maxTokens := 131072
	maxDimension := 2048
	remote := []modelModule.ListModelResponse{
		{Name: "nvidia/keep", MaxOutput: &maxTokens, ModelTypes: []string{"chat", "vision"}},
		{Name: "nvidia/new-embed", MaxOutput: ptrService(8192), MaxDimension: &maxDimension, Dimensions: []int{1024, 2048}, ModelTypes: []string{"embedding"}},
	}

	err := NewModelProviderService().reconcileNvidiaInstanceModels(context.Background(), db, provider, instance, remote)
	if err != nil {
		t.Fatalf("reconcileNvidiaInstanceModels() error = %v", err)
	}

	var got []*entity.TenantModel
	if err = db.Order("model_name").Find(&got).Error; err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(got) != 2 || got[0].ModelName != "nvidia/keep" || got[1].ModelName != "nvidia/new-embed" {
		t.Fatalf("models = %#v, want keep and new", got)
	}
	if got[0].ID != "keep-id" || got[0].Status != "inactive" {
		t.Fatalf("retained model identity/status = %q/%q", got[0].ID, got[0].Status)
	}
	if got[0].ModelType != int(entity.ModelTypeChat|entity.ModelTypeImage2Text) {
		t.Fatalf("retained model type = %d", got[0].ModelType)
	}
	var keepExtra map[string]interface{}
	if err := json.Unmarshal([]byte(got[0].Extra), &keepExtra); err != nil {
		t.Fatalf("decode retained extra: %v", err)
	}
	if keepExtra["custom"] != "preserved" || keepExtra["verify"] != "success" || int(keepExtra["max_tokens"].(float64)) != maxTokens {
		t.Fatalf("retained extra = %#v", keepExtra)
	}
	var newExtra map[string]interface{}
	if err := json.Unmarshal([]byte(got[1].Extra), &newExtra); err != nil {
		t.Fatalf("decode new extra: %v", err)
	}
	if newExtra["verify"] != entity.ModelVerifyUnknown || int(newExtra["max_dimension"].(float64)) != maxDimension {
		t.Fatalf("new extra = %#v", newExtra)
	}
}

func TestReconcileNvidiaInstanceModelsRejectsEmptyDiscoveryWithoutMutation(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	provider := &entity.TenantModelProvider{ID: "provider-nvidia", TenantID: "tenant-1", ProviderName: "NVIDIA"}
	instance := &entity.TenantModelInstance{ID: "instance-nvidia", ProviderID: provider.ID, InstanceName: "default", Status: "active", Extra: "{}"}
	existing := &entity.TenantModel{ID: "keep-id", ProviderID: provider.ID, InstanceID: instance.ID, ModelName: "nvidia/keep", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: "{}"}
	for _, row := range []interface{}{provider, instance, existing} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}

	err := NewModelProviderService().reconcileNvidiaInstanceModels(context.Background(), db, provider, instance, nil)
	if err == nil {
		t.Fatal("reconcileNvidiaInstanceModels() error = nil, want empty discovery error")
	}
	var count int64
	if err := db.Model(&entity.TenantModel{}).Where("id = ?", existing.ID).Count(&count).Error; err != nil {
		t.Fatalf("count retained model: %v", err)
	}
	if count != 1 {
		t.Fatalf("retained model count = %d, want 1", count)
	}
}

func TestReconcileNvidiaInstanceModelsRollsBackPartialRefresh(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	provider := &entity.TenantModelProvider{ID: "provider-nvidia", TenantID: "tenant-1", ProviderName: "NVIDIA"}
	instance := &entity.TenantModelInstance{ID: "instance-nvidia", ProviderID: provider.ID, InstanceName: "default", Status: "active", Extra: "{}"}
	existing := &entity.TenantModel{
		ID:         "keep-id",
		ProviderID: provider.ID,
		InstanceID: instance.ID,
		ModelName:  "nvidia/keep",
		ModelType:  int(entity.ModelTypeChat),
		Status:     "active",
		Extra:      "{invalid-json",
	}
	for _, row := range []interface{}{provider, instance, existing} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}

	remote := []modelModule.ListModelResponse{
		{Name: "nvidia/new", ModelTypes: []string{"chat"}},
		{Name: "nvidia/keep", ModelTypes: []string{"chat"}},
	}
	err := NewModelProviderService().reconcileNvidiaInstanceModels(context.Background(), db, provider, instance, remote)
	if err == nil {
		t.Fatal("reconcileNvidiaInstanceModels() error = nil, want metadata error")
	}

	var got []*entity.TenantModel
	if err := db.Order("model_name").Find(&got).Error; err != nil {
		t.Fatalf("list models: %v", err)
	}
	if len(got) != 1 || got[0].ID != existing.ID {
		t.Fatalf("models after rollback = %#v, want only original model", got)
	}
}

func ptrService[T any](value T) *T {
	return &value
}

func TestParseModelName(t *testing.T) {
	tests := []struct {
		name         string
		composite    string
		wantModel    string
		wantInstance string
		wantProvider string
		wantErr      bool
	}{
		{
			name:         "three parts: model@instance@provider",
			composite:    "text-embedding-3-small@primary@OpenAI",
			wantModel:    "text-embedding-3-small",
			wantInstance: "primary",
			wantProvider: "OpenAI",
		},
		{
			name:         "two parts: model@provider defaults instance",
			composite:    "BAAI/bge-m3@Builtin",
			wantModel:    "BAAI/bge-m3",
			wantInstance: "default",
			wantProvider: "Builtin",
		},
		{
			name:      "single part bare name returns error",
			composite: "BAAI/bge-m3",
			wantErr:   true,
		},
		{
			name:         "embedded @ in modelName preserved (four parts)",
			composite:    "text-embedding-nomic-embed-text-v1.5@q8_0@default@LM-Studio",
			wantModel:    "text-embedding-nomic-embed-text-v1.5@q8_0",
			wantInstance: "default",
			wantProvider: "LM-Studio",
		},
		{
			name:         "multiple embedded @ in modelName preserved (five parts)",
			composite:    "org/repo@tag@1.0@default@Ollama",
			wantModel:    "org/repo@tag@1.0",
			wantInstance: "default",
			wantProvider: "Ollama",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, instance, provider, err := parseModelName(tt.composite)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseModelName(%q) error = nil, want error", tt.composite)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseModelName(%q) unexpected error: %v", tt.composite, err)
			}
			if model != tt.wantModel {
				t.Errorf("parseModelName(%q) model = %q, want %q", tt.composite, model, tt.wantModel)
			}
			if instance != tt.wantInstance {
				t.Errorf("parseModelName(%q) instance = %q, want %q", tt.composite, instance, tt.wantInstance)
			}
			if provider != tt.wantProvider {
				t.Errorf("parseModelName(%q) provider = %q, want %q", tt.composite, provider, tt.wantProvider)
			}
		})
	}
}

func TestSplitRightAnchoredModelName(t *testing.T) {
	tests := []struct {
		name         string
		composite    string
		wantModel    string
		wantInstance string
		wantProvider string
	}{
		{
			name:         "three parts: model@instance@provider",
			composite:    "text-embedding-3-small@primary@OpenAI",
			wantModel:    "text-embedding-3-small",
			wantInstance: "primary",
			wantProvider: "OpenAI",
		},
		{
			name:         "two parts: model@provider defaults instance",
			composite:    "BAAI/bge-m3@Builtin",
			wantModel:    "BAAI/bge-m3",
			wantInstance: "default",
			wantProvider: "Builtin",
		},
		{
			name:         "single part bare name returns empty provider and instance",
			composite:    "BAAI/bge-m3",
			wantModel:    "BAAI/bge-m3",
			wantInstance: "",
			wantProvider: "",
		},
		{
			// Regression for the CodeRabbit "Major" comment on PR #16468:
			// a 2-segment key whose '@' is part of the model name (not a
			// provider separator) must stay bare. Without this branch the
			// helper would return ("text-embedding-nomic-embed-text-v1.5",
			// "default", "q8_0"), mis-classifying the quantization tag as a
			// provider and missing the TEI fast path's `modelName == teiModel`
			// match when TEI_MODEL is the full embedded string.
			name:         "two parts bare default with embedded '@' stays bare",
			composite:    "text-embedding-nomic-embed-text-v1.5@q8_0",
			wantModel:    "text-embedding-nomic-embed-text-v1.5@q8_0",
			wantInstance: "",
			wantProvider: "",
		},
		{
			name:         "embedded @ in modelName preserved (four parts)",
			composite:    "text-embedding-nomic-embed-text-v1.5@q8_0@default@LM-Studio",
			wantModel:    "text-embedding-nomic-embed-text-v1.5@q8_0",
			wantInstance: "default",
			wantProvider: "LM-Studio",
		},
		{
			name:         "multiple embedded @ in modelName preserved (five parts)",
			composite:    "org/repo@tag@1.0@default@Ollama",
			wantModel:    "org/repo@tag@1.0",
			wantInstance: "default",
			wantProvider: "Ollama",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, instance, provider := splitRightAnchoredModelName(tt.composite)
			if model != tt.wantModel {
				t.Errorf("splitRightAnchoredModelName(%q) model = %q, want %q", tt.composite, model, tt.wantModel)
			}
			if instance != tt.wantInstance {
				t.Errorf("splitRightAnchoredModelName(%q) instance = %q, want %q", tt.composite, instance, tt.wantInstance)
			}
			if provider != tt.wantProvider {
				t.Errorf("splitRightAnchoredModelName(%q) provider = %q, want %q", tt.composite, provider, tt.wantProvider)
			}
		})
	}
}
