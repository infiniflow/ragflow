package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
)

// loadCanvasDSLJSON returns the DSL JSON for a custom canvas pipeline. The
// canvas's dsl column holds the same component-graph structure that built-in
// templates use, so it can be validated by the same schema extractor. It is a
// package-level function so both document and knowledge-base updates reuse it.
func loadCanvasDSLJSON(canvasID string) ([]byte, error) {
	if strings.TrimSpace(canvasID) == "" {
		return nil, fmt.Errorf("empty canvas id")
	}
	canvas, err := dao.NewUserCanvasDAO().GetByID(canvasID)
	if err != nil {
		if errors.Is(err, dao.ErrUserCanvasNotFound) {
			return nil, fmt.Errorf("canvas %s not found", canvasID)
		}
		return nil, fmt.Errorf("load canvas %s: %w", canvasID, err)
	}
	if len(canvas.DSL) == 0 {
		return nil, fmt.Errorf("canvas %s has no DSL", canvasID)
	}
	raw, err := json.Marshal(canvas.DSL)
	if err != nil {
		return nil, fmt.Errorf("marshal canvas %s DSL: %w", canvasID, err)
	}
	return raw, nil
}

// LoadPipelineDSL loads the DSL JSON for a pipeline identified by parserID
// (built-in) or pipelineID (custom canvas). When both are provided, isCanvas
// selects which one to use.
func LoadPipelineDSL(isCanvas bool, parserID string, pipelineID *string) ([]byte, error) {
	if isCanvas {
		return loadCanvasDSLJSON(strings.TrimSpace(*pipelineID))
	}
	registry, err := pipelinepkg.DefaultRegistry()
	if err != nil {
		return nil, fmt.Errorf("builtin pipeline registry: %w", err)
	}
	if !registry.IsValid(parserID) {
		return nil, fmt.Errorf("unknown builtin parser_id: %s", parserID)
	}
	dslStr, err := pipelinepkg.LoadBuiltinDSL(parserID)
	if err != nil {
		return nil, fmt.Errorf("load builtin DSL for %q: %w", parserID, err)
	}
	return []byte(dslStr), nil
}

// ResolveComponentParamsDefaults loads the DSL for the target pipeline and
// returns the component params defaults as an entity.JSONMap {cpnID: {param: value}}.
// For builtin templates the DSL is loaded from the embedded registry; for custom
// canvas pipelines it is loaded from the canvas row in the database.
func ResolveComponentParamsDefaults(parserID string, pipelineID *string) (entity.JSONMap, error) {
	isCanvas := pipelineID != nil && strings.TrimSpace(*pipelineID) != ""
	var cp map[string]map[string]any
	var err error
	if isCanvas {
		dslJSON, lerr := loadCanvasDSLJSON(strings.TrimSpace(*pipelineID))
		if lerr != nil {
			return nil, fmt.Errorf("load canvas DSL: %w", lerr)
		}
		cp, err = pipelinepkg.ComponentParamsDefaults(dslJSON)
	} else {
		registry, regErr := pipelinepkg.DefaultRegistry()
		if regErr != nil {
			return nil, fmt.Errorf("builtin registry: %w", regErr)
		}
		if !registry.IsValid(parserID) {
			return nil, fmt.Errorf("unknown builtin parser_id: %q", parserID)
		}
		dslStr, dslErr := pipelinepkg.LoadBuiltinDSL(parserID)
		if dslErr != nil {
			return nil, fmt.Errorf("load builtin DSL: %w", dslErr)
		}
		cp, err = pipelinepkg.ComponentParamsDefaults([]byte(dslStr))
	}
	if err != nil {
		return nil, err
	}
	out := make(entity.JSONMap, len(cp))
	for k, v := range cp {
		out[k] = v
	}
	return out, nil
}

// ValidateDatasetEmbeddingModels checks that all knowledge bases in the list
// either have an embedding model or none do, and that they all use the same model.
func ValidateDatasetEmbeddingModels(kbs []*entity.Knowledgebase) error {
	embdIDs := make(map[string]struct{})
	hasEmbd := false
	noEmbd := false
	for _, kb := range kbs {
		if kb.EmbdID != "" {
			hasEmbd = true
			baseName := kb.EmbdID
			if idx := strings.LastIndex(kb.EmbdID, "@"); idx > 0 {
				baseName = kb.EmbdID[:idx]
				// Strip the second-to-last @-segment too (instance name),
				// matching Python's _base_model_name which uses rsplit("@", 2).
				if idx2 := strings.LastIndex(baseName, "@"); idx2 > 0 {
					baseName = baseName[:idx2]
				}
			}
			embdIDs[baseName] = struct{}{}
		} else {
			noEmbd = true
		}
	}
	if hasEmbd && noEmbd {
		return fmt.Errorf("Cannot search across datasets where some have embedding models and others do not.")
	}
	if len(embdIDs) > 1 {
		return fmt.Errorf("Datasets use different embedding models: %v", getEmbdIDs(kbs))
	}
	return nil
}

func getEmbdIDs(kbs []*entity.Knowledgebase) []string {
	ids := make([]string, len(kbs))
	for i, kb := range kbs {
		ids[i] = kb.EmbdID
	}
	return ids
}

// Backward-compat lowercase aliases for callers within the service package.
// These will be removed when callers are updated to use the exported names.

func loadPipelineDSL(isCanvas bool, parserID string, pipelineID *string) ([]byte, error) {
	return LoadPipelineDSL(isCanvas, parserID, pipelineID)
}

func resolveComponentParamsDefaults(parserID string, pipelineID *string) (entity.JSONMap, error) {
	return ResolveComponentParamsDefaults(parserID, pipelineID)
}

func validateDatasetEmbeddingModels(kbs []*entity.Knowledgebase) error {
	return ValidateDatasetEmbeddingModels(kbs)
}
