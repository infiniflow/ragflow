package service

import (
	"context"
	"encoding/json"
	"fmt"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"

	"gorm.io/gorm"
)

const dataflowCanvasCategory = "dataflow_canvas"

const rebaseBatchSize = 256

// rebaseBoundPipelineParserConfigs drops parser_config entries on datasets and
// documents bound to a just-edited dataflow canvas that merely mirror the
// canvas's previous defaults. The bind-time snapshot overrides the DSL at run
// time, so without this rebase an edited pipeline never reaches bound datasets
// short of deleting and recreating them.
func rebaseBoundPipelineParserConfigs(ctx context.Context, tx *gorm.DB, canvasID string, oldDSL, newDSL entity.JSONMap) error {
	oldJSON, err := json.Marshal(oldDSL)
	if err != nil {
		return fmt.Errorf("rebound pipeline configs: marshal old dsl: %w", err)
	}
	newJSON, err := json.Marshal(newDSL)
	if err != nil {
		return fmt.Errorf("rebound pipeline configs: marshal new dsl: %w", err)
	}
	if string(oldJSON) == string(newJSON) {
		return nil
	}
	if err := rebaseModelParserConfigs(ctx, tx, canvasID, &entity.Knowledgebase{}, oldJSON, newJSON); err != nil {
		return fmt.Errorf("rebase dataset parser configs for canvas %s: %w", canvasID, err)
	}
	if err := rebaseModelParserConfigs(ctx, tx, canvasID, &entity.Document{}, oldJSON, newJSON); err != nil {
		return fmt.Errorf("rebase document parser configs for canvas %s: %w", canvasID, err)
	}
	return nil
}

type parserConfigRow struct {
	ID           string         `gorm:"column:id"`
	ParserConfig entity.JSONMap `gorm:"column:parser_config"`
}

func rebaseModelParserConfigs(ctx context.Context, tx *gorm.DB, pipelineID string, model any, oldJSON, newJSON []byte) error {
	var rows []parserConfigRow
	result := tx.WithContext(ctx).
		Model(model).
		Select("id", "parser_config").
		Where("pipeline_id = ?", pipelineID).
		Order("id").
		FindInBatches(&rows, rebaseBatchSize, func(batchDB *gorm.DB, _ int) error {
			for _, row := range rows {
				rebased := pipelinepkg.RebaseStoredParamsOnPipelineEdit(map[string]any(row.ParserConfig), oldJSON, newJSON)
				if jsonMapsEqual(rebased, map[string]any(row.ParserConfig)) {
					continue
				}
				if err := batchDB.Model(model).Where("id = ?", row.ID).
					Update("parser_config", entity.JSONMap(rebased)).Error; err != nil {
					return fmt.Errorf("update parser config %q: %w", row.ID, err)
				}
			}
			return nil
		})
	if result.Error != nil {
		if dao.IsNotFoundErr(result.Error) {
			return nil
		}
		return result.Error
	}
	return nil
}

func jsonMapsEqual(a, b map[string]any) bool {
	ab, aerr := json.Marshal(a)
	bb, berr := json.Marshal(b)
	return aerr == nil && berr == nil && string(ab) == string(bb)
}
