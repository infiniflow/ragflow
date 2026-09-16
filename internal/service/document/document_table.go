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
//

package document

import (
	"context"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SaveDocumentTableColumns persists the parser-discovered column names on the document
// and filters stale column roles so the document role selector can offer them without re-reading the file.
func (s *DocumentService) SaveDocumentTableColumns(ctx context.Context, docID string, newNames []string) error {
	if len(newNames) == 0 || docID == "" || dao.DB == nil {
		return nil
	}

	return dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var doc entity.Document
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", docID).
			First(&doc).Error; err != nil {
			return err
		}

		if doc.ParserConfig == nil {
			doc.ParserConfig = entity.JSONMap{}
		}
		seen := make(map[string]struct{}, len(newNames))
		names := make([]string, 0, len(newNames))
		for _, n := range newNames {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			if _, ok := seen[n]; !ok {
				seen[n] = struct{}{}
				names = append(names, n)
			}
		}

		doc.ParserConfig["table_column_names"] = names
		doc.ParserConfig["table_column_roles"] = filterTableColumnRoles(doc.ParserConfig["table_column_roles"], seen)
		for key, value := range doc.ParserConfig {
			if !strings.HasPrefix(key, "Parser:") {
				continue
			}
			componentConfig, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			spreadsheet, ok := componentConfig["spreadsheet"].(map[string]interface{})
			if !ok {
				continue
			}
			spreadsheet["column_names"] = names
			spreadsheet["column_roles"] = filterTableColumnRoles(spreadsheet["column_roles"], seen)
		}
		return tx.Model(&entity.Document{}).Where("id = ?", docID).Update("parser_config", doc.ParserConfig).Error
	})
}

// SaveKBTableFieldMap merges newly discovered or updated metadata fields into the
// knowledgebase's parser_config["field_map"] for NL2SQL querying.
func (s *DocumentService) SaveKBTableFieldMap(ctx context.Context, kbID string, newFieldMap map[string]interface{}) error {
	if len(newFieldMap) == 0 || kbID == "" || dao.DB == nil {
		return nil
	}

	return dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var kb entity.Knowledgebase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", kbID, string(entity.StatusValid)).
			First(&kb).Error; err != nil {
			return err
		}

		if kb.ParserConfig == nil {
			kb.ParserConfig = entity.JSONMap{}
		}
		fm, ok := kb.ParserConfig["field_map"].(map[string]interface{})
		if !ok || fm == nil {
			fm = make(map[string]interface{}, len(newFieldMap))
		}
		for k, v := range newFieldMap {
			fm[k] = v
		}
		kb.ParserConfig["field_map"] = fm
		return tx.Model(&entity.Knowledgebase{}).Where("id = ?", kbID).Update("parser_config", kb.ParserConfig).Error
	})
}

func filterTableColumnRoles(raw any, columns map[string]struct{}) map[string]interface{} {
	filtered := make(map[string]interface{})
	switch roles := raw.(type) {
	case map[string]interface{}:
		for column, role := range roles {
			if _, exists := columns[column]; exists {
				filtered[column] = role
			}
		}
	case map[string]string:
		for column, role := range roles {
			if _, exists := columns[column]; exists {
				filtered[column] = role
			}
		}
	}
	return filtered
}
