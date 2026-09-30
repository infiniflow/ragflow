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
	"ragflow/internal/common"
	"strings"
	"unicode"

	infinity "github.com/infiniflow/infinity-go-sdk"

	"go.uber.org/zap"
)

// dropTable drops a table from Infinity
func (e *Engine) dropTable(ctx context.Context, tableName string) error {
	if tableName == "" {
		return fmt.Errorf("table name cannot be empty")
	}

	db, release, err := e.client.checkoutDatabase(ctx, "common.go")
	if err != nil {
		return fmt.Errorf("failed to get database: %w", err)
	}
	defer release()

	// Check if table exists
	exists, err := e.tableExistsWithDB(db, tableName)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("table '%s' does not exist", tableName)
	}

	_, err = db.DropTable(tableName, infinity.ConflictTypeError)
	if err != nil {
		return fmt.Errorf("failed to drop table: %w", err)
	}

	common.Info("Infinity dropped table", zap.String("tableName", tableName))
	return nil
}

// tableExists checks if a table exists in Infinity
func (e *Engine) tableExists(ctx context.Context, tableName string) (bool, error) {
	if tableName == "" {
		return false, fmt.Errorf("table name cannot be empty")
	}

	db, release, err := e.client.checkoutDatabase(ctx, "common.go")
	if err != nil {
		return false, fmt.Errorf("failed to get database: %w", err)
	}
	defer release()

	return e.tableExistsWithDB(db, tableName)
}

func (e *Engine) tableExistsWithDB(db *infinity.Database, tableName string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("database is nil")
	}

	// Try to get the table - if it exists, no error
	_, err := db.GetTable(tableName)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "doesn't exist") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check table existence: %w", err)
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

// keywordFilterCondition renders a filter for a *_kwd column.
//
// Infinity declares these columns as analyzed varchars, so filter_fulltext()
// tokenizes the query and a multi-token value matches NOTHING — while every
// navigation cluster name is multi-token, so nav's child lookup (parent_kwd=…),
// its cluster updates and cleanupEmptyCluster all silently matched no row.
// Elasticsearch matches a plain *_kwd column exactly (keyword + term/terms), so
// a whitespace-bearing value becomes `col = 'value'`: it matched no row before,
// so this cannot narrow an existing match, and single-token values keep
// filter_fulltext for the legacy ###-joined columns (see fieldJSONList).
//
// tag_kwd and toc_kwd are the exception: convertMatchingField turns them into a
// full-text index reference ("tag_kwd@ft_tag_kwd_whitespace__"), which is not a
// column and cannot be compared with `=` (Infinity rejects the whole statement).
// Their cells hold several ###-joined values, and that index is what gives them
// the membership match ES gets from its keyword array, so a multi-token value is
// matched as a quoted phrase through it.
func keywordFilterCondition(field string, value string) string {
	escaped := escapeFilterValue(value)
	multiToken := strings.ContainsFunc(value, unicode.IsSpace)
	indexRef := convertMatchingField(field)

	if indexRef == field {
		if multiToken {
			return fmt.Sprintf("%s = '%s'", field, escaped)
		}
		return fmt.Sprintf("filter_fulltext('%s', '%s')", field, escaped)
	}
	if !multiToken {
		return fmt.Sprintf("filter_fulltext('%s', '%s')", indexRef, escaped)
	}
	if strings.Contains(value, `"`) {
		// Cannot be phrased: compare the raw column so the value still matches a
		// single-valued cell instead of failing the statement.
		return fmt.Sprintf("%s = '%s'", field, escaped)
	}
	return fmt.Sprintf("filter_fulltext('%s', '\"%s\"')", indexRef, escaped)
}

// fieldJSON reports fields stored as Infinity JSON columns. The Infinity Go
// SDK expects JSON columns as encoded strings, not Go slices or maps. Column
// names are matched case-insensitively so the write path (transformChunkFields)
// and the read path (decodeJSONFields) agree on every spelling.
func fieldJSON(fieldName string) bool {
	switch strings.ToLower(fieldName) {
	case "source_chunk_ids", "source_doc_ids", "compilation_template_ids",
		"doc_ids_kwd", "entity_names_kwd", "entity_names", "outlinks_kwd",
		"related_kb_pages_kwd", "claims", "page_ids", "source_chunk_hashes",
		"rechunked_from_chunk_ids", "aliases":
		return true
	default:
		return false
	}
}

// fieldJSONList reports JSON columns whose value is an array and whose filter
// semantics are membership rather than whole-document equality.
func fieldJSONList(fieldName string) bool {
	switch strings.ToLower(fieldName) {
	case "source_chunk_ids", "source_doc_ids", "compilation_template_ids",
		"doc_ids_kwd", "entity_names_kwd", "entity_names", "outlinks_kwd",
		"related_kb_pages_kwd", "claims", "page_ids",
		"rechunked_from_chunk_ids", "aliases":
		return true
	default:
		return false
	}
}

// joinBalanced renders a boolean chain as a balanced tree instead of the flat,
// left-deep chain strings.Join produces. Infinity's filter string is parsed by
// the SDK into a Thrift expression tree (infinity-go-sdk expression_parser.go)
// and sent as that tree, so a left-deep chain of N terms nests N levels deep: a
// filter with 22 OR'ed filter_fulltext(...) clauses (the wiki contribution and
// graph reads build exactly that) made Infinity abort the connection with
//
//	Thrift: ... TProtocolException: Exceeded depth limit
//
// which our callers then see as "InfinityException(7018, Failed to execute
// query: EOF)". A balanced tree keeps the depth at log2(N) — 22 terms pass
// against a live Infinity, verified — while matching exactly the same rows.
// The parenthesization is boolean-equivalent to the flat chain (a single term
// needs no parentheses).
func joinBalanced(parts []string, op string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		// Keep the single-element form parenthesized: callers (and tests) relied
		// on a self-contained group, e.g. "(1=0)" for a never-matching list.
		return "(" + parts[0] + ")"
	case 2:
		return "(" + parts[0] + op + parts[1] + ")"
	default:
		mid := (len(parts) + 1) / 2
		return "(" + joinBalanced(parts[:mid], op) + op + joinBalanced(parts[mid:], op) + ")"
	}
}

func jsonListFilterConditions(fieldName string, value interface{}, tableColumns map[string]struct {
	Type    string
	Default interface{}
}) []string {
	values := []interface{}{value}
	switch typed := value.(type) {
	case []string:
		values = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			values = append(values, item)
		}
	case []interface{}:
		values = typed
	}

	column, found := tableColumns[fieldName]
	if !found {
		return []string{"1=0"}
	}
	columnType := ""
	columnType = strings.ToLower(column.Type)
	conditions := make([]string, 0, len(values))
	for _, item := range values {
		switch {
		case strings.Contains(columnType, "json"):
			literal, err := json.Marshal(item)
			if err != nil {
				continue
			}
			conditions = append(conditions, fmt.Sprintf("json_contains(%s, '%s')",
				fieldName, strings.ReplaceAll(string(literal), "'", "''")))
		case strings.Contains(columnType, "char"):
			text, ok := item.(string)
			if !ok || text == "" {
				continue
			}
			conditions = append(conditions, fmt.Sprintf("filter_fulltext('%s', '%s')",
				convertMatchingField(fieldName), strings.ReplaceAll(text, "'", "''")))
		}
	}
	if len(conditions) == 0 {
		return []string{"1=0"}
	}
	return conditions
}

func hasJSONListFilter(condition map[string]interface{}) bool {
	for fieldName := range condition {
		if fieldJSONList(fieldName) {
			return true
		}
	}
	return false
}

func loadTableColumns(table *infinity.Table) (map[string]struct {
	Type    string
	Default interface{}
}, error) {
	columns := make(map[string]struct {
		Type    string
		Default interface{}
	})
	response, err := table.ShowColumns()
	if err != nil {
		return nil, err
	}
	result, ok := response.(*infinity.QueryResult)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", response)
	}
	names := result.Data["name"]
	types := result.Data["type"]
	defaults := result.Data["default"]
	for i, rawName := range names {
		name, _ := rawName.(string)
		columnType := ""
		if i < len(types) {
			columnType, _ = types[i].(string)
		}
		var defaultValue interface{}
		if i < len(defaults) {
			defaultValue = defaults[i]
		}
		columns[name] = struct {
			Type    string
			Default interface{}
		}{Type: columnType, Default: defaultValue}
	}
	return columns, nil
}

// existsCondition builds a NOT EXISTS or field!=" condition
func existsCondition(field string, tableColumns map[string]struct {
	Type    string
	Default interface{}
}) string {
	col, colOk := tableColumns[field]
	if !colOk {
		common.Warn(fmt.Sprintf("Column '%s' not found in table columns", field))
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

		// JSON-list fields use member containment. Legacy tables stored these
		// columns as ###-joined varchar values, so retain their full-text fallback.
		if fieldJSONList(k) && tableColumns != nil {
			if jsonConditions := jsonListFilterConditions(k, v, tableColumns); len(jsonConditions) > 0 {
				conditions = append(conditions, joinBalanced(jsonConditions, " OR "))
			}
			continue
		}

		// Handle keyword fields -> exact match, or filter_fulltext for the
		// single-token values Infinity can tokenize (see keywordFilterCondition).
		if fieldKeyword(k) {
			var orConds []string
			addKeyword := func(item string) {
				// Blank entries are dropped exactly as the search path drops them:
				// Infinity rejects an empty full-text query ("Trying to match:  on
				// fields: <column> failed", 3052) and fails the whole
				// UpdateChunks/DeleteChunks statement, so one empty element in a
				// list built from optional values must not poison it. A condition
				// left with no clause at all is refused by the callers' "1=1"
				// guard, so this cannot widen an update or delete.
				if strings.TrimSpace(item) == "" {
					return
				}
				orConds = append(orConds, keywordFilterCondition(k, item))
			}

			switch val := v.(type) {
			case []string:
				for _, item := range val {
					addKeyword(item)
				}
			case []interface{}:
				for _, item := range val {
					addKeyword(fmt.Sprintf("%v", item))
				}
			case string:
				addKeyword(val)
			default:
				addKeyword(fmt.Sprintf("%v", val))
			}

			if len(orConds) > 0 {
				conditions = append(conditions, joinBalanced(orConds, " OR "))
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
		if strListVal, ok := v.([]string); ok {
			var inVals []string
			for _, item := range strListVal {
				item = strings.ReplaceAll(item, "'", "''")
				inVals = append(inVals, fmt.Sprintf("'%s'", item))
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

// columnExists checks if a column exists in the table
func (e *Engine) columnExists(table *infinity.Table, columnName string) (bool, error) {
	colsResp, err := table.ShowColumns()
	if err != nil {
		return false, err
	}

	result, ok := colsResp.(*infinity.QueryResult)
	if !ok {
		return false, fmt.Errorf("unexpected response type: %T", colsResp)
	}

	// ShowColumns returns a result set where Data contains arrays of column values
	if nameArr, ok := result.Data["name"]; ok {
		for i := 0; i < len(nameArr); i++ {
			colName, _ := nameArr[i].(string)
			if colName == columnName {
				return true, nil
			}
		}
	}
	return false, nil
}

// buildChunkTableName returns the chunk table name for a dataset
// Skill Table: table name is just baseName (e.g., "skill_abc123_def456")
// Regular chunk Table: table name is {baseName}_{datasetID}
func buildChunkTableName(baseName, datasetID string) string {
	if datasetID == "skill" {
		return baseName
	}
	return fmt.Sprintf("%s_%s", baseName, datasetID)
}

// buildMetadataTableName returns the metadata table name for a tenant
func buildMetadataTableName(tenantID string) string {
	return fmt.Sprintf("ragflow_doc_meta_%s", tenantID)
}
