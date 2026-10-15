package chunk

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/service"
	"ragflow/internal/utility"
)

func (s *ChunkService) entityChunkIDs(ctx context.Context, indexName, datasetID, documentID, entityID string) ([]string, error) {
	result, err := s.docEngine.Search(ctx, &types.SearchRequest{
		IndexNames:         []string{indexName},
		KbIDs:              []string{datasetID},
		Limit:              1,
		IncludeUnavailable: true,
		SelectFields:       []string{"id", "kb_id", "doc_id", "type_kwd", "knowledge_graph_kwd", "content_with_weight", "source_chunk_ids"},
		Filter: map[string]interface{}{
			"id":                  entityID,
			"doc_id":              documentID,
			"knowledge_graph_kwd": "entity",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get structure entity: %w", err)
	}
	if result == nil || len(result.Chunks) == 0 {
		return nil, fmt.Errorf("%w: structure entity", common.ErrNotFound)
	}

	row := result.Chunks[0]
	if row["id"] != entityID ||
		!slices.Contains(utility.ConvertToStringSlice(row["kb_id"]), datasetID) ||
		!slices.Contains(utility.ConvertToStringSlice(row["doc_id"]), documentID) ||
		(!slices.Contains(utility.ConvertToStringSlice(row["type_kwd"]), "entity") &&
			!slices.Contains(utility.ConvertToStringSlice(row["knowledge_graph_kwd"]), "entity")) {
		return nil, fmt.Errorf("%w: structure entity", common.ErrNotFound)
	}
	var payload map[string]interface{}
	if content, ok := row["content_with_weight"].(string); ok {
		if err := json.Unmarshal([]byte(content), &payload); err != nil {
			payload = nil
		}
	}
	return service.StructureSourceChunkIDs(payload, row), nil
}
