package service

import (
	"context"
	"fmt"

	"ragflow/internal/entity/models"
	"ragflow/internal/permission"
)

// ProbeEmbeddingDimension returns the vector size an embedding model produces.
// Provider metadata does not guarantee one (several models accept more than one
// size), so the size is read from one real encode of a probe text.
func ProbeEmbeddingDimension(ctx context.Context, factory *ModelFactory, tenantID, embdID string) (int, error) {
	if factory == nil {
		return 0, fmt.Errorf("model provider not set")
	}
	if embdID == "" {
		return 0, fmt.Errorf("embedding model ID not configured")
	}

	embeddingModel, err := factory.NewEmbeddingModel(permission.WithTenantID(ctx, tenantID), embdID)
	if err != nil {
		return 0, fmt.Errorf("failed to get embedding model: %w", err)
	}

	response, err := embeddingModel.Embed(ctx, models.EmbedRequest{Texts: []string{"ok"}}, nil, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to encode test text: %w", err)
	}
	if len(response) == 0 || len(response[0].Embedding) == 0 {
		return 0, fmt.Errorf("embedding returned empty vector")
	}
	return len(response[0].Embedding), nil
}
