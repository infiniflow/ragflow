package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/dao/transaction"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"

	"gorm.io/gorm"
)

// ErrPipelineVersionMissing is the single multi-version Missing sentinel. It is
// defined once in the dao package (dao.ErrPipelineVersionMissing) and re-exported
// here so existing callers (e.g. dataset CRUD) keep a stable symbol while the
// error identity matches across the service and task layers (review: merge the
// duplicate service/task sentinels into one implementation).
var ErrPipelineVersionMissing = dao.ErrPipelineVersionMissing

// ErrPipelineVersionEmpty is re-exported from dao so callers (dataset/document
// CRUD) keep a single stable symbol. It signals a Pipeline-mode Save with no
// concrete pinned version (design C5/G3: empty version is rejected, not silently
// backfilled to the canvas latest).
var ErrPipelineVersionEmpty = dao.ErrPipelineVersionEmpty

// loadCanvasDSLJSON returns the DSL JSON for a custom canvas pipeline.
//
// For dataflow canvases (canvas_category = dataflow_canvas) the read is versioned
// per design §6.1: an explicit versionID resolves that exact version; otherwise
// the latest version is used, and a canvas with no version rows surfaces
// ErrPipelineVersionMissing (no fallback to the legacy draft column). Agent
// canvases are out of the multi-version scope (design §1) and keep reading the
// live draft column directly. It is a package-level function so both document
// and knowledge-base updates reuse it.
func loadCanvasDSLJSON(ctx context.Context, canvasID string, versionID *string) ([]byte, error) {
	if strings.TrimSpace(canvasID) == "" {
		return nil, fmt.Errorf("empty canvas id")
	}
	canvas, err := dao.NewUserCanvasDAO().GetByID(ctx, dao.DB, canvasID)
	if err != nil {
		if errors.Is(err, dao.ErrUserCanvasNotFound) {
			return nil, fmt.Errorf("canvas %s not found", canvasID)
		}
		return nil, fmt.Errorf("load canvas %s: %w", canvasID, err)
	}
	if canvas.CanvasCategory != "dataflow_canvas" {
		// Agent canvases: out of scope for multi-version; read the live draft.
		if len(canvas.DSL) == 0 {
			return nil, fmt.Errorf("canvas %s has no DSL", canvasID)
		}
		return marshalCanvasDSL(canvasID, canvas.DSL)
	}
	// Dataflow canvases are versioned (design section 6.1). An explicit versionID
	// pins that exact version; an empty versionID resolves the latest published
	// version. A missing, mis-owned, or empty-DSL version surfaces
	// ErrPipelineVersionMissing -- there is no legacy-draft fallback for dataflow.
	// The resolution is delegated to dao.LoadCanvasVersionDSL so the service and
	// task layers share one implementation (review: merge duplicate resolvers).
	pin := ""
	if versionID != nil {
		pin = strings.TrimSpace(*versionID)
	}
	return dao.LoadCanvasVersionDSL(ctx, dao.DB, canvasID, pin)
}

func marshalCanvasDSL(canvasID string, dsl entity.JSONMap) ([]byte, error) {
	raw, err := json.Marshal(dsl)
	if err != nil {
		return nil, fmt.Errorf("marshal canvas %s DSL: %w", canvasID, err)
	}
	return raw, nil
}

// LoadPipelineDSL loads the DSL JSON for a pipeline identified by parserID
// (built-in) or pipelineID (custom canvas). When both are provided, isPipeline
// selects which one to use. versionID selects a specific canvas version; when nil
// the latest version (or legacy canvas dsl) is used.
func LoadPipelineDSL(ctx context.Context, isPipeline bool, parserID string, pipelineID *string, versionID *string) ([]byte, error) {
	if isPipeline {
		return loadCanvasDSLJSON(ctx, strings.TrimSpace(*pipelineID), versionID)
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
// canvas pipelines it is loaded from the canvas version (or legacy canvas row).
func ResolveComponentParamsDefaults(ctx context.Context, parserID string, pipelineID *string, versionID *string) (entity.JSONMap, error) {
	isPipeline := pipelineID != nil && strings.TrimSpace(*pipelineID) != ""
	var cp map[string]map[string]any
	var err error
	if isPipeline {
		dslJSON, lerr := loadCanvasDSLJSON(ctx, strings.TrimSpace(*pipelineID), versionID)
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
// either have an embedding model or none do, and that they all resolve to the
// same base embedding model name (e.g. "BAAI/bge-m3"). Embedding references
// are resolved through tenant_model first, so datasets storing a raw
// tenant_model id and datasets storing a legacy "model@instance@provider"
// composite compare equal when they point at the same model.
func ValidateDatasetEmbeddingModels(ctx context.Context, db *gorm.DB, kbs []*entity.Knowledgebase) error {
	embdNames := make(map[string]struct{})
	hasEmbd := false
	noEmbd := false
	names := transaction.NewEmbeddingBaseNameResolver(dao.NewKnowledgebaseDAO().EmbeddingBaseName)
	for _, kb := range kbs {
		if kb.EmbdID != "" {
			hasEmbd = true
			embdNames[names.Resolve(ctx, db, kb)] = struct{}{}
		} else {
			noEmbd = true
		}
	}
	if hasEmbd && noEmbd {
		return fmt.Errorf("cannot search across datasets where some have embedding models and others do not")
	}
	if len(embdNames) > 1 {
		return fmt.Errorf("datasets use different embedding models: %v", getEmbdIDs(kbs))
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
