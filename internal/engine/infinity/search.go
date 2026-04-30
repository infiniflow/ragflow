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
	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/utility"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"ragflow/internal/logger"

	infinity "github.com/infiniflow/infinity-go-sdk"
	"go.uber.org/zap"
)

// Search searches the Infinity engine for matching chunks.
// It supports three matching types: MatchTextExpr (full-text), MatchDenseExpr (vector), and FusionExpr (combined).
// If no match expressions are provided, Search relies solely on filter (e.g., doc_id, available_int) to find results.
func (e *infinityEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	logger.Debug("Search in Infinity started", zap.Any("indexNames", req.IndexNames))
	if logger.IsDebugEnabled() {
		// Format match expressions for logging
		var matchExprsStr string
		for i, expr := range req.MatchExprs {
			switch e := expr.(type) {
			case *types.MatchTextExpr:
				matchExprsStr += fmt.Sprintf("    [%d] MatchTextExpr: fields=%v, matchingText=%s, topN=%d, extraOptions=%v\n", i, e.Fields, e.MatchingText, e.TopN, e.ExtraOptions)
			case *types.MatchDenseExpr:
				matchExprsStr += fmt.Sprintf("    [%d] MatchDenseExpr: vectorColumn=%s, vectorSize=%d, topN=%d, extraOptions=%v\n", i, e.VectorColumnName, len(e.EmbeddingData), e.TopN, e.ExtraOptions)
			case *types.FusionExpr:
				matchExprsStr += fmt.Sprintf("    [%d] FusionExpr: method=%s, topN=%d, fusionParams=%v\n", i, e.Method, e.TopN, e.FusionParams)
			default:
				matchExprsStr += fmt.Sprintf("    [%d] unknown type\n", i)
			}
		}
		logger.Debug(fmt.Sprintf("Search request:\n"+
			"    indexNames=%v\n"+
			"    KbIDs=%v\n"+
			"    offset=%d, limit=%d\n"+
			"    SelectFields=%v\n"+
			"    Filter=%v\n"+
			"    MatchExprs:\n%s    orderBy=%v\n"+
			"    RankFeature=%v",
			req.IndexNames, req.KbIDs, req.Offset, req.Limit, req.SelectFields, req.Filter, matchExprsStr, req.OrderBy, req.RankFeature))
	}

	if len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}

	// Get retrieval parameters with defaults
	pageSize := req.Limit
	if pageSize <= 0 {
		pageSize = 30
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to get database: %w", err)
	}

	isMetadataTable := false
	isSkillIndex := false
	for _, idx := range req.IndexNames {
		if strings.HasPrefix(idx, "ragflow_doc_meta_") {
			isMetadataTable = true
			break
		}
		if strings.HasPrefix(idx, "skill_") {
			isSkillIndex = true
			break
		}
	}

	var outputColumns []string
	if isMetadataTable {
		outputColumns = []string{"id", "kb_id", "meta_fields"}
	} else if isSkillIndex {
		outputColumns = []string{
			"skill_id", "space_id", "folder_id", "name", "tags", "description", "content",
			"version", "status", "create_time", "update_time",
		}
		outputColumns = convertSelectFields(outputColumns, true)
	} else {
		outputColumns = []string{
			"id", "doc_id", "kb_id", "content_ltks", "content_with_weight",
			"title_tks", "docnm_kwd", "img_id", "available_int", "important_kwd",
			"position_int", "page_num_int", "top_int", "chunk_order_int",
			"create_timestamp_flt", "knowledge_graph_kwd", "question_kwd", "question_tks",
			"doc_type_kwd", "mom_id", "tag_kwd", "pagerank_fea", "tag_feas",
		}
		outputColumns = convertSelectFields(outputColumns)
	}

	hasTextMatch := false
	hasVectorMatch := false
	var matchText *types.MatchTextExpr
	var matchDense *types.MatchDenseExpr
	if req.MatchExprs != nil && len(req.MatchExprs) > 0 {
		for _, expr := range req.MatchExprs {
			if expr == nil {
				continue
			}
			switch e := expr.(type) {
			case string:
				if e != "" {
					hasTextMatch = true
					matchText = &types.MatchTextExpr{
						MatchingText: e,
						TopN:         pageSize,
					}
				}
			case *types.MatchTextExpr:
				if e.MatchingText != "" {
					hasTextMatch = true
					matchText = e
				}
			case *types.MatchDenseExpr:
				if len(e.EmbeddingData) > 0 {
					hasVectorMatch = true
					matchDense = e
				}
			}
		}
	}

	if hasTextMatch || hasVectorMatch {
		if hasTextMatch {
			outputColumns = append(outputColumns, "score()")
		}
		// similarity() is only allowed by Infinity when there is ONLY MATCH VECTOR.
		// When both text and vector matches exist (hybrid search with Fusion),
		// only score() is valid — Fusion produces a unified SCORE column.
		if hasVectorMatch && !hasTextMatch {
			outputColumns = append(outputColumns, "similarity()")
		}
		// Skill index does not have pagerank_fea and tag_feas columns
		if !isSkillIndex {
			if !slices.Contains(outputColumns, common.PAGERANK_FLD) {
				outputColumns = append(outputColumns, common.PAGERANK_FLD)
			}
			if !slices.Contains(outputColumns, common.TAG_FLD) {
				outputColumns = append(outputColumns, common.TAG_FLD)
			}
		}
	}

	if !slices.Contains(outputColumns, "row_id") && !slices.Contains(outputColumns, "row_id()") {
		outputColumns = append(outputColumns, "row_id()")
	}

	outputColumns = convertSelectFields(outputColumns, isSkillIndex)
	if hasVectorMatch && matchDense != nil && matchDense.VectorColumnName != "" {
		outputColumns = append(outputColumns, matchDense.VectorColumnName)
	}

	var filterParts []string
	if isMetadataTable && len(req.KbIDs) > 0 && req.KbIDs[0] != "" {
		kbIDs := req.KbIDs
		if len(kbIDs) == 1 {
			filterParts = append(filterParts, fmt.Sprintf("kb_id = '%s'", kbIDs[0]))
		} else {
			kbIDStr := strings.Join(kbIDs, "', '")
			filterParts = append(filterParts, fmt.Sprintf("kb_id IN ('%s')", kbIDStr))
		}
	}

	if !isMetadataTable && (hasTextMatch || hasVectorMatch) {
		if req.Filter != nil {
			if availInt, ok := req.Filter["available_int"]; ok {
				filterParts = append(filterParts, fmt.Sprintf("available_int=%v", availInt))
			} else if status, ok := req.Filter["status"]; ok {
				filterParts = append(filterParts, fmt.Sprintf("status='%s'", status))
			} else {
				if isSkillIndex {
					filterParts = append(filterParts, "status='1'")
				} else {
					filterParts = append(filterParts, "available_int=1")
				}
			}
		} else {
			if isSkillIndex {
				filterParts = append(filterParts, "status='1'")
			} else {
				filterParts = append(filterParts, "available_int=1")
			}
		}
	}

	// Build filter string from req.Filter
	if req.Filter != nil {
		filterCopy := req.Filter
		if !isMetadataTable {
			filterCopy = make(map[string]interface{})
			for k, v := range req.Filter {
				if k != "kb_id" {
					filterCopy[k] = v
				}
			}
		}

		condStr := equivalentConditionToStr(filterCopy)
		if condStr != "" {
			filterParts = append(filterParts, condStr)
		}
	}
	filterStr := strings.Join(filterParts, " AND ")

	orderBy := req.OrderBy
	var rankFeature map[string]float64
	if req.RankFeature != nil {
		rankFeature = req.RankFeature
	}

	var fusionExpr *types.FusionExpr
	if len(req.MatchExprs) > 2 {
		if fe, ok := req.MatchExprs[2].(*types.FusionExpr); ok {
			fusionExpr = fe
		}
	}

	var allResults []map[string]interface{}
	totalHits := int64(0)

	for _, indexName := range req.IndexNames {
		var tableNames []string
		if strings.HasPrefix(indexName, "ragflow_doc_meta_") {
			tableNames = []string{indexName}
		} else {
			kbIDs := req.KbIDs
			if len(kbIDs) == 0 {
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

		minMatch := 0.3

		var questionText string
		var vectorData []float64
		textTopN := pageSize
		var originalQuery string
		if matchText != nil {
			questionText = matchText.MatchingText
			textTopN = int(matchText.TopN)
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
			tbl, err := db.GetTable(tableName)
			if err != nil {
				continue
			}
			table := tbl.Output(outputColumns)

			var textFields []string
			if matchText != nil && len(matchText.Fields) > 0 {
				textFields = matchText.Fields
			} else if isSkillIndex {
			textFields = []string{
				"name^10",
				"tags^5",
				"description^3",
				"content^1",
			}
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

			hasTextMatch := questionText != ""
			hasVectorMatch := len(vectorData) > 0
			// Add text match if question is provided
			if hasTextMatch {
				extraOptions := map[string]string{
					"minimum_should_match": fmt.Sprintf("%d%%", int(minMatch*100)),
				}

				if filterStr != "" {
					extraOptions["filter"] = filterStr
				}

				if rankFeature != nil {
					var rankFeaturesList []string
					for featureName, weight := range rankFeature {
						rankFeaturesList = append(rankFeaturesList, fmt.Sprintf("%s^%s^%.0f", common.TAG_FLD, featureName, weight))
					}
					if len(rankFeaturesList) > 0 {
						extraOptions["rank_features"] = strings.Join(rankFeaturesList, ",")
					}
				}

				if originalQuery != "" {
					extraOptions["original_query"] = originalQuery
				}

				table = table.MatchText(fields, questionText, textTopN, extraOptions)

				logger.Debug(fmt.Sprintf(
					"MatchTextExpr:\n"+
						"    fields=%s\n"+
						"    matching_text=%s\n"+
						"    topn=%d\n"+
						"    extra_options=%v",
					fields, questionText, textTopN, extraOptions,
				))
			}

			// Add vector match if provided
			if hasVectorMatch {
				vectorSize := len(vectorData)
				fieldName := fmt.Sprintf("q_%d_vec", vectorSize)
				dataType := "float"
				distanceType := "cosine"

				if matchDense != nil {
					if matchDense.VectorColumnName != "" {
						fieldName = matchDense.VectorColumnName
					}
					if matchDense.EmbeddingDataType != "" {
						dataType = matchDense.EmbeddingDataType
					}
					if matchDense.DistanceType != "" {
						distanceType = matchDense.DistanceType
					}
				}

				vectorTopN := pageSize
				if matchDense != nil && matchDense.TopN > 0 {
					vectorTopN = int(matchDense.TopN)
				}

			denseFilterStr := filterStr
			if denseFilterStr == "" {
				if isSkillIndex {
					denseFilterStr = "status='1'"
				} else {
					denseFilterStr = "available_int=1"
				}
			}

				if hasTextMatch && fusionExpr == nil {
					fieldsStr := strings.Join(convertedFields, ",")
					filterFulltext := fmt.Sprintf("filter_fulltext('%s', '%s')", fieldsStr, questionText)
					denseFilterStr = fmt.Sprintf("(%s) AND %s", denseFilterStr, filterFulltext)
				}
				extraOptions := map[string]string{
					"threshold": utility.FloatToString(0.0),
					"filter":    denseFilterStr,
				}

				logger.Debug("MatchDense for hybrid search",
					zap.String("fieldName", fieldName),
					zap.String("distanceType", distanceType),
					zap.Int("topN", vectorTopN),
					zap.Bool("hasFusion", fusionExpr != nil))

				table = table.MatchDense(fieldName, vectorData, dataType, distanceType, vectorTopN, extraOptions)
			}

			// Add fusion (for text + vector combination)
			if hasTextMatch && hasVectorMatch && fusionExpr != nil {
				fusionMethod := fusionExpr.Method
				fusionTopK := fusionExpr.TopN
				if fusionTopK == 0 {
					fusionTopK = pageSize
				}
				fusionParams := map[string]interface{}{
					"normalize": "atan",
				}
				if fusionExpr.FusionParams != nil {
					for k, v := range fusionExpr.FusionParams {
						fusionParams[k] = v
					}
				}

				logger.Debug("Applying Fusion for hybrid search",
					zap.String("method", fusionMethod),
					zap.Int("topN", fusionTopK),
					zap.Any("params", fusionParams))

				table = table.Fusion(fusionMethod, fusionTopK, fusionParams)
			}

			// Add order_by if provided
			if orderBy != nil && len(orderBy.Fields) > 0 {
				var sortFields [][2]interface{}
				for _, orderField := range orderBy.Fields {
					sortType := infinity.SortTypeAsc
					if orderField.Type == types.SortDesc {
						sortType = infinity.SortTypeDesc
					}
					sortFields = append(sortFields, [2]interface{}{orderField.Field, sortType})
				}
				table = table.Sort(sortFields)
			}

			// Add filter when there's no text/vector match (like metadata queries)
			if !hasTextMatch && !hasVectorMatch && filterStr != "" {
				logger.Debug(fmt.Sprintf("Adding filter for no-match query: %s", filterStr))
				table = table.Filter(filterStr)
			}

			// Set limit and offset
			table = table.Limit(pageSize)
			if offset > 0 {
				table = table.Offset(offset)
			}

			// Request total_hits_count from Infinity
			table = table.Option(map[string]interface{}{"total_hits_count": true})

			// Execute query
			df, err := table.ToDataFrame()
			if err != nil {
				logger.Warn("Infinity query failed",
					zap.String("tableName", tableName),
					zap.Bool("hasTextMatch", hasTextMatch),
					zap.Bool("hasVectorMatch", hasVectorMatch),
					zap.Bool("hasFusion", fusionExpr != nil),
					zap.Error(err))
				continue
			}

			// Convert DataFrame to chunks format (column-oriented to row-oriented)
			chunks := make([]map[string]interface{}, 0)
			for colName, colData := range df.ColumnData {
				for i, val := range colData {
					for len(chunks) <= i {
						chunks = append(chunks, make(map[string]interface{}))
					}
					chunks[i][colName] = val
				}
			}

			// Apply field name mapping and row_id handling
			// Skill index uses different schema
			// so we skip the document-specific field mappings
			if !isSkillIndex {
				if _, err := GetFields(chunks, nil); err != nil {
					return nil, fmt.Errorf("failed to get fields: %w", err)
				}
			} else {
				// For skill index, only handle ROW_ID -> row_id() mapping
				for _, chunk := range chunks {
					if val, ok := chunk["ROW_ID"]; ok {
						chunk["row_id()"] = val
						delete(chunk, "ROW_ID")
					}
				}
			}

			// Parse total_hits_count from ExtraInfo
			var tableTotal int64
			if df.ExtraInfo != "" {
				var extraResult map[string]interface{}
				if err := json.Unmarshal([]byte(df.ExtraInfo), &extraResult); err == nil {
					if count, ok := extraResult["total_hits_count"].(float64); ok {
						tableTotal = int64(count)
					}
				}
			}

			searchResult := &types.SearchResult{
				Chunks: chunks,
				Total:  tableTotal,
			}

			allResults = append(allResults, searchResult.Chunks...)
			totalHits += searchResult.Total
		}
	}

	if hasTextMatch || hasVectorMatch {
		scoreColumn := ""
		if hasTextMatch && hasVectorMatch {
			scoreColumn = "SCORE"
		} else if hasTextMatch {
			scoreColumn = "SCORE"
		} else if hasVectorMatch {
			scoreColumn = "SIMILARITY"
		}
		pagerankField := common.PAGERANK_FLD
		if isSkillIndex {
			pagerankField = "" // Skill index has no pagerank field
		}

		allResults = calculateScores(allResults, scoreColumn, pagerankField)
		allResults = sortByScore(allResults, len(allResults))
	}

	if len(allResults) > pageSize {
		allResults = allResults[:pageSize]
	}

	logger.Debug("Search in Infinity completed", zap.Int("returnedRows", len(allResults)), zap.Int64("totalHits", totalHits))

	return &types.SearchResult{
		Chunks: allResults,
		Total:  totalHits,
	}, nil
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

// convertMatchingField converts field names for matching
// For regular document indices: maps _tks/_kwd fields to column@index_name format
// For skill indices: maps raw field names to column@index_name format
// Infinity requires column@index_name when a column has multiple full-text indexes
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
		// Skill index fields
		"name":               "name@ft_name_rag_coarse",
		"tags":               "tags@ft_tags_rag_coarse",
		"description":        "description@ft_description_rag_coarse",
		"content":            "content@ft_content_rag_coarse",
	}

	if newField, ok := fieldMapping[field]; ok {
		parts[0] = newField
	}

	return strings.Join(parts, "^")
}

// escapeFilterValue escapes single quotes for filter values
func escapeFilterValue(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// equivalentConditionToStr converts a condition map to an Infinity filter string
func equivalentConditionToStr(condition map[string]interface{}) string {
	if len(condition) == 0 {
		return ""
	}

	var cond []string

	for k, v := range condition {
		if k == "_id" || utility.IsEmpty(v) {
			continue
		}

		// Handle must_not specially
		if k == "must_not" {
			if m, ok := v.(map[string]interface{}); ok {
				for kk, vv := range m {
					if kk == "exists" {
						// For must_not exists, use !='' since we don't have table schema
						cond = append(cond, fmt.Sprintf("NOT (%v!='')", vv))
					}
				}
			}
			continue
		}

		// Handle exists specially (without table schema, use string comparison)
		if k == "exists" {
			cond = append(cond, fmt.Sprintf("%v!=''", v))
			continue
		}

		// Handle keyword fields (using full-text filter)
		if fieldKeyword(k) {
			// For keyword fields, values are always treated as strings for filter_fulltext
			switch val := v.(type) {
			case []string:
				var inCond []string
				for _, item := range val {
					inCond = append(inCond, fmt.Sprintf("filter_fulltext('%s', '%s')",
						convertMatchingField(k), escapeFilterValue(item)))
				}
				if len(inCond) > 0 {
					cond = append(cond, "("+strings.Join(inCond, " or ")+")")
				}
			case []interface{}:
				var inCond []string
				for _, item := range val {
					if s, ok := item.(string); ok {
						inCond = append(inCond, fmt.Sprintf("filter_fulltext('%s', '%s')",
							convertMatchingField(k), escapeFilterValue(s)))
					} else {
						inCond = append(inCond, fmt.Sprintf("filter_fulltext('%s', '%s')",
							convertMatchingField(k), escapeFilterValue(fmt.Sprintf("%v", item))))
					}
				}
				if len(inCond) > 0 {
					cond = append(cond, "("+strings.Join(inCond, " or ")+")")
				}
			case string:
				cond = append(cond, fmt.Sprintf("filter_fulltext('%s', '%s')",
					convertMatchingField(k), escapeFilterValue(val)))
			default:
				cond = append(cond, fmt.Sprintf("filter_fulltext('%s', '%s')",
					convertMatchingField(k), escapeFilterValue(fmt.Sprintf("%v", v))))
			}
			continue
		}

		// Handle list values (mixed types - strings get quotes, numbers don't)
		if list, ok := v.([]interface{}); ok && len(list) > 0 {
			var strItems, numItems []string
			for _, item := range list {
				if s, ok := item.(string); ok {
					strItems = append(strItems, fmt.Sprintf("'%s'", escapeFilterValue(s)))
				} else if n, ok := item.(int); ok {
					numItems = append(numItems, strconv.Itoa(n))
				} else if n, ok := item.(int64); ok {
					numItems = append(numItems, strconv.FormatInt(n, 10))
				} else if f, ok := item.(float64); ok {
					numItems = append(numItems, strconv.FormatFloat(f, 'f', -1, 64))
				} else if s, ok := item.(fmt.Stringer); ok {
					strItems = append(strItems, fmt.Sprintf("'%s'", escapeFilterValue(s.String())))
				} else {
					strItems = append(strItems, fmt.Sprintf("'%s'", escapeFilterValue(fmt.Sprintf("%v", item))))
				}
			}
			if len(strItems) > 0 {
				if len(strItems) == 1 {
					cond = append(cond, fmt.Sprintf("%s=%s", k, strItems[0]))
				} else {
					cond = append(cond, fmt.Sprintf("%s IN (%s)", k, strings.Join(strItems, ", ")))
				}
			}
			if len(numItems) > 0 {
				if len(numItems) == 1 {
					cond = append(cond, fmt.Sprintf("%s=%s", k, numItems[0]))
				} else {
					cond = append(cond, fmt.Sprintf("%s IN (%s)", k, strings.Join(numItems, ", ")))
				}
			}
			continue
		}

		if list, ok := v.([]string); ok && len(list) > 0 {
			if len(list) == 1 {
				cond = append(cond, fmt.Sprintf("%s='%s'", k, escapeFilterValue(list[0])))
			} else {
				var items []string
				for _, item := range list {
					items = append(items, fmt.Sprintf("'%s'", escapeFilterValue(item)))
				}
				cond = append(cond, fmt.Sprintf("%s IN (%s)", k, strings.Join(items, ", ")))
			}
			continue
		}

		if list, ok := v.([]int); ok && len(list) > 0 {
			if len(list) == 1 {
				cond = append(cond, fmt.Sprintf("%s=%d", k, list[0]))
			} else {
				var strs []string
				for _, n := range list {
					strs = append(strs, strconv.Itoa(n))
				}
				cond = append(cond, fmt.Sprintf("%s IN (%s)", k, strings.Join(strs, ", ")))
			}
			continue
		}

		// Handle numeric values (no quotes)
		if utility.IsNumericValue(v) {
			cond = append(cond, fmt.Sprintf("%s=%v", k, v))
			continue
		}

		// Handle string values (with quotes and escaping)
		if str, ok := v.(string); ok {
			cond = append(cond, fmt.Sprintf("%s='%s'", k, escapeFilterValue(str)))
			continue
		}

		// Fallback: treat as string
		cond = append(cond, fmt.Sprintf("%s='%s'", k, escapeFilterValue(fmt.Sprintf("%v", v))))
	}

	if len(cond) == 0 {
		return ""
	}
	return strings.Join(cond, " AND ")
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
		if pagerankField != "" {
			if prVal, ok := chunks[i][pagerankField]; ok {
				if f, ok := utility.ToFloat64(prVal); ok {
					score += f
				}
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
	sort.Slice(chunks, func(i, j int) bool {
		scoreI := getChunkScore(chunks[i])
		scoreJ := getChunkScore(chunks[j])
		return scoreI > scoreJ
	})

	// Limit
	if len(chunks) > limit && limit > 0 {
		chunks = chunks[:limit]
	}

	return chunks
}

// getChunkScore extracts the score from a chunk
func getChunkScore(chunk map[string]interface{}) float64 {
	if v, ok := chunk["_score"].(float64); ok {
		return v
	}
	if v, ok := chunk["SCORE"].(float64); ok {
		return v
	}
	if v, ok := chunk["SIMILARITY"].(float64); ok {
		return v
	}
	return 0.0
}

// GetAggregation aggregates field values from search results.
//
// Example:
// input chunks:
//
//	[{"docnm_kwd": "docA"}, {"docnm_kwd": "docA"}, {"docnm_kwd": "docB"}]
//
// GetAggregation(chunks, "docnm_kwd") returns:
//
//	[{"key": "docA", "count": 2}, {"key": "docB", "count": 1}]
//
// For tag_kwd field, splits values by "###" separator.
// For other fields, uses comma separation.
func (e *infinityEngine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	if len(chunks) == 0 {
		return []map[string]interface{}{}
	}

	// Check if field exists in first chunk
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

	// Count occurrences
	tagCounts := make(map[string]int)
	for _, chunk := range chunks {
		value, ok := chunk[fieldName]
		if !ok || value == nil {
			continue
		}

		// Handle string value
		if valueStr, ok := value.(string); ok {
			if valueStr == "" {
				continue
			}

			var tags []string
			// Split by "###" for tag_kwd field
			if fieldName == "tag_kwd" && strings.Contains(valueStr, "###") {
				for _, tag := range strings.Split(valueStr, "###") {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tags = append(tags, tag)
					}
				}
			} else {
				// Fallback to comma separation
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

		// Handle list value
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

	// Convert to slice and sort by count descending
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

// GetDocIDs extracts document IDs from search results.
// Extracts "id" field from each chunk and returns as a list.
func (e *infinityEngine) GetDocIDs(chunks []map[string]interface{}) []string {
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

// GetHighlight generates highlighted text snippets for search results.
// Matches keywords in text and wraps them with <em> tags.
func (e *infinityEngine) GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string {
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
			englishCount := 0
			totalCount := 0
			for _, r := range segment {
				if unicode.IsLetter(r) {
					totalCount++
					if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
						englishCount++
					}
				}
			}
			isEnglish := totalCount > 0 && float64(englishCount)/float64(totalCount) > 0.5
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