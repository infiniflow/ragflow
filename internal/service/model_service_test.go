package service

import (
	"strings"
	"testing"

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
