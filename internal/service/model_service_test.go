package service

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

func TestValidateEmbeddingDimension(t *testing.T) {
	maxDimension := 2048

	tests := []struct {
		name      string
		model     *modelModule.Model
		requested int
		wantErr   string
	}{
		{
			name:      "allows unset requested dimension",
			model:     &modelModule.Model{MaxDimension: &maxDimension, Dimensions: []int{256, 512}},
			requested: 0,
		},
		{
			name:      "allows missing model schema",
			model:     nil,
			requested: 256,
		},
		{
			name:      "allows dimension listed in explicit options",
			model:     &modelModule.Model{Name: "embedding-3", MaxDimension: &maxDimension, Dimensions: []int{256, 512, 1024, 2048}},
			requested: 1024,
		},
		{
			name:      "rejects dimension not listed in explicit options",
			model:     &modelModule.Model{Name: "embedding-3", MaxDimension: &maxDimension, Dimensions: []int{256, 512, 1024, 2048}},
			requested: 1536,
			wantErr:   "supported dimensions",
		},
		{
			name:      "allows custom dimension within max dimension",
			model:     &modelModule.Model{Name: "flex-embedding", MaxDimension: &maxDimension},
			requested: 1536,
		},
		{
			name:      "rejects custom dimension above max dimension",
			model:     &modelModule.Model{Name: "flex-embedding", MaxDimension: &maxDimension},
			requested: 4096,
			wantErr:   "max dimension",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEmbeddingDimension(tt.model, tt.requested)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateEmbeddingDimension() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateEmbeddingDimension() expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateEmbeddingDimension() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestModelInfoWithTenantExtraAppliesEmbeddingDimensions(t *testing.T) {
	factoryMaxDimension := 2048
	modelInfo := &modelModule.Model{
		Name:         "embedding-3",
		MaxDimension: &factoryMaxDimension,
		Dimensions:   []int{1024, 2048},
		ModelTypes:   []string{"embedding"},
		ModelTypeMap: map[string]bool{"embedding": true},
	}
	modelEntity := &entity.TenantModel{
		Extra: `{"max_dimension":768,"dimensions":[384,768],"model_types":["embedding"]}`,
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
	if len(merged.Dimensions) != 2 || merged.Dimensions[0] != 384 || merged.Dimensions[1] != 768 {
		t.Fatalf("Dimensions = %v, want [384 768]", merged.Dimensions)
	}
	if err := validateEmbeddingDimension(merged, 1024); err == nil || !strings.Contains(err.Error(), "supported dimensions") {
		t.Fatalf("validateEmbeddingDimension() error = %v, want supported dimensions error", err)
	}
	if err := validateEmbeddingDimension(merged, 768); err != nil {
		t.Fatalf("validateEmbeddingDimension() error = %v", err)
	}
	if modelInfo.MaxDimension == nil || *modelInfo.MaxDimension != factoryMaxDimension {
		t.Fatalf("factory MaxDimension was mutated: %v", modelInfo.MaxDimension)
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
		&entity.TenantModel{ID: "model-1", ProviderID: "provider-1", InstanceID: "instance-1", ModelName: "gpt-test", ModelType: "chat", Status: "active"},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("failed to seed %T: %v", row, err)
		}
	}
}

func TestModelProviderServiceUpdateModelStatusByID(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedModelProviderServiceScope(t, db)

	code, err := NewModelProviderService().UpdateModelStatus("OpenAI", "default", "", "user-1", "model-1", "inactive")
	if err != nil {
		t.Fatalf("UpdateModelStatus() error = %v", err)
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

func TestModelProviderServiceUpdateModelStatusRejectsInvalidStatus(t *testing.T) {
	code, err := NewModelProviderService().UpdateModelStatus("OpenAI", "default", "", "user-1", "model-1", "disabled")
	if err == nil {
		t.Fatalf("UpdateModelStatus() error = nil, want invalid status error")
	}
	if code != common.CodeBadRequest {
		t.Fatalf("code = %v, want %v", code, common.CodeBadRequest)
	}
	if !strings.Contains(err.Error(), "status must be active or inactive") {
		t.Fatalf("error = %v, want status validation message", err)
	}
}

func TestModelProviderServiceUpdateModelStatusRejectsMissingModelSelector(t *testing.T) {
	code, err := NewModelProviderService().UpdateModelStatus("OpenAI", "default", "", "user-1", "", "active")
	if err == nil {
		t.Fatalf("UpdateModelStatus() error = nil, want missing model selector error")
	}
	if code != common.CodeBadRequest {
		t.Fatalf("code = %v, want %v", code, common.CodeBadRequest)
	}
	if !strings.Contains(err.Error(), "model name or model ID is required") {
		t.Fatalf("error = %v, want missing model selector message", err)
	}
}

func TestModelProviderServiceUpdateModelStatusRejectsWrongScopedModelID(t *testing.T) {
	db := setupModelProviderServiceTestDB(t)
	useModelProviderServiceTestDB(t, db)
	seedModelProviderServiceScope(t, db)
	if err := db.Create(&entity.TenantModelInstance{ID: "instance-2", ProviderID: "provider-1", InstanceName: "other", APIKey: "sk-test", Status: "active", Extra: "{}"}).Error; err != nil {
		t.Fatalf("failed to seed second instance: %v", err)
	}

	code, err := NewModelProviderService().UpdateModelStatus("OpenAI", "other", "", "user-1", "model-1", "inactive")
	if err == nil {
		t.Fatalf("UpdateModelStatus() error = nil, want not found error")
	}
	if code != common.CodeNotFound {
		t.Fatalf("code = %v, want %v", code, common.CodeNotFound)
	}
}
