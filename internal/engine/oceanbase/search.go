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

package oceanbase

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"

	"go.uber.org/zap"
)

type searchPlan struct {
	text   *types.MatchTextExpr
	dense  *types.MatchDenseExpr
	fusion *types.FusionExpr
}

// Search executes filter, full-text, vector, or fusion search. When explicitly
// enabled and supported, the exact text+dense+fusion form first uses
// DBMS_HYBRID_SEARCH.SEARCH and falls back once to SQL only for recognized
// feature/package availability errors.
func (e *Engine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if req == nil || len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}
	types.LogSearchRequest("OceanBase", req)
	plan := parseSearchPlan(req.MatchExprs)
	if !e.flags.enableFullTextSearch && plan.text != nil && plan.dense != nil {
		plan.text = nil
		plan.fusion = nil
	}
	if plan.fusion != nil {
		weight := fusionVectorWeight(plan.fusion)
		if weight <= 0 {
			plan.dense, plan.fusion = nil, nil
		} else if weight >= 1 {
			plan.text, plan.fusion = nil, nil
		}
	}

	tableNames := uniqueStrings(req.IndexNames)
	mergeTables := len(tableNames) > 1
	candidateLimit := globalSearchCandidateLimit(req)
	effectivePlan := plan
	if mergeTables {
		effectivePlan = expandSearchPlan(plan, candidateLimit)
	}
	hiddenSortFields := searchHiddenSortFields(req, mergeTables)

	result := &types.SearchResult{Chunks: []map[string]interface{}{}}
	for _, tableName := range tableNames {
		if err := validateIdentifier(tableName); err != nil {
			return nil, err
		}
		exists, err := e.tableExists(ctx, tableName)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		kind := tableKind(tableName, req.KbIDs...)
		effectiveReq := *req
		effectiveReq.SelectFields = append([]string(nil), req.SelectFields...)
		if mergeTables {
			effectiveReq.Offset = 0
			effectiveReq.Limit = candidateLimit
			for _, field := range hiddenSortFields {
				effectiveReq.SelectFields = append(effectiveReq.SelectFields, field)
			}
		}
		if kind == "memory" && containsString(effectiveReq.SelectFields, "content_embed") {
			expectedVectorColumn := ""
			if plan.dense != nil {
				expectedVectorColumn = plan.dense.VectorColumnName
			}
			vectorColumn, vectorErr := e.findVectorColumn(ctx, tableName, expectedVectorColumn)
			if vectorErr != nil {
				return nil, vectorErr
			}
			fields := make([]string, 0, len(effectiveReq.SelectFields))
			for _, field := range effectiveReq.SelectFields {
				if field == "content_embed" {
					if vectorColumn != "" {
						fields = append(fields, vectorColumn)
					}
					continue
				}
				fields = append(fields, field)
			}
			effectiveReq.SelectFields = fields
		}
		condition := copyMap(req.Filter)
		if kind == "memory" {
			if len(req.KbIDs) > 0 {
				condition["memory_id"] = req.KbIDs
			}
			if _, present := condition["must_not"]; !present {
				condition["must_not"] = map[string]interface{}{"exists": "forget_at"}
			}
		} else if kind == "chunk" && len(req.KbIDs) > 0 {
			condition["kb_id"] = req.KbIDs
		}

		if e.hybridAvailable.Load() && isDBMSHybridPlan(plan) {
			chunks, used, hybridErr := e.searchWithDBMS(ctx, tableName, kind, condition, &effectiveReq, effectivePlan)
			if hybridErr != nil {
				if !isHybridUnavailableError(hybridErr) {
					return nil, hybridErr
				}
				e.hybridAvailable.Store(false)
				common.Warn("DBMS hybrid search unavailable; using SQL search", zap.Error(hybridErr))
			} else if used {
				result.Chunks = append(result.Chunks, chunks...)
				result.Total += int64(len(chunks))
				continue
			}
		}

		chunks, total, err := e.searchTableWithSQL(ctx, tableName, kind, condition, &effectiveReq, effectivePlan)
		if err != nil {
			return nil, err
		}
		result.Chunks = append(result.Chunks, chunks...)
		result.Total += total
	}
	if result.Total == 0 {
		result.Total = int64(len(result.Chunks))
	}
	if mergeTables {
		result.Chunks = mergeSearchChunks(result.Chunks, req, plan)
		for _, chunk := range result.Chunks {
			for _, field := range hiddenSortFields {
				delete(chunk, field)
			}
		}
	}
	return result, nil
}

func (e *Engine) searchTableWithSQL(ctx context.Context, tableName, kind string, condition map[string]interface{}, req *types.SearchRequest, plan searchPlan) ([]map[string]interface{}, int64, error) {
	fieldsSQL, _, err := buildSelectFields(req.SelectFields, kind)
	if err != nil {
		return nil, 0, err
	}
	filterSQL, filterArgs, err := buildFilter(condition, kind)
	if err != nil {
		return nil, 0, err
	}
	offset := max(req.Offset, 0)
	limit := req.Limit
	if limit <= 0 {
		limit = 30
	}

	switch {
	case plan.text != nil && plan.dense != nil:
		qualifiedFieldsSQL, _, err := buildQualifiedSelectFields(req.SelectFields, kind, "t")
		if err != nil {
			return nil, 0, err
		}
		return e.searchFusionSQL(ctx, tableName, kind, fieldsSQL, qualifiedFieldsSQL, filterSQL, filterArgs, offset, limit, plan)
	case plan.dense != nil:
		return e.searchVectorSQL(ctx, tableName, kind, fieldsSQL, filterSQL, filterArgs, offset, limit, plan.dense)
	case plan.text != nil:
		return e.searchFullTextSQL(ctx, tableName, kind, fieldsSQL, filterSQL, filterArgs, offset, limit, plan.text)
	default:
		count, err := scanCount(e.db.QueryRowContext(ctx, "SELECT COUNT("+quoteIdentifier(identifierField(kind))+") FROM "+quoteIdentifier(tableName)+" WHERE "+filterSQL, filterArgs...))
		if err != nil || count == 0 {
			return []map[string]interface{}{}, count, err
		}
		orderSQL, err := buildOrderBy(req.OrderBy, kind)
		if err != nil {
			return nil, 0, err
		}
		query := fmt.Sprintf("SELECT %s FROM %s WHERE %s%s LIMIT %d, %d", fieldsSQL, quoteIdentifier(tableName), filterSQL, orderSQL, offset, limit)
		rows, err := e.queryRows(ctx, query, filterArgs...)
		return decodeRows(rows, kind), count, err
	}
}

func (e *Engine) searchFullTextSQL(ctx context.Context, tableName, kind, fieldsSQL, filterSQL string, filterArgs []interface{}, offset, limit int, text *types.MatchTextExpr) ([]map[string]interface{}, int64, error) {
	filterExpr, filterTextArgs, scoreExpr, scoreArgs := e.fullTextExpressions(kind, text)
	hint := e.fullTextHint(tableName, kind)
	countQuery := fmt.Sprintf("SELECT %sCOUNT(%s) FROM %s WHERE %s AND %s", hint, quoteIdentifier(identifierField(kind)), quoteIdentifier(tableName), filterSQL, filterExpr)
	countArgs := appendCopy(filterArgs, filterTextArgs...)
	count, err := scanCount(e.db.QueryRowContext(ctx, countQuery, countArgs...))
	if err != nil || count == 0 {
		return []map[string]interface{}{}, count, err
	}
	query := fmt.Sprintf("SELECT %s%s, %s AS _score FROM %s WHERE %s AND %s ORDER BY _score DESC LIMIT %d, %d",
		hint, fieldsSQL, scoreExpr, quoteIdentifier(tableName), filterSQL, filterExpr, offset, minPositive(limit, text.TopN))
	args := appendCopy(scoreArgs, filterArgs...)
	args = append(args, filterTextArgs...)
	rows, err := e.queryRows(ctx, query, args...)
	return decodeRows(rows, kind), count, err
}

func (e *Engine) searchVectorSQL(ctx context.Context, tableName, kind, fieldsSQL, filterSQL string, filterArgs []interface{}, offset, limit int, dense *types.MatchDenseExpr) ([]map[string]interface{}, int64, error) {
	if err := validateVectorExpr(dense); err != nil {
		return nil, 0, err
	}
	vector, err := encodeVector(dense.EmbeddingData)
	if err != nil {
		return nil, 0, err
	}
	threshold := denseSimilarity(dense)
	column := quoteIdentifier(dense.VectorColumnName)
	scoreExpr := "(1 - COSINE_DISTANCE(" + column + ", ?))"
	countQuery := fmt.Sprintf("SELECT COUNT(%s) FROM %s WHERE %s AND %s >= ?", quoteIdentifier(identifierField(kind)), quoteIdentifier(tableName), filterSQL, scoreExpr)
	countArgs := appendCopy(filterArgs, vector, threshold)
	count, err := scanCount(e.db.QueryRowContext(ctx, countQuery, countArgs...))
	if err != nil || count == 0 {
		return []map[string]interface{}{}, count, err
	}
	query := fmt.Sprintf("SELECT %s, %s AS _score FROM %s WHERE %s AND %s >= ? ORDER BY COSINE_DISTANCE(%s, ?) APPROXIMATE LIMIT %d OFFSET %d",
		fieldsSQL, scoreExpr, quoteIdentifier(tableName), filterSQL, scoreExpr, column, minPositive(limit, dense.TopN), offset)
	args := []interface{}{vector}
	args = append(args, filterArgs...)
	args = append(args, vector, threshold, vector)
	rows, err := e.queryRows(ctx, query, args...)
	return decodeRows(rows, kind), count, err
}

func (e *Engine) searchFusionSQL(ctx context.Context, tableName, kind, fieldsSQL, qualifiedFieldsSQL, filterSQL string, filterArgs []interface{}, offset, limit int, plan searchPlan) ([]map[string]interface{}, int64, error) {
	if err := validateVectorExpr(plan.dense); err != nil {
		return nil, 0, err
	}
	vector, err := encodeVector(plan.dense.EmbeddingData)
	if err != nil {
		return nil, 0, err
	}
	textFilter, textFilterArgs, textScore, textScoreArgs := e.fullTextExpressions(kind, plan.text)
	threshold := denseSimilarity(plan.dense)
	vectorWeight := fusionVectorWeight(plan.fusion)
	textWeight := 1 - vectorWeight
	vectorColumn := quoteIdentifier(plan.dense.VectorColumnName)
	vectorScore := "(1 - COSINE_DISTANCE(" + vectorColumn + ", ?))"
	candidates := positiveOr(plan.text.TopN, limit) + positiveOr(plan.dense.TopN, limit)
	hint := e.fullTextHint(tableName, kind)

	if !e.flags.useFullTextFirstFusionSearch {
		return e.searchSymmetricFusionSQL(ctx, tableName, kind, qualifiedFieldsSQL, filterSQL, filterArgs, offset, limit, candidates, hint,
			textFilter, textFilterArgs, textScore, textScoreArgs, vector, vectorColumn, vectorScore, threshold, textWeight, vectorWeight, plan)
	}

	cte := fmt.Sprintf("WITH fulltext_results AS (SELECT %s*, %s AS relevance FROM %s WHERE %s AND %s ORDER BY relevance DESC LIMIT %d)",
		hint, textScore, quoteIdentifier(tableName), filterSQL, textFilter, candidates)
	countQuery := cte + " SELECT COUNT(*) FROM fulltext_results WHERE " + vectorScore + " >= ?"
	countArgs := appendCopy(textScoreArgs, filterArgs...)
	countArgs = append(countArgs, textFilterArgs...)
	countArgs = append(countArgs, vector, threshold)
	count, err := scanCount(e.db.QueryRowContext(ctx, countQuery, countArgs...))
	if err != nil || count == 0 {
		return []map[string]interface{}{}, count, err
	}
	score := fmt.Sprintf("(relevance * %s + %s * %s)",
		formatFloat(textWeight), vectorScore, formatFloat(vectorWeight))
	if kind == "chunk" {
		score = strings.TrimSuffix(score, ")") + " + (CAST(IFNULL(pagerank_fea, 0) AS DECIMAL(10, 2)) / 100))"
	}
	query := fmt.Sprintf("%s SELECT %s, %s AS _score FROM fulltext_results WHERE %s >= ? ORDER BY _score DESC LIMIT %d, %d",
		cte, fieldsSQL, score, vectorScore, offset, limit)
	args := appendCopy(textScoreArgs, filterArgs...)
	args = append(args, textFilterArgs...)
	args = append(args, vector, vector, threshold)
	rows, err := e.queryRows(ctx, query, args...)
	return decodeRows(rows, kind), count, err
}

func (e *Engine) searchSymmetricFusionSQL(ctx context.Context, tableName, kind, fieldsSQL, filterSQL string, filterArgs []interface{}, offset, limit, candidates int, hint, textFilter string, textFilterArgs []interface{}, textScore string, textScoreArgs []interface{}, vector, vectorColumn, vectorScore string, threshold, textWeight, vectorWeight float64, plan searchPlan) ([]map[string]interface{}, int64, error) {
	fullTextLimit := positiveOr(plan.text.TopN, candidates)
	vectorLimit := positiveOr(plan.dense.TopN, candidates)
	pagerankColumn := ""
	if kind == "chunk" {
		pagerankColumn = ", pagerank_fea"
	}
	identifier := quoteIdentifier(identifierField(kind))
	cte := fmt.Sprintf("WITH fulltext_results AS (SELECT %s%s AS id%s, %s AS relevance FROM %s WHERE %s AND %s ORDER BY relevance DESC LIMIT %d), "+
		"vector_results AS (SELECT %s AS id%s, %s AS similarity FROM %s WHERE %s AND %s >= ? ORDER BY COSINE_DISTANCE(%s, ?) APPROXIMATE LIMIT %d)",
		hint, identifier, pagerankColumn, textScore, quoteIdentifier(tableName), filterSQL, textFilter, fullTextLimit,
		identifier, pagerankColumn, vectorScore, quoteIdentifier(tableName), filterSQL, vectorScore, vectorColumn, vectorLimit)
	join := " FROM fulltext_results f FULL OUTER JOIN vector_results v ON f.id = v.id"
	countArgs := appendCopy(textScoreArgs, filterArgs...)
	countArgs = append(countArgs, textFilterArgs...)
	countArgs = append(countArgs, vector)
	countArgs = append(countArgs, filterArgs...)
	countArgs = append(countArgs, vector, threshold, vector)
	count, err := scanCount(e.db.QueryRowContext(ctx, cte+" SELECT COUNT(*)"+join, countArgs...))
	if err != nil || count == 0 {
		return []map[string]interface{}{}, count, err
	}
	score := fmt.Sprintf("(IFNULL(f.relevance, 0) * %s + IFNULL(v.similarity, 0) * %s)",
		formatFloat(textWeight), formatFloat(vectorWeight))
	if kind == "chunk" {
		score = strings.TrimSuffix(score, ")") + " + (CAST(IFNULL(f.pagerank_fea, 0) AS DECIMAL(10, 2)) / 100))"
	}
	query := cte + fmt.Sprintf(" SELECT %s, %s AS _score FROM (SELECT COALESCE(f.id, v.id) AS id, %s AS score%s) c JOIN %s t ON c.id = t.%s ORDER BY c.score DESC LIMIT %d, %d",
		fieldsSQL, "c.score", score, join, quoteIdentifier(tableName), identifier, offset, limit)
	rows, err := e.queryRows(ctx, query, countArgs...)
	return decodeRows(rows, kind), count, err
}

func (e *Engine) fullTextExpressions(kind string, text *types.MatchTextExpr) (string, []interface{}, string, []interface{}) {
	query := text.MatchingText
	if text.ExtraOptions != nil {
		if original := stringValue(text.ExtraOptions["original_query"]); original != "" {
			query = strings.TrimSpace(original)
		}
	}
	fields, weights := e.fullTextFields(kind, text)
	filterParts := make([]string, len(fields))
	scoreParts := make([]string, len(fields))
	filterArgs := make([]interface{}, len(fields))
	scoreArgs := make([]interface{}, len(fields))
	for i, field := range fields {
		expression := fmt.Sprintf("MATCH (%s) AGAINST (? IN NATURAL LANGUAGE MODE)", quoteIdentifier(field))
		filterParts[i] = expression
		scoreParts[i] = expression + " * " + formatFloat(weights[i])
		filterArgs[i] = query
		scoreArgs[i] = query
	}
	return "(" + strings.Join(filterParts, " OR ") + ")", filterArgs,
		"(" + strings.Join(scoreParts, " + ") + ")", scoreArgs
}

func (e *Engine) fullTextFields(kind string, text *types.MatchTextExpr) ([]string, []float64) {
	var specifications []string
	switch kind {
	case "memory":
		specifications = []string{"content_ltks", "tokenized_content_ltks"}
	case "skill":
		specifications = text.Fields
		if len(specifications) == 0 {
			specifications = []string{"name_tks^10", "tags_tks^5", "description_tks^3", "content_tks"}
		}
	default:
		if e.flags.searchOriginalContent {
			specifications = []string{"docnm_kwd^10", "content_with_weight", "important_tks^20", "question_tks^20"}
		} else {
			specifications = []string{"title_tks^10", "title_sm_tks^5", "important_tks^20", "question_tks^20", "content_ltks^2", "content_sm_ltks"}
		}
	}
	fields := make([]string, 0, len(specifications))
	weights := make([]float64, 0, len(specifications))
	for _, specification := range specifications {
		parts := strings.SplitN(specification, "^", 2)
		field := parts[0]
		if kind == "skill" && !strings.HasSuffix(field, "_tks") {
			field += "_tks"
		}
		weight := 1.0
		if len(parts) == 2 {
			if parsed, err := strconv.ParseFloat(parts[1], 64); err == nil {
				weight = parsed
			}
		}
		fields = append(fields, field)
		weights = append(weights, weight)
	}
	var total float64
	for _, weight := range weights {
		total += weight
	}
	if total <= 0 && len(weights) > 0 {
		total = float64(len(weights))
		for i := range weights {
			weights[i] = 1
		}
	}
	for i := range weights {
		weights[i] /= total
	}
	return fields, weights
}

func (e *Engine) fullTextHint(tableName, kind string) string {
	if !e.flags.useFullTextHint || kind == "skill" {
		return ""
	}
	fields, _ := e.fullTextFields(kind, &types.MatchTextExpr{})
	indexes := make([]string, len(fields))
	for i, field := range fields {
		indexes[i] = "fts_idx_" + field
	}
	return fmt.Sprintf("/*+ UNION_MERGE(%s %s) */ ", tableName, strings.Join(indexes, " "))
}

func buildOrderBy(orderBy *types.OrderByExpr, kind string) (string, error) {
	if orderBy == nil || len(orderBy.Fields) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(orderBy.Fields))
	for _, order := range orderBy.Fields {
		column := order.Field
		if kind == "memory" {
			column = mapMemoryField(column)
		}
		if kind == "chunk" && column == "chunk_order_int" {
			column = "_order_id"
		}
		if !validColumns(kind)[column] {
			return "", fmt.Errorf("unknown order field: %s", order.Field)
		}
		expression := quoteIdentifier(column)
		if kind == "chunk" && arrayColumns[column] {
			expression = "ARRAY_AVG(" + expression + ")"
		}
		direction := "ASC"
		if order.Type == types.SortDesc {
			direction = "DESC"
		}
		parts = append(parts, expression+" "+direction)
	}
	return " ORDER BY " + strings.Join(parts, ", "), nil
}

func parseSearchPlan(expressions []interface{}) searchPlan {
	var plan searchPlan
	for _, expression := range expressions {
		switch value := expression.(type) {
		case string:
			if value != "" {
				plan.text = &types.MatchTextExpr{MatchingText: value}
			}
		case *types.MatchTextExpr:
			if value != nil && value.MatchingText != "" {
				plan.text = value
			}
		case *types.MatchDenseExpr:
			if value != nil && len(value.EmbeddingData) > 0 {
				plan.dense = value
			}
		case *types.FusionExpr:
			plan.fusion = value
		}
	}
	return plan
}

func validateVectorExpr(dense *types.MatchDenseExpr) error {
	if dense == nil || len(dense.EmbeddingData) == 0 {
		return fmt.Errorf("vector expression is empty")
	}
	if dense.EmbeddingDataType != "" && dense.EmbeddingDataType != "float" {
		return fmt.Errorf("embedding data type %q is not float", dense.EmbeddingDataType)
	}
	if !vectorColumnPattern.MatchString(dense.VectorColumnName) {
		return fmt.Errorf("invalid vector column: %s", dense.VectorColumnName)
	}
	return nil
}

func denseSimilarity(dense *types.MatchDenseExpr) float64 {
	if dense.ExtraOptions != nil {
		switch value := dense.ExtraOptions["similarity"].(type) {
		case float64:
			return value
		case float32:
			return float64(value)
		case int:
			return float64(value)
		}
	}
	return 0
}

func fusionVectorWeight(fusion *types.FusionExpr) float64 {
	if fusion == nil || fusion.FusionParams == nil {
		return 0.5
	}
	weights := strings.Split(stringValue(fusion.FusionParams["weights"]), ",")
	if len(weights) != 2 {
		return 0.5
	}
	weight, err := strconv.ParseFloat(strings.TrimSpace(weights[1]), 64)
	if err != nil {
		return 0.5
	}
	return weight
}

func decodeRows(rows []map[string]interface{}, kind string) []map[string]interface{} {
	result := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		result[i] = decodeLogicalRow(row, kind)
	}
	return result
}

func formatFloat(value float64) string { return strconv.FormatFloat(value, 'g', -1, 64) }

func minPositive(first, second int) int {
	if first <= 0 {
		return second
	}
	if second <= 0 || first < second {
		return first
	}
	return second
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func appendCopy(values []interface{}, extra ...interface{}) []interface{} {
	result := make([]interface{}, 0, len(values)+len(extra))
	result = append(result, values...)
	result = append(result, extra...)
	return result
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func globalSearchCandidateLimit(req *types.SearchRequest) int {
	return max(req.Offset, 0) + positiveOr(req.Limit, 30)
}

func expandSearchPlan(plan searchPlan, candidateLimit int) searchPlan {
	expanded := plan
	if plan.text != nil {
		text := *plan.text
		text.TopN = max(text.TopN, candidateLimit)
		expanded.text = &text
	}
	if plan.dense != nil {
		dense := *plan.dense
		dense.TopN = max(dense.TopN, candidateLimit)
		expanded.dense = &dense
	}
	return expanded
}

func searchHiddenSortFields(req *types.SearchRequest, mergeTables bool) []string {
	if !mergeTables || req.OrderBy == nil || len(req.OrderBy.Fields) == 0 || len(req.SelectFields) == 0 || containsString(req.SelectFields, "*") {
		return nil
	}
	fields := make([]string, 0, len(req.OrderBy.Fields))
	for _, order := range req.OrderBy.Fields {
		if order.Field == "_score" || containsString(req.SelectFields, order.Field) || containsString(fields, order.Field) {
			continue
		}
		fields = append(fields, order.Field)
	}
	return fields
}

func mergeSearchChunks(chunks []map[string]interface{}, req *types.SearchRequest, plan searchPlan) []map[string]interface{} {
	if req.OrderBy != nil && len(req.OrderBy.Fields) > 0 {
		sort.SliceStable(chunks, func(i, j int) bool {
			for _, order := range req.OrderBy.Fields {
				comparison := compareSearchValues(chunks[i][order.Field], chunks[j][order.Field], order.Field)
				if comparison == 0 {
					continue
				}
				if order.Type == types.SortDesc {
					return comparison > 0
				}
				return comparison < 0
			}
			return false
		})
	} else if plan.text != nil || plan.dense != nil {
		sort.SliceStable(chunks, func(i, j int) bool {
			return compareSearchValues(chunks[i]["_score"], chunks[j]["_score"], "_score") > 0
		})
	}
	offset := min(max(req.Offset, 0), len(chunks))
	limit := positiveOr(req.Limit, 30)
	end := min(offset+limit, len(chunks))
	return chunks[offset:end]
}

func compareSearchValues(left, right interface{}, field string) int {
	if left == nil {
		if right == nil {
			return 0
		}
		return -1
	}
	if right == nil {
		return 1
	}
	leftNumber, leftNumeric := searchSortNumber(left, field)
	rightNumber, rightNumeric := searchSortNumber(right, field)
	if leftNumeric && rightNumeric {
		switch {
		case leftNumber < rightNumber:
			return -1
		case leftNumber > rightNumber:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(fmt.Sprint(left), fmt.Sprint(right))
}

func searchSortNumber(value interface{}, field string) (float64, bool) {
	if values, ok := interfaceSlice(value); ok {
		if len(values) == 0 {
			return 0, false
		}
		var total float64
		for _, item := range values {
			number, ok := searchSortNumber(item, field)
			if !ok {
				return 0, false
			}
			total += number
		}
		return total / float64(len(values)), true
	}
	if number, ok := numberToFloat(value); ok {
		return number, true
	}
	if number, ok := value.(json.Number); ok {
		parsed, err := number.Float64()
		return parsed, err == nil
	}
	if strings.HasSuffix(field, "_int") || strings.HasSuffix(field, "_flt") || field == "_score" {
		parsed, err := strconv.ParseFloat(fmt.Sprint(value), 64)
		return parsed, err == nil
	}
	return 0, false
}

func isDBMSHybridPlan(plan searchPlan) bool {
	return plan.text != nil && plan.dense != nil && plan.fusion != nil
}

func (e *Engine) searchWithDBMS(ctx context.Context, tableName, kind string, condition map[string]interface{}, req *types.SearchRequest, plan searchPlan) ([]map[string]interface{}, bool, error) {
	body, ok := buildDBMSBody(kind, condition, req, plan)
	if !ok {
		return nil, false, nil
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, false, err
	}
	var raw []byte
	if err := e.db.QueryRowContext(ctx, "SELECT DBMS_HYBRID_SEARCH.SEARCH(?, ?)", tableName, string(encoded)).Scan(&raw); err != nil {
		return nil, false, fmt.Errorf("DBMS hybrid search: %w", err)
	}
	if len(raw) == 0 {
		return []map[string]interface{}{}, true, nil
	}
	var documents []map[string]interface{}
	if err := json.Unmarshal(raw, &documents); err == nil {
		return decodeRows(documents, kind), true, nil
	}
	var response map[string]interface{}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, false, fmt.Errorf("decode DBMS hybrid search response: %w", err)
	}
	documents = extractHybridHits(response)
	return decodeRows(documents, kind), true, nil
}

func buildDBMSBody(kind string, condition map[string]interface{}, req *types.SearchRequest, plan searchPlan) (map[string]interface{}, bool) {
	filters := make([]interface{}, 0, len(condition))
	valid := validColumns(kind)
	for rawField, value := range condition {
		field := rawField
		if kind == "memory" {
			field = mapMemoryField(field)
		} else if kind == "chunk" {
			field = mapChunkField(field)
		} else if kind == "skill" && field == "id" {
			field = "skill_id"
		}
		if !valid[field] {
			return nil, false
		}
		if field == "available_int" {
			if fmt.Sprint(value) == "0" {
				filters = append(filters, map[string]interface{}{"range": map[string]interface{}{field: map[string]interface{}{"lt": 1}}})
			} else {
				filters = append(filters, map[string]interface{}{"bool": map[string]interface{}{"must_not": map[string]interface{}{"range": map[string]interface{}{field: map[string]interface{}{"lt": 1}}}}})
			}
		} else if isEmptyFilterValue(value) {
			continue
		} else if values, ok := interfaceSlice(value); ok {
			filters = append(filters, map[string]interface{}{"terms": map[string]interface{}{field: values}})
		} else {
			filters = append(filters, map[string]interface{}{"term": map[string]interface{}{field: value}})
		}
	}
	queryText := plan.text.MatchingText
	minimumShouldMatch := interface{}(0.0)
	if plan.text.ExtraOptions != nil {
		if value := plan.text.ExtraOptions["minimum_should_match"]; value != nil {
			minimumShouldMatch = value
		}
	}
	if value, ok := minimumShouldMatch.(float64); ok {
		minimumShouldMatch = common.FormatMinimumShouldMatchPercent(value)
	} else if value, ok := minimumShouldMatch.(float32); ok {
		minimumShouldMatch = common.FormatMinimumShouldMatchPercent(float64(value))
	}
	boolQuery := map[string]interface{}{
		"must": []interface{}{map[string]interface{}{"query_string": map[string]interface{}{
			"fields": tokenizedDBMSFields(kind), "type": "best_fields", "query": queryText,
			"minimum_should_match": minimumShouldMatch, "boost": 1,
		}}},
		"filter": filters,
		"boost":  1 - fusionVectorWeight(plan.fusion),
	}
	if len(req.RankFeature) > 0 {
		should := make([]interface{}, 0, len(req.RankFeature))
		for field, boost := range req.RankFeature {
			if field != "pagerank_fea" {
				field = "tag_feas." + field
			}
			should = append(should, map[string]interface{}{"rank_feature": map[string]interface{}{
				"field": field, "linear": map[string]interface{}{}, "boost": boost,
			}})
		}
		boolQuery["should"] = should
	}
	body := map[string]interface{}{
		"query": map[string]interface{}{"bool": boolQuery},
		"knn": map[string]interface{}{
			"field": plan.dense.VectorColumnName, "k": positiveOr(plan.dense.TopN, req.Limit),
			"num_candidates": positiveOr(plan.dense.TopN, req.Limit) * 2,
			"query_vector":   plan.dense.EmbeddingData, "filter": map[string]interface{}{"bool": boolQuery},
			"similarity": denseSimilarity(plan.dense),
		},
		"from": max(req.Offset, 0), "size": positiveOr(req.Limit, 30),
	}
	if req.OrderBy != nil && len(req.OrderBy.Fields) > 0 {
		sorts := make([]interface{}, 0, len(req.OrderBy.Fields))
		for _, order := range req.OrderBy.Fields {
			direction := "asc"
			if order.Type == types.SortDesc {
				direction = "desc"
			}
			orderInfo := map[string]interface{}{"order": direction}
			if order.Field == "page_num_int" || order.Field == "top_int" {
				orderInfo["unmapped_type"] = "float"
				orderInfo["mode"] = "avg"
				orderInfo["numeric_type"] = "double"
			} else if strings.HasSuffix(order.Field, "_int") || strings.HasSuffix(order.Field, "_flt") {
				orderInfo["unmapped_type"] = "float"
			} else {
				orderInfo["unmapped_type"] = "text"
			}
			sorts = append(sorts, map[string]interface{}{order.Field: orderInfo})
		}
		body["sort"] = sorts
	}
	return body, true
}

func tokenizedDBMSFields(kind string) []string {
	switch kind {
	case "memory":
		return []string{"content_ltks", "tokenized_content_ltks"}
	case "skill":
		return []string{"name_tks^10", "tags_tks^5", "description_tks^3", "content_tks"}
	default:
		return []string{"title_tks^10", "title_sm_tks^5", "important_tks^20", "question_tks^20", "content_ltks^2", "content_sm_ltks"}
	}
}

func extractHybridHits(response map[string]interface{}) []map[string]interface{} {
	hitsObject, ok := response["hits"].(map[string]interface{})
	if !ok {
		return nil
	}
	hits, ok := hitsObject["hits"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]interface{}, 0, len(hits))
	for _, item := range hits {
		hit, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		document, _ := hit["_source"].(map[string]interface{})
		if document == nil {
			document = hit
		}
		if score, ok := hit["_score"]; ok {
			document["_score"] = score
		}
		result = append(result, document)
	}
	return result
}

func isHybridUnavailableError(err error) bool {
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "dbms_hybrid_search") {
		return false
	}
	markers := []string{"does not exist", "not exist", "unknown", "not supported", "ora-00904", "1305"}
	for _, marker := range markers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
