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
	"context"
	"encoding/json"
	"fmt"
	"ragflow/internal/engine/types"
	"ragflow/internal/utility"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	infinity "github.com/infiniflow/infinity-go-sdk"
)

const (
	PAGERANK_FLD = "pagerank_fea"
	TAG_FLD      = "tag_feas"
)

// floatToString formats a float like Python's str() - adds ".0" if needed
func floatToString(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") && !strings.Contains(s, "e") {
		s = s + ".0"
	}
	return s
}

// fieldKeyword checks if field is a keyword field
func fieldKeyword(fieldName string) bool {
	// Treat "*_kwd" tag-like columns as keyword lists except knowledge_graph_kwd
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

// equivalentConditionToStr converts condition dict to filter string
func equivalentConditionToStr(condition map[string]interface{}, tableColumns map[string]struct {
	Type    string
	Default interface{}
}) string {
	if len(condition) == 0 {
		return ""
	}

	var conditions []string

	for k, v := range condition {
		if !strings.HasPrefix(k, "_") {
			continue
		}
		if v == nil || v == "" {
			continue
		}

		// Handle keyword fields with filter_fulltext
		if fieldKeyword(k) {
			if listVal, isList := v.([]interface{}); isList {
				var orConds []string
				for _, item := range listVal {
					if strItem, ok := item.(string); ok {
						strItem = strings.ReplaceAll(strItem, "'", "''")
						orConds = append(orConds, fmt.Sprintf("filter_fulltext('%s', '%s')", convertMatchingField(k), strItem))
					}
				}
				if len(orConds) > 0 {
					conditions = append(conditions, "("+strings.Join(orConds, " OR ")+")")
				}
			} else if strVal, ok := v.(string); ok {
				strVal = strings.ReplaceAll(strVal, "'", "''")
				conditions = append(conditions, fmt.Sprintf("filter_fulltext('%s', '%s')", convertMatchingField(k), strVal))
			}
		} else if listVal, isList := v.([]interface{}); isList {
			// Handle IN conditions
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
		} else if k == "must_not" {
			// Handle must_not conditions
			if mustNotMap, ok := v.(map[string]interface{}); ok {
				if existsVal, ok := mustNotMap["exists"]; ok {
					if existsField, ok := existsVal.(string); ok {
						col, colOk := tableColumns[existsField]
						if colOk && strings.Contains(strings.ToLower(col.Type), "char") {
							conditions = append(conditions, fmt.Sprintf(" %s!='' ", existsField))
						} else {
							conditions = append(conditions, fmt.Sprintf("%s!=null", existsField))
						}
					}
				}
			}
		} else if strVal, ok := v.(string); ok {
			strVal = strings.ReplaceAll(strVal, "'", "''")
			conditions = append(conditions, fmt.Sprintf("%s='%s'", k, strVal))
		} else if k == "exists" {
			if existsField, ok := v.(string); ok {
				col, colOk := tableColumns[existsField]
				if colOk && strings.Contains(strings.ToLower(col.Type), "char") {
					conditions = append(conditions, fmt.Sprintf(" %s!='' ", existsField))
				} else {
					conditions = append(conditions, fmt.Sprintf("%s!=null", existsField))
				}
			}
		} else {
			conditions = append(conditions, fmt.Sprintf("%s=%v", k, v))
		}
	}

	if len(conditions) == 0 {
		return ""
	}
	return strings.Join(conditions, " AND ")
}

// SearchRequest Infinity search request (legacy, kept for backward compatibility)
type SearchRequest struct {
	TableName   string
	ColumnNames []string
	MatchText   *MatchTextExpr
	MatchDense  *MatchDenseExpr
	Fusion      *FusionExpr
	Offset      int
	Limit       int
	Filter      map[string]interface{}
	OrderBy     *types.OrderByExpr
}

// SearchResponse Infinity search response
type SearchResponse struct {
	Rows  []map[string]interface{}
	Total int64
}

// MatchTextExpr text match expression
type MatchTextExpr struct {
	Fields       []string
	MatchingText string
	TopN         int
	ExtraOptions map[string]interface{}
}

// MatchDenseExpr vector match expression
type MatchDenseExpr struct {
	VectorColumnName  string
	EmbeddingData     []float64
	EmbeddingDataType string
	DistanceType      string
	TopN              int
	ExtraOptions      map[string]interface{}
}

// FusionExpr fusion expression
type FusionExpr struct {
	Method       string
	TopN         int
	Weights      []float64
	FusionParams map[string]interface{}
}

// Search executes search with unified types.SearchRequest
func (e *infinityEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	return e.searchUnified(ctx, req)
}

// convertSelectFields converts field names to Infinity format
func convertSelectFields(output []string) []string {
	fieldMapping := map[string]string{
		"docnm_kwd":           "docnm",
		"title_tks":           "docnm",
		"title_sm_tks":        "docnm",
		"important_kwd":       "important_keywords",
		"important_tks":       "important_keywords",
		"question_kwd":        "questions",
		"question_tks":        "questions",
		"content_with_weight": "content",
		"content_ltks":        "content",
		"content_sm_ltks":     "content",
		"authors_tks":         "authors",
		"authors_sm_tks":      "authors",
	}

	needEmptyCount := false
	for i, field := range output {
		if field == "important_kwd" {
			needEmptyCount = true
		}
		if newField, ok := fieldMapping[field]; ok {
			output[i] = newField
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	result := []string{}
	for _, f := range output {
		if f != "" && !seen[f] {
			seen[f] = true
			result = append(result, f)
		}
	}

	// Add id and empty count if needed
	hasID := false
	for _, f := range result {
		if f == "id" {
			hasID = true
			break
		}
	}
	if !hasID {
		result = append([]string{"id"}, result...)
	}

	if needEmptyCount {
		result = append(result, "important_kwd_empty_count")
	}

	return result
}

// isChinese checks if a string contains Chinese characters
func isChinese(s string) bool {
	for _, r := range s {
		if '\u4e00' <= r && r <= '\u9fff' {
			return true
		}
	}
	return false
}

// hasSubTokens checks if the text has sub-tokens after fine-grained tokenization
// - Returns False if len < 3
// - Returns False if text is only ASCII alphanumeric
// - Returns True otherwise (meaning there are sub-tokens)
func hasSubTokens(s string) bool {
	if utf8.RuneCountInString(s) < 3 {
		return false
	}
	isASCIIOnly := true
	for _, r := range s {
		if r > 127 {
			isASCIIOnly = false
			break
		}
	}
	if isASCIIOnly {
		// Check if it's only alphanumeric and allowed special chars
		for _, r := range s {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '.' || r == '+' || r == '#' || r == '_' || r == '*' || r == '-') {
				isASCIIOnly = false
				break
			}
		}
		if isASCIIOnly {
			return false
		}
	}
	// Has sub-tokens if it's Chinese and length >= 3
	return isChinese(s)
}

// formatQuestion formats the question
// - If len < 3: returns ((query)^1.0)
// - If has sub-tokens: adds fuzzy search ((query OR "query" OR ("query"~2)^0.5)^1.0)
// - Otherwise: returns ((query)^1.0)
func formatQuestion(question string) string {
	// Trim whitespace
	question = strings.TrimSpace(question)
	fmt.Printf("[DEBUG formatQuestion] input: %q, len: %d, hasSubTokens: %v\n", question, len(question), hasSubTokens(question))

	// If no sub-tokens, use simple format
	if !hasSubTokens(question) {
		result := fmt.Sprintf("((%s)^1.0)", question)
		fmt.Printf("[DEBUG formatQuestion] simple: %s\n", result)
		return result
	}

	result := fmt.Sprintf("((%s OR \"%s\" OR (\"%s\"~2)^0.5)^1.0)", question, question, question)
	fmt.Printf("[DEBUG formatQuestion] fuzzy: %s\n", result)
	return result
}

// convertMatchingField converts field names for matching
func convertMatchingField(fieldWeightStr string) string {
	// Split on ^ to get field name
	parts := strings.Split(fieldWeightStr, "^")
	field := parts[0]

	// Field name conversion
	fieldMapping := map[string]string{
		"docnm_kwd":           "docnm@ft_docnm_rag_coarse",
		"title_tks":           "docnm@ft_docnm_rag_coarse",
		"title_sm_tks":        "docnm@ft_docnm_rag_fine",
		"important_kwd":       "important_keywords@ft_important_keywords_rag_coarse",
		"important_tks":       "important_keywords@ft_important_keywords_rag_fine",
		"question_kwd":        "questions@ft_questions_rag_coarse",
		"question_tks":        "questions@ft_questions_rag_fine",
		"content_with_weight": "content@ft_content_rag_coarse",
		"content_ltks":        "content@ft_content_rag_coarse",
		"content_sm_ltks":     "content@ft_content_rag_fine",
		"authors_tks":         "authors@ft_authors_rag_coarse",
		"authors_sm_tks":      "authors@ft_authors_rag_fine",
		"tag_kwd":             "tag_kwd@ft_tag_kwd_whitespace__",
	}

	if newField, ok := fieldMapping[field]; ok {
		parts[0] = newField
	}

	return strings.Join(parts, "^")
}

// searchUnified handles the unified engine.SearchRequest
func (e *infinityEngine) searchUnified(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}

	// Get retrieval parameters with defaults
	topK := req.TopK
	if topK <= 0 {
		topK = 1024
	}

	pageSize := req.Limit
	if pageSize <= 0 {
		pageSize = 30
	}

	// Use Offset/Limit if provided (matching Python's offset, limit passed to dataStore.search)
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	// Get database
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to get database: %w", err)
	}

	// Determine if this is a metadata table
	isMetadataTable := false
	for _, idx := range req.IndexNames {
		if strings.HasPrefix(idx, "ragflow_doc_meta_") {
			isMetadataTable = true
			break
		}
	}

	// Build output columns
	// For metadata tables, only use: id, kb_id, meta_fields
	// For chunk tables, use all the standard fields
	// Matching Python's search() fields (rag/nlp/search.py L92-95):
	// ["docnm_kwd", "content_ltks", "kb_id", "img_id", "title_tks", "important_kwd", "position_int",
	//  "doc_id", "chunk_order_int", "page_num_int", "top_int", "create_timestamp_flt", "knowledge_graph_kwd",
	//  "question_kwd", "question_tks", "doc_type_kwd",
	//  "available_int", "content_with_weight", "mom_id", PAGERANK_FLD, TAG_FLD, "row_id()"]
	var outputColumns []string
	if isMetadataTable {
		outputColumns = []string{"id", "kb_id", "meta_fields"}
	} else {
		outputColumns = []string{
			"id",
			"doc_id",
			"kb_id",
			"content_ltks",
			"content_with_weight",
			"title_tks",
			"docnm_kwd",
			"img_id",
			"available_int",
			"important_kwd",
			"position_int",
			"page_num_int",
			"top_int",
			"chunk_order_int",
			"create_timestamp_flt",
			"knowledge_graph_kwd",
			"question_kwd",
			"question_tks",
			"doc_type_kwd",
			"mom_id",
			"tag_kwd",
		}
	}
	outputColumns = convertSelectFields(outputColumns)

	// Determine if text or vector search based on MatchExprs (matching Python: matchExprs = [matchText, matchDense, fusionExpr])
	hasTextMatch := false
	hasVectorMatch := false
	var matchText *MatchTextExpr
	var matchDense *types.MatchDenseExpr
	if req.MatchExprs != nil && len(req.MatchExprs) > 0 {
		for _, expr := range req.MatchExprs {
			if expr == nil {
				continue
			}
			switch e := expr.(type) {
			case *MatchTextExpr:
				hasTextMatch = true
				matchText = e
			case *types.MatchDenseExpr:
				hasVectorMatch = true
				matchDense = e
			}
		}
	}
	// If MatchExprs is empty/nil, both remain false (keyword-only search won't happen)

	// Determine score column
	scoreColumn := ""
	if hasTextMatch {
		scoreColumn = "SCORE"
	} else if hasVectorMatch {
		scoreColumn = "SIMILARITY"
	}

	// Add score column if needed
	if hasTextMatch || hasVectorMatch {
		if hasTextMatch {
			outputColumns = append(outputColumns, "score()")
		} else if hasVectorMatch {
			outputColumns = append(outputColumns, "similarity()")
		}
		// Add pagerank and tag fields (matching Python L142-143)
		if !contains(outputColumns, PAGERANK_FLD) {
			outputColumns = append(outputColumns, PAGERANK_FLD)
		}
		if !contains(outputColumns, TAG_FLD) {
			outputColumns = append(outputColumns, TAG_FLD)
		}
	}

	// Add row_id() - Infinity requires the function syntax for internal row ID
	if !contains(outputColumns, "row_id") && !contains(outputColumns, "row_id()") {
		outputColumns = append(outputColumns, "row_id()")
	}

	// Remove duplicates
	outputColumns = convertSelectFields(outputColumns)

	// Add vector column if vector search (matching Python: chunk.get(vector_column))
	// This is needed for insert_citations() in Dialog flow
	if hasVectorMatch && req.MatchExprs != nil && len(req.MatchExprs) > 1 {
		if matchDense, ok := req.MatchExprs[1].(*types.MatchDenseExpr); ok {
			outputColumns = append(outputColumns, matchDense.VectorColumnName)
		}
	}

	// Build filter string
	var filterParts []string

	// For metadata tables, add kb_id filter if provided
	if isMetadataTable && len(req.KbIDs) > 0 && req.KbIDs[0] != "" {
		kbIDs := req.KbIDs
		if len(kbIDs) == 1 {
			filterParts = append(filterParts, fmt.Sprintf("kb_id = '%s'", kbIDs[0]))
		} else {
			kbIDStr := strings.Join(kbIDs, "', '")
			filterParts = append(filterParts, fmt.Sprintf("kb_id IN ('%s')", kbIDStr))
		}
	}

	// DocIDs filters by doc_id (document ID) to find all chunks belonging to a document
	// This is used by ChunkService.List() to list all chunks for a document
	if len(req.DocIDs) > 0 {
		if len(req.DocIDs) == 1 {
			filterParts = append(filterParts, fmt.Sprintf("doc_id = '%s'", req.DocIDs[0]))
		} else {
			docIDs := strings.Join(req.DocIDs, "', '")
			filterParts = append(filterParts, fmt.Sprintf("doc_id IN ('%s')", docIDs))
		}
	}

	// Only add available_int filter when there's text/vector match or available_int is explicitly set in MetaDataFilter
	// This matches Python's behavior where chunk_list doesn't filter by available_int
	if !isMetadataTable && (hasTextMatch || hasVectorMatch) {
		if req.MetaDataFilter != nil {
			if availInt, ok := req.MetaDataFilter["available_int"]; ok {
				filterParts = append(filterParts, fmt.Sprintf("available_int=%v", availInt))
			} else {
				filterParts = append(filterParts, "available_int=1")
			}
		} else {
			filterParts = append(filterParts, "available_int=1")
		}
	}

	// Handle MetaDataFilter - skip if method is "auto" or "semi_auto" (requires LLM)
	// Note: auto/semi_auto filtering is handled upstream in metadata_filter.go before searchUnified is called
	// For simple filters, convert them to filter conditions
	// Note: For chunk tables (not metadata table), skip kb_id filter as tables are already separated per KB
	// This matches Python's infinity_conn.py L155-157
	if req.MetaDataFilter != nil {
		if method, ok := req.MetaDataFilter["method"].(string); ok {
			if method == "auto" || method == "semi_auto" {
				// Skip silently - upstream already handled this and logged if LLM failed
			} else {
				// Process other filter conditions
				for k, v := range req.MetaDataFilter {
					if k == "method" || k == "kb_id" {
						continue
					}
					filterParts = append(filterParts, fmt.Sprintf("%s='%v'", k, v))
				}
			}
		} else {
			// No method specified, treat as simple key-value filters
			for k, v := range req.MetaDataFilter {
				// Skip kb_id for chunk tables (matching Python behavior)
				// kb_id is NOT added to filter for chunk tables since tables are already per-KB
				if k == "kb_id" {
					continue
				}
				// Handle slice values (like kb_id=[]string{"uuid"}) properly
				// to avoid formatting as "[uuid]" instead of "uuid"
				switch val := v.(type) {
				case []string:
					if len(val) == 1 {
						filterParts = append(filterParts, fmt.Sprintf("%s='%s'", k, val[0]))
					} else if len(val) > 1 {
						filterParts = append(filterParts, fmt.Sprintf("%s IN ('%s')", k, strings.Join(val, "', '")))
					}
				case []int:
					if len(val) == 1 {
						filterParts = append(filterParts, fmt.Sprintf("%s=%d", k, val[0]))
					} else if len(val) > 1 {
						strs := make([]string, len(val))
						for i, n := range val {
							strs[i] = strconv.Itoa(n)
						}
						filterParts = append(filterParts, fmt.Sprintf("%s IN (%s)", k, strings.Join(strs, ",")))
					}
				default:
					filterParts = append(filterParts, fmt.Sprintf("%s='%v'", k, v))
				}
			}
		}
	}

	filterStr := strings.Join(filterParts, " AND ")

	// Build order_by (matching Python's orderBy = OrderByExpr())
	orderBy := req.OrderBy

	// rank_feature support
	var rankFeature map[string]float64
	if req.RankFeature != nil {
		rankFeature = req.RankFeature
	}

	// Extract fusionExpr from MatchExprs (index 2)
	var fusionExpr *types.FusionExpr
	if len(req.MatchExprs) > 2 {
		if fe, ok := req.MatchExprs[2].(*types.FusionExpr); ok {
			fusionExpr = fe
		}
	}

	// Results from all tables
	var allResults []map[string]interface{}
	totalHits := int64(0)

	// Search across all tables
	for _, indexName := range req.IndexNames {
		// Determine table names to search
		var tableNames []string
		if strings.HasPrefix(indexName, "ragflow_doc_meta_") {
			tableNames = []string{indexName}
		} else {
			// For each KB ID, create a table name
			kbIDs := req.KbIDs
			if len(kbIDs) == 0 {
				// If no KB IDs, use the index name directly
				kbIDs = []string{""}
			}
			for _, kbID := range kbIDs {
				if kbID == "" {
					tableNames = append(tableNames, indexName)
				} else {
					tableNames = append(tableNames, fmt.Sprintf("%s_%s", indexName, kbID))
				}
			}
		}

		// Search each table
		// 1. First try with min_match=0.3 (30%)
		// 2. If no results and has doc_id filter: search without match
		// 3. If no results and no doc_id filter: retry with min_match=0.1 (10%) and lower similarity
		minMatch := 0.3
		hasDocIDFilter := len(req.DocIDs) > 0

		// Extract question text and vector data from MatchExprs for reuse in retry loops
		var questionText string
		var vectorData []float64
		var textTopN int = topK  // Default to topK if no MatchTextExpr
		var originalQuery string // Extract original_query from MatchTextExpr for Infinity extra_options
		if matchText != nil {
			questionText = matchText.MatchingText
			textTopN = int(matchText.TopN) // Use MatchTextExpr.TopN (typically 100) for text match
			// Extract original_query from ExtraOptions
			if matchText.ExtraOptions != nil {
				if oq, ok := matchText.ExtraOptions["original_query"].(string); ok {
					originalQuery = oq
				}
			}
		}
		if matchDense != nil {
			vectorData = matchDense.EmbeddingData
		}

		for _, tableName := range tableNames {
			fmt.Printf("[DEBUG] Searching table: %s\n", tableName)
			// Try to get table
			_, err := db.GetTable(tableName)
			if err != nil {
				// Table doesn't exist, skip
				continue
			}

			// Build query for this table
			result, err := e.executeTableSearch(db, tableName, outputColumns, questionText, vectorData, filterStr, topK, textTopN, pageSize, offset, orderBy, rankFeature, originalQuery, req.SimilarityThreshold, minMatch, fusionExpr, matchText)
			if err != nil {
				// Skip this table on error
				continue
			}

			allResults = append(allResults, result.Chunks...)
			totalHits += result.Total
		}

		// If no results, try fallback strategies
		if totalHits == 0 && (hasTextMatch || hasVectorMatch) {
			fmt.Printf("[DEBUG] No results, trying fallback strategies\n")
			allResults = nil
			totalHits = 0

			if hasDocIDFilter {
				// If has doc_id filter, search without match
				fmt.Printf("[DEBUG] Retry with no match (has doc_id filter)\n")
				for _, tableName := range tableNames {
					_, err := db.GetTable(tableName)
					if err != nil {
						continue
					}
					// Search without match - pass empty question and vector
					result, err := e.executeTableSearch(db, tableName, outputColumns, "", nil, filterStr, topK, topK, pageSize, offset, orderBy, rankFeature, "", req.SimilarityThreshold, 0.0, fusionExpr, nil)
					if err != nil {
						continue
					}
					allResults = append(allResults, result.Chunks...)
					totalHits += result.Total
				}
			} else {
				// Retry with lower min_match and similarity
				fmt.Printf("[DEBUG] Retry with min_match=0.1, similarity=0.17\n")
				lowerThreshold := 0.17
				for _, tableName := range tableNames {
					_, err := db.GetTable(tableName)
					if err != nil {
						continue
					}
					result, err := e.executeTableSearch(db, tableName, outputColumns, questionText, vectorData, filterStr, topK, textTopN, pageSize, offset, orderBy, rankFeature, originalQuery, lowerThreshold, 0.1, fusionExpr, matchText)
					if err != nil {
						fmt.Printf("[DEBUG] executeTableSearch error for table %s: %v\n", tableName, err)
						continue
					}
					allResults = append(allResults, result.Chunks...)
					totalHits += result.Total
				}
			}
		}
	}

	if hasTextMatch || hasVectorMatch {
		allResults = calculateScores(allResults, scoreColumn)
	}

	if hasTextMatch || hasVectorMatch {
		allResults = sortByScore(allResults, len(allResults))
	}

	// Apply threshold filter to combined results
	fmt.Printf("[DEBUG] Threshold check: SimilarityThreshold=%f, hasVectorMatch=%v, hasTextMatch=%v\n", req.SimilarityThreshold, hasVectorMatch, hasTextMatch)
	if req.SimilarityThreshold > 0 && hasVectorMatch {
		var filteredResults []map[string]interface{}
		for _, chunk := range allResults {
			score := getScore(chunk)
			chunkID := ""
			if id, ok := chunk["id"]; ok {
				chunkID = fmt.Sprintf("%v", id)
			}
			fmt.Printf("[DEBUG] Threshold filter: id=%s, score=%f, threshold=%f, pass=%v\n", chunkID, score, req.SimilarityThreshold, score >= req.SimilarityThreshold)
			if score >= req.SimilarityThreshold {
				filteredResults = append(filteredResults, chunk)
			}
		}
		fmt.Printf("[DEBUG] After threshold filter (combined): %d -> %d chunks\n", len(allResults), len(filteredResults))
		allResults = filteredResults
	}

	// Limit to pageSize
	if len(allResults) > pageSize {
		allResults = allResults[:pageSize]
	}

	fmt.Printf("[INFO] searchUnified completed: returnedRows=%d, totalHits=%d, indexName=%s\n", len(allResults), totalHits, req.IndexNames[0])

	return &types.SearchResult{
		Chunks: allResults,
		Total:  totalHits,
	}, nil
}

// calculateScores calculates _score = score_column + pagerank
// For Infinity: pagerank is included in _score by the database
// For ES: pagerank is added separately via computeRankFeatureScores
func calculateScores(chunks []map[string]interface{}, scoreColumn string) []map[string]interface{} {
	fmt.Printf("[DEBUG] calculateScores: scoreColumn=%s\n", scoreColumn)
	for i := range chunks {
		score := 0.0
		if scoreVal, ok := chunks[i][scoreColumn]; ok {
			if f, ok := utility.ToFloat64(scoreVal); ok {
				score += f
			}
		}
		chunks[i]["_score"] = score
	}
	return chunks
}

// sortByScore sorts by _score descending and limits
func sortByScore(chunks []map[string]interface{}, limit int) []map[string]interface{} {
	if len(chunks) == 0 {
		return chunks
	}

	// Sort by _score descending
	for i := 0; i < len(chunks)-1; i++ {
		for j := i + 1; j < len(chunks); j++ {
			scoreI := getScore(chunks[i])
			scoreJ := getScore(chunks[j])
			if scoreI < scoreJ {
				chunks[i], chunks[j] = chunks[j], chunks[i]
			}
		}
	}

	// Limit
	if len(chunks) > limit && limit > 0 {
		chunks = chunks[:limit]
	}

	return chunks
}

func getScore(chunk map[string]interface{}) float64 {
	// Check _score first
	if score, ok := chunk["_score"].(float64); ok {
		return score
	}
	if score, ok := chunk["_score"].(int); ok {
		return float64(score)
	}
	if score, ok := chunk["_score"].(int64); ok {
		return float64(score)
	}
	// Fallback to SCORE (for fusion) or SIMILARITY (for vector-only)
	if score, ok := chunk["SCORE"].(float64); ok {
		return score
	}
	if score, ok := chunk["SIMILARITY"].(float64); ok {
		return score
	}
	return 0.0
}

// executeTableSearch executes search on a single table
func (e *infinityEngine) executeTableSearch(db *infinity.Database, tableName string, outputColumns []string, question string, vector []float64, filterStr string, topK, textTopN, pageSize, offset int, orderBy *types.OrderByExpr, rankFeature map[string]float64, originalQuery string, similarityThreshold float64, minMatch float64, fusionExpr *types.FusionExpr, matchTextExpr *MatchTextExpr) (*types.SearchResult, error) {
	fmt.Printf("[START] executeTableSearch: tableName=%s, question=%s, topK=%d, pageSize=%d, offset=%d, similarityThreshold=%f, minMatch=%f\n",
		tableName, question, topK, pageSize, offset, similarityThreshold, minMatch)

	// Debug: log text fields if available
	if matchTextExpr != nil && len(matchTextExpr.Fields) > 0 {
		fmt.Printf("[DEBUG] executeTableSearch using matchTextExpr.Fields: %v\n", matchTextExpr.Fields)
	}

	// Get table
	table, err := db.GetTable(tableName)
	if err != nil {
		return nil, err
	}

	// Build query using Table's chainable methods
	hasTextMatch := question != ""
	hasVectorMatch := len(vector) > 0

	table = table.Output(outputColumns)

	// Use matchTextExpr.Fields if provided, otherwise fallback to default fields (matching Python infinity_conn.py L183)
	var textFields []string
	if matchTextExpr != nil && len(matchTextExpr.Fields) > 0 {
		textFields = matchTextExpr.Fields
	} else {
		textFields = []string{
			"title_tks^10",
			"title_sm_tks^5",
			"important_kwd^30",
			"important_tks^20",
			"question_tks^20",
			"content_ltks^2",
			"content_sm_ltks",
		}
	}

	// Convert field names for Infinity
	var convertedFields []string
	for _, f := range textFields {
		cf := convertMatchingField(f)
		convertedFields = append(convertedFields, cf)
	}
	fields := strings.Join(convertedFields, ",")

	// Question is already formatted by QueryBuilder (MatchTextExpr.MatchingText),
	// so use it directly without re-formatting to avoid double-formatting issues.
	formattedQuestion := question

	// Add text match if question is provided
	if hasTextMatch {
		extraOptions := map[string]string{
			"minimum_should_match": fmt.Sprintf("%d%%", int(minMatch*100)),
		}

		// Add filter to extraOptions (matching Python's infinity_conn.py L181-182)
		// Python adds filter_cond to MatchTextExpr.extra_options if filter_cond is truthy
		// filterStr is built from condition and includes available_int=1 (and doc_id IN (...) if hasDocIDFilter)
		if filterStr != "" {
			extraOptions["filter"] = filterStr
		}

		// Add rank_features support
		if rankFeature != nil {
			var rankFeaturesList []string
			for featureName, weight := range rankFeature {
				rankFeaturesList = append(rankFeaturesList, fmt.Sprintf("%s^%s^%.0f", TAG_FLD, featureName, weight))
			}
			if len(rankFeaturesList) > 0 {
				extraOptions["rank_features"] = strings.Join(rankFeaturesList, ",")
			}
		}

		// Add original_query if provided (matching Python's extra_options)
		if originalQuery != "" {
			extraOptions["original_query"] = originalQuery
		}

		table = table.MatchText(fields, formattedQuestion, textTopN, extraOptions)
		fmt.Printf("[DEBUG] MatchTextExpr: fields=%s, matching_text=%s, topn=%d, extra_options=%v\n", fields, formattedQuestion, textTopN, extraOptions)
	}

	// Add vector match if provided
	if hasVectorMatch {
		vectorSize := len(vector)
		fieldName := fmt.Sprintf("q_%d_vec", vectorSize)

		// Build filter for MatchDense - matching Python's approach
		// Python builds: (available_int=1) AND filter_fulltext('fields', 'question')
		filterStr := "available_int=1"
		if hasTextMatch {
			// Build filter_fulltext expression like Python does
			fieldsStr := strings.Join(convertedFields, ",")
			filterFulltext := fmt.Sprintf("filter_fulltext('%s', '%s')", fieldsStr, question)
			filterStr = fmt.Sprintf("(%s) AND %s", filterStr, filterFulltext)
		}
		extraOptions := map[string]string{
			"threshold": floatToString(similarityThreshold),
			"filter":    filterStr,
		}

		fmt.Printf("[DEBUG] MatchDenseExpr: field=%s, topn=%d, extra_options=%v\n", fieldName, topK, extraOptions)

		table = table.MatchDense(fieldName, vector, "float", "cosine", topK, extraOptions)
	}

	// Add fusion (for text+vector combination)
	if hasTextMatch && hasVectorMatch && fusionExpr != nil {
		fusionMethod := fusionExpr.Method
		fusionTopK := fusionExpr.TopN
		if fusionTopK == 0 {
			fusionTopK = topK
		}
		// Python adds normalize="atan" for weighted_sum to avoid zero scores for last doc
		// Reference: memory/utils/infinity_conn.py L209-211
		fusionParams := map[string]interface{}{
			"normalize": "atan",
		}
		if fusionExpr.FusionParams != nil {
			for k, v := range fusionExpr.FusionParams {
				fusionParams[k] = v
			}
		}
		fmt.Printf("[DEBUG] FusionExpr: method=%s, topn=%d, fusion_params=%v\n", fusionMethod, fusionTopK, fusionParams)
		table = table.Fusion(fusionMethod, fusionTopK, fusionParams)
	}

	// Add order_by if provided
	if orderBy != nil && len(orderBy.Fields) > 0 {
		var sortFields [][2]interface{}
		for _, field := range orderBy.Fields {
			sortType := infinity.SortTypeAsc
			if field.Type == types.SortDesc {
				sortType = infinity.SortTypeDesc
			}
			sortFields = append(sortFields, [2]interface{}{field.Field, sortType})
		}
		table = table.Sort(sortFields)
	}

	// Add filter when there's no text/vector match (like metadata queries)
	if !hasTextMatch && !hasVectorMatch && filterStr != "" {
		fmt.Printf("[DEBUG] Adding filter for no-match query: %s\n", filterStr)
		table = table.Filter(filterStr)
	}

	// Set limit and offset
	// Use topK to get more results from Infinity, then filter/sort in Go
	table = table.Limit(topK)
	if offset > 0 {
		table = table.Offset(offset)
	}

	// Request total_hits_count from Infinity (matching Python's builder.option({"total_hits_count": True}))
	table = table.Option(map[string]interface{}{"total_hits_count": true})

	// Execute query - get the raw query and execute via SDK
	result, err := e.executeQuery(table)
	if err != nil {
		return nil, err
	}

	// Debug logging - show returned chunks
	scoreColumn := "SIMILARITY"
	if hasTextMatch {
		scoreColumn = "SCORE"
	}
	result.Chunks = calculateScores(result.Chunks, scoreColumn)

	// Sort by score
	result.Chunks = sortByScore(result.Chunks, len(result.Chunks))

	if len(result.Chunks) > pageSize {
		result.Chunks = result.Chunks[:pageSize]
	}
	// Note: result.Total is already set correctly by executeQuery from ExtraInfo
	// Do not overwrite with len(result.Chunks) as that would lose the correct total
	fmt.Printf("[END] executeTableSearch: returned %d chunks, total=%d\n", len(result.Chunks), result.Total)

	return result, nil
}

// executeQuery executes the query and returns results
func (e *infinityEngine) executeQuery(table *infinity.Table) (*types.SearchResult, error) {
	// Use ToResult() to execute query
	result, err := table.ToResult()
	if err != nil {
		return nil, fmt.Errorf("Infinity query failed: %w", err)
	}

	// Convert result to SearchResponse format
	// The SDK returns QueryResult with Data as map[string][]interface{}
	qr, ok := result.(*infinity.QueryResult)
	if !ok {
		fmt.Printf("[DEBUG] ToResult() returned unexpected type: %T (expected *infinity.QueryResult)\n", result)
		fmt.Printf("[DEBUG] Result value: %+v\n", result)
		return &types.SearchResult{
			Chunks: []map[string]interface{}{},
			Total:  0,
		}, nil
	}

	// Convert to chunks format
	chunks := make([]map[string]interface{}, 0)
	for colName, colData := range qr.Data {
		for i, val := range colData {
			// Ensure we have a row for this index
			for len(chunks) <= i {
				chunks = append(chunks, make(map[string]interface{}))
			}
			chunks[i][colName] = val
		}
	}

	// Handle Infinity SDK bug: meta_fields may come back as a single concatenated
	// byte slice containing all rows' meta_fields instead of one per row.
	// Detect this and split into individual values per row.
	if metaFieldsData, ok := qr.Data["meta_fields"]; ok && len(metaFieldsData) > 0 {
		if len(metaFieldsData) == 1 && len(chunks) > 1 {
			// meta_fields is concatenated - need to split it
			if metaFieldsBytes, ok := metaFieldsData[0].([]uint8); ok {
				parsedMetaFields := parseLengthPrefixedJSON(metaFieldsBytes)
				if len(parsedMetaFields) == len(chunks) {
					// Successfully parsed - distribute to each chunk
					for i, mf := range parsedMetaFields {
						chunks[i]["meta_fields"] = mf
					}
					fmt.Printf("[DEBUG] Split concatenated meta_fields into %d individual values\n", len(parsedMetaFields))
				} else if len(parsedMetaFields) > 0 {
					// Partial match - assign first parsed to first chunk, clear others
					fmt.Printf("[DEBUG] Parsed %d meta_fields but have %d chunks\n", len(parsedMetaFields), len(chunks))
				}
			}
		}
	}

	// Handle ROW_ID - it may come back as concatenated raw int64 values (not JSON)
	// Similar to meta_fields, Infinity may return it as a single concatenated byte slice
	if rowIDData, ok := qr.Data["ROW_ID"]; ok && len(rowIDData) > 0 {
		if len(rowIDData) == 1 && len(chunks) > 1 {
			// ROW_ID is concatenated - need to split into individual int64 values
			if rowIDBytes, ok := rowIDData[0].([]uint8); ok {
				fmt.Printf("[DEBUG] ROW_ID concatenated bytes length: %d\n", len(rowIDBytes))
				// Each row_id is an int64 (8 bytes)
				const int64Size = 8
				if len(rowIDBytes)%int64Size == 0 && len(rowIDBytes)/int64Size == len(chunks) {
					for i := 0; i < len(chunks); i++ {
						offset := i * int64Size
						// Parse little-endian int64
						val := int64(rowIDBytes[offset]) |
							int64(rowIDBytes[offset+1])<<8 |
							int64(rowIDBytes[offset+2])<<16 |
							int64(rowIDBytes[offset+3])<<24 |
							int64(rowIDBytes[offset+4])<<32 |
							int64(rowIDBytes[offset+5])<<40 |
							int64(rowIDBytes[offset+6])<<48 |
							int64(rowIDBytes[offset+7])<<56
						chunks[i]["ROW_ID"] = val
					}
					fmt.Printf("[DEBUG] Successfully parsed %d ROW_ID values from concatenated bytes\n", len(chunks))
				} else {
					fmt.Printf("[DEBUG] ROW_ID bytes length %d doesn't match %d chunks or isn't divisible by 8\n", len(rowIDBytes), len(chunks))
					// Fallback: copy the value to all chunks
					for i := range chunks {
						chunks[i]["ROW_ID"] = rowIDData[0]
					}
				}
			}
		}
	}

	// Post-process: convert nil/empty values to empty slices for array-like fields
	arrayFields := map[string]bool{
		"doc_type_kwd":    true,
		"important_kwd":   true,
		"important_tks":   true,
		"question_tks":    true,
		"authors_tks":     true,
		"authors_sm_tks":  true,
		"title_tks":       true,
		"title_sm_tks":    true,
		"content_ltks":    true,
		"content_sm_ltks": true,
	}
	for i := range chunks {
		for colName := range arrayFields {
			if val, ok := chunks[i][colName]; !ok || val == nil || val == "" {
				chunks[i][colName] = []interface{}{}
			}
		}
		// Convert position_int from hex string to array format
		if posVal, ok := chunks[i]["position_int"].(string); ok {
			chunks[i]["position_int"] = utility.ConvertHexToPositionIntArray(posVal)
		} else {
			chunks[i]["position_int"] = []interface{}{}
		}
		// Convert page_num_int and top_int from hex string to array
		for _, colName := range []string{"page_num_int", "top_int"} {
			if val, ok := chunks[i][colName].(string); ok {
				chunks[i][colName] = utility.ConvertHexToIntArray(val)
			}
		}
	}

	// Apply reverse field name mapping (matching Python's get_fields in infinity_conn.py L682-760)
	// This undoes the convertSelectFields renaming so chunks use original field names
	chunks = reverseConvertFields(chunks)

	// Parse total_hits_count from ExtraInfo (matching Python's extra_result["total_hits_count"])
	// ExtraInfo is a JSON string like: {"total_hits_count": 100}
	var totalHits int64
	if qr.ExtraInfo != "" {
		var extraResult map[string]interface{}
		if err := json.Unmarshal([]byte(qr.ExtraInfo), &extraResult); err == nil {
			if count, ok := extraResult["total_hits_count"].(float64); ok {
				totalHits = int64(count)
			}
		}
	}

	return &types.SearchResult{
		Chunks: chunks,
		Total:  totalHits,
	}, nil
}

// reverseConvertFields undoes the convertSelectFields renaming to restore original field names
// This mirrors Python's get_fields() in infinity_conn.py (L682-760) which reverses field mappings
func reverseConvertFields(chunks []map[string]interface{}) []map[string]interface{} {
	if len(chunks) == 0 {
		return chunks
	}

	// Fields that should NOT be in the output (Python L755-757 deletes these)
	fieldsToRemove := []string{"docnm", "important_keywords", "questions", "content", "authors"}

	for i := range chunks {
		// Apply getFields first to handle the main field mappings
		getFields(chunks[i])

		// Handle row_id mapping (Python L723-724)
		// Infinity returns "ROW_ID" (uppercase) - check all variants
		if val, ok := chunks[i]["row_id"]; ok {
			chunks[i]["row_id()"] = val
			delete(chunks[i], "row_id")
		} else if val, ok := chunks[i]["row_id()"]; ok {
			chunks[i]["row_id()"] = val
		} else if val, ok := chunks[i]["ROW_ID"]; ok {
			chunks[i]["row_id()"] = val
			delete(chunks[i], "ROW_ID")
		}

		// Remove original renamed fields (matching Python L755-757)
		for _, field := range fieldsToRemove {
			delete(chunks[i], field)
		}
	}

	return chunks
}

// contains checks if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// parseLengthPrefixedJSON parses Infinity's length-prefixed JSON format
// Format: [4-byte length (little-endian)][JSON][4-byte length][JSON]...
// Returns all JSON objects found
func parseLengthPrefixedJSON(data []byte) []map[string]interface{} {
	if len(data) < 4 {
		return nil
	}

	var results []map[string]interface{}
	offset := 0

	for offset+4 <= len(data) {
		// Read 4-byte length (little-endian)
		length := uint32(data[offset]) | uint32(data[offset+1])<<8 |
			uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24

		if length == 0 || offset+4+int(length) > len(data) {
			break
		}

		jsonStart := offset + 4
		jsonEnd := jsonStart + int(length)
		jsonBytes := data[jsonStart:jsonEnd]

		var result map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &result); err == nil {
			results = append(results, result)
		}

		offset = jsonEnd
	}

	return results
}

// GetAggregation aggregates field values from search results.
// Corresponds to Python's infinity_conn_base.py:get_aggregation() (L547-588).
// For tag_kwd field, splits values by "###". For other fields, uses comma separation.
// Returns list of {key, count} maps sorted by count descending.
func GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	if len(chunks) == 0 {
		return []map[string]interface{}{}
	}

	// Check if field exists in first chunk (matching Python: field_name not in df.columns)
	hasField := false
	for _, chunk := range chunks {
		if _, ok := chunk[fieldName]; ok {
			hasField = true
			break
		}
	}
	if !hasField {
		return []map[string]interface{}{}
	}

	// Count occurrences (matching Python: tag_counter = Counter())
	tagCounts := make(map[string]int)
	for _, chunk := range chunks {
		value, ok := chunk[fieldName]
		if !ok || value == nil {
			continue
		}

		// Handle string value (matching Python L570-580)
		if valueStr, ok := value.(string); ok {
			if valueStr == "" {
				continue
			}

			var tags []string
			// Split by "###" for tag_kwd field (matching Python L572-573)
			if fieldName == "tag_kwd" && strings.Contains(valueStr, "###") {
				for _, tag := range strings.Split(valueStr, "###") {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tags = append(tags, tag)
					}
				}
			} else {
				// Fallback to comma separation (matching Python L575-576)
				for _, tag := range strings.Split(valueStr, ",") {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tags = append(tags, tag)
					}
				}
			}

			for _, tag := range tags {
				tagCounts[tag]++
			}
			continue
		}

		// Handle list value (matching Python L581-585)
		if valueList, ok := value.([]interface{}); ok {
			for _, item := range valueList {
				if itemStr, ok := item.(string); ok {
					tag := strings.TrimSpace(itemStr)
					if tag != "" {
						tagCounts[tag]++
					}
				}
			}
		}
	}

	if len(tagCounts) == 0 {
		return []map[string]interface{}{}
	}

	// Convert to slice and sort by count descending (matching Python: tag_counter.most_common())
	type tagCountPair struct {
		tag   string
		count int
	}
	pairs := make([]tagCountPair, 0, len(tagCounts))
	for tag, count := range tagCounts {
		pairs = append(pairs, tagCountPair{tag, count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].count > pairs[j].count
	})

	// Convert to []map[string]interface{} directly
	result := make([]map[string]interface{}, len(pairs))
	for i, p := range pairs {
		result[i] = map[string]interface{}{"key": p.tag, "count": p.count}
	}

	return result
}

// GetTotal extracts total hits from search response options.
// Corresponds to Python's infinity_conn_base.py:get_total() (L488-491).
// In Python: get_total returns res[1] (total count from tuple) or len(res) (DataFrame length).
// In Go: total is stored in Options["total"] by searchUnified().
func GetTotal(options map[string]interface{}) int64 {
	if options == nil {
		return 0
	}
	if total, ok := options["total"].(int64); ok {
		return total
	}
	if total, ok := options["total"].(int); ok {
		return int64(total)
	}
	return 0
}

// GetDocIDs extracts document IDs from search results.
// Corresponds to Python's infinity_conn_base.py:get_doc_ids() (L493-496).
// Extracts "id" field from each chunk and returns as a list.
func GetDocIDs(chunks []map[string]interface{}) []string {
	if len(chunks) == 0 {
		return nil
	}
	ids := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if id, ok := chunk["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// GetFields builds a field map from chunks, keyed by chunk ID.
// Corresponds to Python's infinity_conn.py:get_fields() (L682-761) and search.py (line 170).
// When fields is nil/empty, returns all fields from chunks.
// When fields is provided, only includes those fields.
func GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})
	if len(chunks) == 0 {
		return result
	}

	// If fields is provided, create a set for lookup
	var fieldSet map[string]bool
	if len(fields) > 0 {
		fieldSet = make(map[string]bool)
		for _, f := range fields {
			fieldSet[f] = true
		}
	}

	// Build field map for each chunk
	for _, chunk := range chunks {
		if id, ok := chunk["id"].(string); ok {
			fieldMap := make(map[string]interface{})
			// Include all fields or only the requested fields
			for field, value := range chunk {
				if fieldSet == nil || fieldSet[field] {
					fieldMap[field] = value
				}
			}
			result[id] = fieldMap
		}
	}
	return result
}

// GetHighlight generates highlighted text snippets for search results.
// Corresponds to Python's infinity_conn_base.py:get_highlight() (L502-545).
// Matches keywords in text and wraps them with <em> tags.
func GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string {
	result := make(map[string]string)
	if len(chunks) == 0 || len(keywords) == 0 {
		return result
	}

	// Check if field exists
	hasField := false
	for _, chunk := range chunks {
		if _, ok := chunk[fieldName]; ok {
			hasField = true
			break
		}
	}
	if !hasField {
		// Try alternative field names
		if fieldName == "content_with_weight" {
			if _, ok := chunks[0]["content"]; ok {
				fieldName = "content"
				hasField = true
			}
		}
	}
	if !hasField {
		return result
	}

	emTag := regexp.MustCompile(`<em>[^<>]+</em>`)

	for _, chunk := range chunks {
		id := ""
		if idVal, ok := chunk["id"].(string); ok {
			id = idVal
		}

		txt, ok := chunk[fieldName].(string)
		if !ok || txt == "" {
			continue
		}

		// Check if already highlighted
		if emTag.MatchString(txt) {
			result[id] = txt
			continue
		}

		// Replace newlines with spaces
		txt = regexp.MustCompile(`[\r\n]`).ReplaceAllString(txt, " ")

		// Split by sentence delimiters
		delimiters := regexp.MustCompile(`[.?!;\n]`)
		segments := delimiters.Split(txt, -1)

		var highlightedSegments []string
		for _, segment := range segments {
			// Check if segment is English or contains keywords
			isEnglish := isEnglishText(segment)
			segmentToCheck := segment
			if isEnglish {
				// For English: match whole words with boundaries
				for _, kw := range keywords {
					re := regexp.MustCompile(`(^|[ .?/'\"\(\)!,:;-])` + regexp.QuoteMeta(kw) + `([ .?/'\"\(\)!,:;-]|$)`)
					segmentToCheck = re.ReplaceAllString(segmentToCheck, "$1<em>"+kw+"</em>$2")
				}
			} else {
				// For non-English: simple keyword replacement (sorted by length desc for longer matches first)
				sortedKeywords := make([]string, len(keywords))
				copy(sortedKeywords, keywords)
				sort.Slice(sortedKeywords, func(i, j int) bool {
					return len(sortedKeywords[i]) > len(sortedKeywords[j])
				})
				for _, kw := range sortedKeywords {
					re := regexp.MustCompile(regexp.QuoteMeta(kw))
					segmentToCheck = re.ReplaceAllString(segmentToCheck, "<em>"+kw+"</em>")
				}
			}

			// Check if any keywords were highlighted
			if emTag.MatchString(segmentToCheck) {
				highlightedSegments = append(highlightedSegments, segmentToCheck)
			}
		}

		if len(highlightedSegments) > 0 {
			result[id] = "..." + strings.Join(highlightedSegments, "...") + "..."
		} else {
			result[id] = txt
		}
	}

	return result
}

// isEnglishText checks if a text is primarily English.
func isEnglishText(text string) bool {
	englishCount := 0
	totalCount := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			totalCount++
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				englishCount++
			}
		}
	}
	if totalCount == 0 {
		return false
	}
	return float64(englishCount)/float64(totalCount) > 0.5
}
