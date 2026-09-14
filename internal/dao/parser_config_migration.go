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

	var knowledgebases []entity.Knowledgebase
	if db.WithContext(ctx).Migrator().HasTable("knowledgebase") {
		if err := db.WithContext(ctx).
			Where("parser_id IN ? AND pipeline_id IS NULL", []string{"general", "naive"}).
			Find(&knowledgebases).Error; err != nil {
			return fmt.Errorf("load knowledgebase parser configs: %w", err)
		}
	}
	var migratedKnowledgebases int
	for _, kb := range knowledgebases {
		config := map[string]interface{}(kb.ParserConfig)
		if !migrateGeneralChunkerConfig(config) {
			continue
		}
		if err := db.WithContext(ctx).Model(&entity.Knowledgebase{}).
			Where("id = ?", kb.ID).
			Update("parser_config", entity.JSONMap(config)).Error; err != nil {
			return fmt.Errorf("update knowledgebase %q parser config: %w", kb.ID, err)
		}
		migratedKnowledgebases++
	}

	var documents []entity.Document
	if db.WithContext(ctx).Migrator().HasTable("document") {
		if err := db.WithContext(ctx).
			Where("parser_id IN ? AND pipeline_id IS NULL", []string{"general", "naive"}).
			Find(&documents).Error; err != nil {
			return fmt.Errorf("load document parser configs: %w", err)
		}
	}
	var migratedDocuments int
	for _, doc := range documents {
		config := map[string]interface{}(doc.ParserConfig)
		if !migrateGeneralChunkerConfig(config) {
			continue
		}
		if err := db.WithContext(ctx).Model(&entity.Document{}).
			Where("id = ?", doc.ID).
			Update("parser_config", entity.JSONMap(config)).Error; err != nil {
			return fmt.Errorf("update document %q parser config: %w", doc.ID, err)
		}
		migratedDocuments++
	}

	if migratedKnowledgebases > 0 || migratedDocuments > 0 {
		common.Info("Migrated general chunker parser configs",
			zap.Int("knowledgebases", migratedKnowledgebases),
			zap.Int("documents", migratedDocuments))
	}
	return nil
}
