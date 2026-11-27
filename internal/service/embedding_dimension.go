package service

import (
	"context"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
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

	// Group failover: a gp- reference whose member fails the probe call
	// flips inactive and retries once on the next member.
	var response []models.EmbeddingData
	err := dao.InvokeWithGroupFailover(ctx, dao.DB, tenantID, entity.ModelTypeEmbedding, embdID,
		func(ctx context.Context) (*models.EmbeddingModel, error) {
			return factory.NewEmbeddingModel(permission.WithTenantID(ctx, tenantID), embdID)
		}, nil,
		func(ctx context.Context, embeddingModel *models.EmbeddingModel) error {
			resp, invErr := embeddingModel.Embed(ctx, models.EmbedRequest{Texts: []string{"ok"}}, nil, nil)
			if invErr != nil {
				return fmt.Errorf("failed to encode test text: %w", invErr)
			}
			if len(resp) == 0 || len(resp[0].Embedding) == 0 {
				return fmt.Errorf("embedding returned empty vector")
			}
			response = resp
			return nil
		}, nil)
	if err != nil {
		return 0, err
	}

	dimension := len(response[0].Embedding)
	common.Info(fmt.Sprintf("Got embedding dimension from API: %d", dimension))
	return dimension, nil
}
