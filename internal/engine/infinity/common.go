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
	"regexp"
	"strings"

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

// analyzerLanguages are the dataset languages that need their own RAG
// analyzer. Infinity's analyzer folds their diacritics to ASCII and skips
// stemming, so a dataset in one of them has to be indexed and queried through
// "rag-<language>" instead of the default "rag-coarse"/"rag-fine" pair.
//
// They are the only two languages whose analyzer differs from the default:
// every other name either selects a Snowball stemmer (english, dutch, german,
// ... -- SNOWBALL_LANGUAGE_MAP in Infinity's rag_analyzer) or is a no-op
// (Chinese, Japanese, Korean, ... use dictionary segmentation, not stemming).
// Naming a stemmer here would switch it for every newly created dataset in
// that language, which is a behaviour change of its own.
//
// Mirrors _ANALYZER_LANGUAGES in common/doc_store/infinity_conn_base.py.
var analyzerLanguages = map[string]struct{}{
	"czech":  {},
	"slovak": {},
}

// ragAnalyzer is the analyzer family whose behaviour the dataset language
// changes. Fields indexed with "whitespace-#" or "rankfeatures" carry
// keywords, not prose.
const ragAnalyzer = "rag"

// analyzerForLanguage returns the analyzer to index and query a dataset in
// language with. Infinity analyzes the query with the analyzer of the fulltext
// index it matches against, so the language has to be baked into the index
// when it is created -- there is no per-query analyzer to pass. Chunk tables
// are per-dataset, which is what makes a per-dataset analyzer possible.
//
// Languages outside analyzerLanguages, and analyzers outside the rag family,
// are returned unchanged.
func analyzerForLanguage(analyzer, language string) string {
	key := strings.ToLower(strings.TrimSpace(language))
	if _, ok := analyzerLanguages[key]; !ok {
		return analyzer
	}
	if base, _, _ := strings.Cut(analyzer, "-"); base != ragAnalyzer {
		return analyzer
	}
	return analyzer + "-" + key
}

// indexNamePartRe matches everything that cannot appear in an index name.
var indexNamePartRe = regexp.MustCompile(`[^a-zA-Z0-9]`)

// fulltextIndexName is the name of the fulltext index on fieldName under
// analyzer. Infinity analyzes a query with the first index it finds for a
// field, ordered by name, so these names decide which of a field's analyzers a
// query is analyzed with.
func fulltextIndexName(fieldName, analyzer string) string {
	return fmt.Sprintf("ft_%s_%s", indexNamePartRe.ReplaceAllString(fieldName, "_"), indexNamePartRe.ReplaceAllString(analyzer, "_"))
}

// wantedFulltextIndexes returns the fulltext indexes fieldName should have
// under language, as name -> analyzer.
func wantedFulltextIndexes(fieldName string, analyzers []string, language string) map[string]string {
	wanted := make(map[string]string, len(analyzers))
	for _, configured := range analyzers {
		analyzer := analyzerForLanguage(configured, language)
		wanted[fulltextIndexName(fieldName, analyzer)] = analyzer
	}
	return wanted
}

// hasForeignRagIndex reports whether fieldName already carries a rag fulltext
// index under another language.
//
// A table's rag analyzer is settled when its first fulltext index is built:
// Infinity analyzes a query with the field's first index by name, so adding
// "ft_content_rag_coarse" next to "ft_content_rag_coarse_slovak" would win on
// name order and silently undo the dataset's language -- and adding the slovak
// one next to an existing default would silently do nothing. Either way the
// table keeps what it has, and a dataset that needs the other analyzer has to
// be reindexed into a fresh table.
//
// Indexes this language does want are not foreign, so a field missing one of
// its variants still gets it.
//
// Mirrors _has_foreign_rag_index in common/doc_store/infinity_conn_base.py.
func hasForeignRagIndex(indexNames []string, fieldName string, wanted map[string]string) bool {
	prefix := fmt.Sprintf("ft_%s_%s", indexNamePartRe.ReplaceAllString(fieldName, "_"), ragAnalyzer)
	for _, name := range indexNames {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if _, ok := wanted[name]; !ok {
			return true
		}
	}
	return false
}

// listIndexNames returns the table's index names. The SDK types its response
// as interface{} and the concrete type lives in its internal package, so read
// it through the getter the thrift codegen provides.
func listIndexNames(table *infinity.Table) ([]string, error) {
	resp, err := table.ListIndexes()
	if err != nil {
		return nil, err
	}
	lister, ok := resp.(interface{ GetIndexNames() []string })
	if !ok {
		return nil, fmt.Errorf("unexpected ListIndexes response type: %T", resp)
	}
	return lister.GetIndexNames(), nil
}
