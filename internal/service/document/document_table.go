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

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SaveDocumentTableColumns publishes the parser-discovered column names on the
// document, and filters stale column roles so the document role selector can
// offer them without re-reading the file.
//
// Discovery is written to the root keys only. The component entries of
// parser_config belong to the canvas DSL, so mirroring a run's findings into
// them would let system output masquerade as an author's configuration.
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
			// Keep the name as the parser wrote it: it is the key both the role
			// lookup and chunk_data use, so trimming or dropping a blank one
			// would orphan that column's role (rag/app/table.py:590-595).
			if _, ok := seen[n]; !ok {
				seen[n] = struct{}{}
				names = append(names, n)
			}
		}

		doc.ParserConfig["table_column_names"] = names
		doc.ParserConfig["table_column_roles"] = filterTableColumnRoles(doc.ParserConfig["table_column_roles"], seen)
		return tx.Model(&entity.Document{}).Where("id = ?", docID).Update("parser_config", doc.ParserConfig).Error
	})
}

// SaveKBTableState publishes a table-parser run's schema to the dataset
// (knowledgebase) parser_config: the discovered column names, plus the
// field_map the SQL retrieval path reads. Mirrors Python's table chunker, which
// updates the knowledgebase with table_column_names + field_map on every parse
// (rag/app/table.py:600-603).
//
// Both keys are REPLACED rather than merged: the discovered schema is the
// current file's schema, so a column that disappeared, or whose role changed
// away from metadata/both, must stop being offered to the SQL prompt. A nil
// fieldMap means the run's engine does not maintain a dataset field_map (its
// chunks carry no chunk_data column), and leaves that key untouched.
func (s *DocumentService) SaveKBTableState(ctx context.Context, kbID string, names []string, fieldMap map[string]interface{}) error {
	if kbID == "" || dao.DB == nil || (len(names) == 0 && fieldMap == nil) {
		return nil
	}

	columns := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, n := range names {
		// Same rule as the document's own copy: the published schema must name
		// the columns exactly as the run indexed them.
		if _, ok := seen[n]; !ok {
			seen[n] = struct{}{}
			columns = append(columns, n)
		}
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
		if len(columns) > 0 {
			kb.ParserConfig["table_column_names"] = columns
		}
		if fieldMap != nil {
			kb.ParserConfig["field_map"] = fieldMap
		}
		return tx.Model(&entity.Knowledgebase{}).Where("id = ?", kbID).Update("parser_config", kb.ParserConfig).Error
	})
}

// filterTableColumnRoles keeps only the roles whose column survives a file's own
// schema, so a role configured against another file's column cannot reach
// ingestion through this document's config. A value that is not a role map — the
// key is absent, or it holds a non-object — filters to an empty map, which every
// resolver reads as "no column carries a role".
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
