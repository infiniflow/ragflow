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
	"fmt"
	"ragflow/internal/engine/types"
	"ragflow/internal/utility"
	"strings"
	"unicode/utf8"

	infinity "github.com/infiniflow/infinity-go-sdk"
)

const (
	PAGERANK_FLD = "pagerank_fea"
	TAG_FLD      = "tag_feas"
)

type SortType int

const (
	SortAsc  SortType = 0
	SortDesc SortType = 1
)

type OrderByExpr struct {
	Fields []OrderByField
}

type OrderByField struct {
	Field string
	Type  SortType
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
	OrderBy     *OrderByExpr
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

// Search executes search (supports unified engine.SearchRequest only)
func (e *infinityEngine) Search(ctx context.Context, req interface{}) (interface{}, error) {
	switch searchReq := req.(type) {
	case *types.SearchRequest:
		return e.searchUnified(ctx, searchReq)
	default:
		return nil, fmt.Errorf("invalid search request type: %T", req)
	}
}

// convertSelectFields converts field names to Infinity format
// isSkillIndex indicates if this is a skill index (uses skill_id instead of id)
func convertSelectFields(output []string, isSkillIndex ...bool) []string {
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

	skillIndex := false
	if len(isSkillIndex) > 0 {
		skillIndex = isSkillIndex[0]
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
	// For skill index, use skill_id instead of id
	hasID := false
	idField := "id"
	if skillIndex {
		idField = "skill_id"
	}
	for _, f := range result {
		if f == idField {
			hasID = true
			break
		}
	}
	if !hasID {
		result = append([]string{idField}, result...)
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

	// If no sub-tokens, use simple format
	if !hasSubTokens(question) {
		return fmt.Sprintf("((%s)^1.0)", question)
	}

	return fmt.Sprintf("((%s OR \"%s\" OR (\"%s\"~2)^0.5)^1.0)", question, question, question)
}

// convertMatchingField converts field names for matching (for regular document indices only)
// For skill indices, use original field names directly as they have built-in analyzers
func convertMatchingField(fieldWeightStr string) string {
	// Split on ^ to get field name
	parts := strings.Split(fieldWeightStr, "^")
	field := parts[0]

	// Field name conversion for regular document indices only
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

	// Note: Skill index fields (name, tags, description, content) should NOT be converted
	// They use original field names with built-in analyzers as defined in skill_infinity_mapping.json

	if newField, ok := fieldMapping[field]; ok {
		parts[0] = newField
	}

	return strings.Join(parts, "^")
}

// searchUnified handles the unified engine.SearchRequest
func (e *infinityEngine) searchUnified(ctx context.Context, req *types.SearchRequest) (*types.SearchResponse, error) {
	if len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}

	// Get retrieval parameters with defaults
	topK := req.TopK
	if topK <= 0 {
		topK = 1024
	}

	pageSize := req.Size
	if pageSize <= 0 {
		pageSize = 30
	}

	offset := (req.Page - 1) * pageSize
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

	// Determine if this is a skill index
	isSkillIndex := false
	for _, idx := range req.IndexNames {
		if strings.HasPrefix(idx, "skill_") {
			isSkillIndex = true
			break
		}
	}

	// Build output columns
	// For metadata tables, only use: id, kb_id, meta_fields
	// For skill tables, use skill-specific fields
	// For chunk tables, use all the standard fields
	var outputColumns []string
	if isMetadataTable {
		outputColumns = []string{"id", "kb_id", "meta_fields"}
	} else if isSkillIndex {
		// Skill index uses different field names
		// Note: schema has skill_id as primary identifier, no 'id' column
		outputColumns = []string{
			"skill_id",
			"hub_id",
			"folder_id",
			"name",
			"tags",
			"description",
			"content",
			"version",
			"status",
			"create_time",
			"update_time",
		}
	} else {
		outputColumns = []string{
			"id",
			"doc_id",
			"kb_id",
			"content",
			"content_ltks",
			"content_with_weight",
			"title_tks",
			"docnm_kwd",
			"img_id",
			"available_int",
			"important_kwd",
			"position_int",
			"page_num_int",
			"doc_type_kwd",
			"mom_id",
			"question_tks",
		}
	}
	outputColumns = convertSelectFields(outputColumns, isSkillIndex)

	// Determine if text or vector search
	// Treat "*" as wildcard (match all), disable text match
	hasTextMatch := req.Question != "" && req.Question != "*"
	hasVectorMatch := !req.KeywordOnly && len(req.Vector) > 0

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
		// Add pagerank field (only for regular document indices, not skill index)
		if !isSkillIndex {
			outputColumns = append(outputColumns, PAGERANK_FLD)
		}
	}

	// Remove duplicates
	outputColumns = convertSelectFields(outputColumns, isSkillIndex)

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

	// Only add available_int/status filter when there's text/vector match or AvailableInt is explicitly set
	// This matches Python's behavior where chunk_list doesn't filter by available_int
	if !isMetadataTable && (hasTextMatch || hasVectorMatch || req.AvailableInt != nil) {
		if isSkillIndex {
			// Skill index uses 'status' field instead of 'available_int'
			if req.AvailableInt != nil {
				filterParts = append(filterParts, fmt.Sprintf("status='%d'", *req.AvailableInt))
			} else {
				filterParts = append(filterParts, "status='1'")
			}
		} else {
			if req.AvailableInt != nil {
				filterParts = append(filterParts, fmt.Sprintf("available_int=%d", *req.AvailableInt))
			} else {
				filterParts = append(filterParts, "available_int=1")
			}
		}
	}

	filterStr := strings.Join(filterParts, " AND ")

	// Build order_by
	var orderBy *OrderByExpr
	if req.OrderBy != "" {
		orderBy = &OrderByExpr{Fields: []OrderByField{}}
		// Parse order_by field and direction
		fields := strings.Split(req.OrderBy, ",")
		for _, field := range fields {
			field = strings.TrimSpace(field)
			if strings.HasSuffix(field, " desc") || strings.HasSuffix(field, " DESC") {
				fieldName := strings.TrimSuffix(field, " desc")
				fieldName = strings.TrimSuffix(fieldName, " DESC")
				orderBy.Fields = append(orderBy.Fields, OrderByField{Field: fieldName, Type: SortDesc})
			} else {
				orderBy.Fields = append(orderBy.Fields, OrderByField{Field: field, Type: SortAsc})
			}
		}
	}

	// rank_feature support
	var rankFeature map[string]float64
	if req.RankFeature != nil {
		rankFeature = req.RankFeature
	}

	// Results from all tables
	var allResults []map[string]interface{}
	totalHits := int64(0)

	// Search across all tables
	for _, indexName := range req.IndexNames {
		// Determine table names to search
		var tableNames []string
		if strings.HasPrefix(indexName, "ragflow_doc_meta_") || strings.HasPrefix(indexName, "skill_") {
			// Metadata tables and skill tables use index name directly (no kbID suffix)
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

		for _, tableName := range tableNames {
			// Try to get table
			_, err := db.GetTable(tableName)
			if err != nil {
				// Table doesn't exist, skip
				continue
			}

		// Build query for this table
		result, err := e.executeTableSearch(db, tableName, outputColumns, req.Question, req.Vector, filterStr, topK, pageSize, offset, orderBy, rankFeature, req.SimilarityThreshold, minMatch, req.VectorSimilarityWeight)
		if err != nil {
			// Skip this table on error
			continue
		}

			allResults = append(allResults, result.Chunks...)
			totalHits += result.Total
		}

		// If no results, try fallback strategies
		if totalHits == 0 && (hasTextMatch || hasVectorMatch) {
			allResults = nil
			totalHits = 0

			if hasDocIDFilter {
				// If has doc_id filter, search without match
				for _, tableName := range tableNames {
					_, err := db.GetTable(tableName)
					if err != nil {
						continue
					}
				// Search without match - pass empty question
				result, err := e.executeTableSearch(db, tableName, outputColumns, "", req.Vector, filterStr, topK, pageSize, offset, orderBy, rankFeature, req.SimilarityThreshold, 0.0, req.VectorSimilarityWeight)
				if err != nil {
					continue
				}
					allResults = append(allResults, result.Chunks...)
					totalHits += result.Total
				}
			} else {
				// Retry with lower min_match and similarity
				lowerThreshold := 0.17
				for _, tableName := range tableNames {
					_, err := db.GetTable(tableName)
					if err != nil {
						continue
					}
				result, err := e.executeTableSearch(db, tableName, outputColumns, req.Question, req.Vector, filterStr, topK, pageSize, offset, orderBy, rankFeature, lowerThreshold, 0.1, req.VectorSimilarityWeight)
				if err != nil {
					continue
				}
					allResults = append(allResults, result.Chunks...)
					totalHits += result.Total
				}
			}
		}
	}

	if hasTextMatch || hasVectorMatch {
		// For skill index, don't use pagerank field (it doesn't exist)
		pagerankField := PAGERANK_FLD
		if isSkillIndex {
			pagerankField = ""
		}
		allResults = calculateScores(allResults, scoreColumn, pagerankField)
	}

	if hasTextMatch || hasVectorMatch {
		allResults = sortByScore(allResults, len(allResults))
	}

	// Apply threshold filter to combined results
	if req.SimilarityThreshold > 0 && hasVectorMatch {
		var filteredResults []map[string]interface{}
		for _, chunk := range allResults {
			score := getScore(chunk)
			if score >= req.SimilarityThreshold {
				filteredResults = append(filteredResults, chunk)
			}
		}
		allResults = filteredResults
	}

	// Limit to pageSize
	if len(allResults) > pageSize {
		allResults = allResults[:pageSize]
	}

	return &types.SearchResponse{
		Chunks: allResults,
		Total:  totalHits,
	}, nil
}

// calculateScores calculates _score = score_column + pagerank
func calculateScores(chunks []map[string]interface{}, scoreColumn, pagerankField string) []map[string]interface{} {
	for i := range chunks {
		score := 0.0
		if scoreVal, ok := chunks[i][scoreColumn]; ok {
			if f, ok := utility.ToFloat64(scoreVal); ok {
				score += f
			}
		}
		if pagerankVal, ok := chunks[i][pagerankField]; ok {
			if f, ok := utility.ToFloat64(pagerankVal); ok {
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
func (e *infinityEngine) executeTableSearch(db *infinity.Database, tableName string, outputColumns []string, question string, vector []float64, filterStr string, topK, pageSize, offset int, orderBy *OrderByExpr, rankFeature map[string]float64, similarityThreshold float64, minMatch float64, vectorSimilarityWeight float64) (*types.SearchResponse, error) {

	// Get table
	table, err := db.GetTable(tableName)
	if err != nil {
		return nil, err
	}

	// Build query using Table's chainable methods
	// Treat "*" as wildcard (match all), disable text match
	hasTextMatch := question != "" && question != "*"
	hasVectorMatch := len(vector) > 0

	// Determine if this is a skill index (starts with "skill_")
	isSkillIndex := strings.HasPrefix(tableName, "skill_")

	table = table.Output(outputColumns)

	// Define text fields based on index type
	// Note: SQL query uses format 'name^10,tags^5,description^3,content^1'
	// Both MatchText and filter_fulltext use the same format
	var textFields []string
	if isSkillIndex {
		// Skill index uses original field names
		textFields = []string{
			"name^10",
			"tags^5",
			"description^3",
			"content^1",
		}
	} else {
		// Regular document index uses _tks fields with Infinity's internal format
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
		if isSkillIndex {
			// Skill index: use original field names (same as SQL)
			convertedFields = append(convertedFields, f)
		} else {
			// Regular index: convert to Infinity's internal format
			cf := convertMatchingField(f)
			convertedFields = append(convertedFields, cf)
		}
	}
	fields := strings.Join(convertedFields, ",")

	// Format question
	formattedQuestion := formatQuestion(question)

	// Add text match if question is provided
	if hasTextMatch {
		extraOptions := map[string]string{
			"topn":                 fmt.Sprintf("%d", topK),
			"minimum_should_match": fmt.Sprintf("%d%%", int(minMatch*100)),
		}

		// Add rank_features support
		if rankFeature != nil {
			var rankFeaturesList []string
			for featureName, weight := range rankFeature {
				rankFeaturesList = append(rankFeaturesList, fmt.Sprintf("%s^%s^%f", TAG_FLD, featureName, weight))
			}
			if len(rankFeaturesList) > 0 {
				extraOptions["rank_features"] = strings.Join(rankFeaturesList, ",")
			}
		}

		// Add filter to extraOptions if present (for skill index status filter, etc.)
		if filterStr != "" {
			extraOptions["filter"] = filterStr
		}

		table = table.MatchText(fields, formattedQuestion, topK, extraOptions)
	}

	// Add vector match if provided
	if hasVectorMatch {
		vectorSize := len(vector)
		fieldName := fmt.Sprintf("q_%d_vec", vectorSize)
		threshold := similarityThreshold
		if threshold <= 0 {
			threshold = 0.1 // default
		}
		extraOptions := map[string]string{
			// Add threshold
			"threshold": fmt.Sprintf("%f", threshold),
		}

		// Add filter to MatchDense extra_options
		// Note: Only use basic filter (status='1'), NOT filter_fulltext
		// filter_fulltext is only used in fusion text search, not vector search filter
		if filterStr != "" {
			extraOptions["filter"] = filterStr
		}

		table = table.MatchDense(fieldName, vector, "float", "cosine", topK, extraOptions)
	}

	// Add fusion (for text+vector combination)
	// Fusion weights: first is text weight (1 - vector_similarity_weight), second is vector weight
	// Reference: Python rag/utils/infinity_conn.py L214-218
	if hasTextMatch && hasVectorMatch {
		// Get vector similarity weight from search request
		vectorWeight := vectorSimilarityWeight
		if vectorWeight <= 0 || vectorWeight > 1 {
			vectorWeight = 0.3 // default
		}
		textWeight := 1.0 - vectorWeight

		fusionParams := map[string]interface{}{
			"normalize": "atan",
			"weights":   fmt.Sprintf("%.2f,%.2f", textWeight, vectorWeight),
		}
		table = table.Fusion("weighted_sum", topK, fusionParams)
	}

	// Add order_by if provided
	if orderBy != nil && len(orderBy.Fields) > 0 {
		var sortFields [][2]interface{}
		for _, field := range orderBy.Fields {
			sortType := infinity.SortTypeAsc
			if field.Type == SortDesc {
				sortType = infinity.SortTypeDesc
			}
			sortFields = append(sortFields, [2]interface{}{field.Field, sortType})
		}
		table = table.Sort(sortFields)
	}

	// Add filter when there's no text/vector match (like metadata queries)
	if !hasTextMatch && !hasVectorMatch && filterStr != "" {
		table = table.Filter(filterStr)
	}

	// Set limit and offset
	// Use topK to get more results from Infinity, then filter/sort in Go
	table = table.Limit(topK)
	if offset > 0 {
		table = table.Offset(offset)
	}

	// Execute query - get the raw query and execute via SDK
	result, err := e.executeQuery(table)
	if err != nil {
		return nil, err
	}

	// Calculate scores
	scoreColumn := "SIMILARITY"
	if hasTextMatch {
		scoreColumn = "SCORE"
	}

	// For skill index, don't use pagerank field (it doesn't exist)
	pagerankField := PAGERANK_FLD
	if isSkillIndex {
		pagerankField = ""
	}
	result.Chunks = calculateScores(result.Chunks, scoreColumn, pagerankField)

	// Sort by score
	result.Chunks = sortByScore(result.Chunks, len(result.Chunks))

	if len(result.Chunks) > pageSize {
		result.Chunks = result.Chunks[:pageSize]
	}
	result.Total = int64(len(result.Chunks))

	return result, nil
}

// executeQuery executes the query and returns results
func (e *infinityEngine) executeQuery(table *infinity.Table) (*types.SearchResponse, error) {
	// Use ToResult() to execute query
	result, err := table.ToResult()
	if err != nil {
		return nil, fmt.Errorf("Infinity query failed: %w", err)
	}

	// Convert result to SearchResponse format
	// The SDK returns QueryResult with Data as map[string][]interface{}
	qr, ok := result.(*infinity.QueryResult)
	if !ok {
		return &types.SearchResponse{
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

	return &types.SearchResponse{
		Chunks: chunks,
		Total:  int64(len(chunks)),
	}, nil
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


