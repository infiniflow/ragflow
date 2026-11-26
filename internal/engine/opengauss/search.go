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

package opengauss

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

type searchPlan struct {
	text   *types.MatchTextExpr
	dense  *types.MatchDenseExpr
	fusion *types.FusionExpr
}

var (
	boostSuffixRegex = regexp.MustCompile(`[~^]\d+(?:\.\d+)?`)
	fieldSpecRegex   = regexp.MustCompile(`^(.+?)(?:\^(\d+(?:\.\d+)?))?$`)
)

// sqlArgs accumulates bound arguments while their $n placeholders are embedded
// in the SQL text, keeping numbering and order in lockstep. Sub-clauses built
// standalone (filters, CTEs) are spliced in through embedFilter, which shifts
// their placeholders past everything already consumed.
type sqlArgs struct {
	values []interface{}
}

// add appends values and returns the 1-based placeholder index of the last
// one appended.
func (s *sqlArgs) add(values ...interface{}) int {
	s.values = append(s.values, values...)
	return len(s.values)
}

// embedFilter splices a standalone filter fragment into this statement:
// its $n placeholders are shifted past the arguments already consumed, and
// its arguments are appended in matching order.
func (s *sqlArgs) embedFilter(filterSQL string, filterArgs []interface{}) string {
	shifted := shiftPlaceholders(filterSQL, len(s.values))
	s.values = append(s.values, filterArgs...)
	return shifted
}

// Search dispatches filter, BM25, cosine and hybrid searches over the requested tables.
func (e *Engine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if req == nil || len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}
	types.LogSearchRequest("openGauss", req)
	plan := parseSearchPlan(req.MatchExprs)
	if plan.fusion != nil {
		textWeight, vectorWeight := fusionWeights(plan.fusion)
		if vectorWeight <= 0 {
			plan.dense = nil
		} else if textWeight <= 0 {
			plan.text = nil
		}
	}

	tableNames := common.Deduplicate(req.IndexNames)
	mergeTables := len(tableNames) > 1
	hiddenSortFields := searchHiddenSortFields(req, mergeTables)
	scored := plan.text != nil || plan.dense != nil

	// Single-table searches page inside each SQL statement. Multi-table
	// searches window only once, on the merged set: every table returns the
	// whole candidate range (LIMIT offset+limit OFFSET 0) and
	// mergeSearchChunks applies req's page window after merging — a
	// per-table OFFSET would skip rows that belong in the global window.
	tableOffset := max(req.Offset, 0)
	tableLimit := common.PositiveOr(req.Limit, 30)
	if mergeTables {
		tableLimit = tableOffset + tableLimit
		tableOffset = 0
	}

	type projectionGroup struct {
		chunks []map[string]interface{}
		hidden []string
	}
	var projectedGroups []projectionGroup
	var orderedHits []orderedHit
	var mergedOrderFields map[string]bool
	result := &types.SearchResult{Chunks: []map[string]interface{}{}}
	for _, name := range tableNames {
		tableName := physicalTableName(name)
		if err := validateIdentifier(tableName); err != nil {
			return nil, err
		}
		columns, err := e.listTableColumns(ctx, tableName)
		if err != nil {
			return nil, err
		}
		if len(columns) == 0 {
			continue
		}
		kind := tableKind(tableName, req.KbIDs...)
		vectorColumn := ""
		if kind == "memory" && plan.dense != nil {
			vectorColumn = plan.dense.VectorColumnName
		}
		output := buildSearchOutput(req.SelectFields, kind, columns, scored, hiddenSortFields, vectorColumn)
		if kind == "skill" && len(req.SelectFields) == 0 {
			output = []string{"*"}
		}
		if scored && kind == "chunk" && !slices.Contains(output, "*") && !slices.Contains(output, "kb_id") {
			output = append(output, "kb_id")
		}
		if len(output) == 0 {
			continue
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
		var orderSQL string
		if !scored {
			if mergeTables && req.OrderBy != nil && mergedOrderFields == nil {
				mergedOrderFields = make(map[string]bool)
			}
			orderSQL, err = buildOrderBy(req.OrderBy, kind, columns, mergedOrderFields)
			if err != nil {
				return nil, err
			}
		}
		chunks, total, err := e.searchTable(ctx, tableName, kind, columns, output, condition, orderSQL, tableOffset, tableLimit, plan)
		if err != nil {
			return nil, err
		}
		decoded := e.decodeRows(chunks, kind, vectorColumn)
		if mergedOrderFields != nil && len(req.OrderBy.Fields) > 0 {
			for i, row := range decoded {
				keys := make([]any, len(req.OrderBy.Fields))
				for j, order := range req.OrderBy.Fields {
					column := order.Field
					if kind == "memory" {
						column = mapMemoryField(column)
					}
					if isTextColumn(columns[column].dataType) {
						// ORDER BY uses the stored text, even for encoded keyword lists.
						if raw, ok := chunks[i][column].(string); ok {
							keys[j] = sortText(raw)
						}
					} else {
						keys[j] = row[order.Field]
					}
				}
				orderedHits = append(orderedHits, orderedHit{row, keys})
			}
		}
		result.Chunks = append(result.Chunks, decoded...)
		if len(hiddenSortFields) > 0 && !slices.Contains(output, "*") {
			var hidden []string
			for _, field := range hiddenSortFields {
				if field != identifierColumn(kind) && !(scored && kind == "chunk" && (field == "pagerank_fea" || field == "kb_id")) {
					hidden = append(hidden, field)
				}
			}
			projectedGroups = append(projectedGroups, projectionGroup{decoded, hidden})
		}
		result.Total += total
	}
	if mergedOrderFields != nil {
		for _, order := range req.OrderBy.Fields {
			if !mergedOrderFields[order.Field] {
				return nil, fmt.Errorf("opengauss: unknown order field: %s", order.Field)
			}
		}
	}
	if result.Total == 0 {
		result.Total = int64(len(result.Chunks))
	}
	if mergeTables {
		page := *req
		if plan.topN() > 0 {
			n := plan.topN()
			result.Total = min(result.Total, int64(n))
			if page.Offset >= n {
				result.Chunks = result.Chunks[:0]
				return result, nil
			}
			page.Limit = min(common.PositiveOr(page.Limit, 30), n-max(page.Offset, 0))
		}
		result.Chunks = mergeSearchChunks(result.Chunks, orderedHits, &page, scored)
		for _, group := range projectedGroups {
			for _, chunk := range group.chunks {
				for _, field := range group.hidden {
					delete(chunk, field)
				}
			}
		}
	} else if scored {
		// Each scored result is sorted by
		// score + pagerank (the ES rescore equivalent) and cut to the limit.
		sortScoredChunks(result.Chunks)
	}
	return result, nil
}

// searchTable runs one table's query. offset/limit form the per-table SQL
// window: req's page in single-table mode, the whole candidate range in merge
// mode, where mergeSearchChunks applies the page window after merging.
func (e *Engine) searchTable(ctx context.Context, tableName, kind string, columns map[string]columnMeta, output []string, condition map[string]interface{}, orderSQL string, offset, limit int, plan searchPlan) ([]map[string]interface{}, int64, error) {
	filterSQL, filterArgs, err := e.buildFilter(condition, kind, columns)
	if err != nil {
		return nil, 0, err
	}

	if plan.dense != nil {
		if err := validateVectorExpr(plan.dense); err != nil {
			return nil, 0, err
		}
		if _, ok := columns[plan.dense.VectorColumnName]; !ok && plan.text == nil {
			return nil, 0, nil
		}
	}
	switch {
	case plan.text != nil && plan.dense != nil:
		return e.searchFusion(ctx, tableName, kind, output, filterSQL, filterArgs, offset, limit, plan)
	case plan.dense != nil:
		return e.searchVector(ctx, tableName, kind, output, filterSQL, filterArgs, offset, limit, plan)
	case plan.text != nil:
		return e.searchFullText(ctx, tableName, kind, output, filterSQL, filterArgs, offset, limit, plan)
	default:
		return e.searchFilterOnly(ctx, tableName, kind, output, filterSQL, filterArgs, orderSQL, offset, limit)
	}
}

// buildSearchOutput projects requested fields onto the table's live columns,
// keeping only what exists (get_fields fills the gaps with nil on read),
// always selecting the row identifier, and adding pagerank_fea for scored
// chunk searches so the post-sort can fold it into the score.
func buildSearchOutput(selectFields []string, kind string, columns map[string]columnMeta, scored bool, hiddenSortFields []string, vectorColumn string) []string {
	allColumns := false
	fields := make([]string, 0, len(selectFields)+3)
	for _, field := range selectFields {
		switch field {
		case "_score":
			continue
		case "*":
			allColumns = true
		default:
			fields = append(fields, field)
		}
	}
	if allColumns {
		return []string{"*"}
	}
	fields = append(fields, hiddenSortFields...)
	if kind == "memory" {
		for i, field := range fields {
			if field == "content_embed" {
				fields[i] = vectorColumn
				if vectorColumn == "" {
					for column := range columns {
						if vectorColumnPattern.MatchString(column) {
							fields = append(fields, column)
						}
					}
				}
				continue
			}
			fields[i] = mapMemoryField(field)
		}
	}
	fields = append(fields, identifierColumn(kind))
	if scored && kind == "chunk" {
		fields = append(fields, "pagerank_fea")
	}
	output := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}
		if _, exists := columns[field]; exists && !slices.Contains(output, field) {
			output = append(output, field)
		}
	}
	return output
}

// selectFieldsSQL renders the SELECT list, quoting every field and passing
// "*" through untouched.
func selectFieldsSQL(output []string) string {
	quoted := make([]string, len(output))
	for i, field := range output {
		if field == "*" {
			quoted[i] = "*"
			continue
		}
		quoted[i] = quoteIdent(field)
	}
	return strings.Join(quoted, ", ")
}

// expandOutputIfStar materializes a "*" selection into the live column list
// for queries that must enumerate fields (the fusion COALESCE projection).
func expandOutputIfStar(output []string, columns map[string]columnMeta) []string {
	if len(output) != 1 || output[0] != "*" {
		return output
	}
	names := make([]string, 0, len(columns))
	for column := range columns {
		names = append(names, column)
	}
	sort.Strings(names)
	return names
}

// searchFullText runs the keyword path: a ranked full-text CTE wrapped in a
// query that applies the SQL window over the ranking order.
func (e *Engine) searchFullText(ctx context.Context, tableName, kind string, output []string, filterSQL string, filterArgs []interface{}, offset, limit int, plan searchPlan) ([]map[string]interface{}, int64, error) {
	args := &sqlArgs{}
	scan, err := e.bm25Scan(ctx, tableName, kind, output, filterSQL, filterArgs, plan.text, args)
	if err != nil {
		return nil, 0, err
	}
	if plan.fusion != nil && plan.text.TopN > 0 {
		scan += fmt.Sprintf(" LIMIT %d", plan.text.TopN)
	}
	return e.dataVecPage(ctx, kind, scan, args.values, offset, limit, plan.topN(), true)
}

// searchVector runs the dense path over the expression's q_N_vec column,
// ordered by distance-derived similarity.
func (e *Engine) searchVector(ctx context.Context, tableName, kind string, output []string, filterSQL string, filterArgs []interface{}, offset, limit int, plan searchPlan) ([]map[string]interface{}, int64, error) {
	dense := plan.dense
	if err := validateVectorExpr(dense); err != nil {
		return nil, 0, err
	}
	vector, err := encodeVector(dense.EmbeddingData)
	if err != nil {
		return nil, 0, err
	}
	args := &sqlArgs{}
	cte, err := e.vectorCTE(tableName, kind, output, filterSQL, filterArgs, dense, dense.TopN, denseSimilarity(dense), vector, args)
	if err != nil {
		return nil, 0, err
	}
	return e.dataVecPage(ctx, kind, cte, args.values, offset, limit, plan.topN(), false)
}

// A positive candidate cap uses the distance operator directly for HNSW.
// With no cap, every eligible row participates in the final ranking.
func (e *Engine) vectorCTE(tableName, kind string, output []string, filterSQL string, filterArgs []interface{}, dense *types.MatchDenseExpr, topn int, threshold float64, vector string, args *sqlArgs) (string, error) {
	if err := validateVectorExpr(dense); err != nil {
		return "", err
	}
	filterPart := args.embedFilter(filterSQL, filterArgs)
	first := args.add(vector)
	column := quoteIdent(dense.VectorColumnName)
	filterPart = "(" + filterPart + ") AND " + column + " IS NOT NULL"
	scoreExpr := fmt.Sprintf("(1 - (%s <=> $%d))", column, first)
	inner := fmt.Sprintf("SELECT %s, %s AS _score FROM %s WHERE %s", selectFieldsSQL(output), scoreExpr, quoteIdent(tableName), filterPart)
	if topn > 0 {
		// The distance-only ordering can use HNSW. Keep its boundary peers
		// until the primary key can break the tie in the enclosing query.
		inner += fmt.Sprintf(" ORDER BY %s <=> $%d FETCH FIRST %d ROWS WITH TIES", column, args.add(vector), topn)
		order := "_score DESC, " + identifierColumn(kind)
		if kind == "chunk" {
			order += ", kb_id"
		}
		inner = fmt.Sprintf("SELECT * FROM (%s) AS peers ORDER BY %s LIMIT %d", inner, order, topn)
	}
	thresholdPlaceholder := args.add(threshold)
	return fmt.Sprintf("SELECT * FROM (%s) AS knn WHERE _score >= $%d", inner, thresholdPlaceholder), nil
}

// searchFusion runs the hybrid path: full-text and vector candidates scored
// and combined by the weighted-fusion formula.
func (e *Engine) searchFusion(ctx context.Context, tableName, kind string, output []string, filterSQL string, filterArgs []interface{}, offset, limit int, plan searchPlan) ([]map[string]interface{}, int64, error) {
	if err := validateVectorExpr(plan.dense); err != nil {
		return nil, 0, err
	}
	vector, err := encodeVector(plan.dense.EmbeddingData)
	if err != nil {
		return nil, 0, err
	}
	columns, err := e.listTableColumns(ctx, tableName)
	if err != nil {
		return nil, 0, err
	}
	output = expandOutputIfStar(output, columns)
	args := &sqlArgs{}
	textLimit := common.PositiveOr(plan.text.TopN, plan.topN())
	denseLimit := common.PositiveOr(plan.dense.TopN, plan.topN())
	fulltextSQL, err := e.bm25Scan(ctx, tableName, kind, output, filterSQL, filterArgs, plan.text, args)
	if err != nil {
		return nil, 0, err
	}
	if textLimit > 0 {
		fulltextSQL += fmt.Sprintf(" LIMIT %d", textLimit)
	}
	threshold := denseSimilarity(plan.dense)
	candidateThreshold := threshold
	if kind != "chunk" {
		candidateThreshold = -1
	}
	var vectorSQL string
	if _, ok := columns[plan.dense.VectorColumnName]; !ok {
		vectorSQL = fmt.Sprintf("SELECT %s, 0::float8 AS _score FROM %s WHERE false", selectFieldsSQL(output), quoteIdent(tableName))
	} else {
		vectorSQL, err = e.vectorCTE(tableName, kind, output, filterSQL, filterArgs, plan.dense, denseLimit, candidateThreshold, vector, args)
		if err != nil {
			return nil, 0, err
		}
	}

	// Normalize the full-text score by this table's maximum to [0,1] before
	// the weighted sum: a raw BM25 score fused directly with the
	// [0,1] cosine similarity lets any fulltext hit outrank every vector-only
	// row. The divisor falls back to 1 when fulltext is empty or all-zero so
	// vector-only rows keep a numeric score.
	textWeight, vectorWeight := fusionWeights(plan.fusion)
	fused := fmt.Sprintf("(COALESCE(a._score, 0) / COALESCE(NULLIF((SELECT MAX(_score) FROM filter_fulltext), 0), 1) * %s + COALESCE(b._score, 0) * %s)",
		formatFloat(textWeight), formatFloat(vectorWeight))
	projection := make([]string, 0, len(output)+1)
	for _, field := range output {
		projection = append(projection, fmt.Sprintf("COALESCE(a.%s, b.%s) AS %s", quoteIdent(field), quoteIdent(field), quoteIdent(field)))
	}
	projection = append(projection, fused+" AS _score")
	idColumn := quoteIdent(identifierColumn(kind))
	join := fmt.Sprintf("a.%s = b.%s", idColumn, idColumn)
	if kind == "chunk" {
		join += " AND a.kb_id = b.kb_id"
	}
	scan := fmt.Sprintf("WITH filter_fulltext AS (%s), filter_vector AS (%s) SELECT %s FROM filter_fulltext a FULL OUTER JOIN filter_vector b ON %s", fulltextSQL, vectorSQL, strings.Join(projection, ", "), join)
	// Chunk retrieval applies the user threshold after term/vector reranking.
	if kind != "chunk" {
		scan = fmt.Sprintf("SELECT * FROM (%s) AS fused WHERE %s >= $%d", scan, dataVecRank(kind), args.add(threshold))
	}
	return e.dataVecPage(ctx, kind, scan, args.values, offset, limit, plan.topN(), true)
}

// searchFilterOnly serves requests with no text or vector expression: one
// COUNT for the total plus one filtered, ordered select for the page.
func (e *Engine) searchFilterOnly(ctx context.Context, tableName, kind string, output []string, filterSQL string, filterArgs []interface{}, orderSQL string, offset, limit int) ([]map[string]interface{}, int64, error) {
	identifier := quoteIdent(identifierColumn(kind))
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	var count int64
	if err := tx.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(%s) FROM %s WHERE %s", identifier, quoteIdent(tableName), filterSQL),
		filterArgs...).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("opengauss: count %s: %w", tableName, err)
	}
	if count == 0 {
		return []map[string]interface{}{}, 0, nil
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s%s LIMIT %d OFFSET %d",
		selectFieldsSQL(output), quoteIdent(tableName), filterSQL, orderSQL, limit, offset)
	rows, err := queryRowsTx(ctx, tx, query, filterArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("opengauss: filter search %s: %w", tableName, err)
	}
	return rows, count, nil
}

// A missing sort column is NULL throughout that table. For a merge, record the
// fields seen across tables so the caller can still reject unknown sort fields.
func buildOrderBy(orderBy *types.OrderByExpr, kind string, columns map[string]columnMeta, mergedFields map[string]bool) (string, error) {
	keys := []string{identifierColumn(kind)}
	if kind == "chunk" {
		keys = append(keys, "kb_id")
	}
	if orderBy == nil {
		orderBy = &types.OrderByExpr{}
	}
	parts := make([]string, 0, len(orderBy.Fields)+len(keys))
	for _, order := range orderBy.Fields {
		column := order.Field
		if kind == "memory" {
			column = mapMemoryField(column)
		}
		if _, exists := columns[column]; !exists {
			if mergedFields != nil {
				continue
			}
			return "", fmt.Errorf("opengauss: unknown order field: %s", order.Field)
		}
		if mergedFields != nil {
			mergedFields[order.Field] = true
		}
		if i := slices.Index(keys, column); i >= 0 {
			keys = slices.Delete(keys, i, i+1)
		}
		expression := quoteIdent(column)
		if mergedFields != nil && isTextColumn(columns[column].dataType) {
			expression += ` COLLATE "C"`
		} else if integerArrayColumns[column] {
			expression = fmt.Sprintf("(SELECT AVG(element) FROM unnest(%s) AS element)", expression)
		}
		direction := "ASC NULLS LAST"
		if order.Type == types.SortDesc {
			direction = "DESC NULLS FIRST"
		}
		parts = append(parts, expression+" "+direction)
	}
	for _, key := range keys {
		parts = append(parts, quoteIdent(key))
	}
	return " ORDER BY " + strings.Join(parts, ", "), nil
}

// fullTextQuery strips the ~N/^N boost suffixes the query builder appends for
// ES; the @~@ operand carries its own PARAM:BOOST.
func fullTextQuery(text *types.MatchTextExpr) string {
	return boostSuffixRegex.ReplaceAllString(text.MatchingText, "")
}

// parseFullTextFields splits "name^weight" field specifications into field
// names and weight strings, skipping entries that carry no name.
func parseFullTextFields(text *types.MatchTextExpr) ([]string, []string) {
	if len(text.Fields) == 0 {
		return nil, nil
	}
	fields := make([]string, 0, len(text.Fields))
	weights := make([]string, 0, len(text.Fields))
	for _, specification := range text.Fields {
		match := fieldSpecRegex.FindStringSubmatch(specification)
		if match == nil || match[1] == "" {
			continue
		}
		weight := match[2]
		if weight == "" {
			weight = "1"
		}
		fields = append(fields, match[1])
		weights = append(weights, weight)
	}
	return fields, weights
}

// parseSearchPlan flattens the match expression list into the text, dense,
// and fusion expressions the SQL search paths consume.
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

func (plan searchPlan) topN() int {
	switch {
	case plan.fusion != nil && plan.fusion.TopN > 0:
		if plan.dense == nil && plan.text != nil && plan.text.TopN > 0 {
			return min(plan.text.TopN, plan.fusion.TopN)
		}
		return plan.fusion.TopN
	case plan.dense != nil:
		return plan.dense.TopN
	case plan.text != nil:
		return plan.text.TopN
	default:
		return 0
	}
}

// validateVectorExpr rejects empty, non-float, or malformed vector
// expressions before any SQL is built from them.
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

// denseSimilarity extracts the similarity threshold from the expression's
// extra options.
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

// OpenGauss recall weights are independent: "0.001,1" keeps both legs.
func fusionWeights(fusion *types.FusionExpr) (float64, float64) {
	if fusion == nil {
		return 0.5, 0.5
	}
	textPart, vectorPart, _ := strings.Cut(stringValue(fusion.FusionParams["weights"]), ",")
	vector, err := strconv.ParseFloat(strings.TrimSpace(vectorPart), 64)
	if err != nil {
		return 0.5, 0.5
	}
	text, err := strconv.ParseFloat(strings.TrimSpace(textPart), 64)
	if err != nil {
		return 0.5, 0.5
	}
	return text, vector
}

// sortScoredChunks orders scored chunks by their ranking key, descending.
func sortScoredChunks(chunks []map[string]interface{}) {
	sort.SliceStable(chunks, func(i, j int) bool {
		return scoredChunkSortKey(chunks[i]) > scoredChunkSortKey(chunks[j])
	})
}

// scoredChunkSortKey is a chunk's merged ranking key: _score plus
// pagerank_fea.
func scoredChunkSortKey(chunk map[string]interface{}) float64 {
	score, _ := common.ToFloat(chunk["_score"])
	pagerank, _ := common.ToFloat(chunk["pagerank_fea"])
	return score + pagerank
}

// decodeRows decodes each physical row into logical field names.
func (e *Engine) decodeRows(rows []map[string]interface{}, kind, preferredVector string) []map[string]interface{} {
	result := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		result[i] = e.decodeLogicalRow(row, kind, preferredVector)
	}
	return result
}

type orderedHit struct {
	row  map[string]any
	keys []any
}

type sortText string

// mergeSearchChunks sorts the merged multi-table result — by ranking key when
// scored, otherwise by the request's ORDER BY — then applies the page window.
func mergeSearchChunks(chunks []map[string]interface{}, hits []orderedHit, req *types.SearchRequest, scored bool) []map[string]interface{} {
	if scored {
		sortScoredChunks(chunks)
	} else if len(hits) > 0 {
		sort.SliceStable(hits, func(i, j int) bool {
			for k, order := range req.OrderBy.Fields {
				comparison := compareSearchValues(hits[i].keys[k], hits[j].keys[k], order.Field)
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
		for i := range hits {
			chunks[i] = hits[i].row
		}
	}
	offset := min(max(req.Offset, 0), len(chunks))
	limit := common.PositiveOr(req.Limit, 30)
	end := min(offset+limit, len(chunks))
	return chunks[offset:end]
}

// compareSearchValues orders two sort-field values, with nil sorting last;
// zero means equal or unordered.
func compareSearchValues(left, right interface{}, field string) int {
	if left == nil {
		if right == nil {
			return 0
		}
		return 1
	}
	if right == nil {
		return -1
	}
	if _, ok := left.(sortText); ok {
		return strings.Compare(fmt.Sprint(left), fmt.Sprint(right))
	}
	if _, ok := right.(sortText); ok {
		return strings.Compare(fmt.Sprint(left), fmt.Sprint(right))
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

// searchSortNumber coerces a sort value to a number, summing the elements of
// multi-value fields the way the Python engines sort them.
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
	if number, ok := common.ToFloat(value); ok {
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

// formatFloat renders a float as a minimal SQL literal.
func formatFloat(value float64) string { return strconv.FormatFloat(value, 'g', -1, 64) }

// searchHiddenSortFields lists ORDER BY fields missing from SelectFields, so
// merged SQL can still sort by them before the page window drops rows.
func searchHiddenSortFields(req *types.SearchRequest, mergeTables bool) []string {
	if !mergeTables || req.OrderBy == nil || len(req.OrderBy.Fields) == 0 || slices.Contains(req.SelectFields, "*") {
		return nil
	}
	fields := make([]string, 0, len(req.OrderBy.Fields))
	for _, order := range req.OrderBy.Fields {
		if order.Field == "_score" || slices.Contains(req.SelectFields, order.Field) || slices.Contains(fields, order.Field) {
			continue
		}
		fields = append(fields, order.Field)
	}
	return fields
}

func dataVecRank(kind string) string {
	if kind == "chunk" {
		return "(_score + COALESCE(pagerank_fea, 0))"
	}
	return "_score"
}

// The candidate CTE is shared by the page and total. The LEFT JOIN retains
// the total even when the requested offset is beyond the last candidate.
func (e *Engine) dataVecPage(ctx context.Context, kind, scan string, args []interface{}, offset, limit, topN int, bm25 bool) ([]map[string]interface{}, int64, error) {
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	if bm25 {
		if _, err = tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
			return nil, 0, err
		}
	}
	order := dataVecRank(kind) + " DESC, " + identifierColumn(kind)
	if kind == "chunk" {
		order += ", kb_id"
	}
	ranked := "SELECT * FROM (" + scan + ") AS hits ORDER BY " + order
	if topN > 0 {
		ranked += fmt.Sprintf(" LIMIT %d", topN)
	}
	query := fmt.Sprintf("WITH ranked AS (%s) SELECT page.*, totals.__total FROM (SELECT count(*) AS __total FROM ranked) totals LEFT JOIN (SELECT * FROM ranked ORDER BY %s LIMIT %d OFFSET %d) page ON TRUE ORDER BY %s", ranked, order, limit, offset, order)
	rows, err := queryRowsTx(ctx, tx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	if len(rows) == 0 {
		return nil, 0, fmt.Errorf("opengauss: page query returned no count")
	}
	total, ok := rows[0]["__total"].(int64)
	if !ok {
		return nil, 0, fmt.Errorf("opengauss: unexpected count type %T", rows[0]["__total"])
	}
	if rows[0][identifierColumn(kind)] == nil {
		return nil, total, nil
	}
	for _, row := range rows {
		delete(row, "__total")
	}
	return rows, total, nil
}
