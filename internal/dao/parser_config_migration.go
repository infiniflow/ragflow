//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package dao

import (
	"context"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	parserchunk "ragflow/internal/parser/chunk"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	legacyGeneralChunkerID  = "TokenChunker:SixApplesFall"
	currentGeneralChunkerID = "GeneralChunker:SixApplesFall"
)

var generalChunkerParamKeys = map[string]struct{}{
	"children_delimiters": {},
	"chunk_token_size":    {},
	"delimiter":           {},
	"delimiters":          {},
	"image_context_size":  {},
	"overlapped_percent":  {},
	"table_context_size":  {},
}

// migrateGeneralChunkerConfig rewrites the exact legacy component key used by
// the built-in general pipeline. Explicit values already stored under the new
// key win; unsupported TokenChunker-only fields are dropped because
// GeneralChunker has no delimiter_mode or outputs parameter.
func migrateGeneralChunkerConfig(config map[string]interface{}) bool {
	legacy, ok := config[legacyGeneralChunkerID]
	if !ok {
		return false
	}
	legacyParams, ok := legacy.(map[string]interface{})
	if !ok {
		return false
	}

	if _, exists := config[currentGeneralChunkerID]; !exists {
		params := make(map[string]interface{}, len(legacyParams))
		for key, value := range legacyParams {
			if key == "delimiter" {
				if _, exists := legacyParams["delimiters"]; !exists {
					if delimiters, ok := normalizeLegacyDelimiter(value); ok {
						params["delimiters"] = delimiters
					}
				}
				continue
			}
			if _, supported := generalChunkerParamKeys[key]; supported {
				params[key] = value
			}
		}
		config[currentGeneralChunkerID] = params
	}
	delete(config, legacyGeneralChunkerID)
	return true
}

func migrateGeneralChunkerParserConfigs(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return nil
	}

	var migratedKnowledgebases int
	if db.WithContext(ctx).Migrator().HasTable("knowledgebase") {
		count, err := migrateParserConfigRows(ctx, db, &entity.Knowledgebase{})
		if err != nil {
			return fmt.Errorf("migrate knowledgebase parser configs: %w", err)
		}
		migratedKnowledgebases = count
	}

	var migratedDocuments int
	if db.WithContext(ctx).Migrator().HasTable("document") {
		count, err := migrateParserConfigRows(ctx, db, &entity.Document{})
		if err != nil {
			return fmt.Errorf("migrate document parser configs: %w", err)
		}
		migratedDocuments = count
	}

	if migratedKnowledgebases > 0 || migratedDocuments > 0 {
		common.Info("Migrated general chunker parser configs",
			zap.Int("knowledgebases", migratedKnowledgebases),
			zap.Int("documents", migratedDocuments))
	}
	return nil
}

const parserConfigMigrationBatchSize = 256

type parserConfigMigrationRow struct {
	ID           string         `gorm:"column:id"`
	ParserConfig entity.JSONMap `gorm:"column:parser_config"`
}

func migrateParserConfigRows(ctx context.Context, db *gorm.DB, model any) (int, error) {
	var migrated int
	query := db.WithContext(ctx).
		Model(model).
		Select("id", "parser_config").
		Where("parser_id IN ? AND (pipeline_id IS NULL OR pipeline_id = '')", []string{"general", "naive"}).
		Order("id")
	var rows []parserConfigMigrationRow
	result := query.FindInBatches(&rows, parserConfigMigrationBatchSize, func(batchDB *gorm.DB, _ int) error {
		return batchDB.Transaction(func(tx *gorm.DB) error {
			for _, row := range rows {
				config := map[string]interface{}(row.ParserConfig)
				if !migrateGeneralChunkerConfig(config) {
					continue
				}
				if err := tx.Model(model).Where("id = ?", row.ID).
					Update("parser_config", entity.JSONMap(config)).Error; err != nil {
					return fmt.Errorf("update parser config %q: %w", row.ID, err)
				}
				migrated++
			}
			return nil
		})
	})
	if result.Error != nil {
		return migrated, result.Error
	}
	return migrated, nil
}

func normalizeLegacyDelimiter(value any) (any, bool) {
	switch value := value.(type) {
	case string:
		delimiters := parserchunk.ParseDelimiterField(value)
		out := make([]interface{}, len(delimiters))
		for i, delimiter := range delimiters {
			out[i] = delimiter
		}
		return out, true
	case []string:
		out := make([]interface{}, len(value))
		for i, delimiter := range value {
			out[i] = delimiter
		}
		return out, true
	case []interface{}:
		return value, true
	default:
		return nil, false
	}
}
