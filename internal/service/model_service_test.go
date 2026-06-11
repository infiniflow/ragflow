package service

import (
	"strings"
	"testing"

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
