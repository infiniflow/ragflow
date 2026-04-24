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

package infinity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/logger"

	infinity "github.com/infiniflow/infinity-go-sdk"
)

// Delete deletes rows from either a dataset table or metadata table.
// If indexName starts with "ragflow_doc_meta_", it's a metadata table.
// Otherwise, it's a dataset table: {indexName}_{datasetID}
func (e *infinityEngine) Delete(ctx context.Context, condition map[string]interface{}, indexName string, datasetID string) (int64, error) {
	var tableName string
	if strings.HasPrefix(indexName, "ragflow_doc_meta_") {
		tableName = indexName
	} else {
		tableName = fmt.Sprintf("%s_%s", indexName, datasetID)
	}

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return 0, fmt.Errorf("failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		logger.Warn(fmt.Sprintf("Table %s does not exist, skipping delete", tableName))
		return 0, nil
	}

	// Get table columns for building filter
	clmns := make(map[string]struct {
		Type    string
		Default interface{}
	})
	colsResp, err := table.ShowColumns()
	if err != nil {
		return 0, fmt.Errorf("failed to get columns: %w", err)
	}
	result, ok := colsResp.(*infinity.QueryResult)
	if ok {
		if nameArr, ok := result.Data["name"]; ok {
			if typeArr, ok := result.Data["type"]; ok {
				if defArr, ok := result.Data["default"]; ok {
					for i := 0; i < len(nameArr); i++ {
						colName, _ := nameArr[i].(string)
						colType, _ := typeArr[i].(string)
						var colDefault interface{}
						if i < len(defArr) {
							colDefault = defArr[i]
						}
						clmns[colName] = struct {
							Type    string
							Default interface{}
						}{colType, colDefault}
					}
				}
			}
		}
	}

	// Build filter from condition
	filter := buildFilterFromCondition(condition, clmns)

	delResp, err := table.Delete(filter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete: %w", err)
	}

	return delResp.DeletedRows, nil
}

// DropTable deletes a table/index
func (e *infinityEngine) DropTable(ctx context.Context, indexName string) error {
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("Failed to get database: %w", err)
	}

	_, err = db.DropTable(indexName, infinity.ConflictTypeIgnore)
	if err != nil {
		return fmt.Errorf("Failed to drop table: %w", err)
	}
	return nil
}

// TableExists checks if table/index exists
func (e *infinityEngine) TableExists(ctx context.Context, indexName string) (bool, error) {
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return false, fmt.Errorf("Failed to get database: %w", err)
	}

	_, err = db.GetTable(indexName)
	if err != nil {
		// Check if error is "table not found"
		errLower := strings.ToLower(err.Error())
		if strings.Contains(errLower, "not found") || strings.Contains(errLower, "notexist") || strings.Contains(errLower, "doesn't exist") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// fieldInfo represents a field in the infinity mapping schema
type fieldInfo struct {
	Type      string      `json:"type"`
	Default   interface{} `json:"default"`
	Analyzer  interface{} `json:"analyzer"`   // string or []string
	IndexType interface{} `json:"index_type"` // string or map
	Comment   string      `json:"comment"`
}

// orderedFields preserves the order of fields as defined in JSON
type orderedFields struct {
	Keys   []string
	Fields map[string]fieldInfo
}

func (o *orderedFields) UnmarshalJSON(data []byte) error {
	// Parse JSON manually to preserve key order
	// Look for key names by scanning the JSON string
	// This is a simple approach: find {"key": value, "key2": value2...}
	o.Fields = make(map[string]fieldInfo)
	o.Keys = make([]string, 0)

	// Use a streaming JSON parser approach
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); ok && delim == '{' {
		for dec.More() {
			// Read key
			tok, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := tok.(string)
			if !ok {
				continue
			}
			o.Keys = append(o.Keys, key)

			// Read value into fieldInfo
			var field fieldInfo
			if err := dec.Decode(&field); err != nil {
				return err
			}
			o.Fields[key] = field
		}
	}
	return nil
}

// fieldKeyword checks if field is a keyword field
func fieldKeyword(fieldName string) bool {
	if fieldName == "source_id" {
		return true
	}
	if strings.HasSuffix(fieldName, "_kwd") &&
		fieldName != "knowledge_graph_kwd" &&
		fieldName != "docnm_kwd" &&
		fieldName != "important_kwd" &&
		fieldName != "question_kwd" {
		return true
	}
	return false
}

// existsCondition builds a NOT EXISTS or field!=" condition
func existsCondition(field string, tableColumns map[string]struct {
	Type    string
	Default interface{}
}) string {
	col, colOk := tableColumns[field]
	if !colOk {
		logger.Warn(fmt.Sprintf("Column '%s' not found in table columns", field))
		return fmt.Sprintf("%s!=null", field)
	}
	if strings.Contains(strings.ToLower(col.Type), "char") {
		if col.Default != nil {
			return fmt.Sprintf(" %s!='%v' ", field, col.Default)
		}
		return fmt.Sprintf(" %s!='' ", field)
	}
	if col.Default != nil {
		return fmt.Sprintf("%s!=%v", field, col.Default)
	}
	return fmt.Sprintf("%s!=null", field)
}

func buildFilterFromCondition(condition map[string]interface{}, tableColumns map[string]struct {
	Type    string
	Default interface{}
}) string {
	var conditions []string

	for k, v := range condition {
		if v == nil {
			continue
		}
		if strVal, ok := v.(string); ok && strVal == "" {
			continue
		}

		// Handle must_not conditions -> NOT (...)
		if k == "must_not" {
			if mustNotMap, ok := v.(map[string]interface{}); ok {
				for kk, vv := range mustNotMap {
					if kk == "exists" {
						if existsField, ok := vv.(string); ok {
							conditions = append(conditions, fmt.Sprintf("NOT (%s)", existsCondition(existsField, tableColumns)))
						}
					}
				}
			}
			continue
		}

		// Handle keyword fields -> filter_fulltext with converted field name
		if fieldKeyword(k) {
			var orConds []string
			addFullText := func(item string) {
				item = strings.ReplaceAll(item, "'", "''")
				orConds = append(orConds, fmt.Sprintf("filter_fulltext('%s', '%s')", convertMatchingField(k), item))
			}

			switch val := v.(type) {
			case []string:
				for _, item := range val {
					addFullText(item)
				}
			case []interface{}:
				for _, item := range val {
					addFullText(fmt.Sprintf("%v", item))
				}
			case string:
				addFullText(val)
			default:
				addFullText(fmt.Sprintf("%v", val))
			}

			if len(orConds) > 0 {
				conditions = append(conditions, "("+strings.Join(orConds, " OR ")+")")
			}
			continue
		}

		// Handle list values (IN condition)
		if listVal, ok := v.([]interface{}); ok {
			var inVals []string
			for _, item := range listVal {
				if strItem, ok := item.(string); ok {
					strItem = strings.ReplaceAll(strItem, "'", "''")
					inVals = append(inVals, fmt.Sprintf("'%s'", strItem))
				} else {
					inVals = append(inVals, fmt.Sprintf("%v", item))
				}
			}
			if len(inVals) > 0 {
				conditions = append(conditions, fmt.Sprintf("%s IN (%s)", k, strings.Join(inVals, ", ")))
			}
			continue
		}

		// Handle exists condition
		if k == "exists" {
			if existsField, ok := v.(string); ok {
				conditions = append(conditions, existsCondition(existsField, tableColumns))
			}
			continue
		}

		// Handle string values
		if strVal, ok := v.(string); ok {
			strVal = strings.ReplaceAll(strVal, "'", "''")
			conditions = append(conditions, fmt.Sprintf("%s='%s'", k, strVal))
			continue
		}

		// Handle other values
		conditions = append(conditions, fmt.Sprintf("%s=%v", k, v))
	}

	if len(conditions) == 0 {
		return "1=1"
	}
	return strings.Join(conditions, " AND ")
}
