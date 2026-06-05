package service

import (
	"context"

	"ragflow/internal/engine"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service/kg"
)

// KGSearchRetrieval performs a full knowledge graph retrieval.
func KGSearchRetrieval(
	ctx context.Context,
	docEngine engine.DocEngine,
	chatModel *modelModule.ChatModel,
	embModel *modelModule.EmbeddingModel,
	kbIDs []string,
	tenantIDs []string,
	question string,
) (map[string]interface{}, error) {
	return kg.KGSearchRetrieval(ctx, docEngine, chatModel, embModel, kbIDs, tenantIDs, question)
}
