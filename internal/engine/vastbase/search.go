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

package vastbase

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
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

// Search executes filter, full-text, vector, or hybrid search. Chunk and skill
// tables use the ParadeDB-style bm25 operator @~@ with one scan per column
// (UNION ALL, never OR — OR plans pick a BitmapOr node that nulls bm25_score)
// fused with a pure KNN scan whose ORDER BY stays bare so the graph_index
// stays eligible; memory tables use to_tsquery/ts_rank instead, since they
// carry no full-text index.
func (e *Engine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if req == nil || len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}
	types.LogSearchRequest("Vastbase", req)
	plan := parseSearchPlan(req.MatchExprs)
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
	scored := effectivePlan.text != nil || effectivePlan.dense != nil

	// Single-table searches page inside each SQL statement. Multi-table
	// searches window only once, on the merged set: every table returns the
	// whole candidate range (LIMIT offset+limit OFFSET 0) and
	// mergeSearchChunks applies req's page window after merging — a
	// per-table OFFSET would skip rows that belong in the global window.
	tableOffset := max(req.Offset, 0)
	tableLimit := positiveOr(req.Limit, 30)
	if mergeTables {
		tableLimit = tableOffset + tableLimit
		tableOffset = 0
	}

	result := &types.SearchResult{Chunks: []map[string]interface{}{}}
	for _, tableName := range tableNames {
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
		output := buildSearchOutput(req.SelectFields, kind, columns, scored, hiddenSortFields)
		if len(output) == 0 {
			continue
		}
		if kind == "memory" && effectivePlan.dense != nil {
			// Memory fusion computes the vector score on the CTE rows, so the
			// vector column must be selected even when not requested.
			column := effectivePlan.dense.VectorColumnName
			if _, exists := columns[column]; exists && !containsString(output, column) {
				output = append(output, column)
			}
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
		chunks, total, err := e.searchTable(ctx, tableName, kind, columns, output, condition, req, tableOffset, tableLimit, effectivePlan)
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
		result.Chunks = mergeSearchChunks(result.Chunks, req, scored)
		for _, chunk := range result.Chunks {
			for _, field := range hiddenSortFields {
				delete(chunk, field)
			}
		}
	} else if scored {
		// Python connector post-processing: every scored result is re-sorted by
		// score + pagerank (the ES rescore equivalent) and cut to the limit.
		sortScoredChunks(result.Chunks)
	}
	return result, nil
}

// searchTable runs one table's query. offset/limit form the per-table SQL
// window: req's page in single-table mode, the whole candidate range in merge
// mode, where mergeSearchChunks applies the page window after merging.
func (e *Engine) searchTable(ctx context.Context, tableName, kind string, columns map[string]columnMeta, output []string, condition map[string]interface{}, req *types.SearchRequest, offset, limit int, plan searchPlan) ([]map[string]interface{}, int64, error) {
	filterSQL, filterArgs, err := buildFilter(condition, kind, columns)
	if err != nil {
		return nil, 0, err
	}

	switch {
	case plan.text != nil && plan.dense != nil:
		if kind == "memory" {
			return e.searchMemoryFusion(ctx, tableName, kind, output, filterSQL, filterArgs, offset, limit, plan)
		}
		return e.searchFusion(ctx, tableName, kind, output, filterSQL, filterArgs, offset, limit, plan)
	case plan.dense != nil:
		return e.searchVector(ctx, tableName, kind, output, filterSQL, filterArgs, offset, limit, plan.dense)
	case plan.text != nil:
		if kind == "memory" {
			return e.searchMemoryFullText(ctx, tableName, kind, output, filterSQL, filterArgs, offset, limit, plan.text)
		}
		return e.searchFullText(ctx, tableName, kind, output, filterSQL, filterArgs, offset, limit, plan)
	default:
		return e.searchFilterOnly(ctx, tableName, kind, output, filterSQL, filterArgs, req, offset, limit)
	}
}

// buildSearchOutput projects requested fields onto the table's live columns,
// keeping only what exists (get_fields fills the gaps with nil on read),
// always selecting the row identifier, and adding pagerank_fea for scored
// chunk searches so the post-sort can fold it into the score.
func buildSearchOutput(selectFields []string, kind string, columns map[string]columnMeta, scored bool, hiddenSortFields []string) []string {
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
	if kind == "memory" {
		for i, field := range fields {
			if field == "content_embed" {
				fields[i] = memoryVectorColumn(columns)
				continue
			}
			fields[i] = mapMemoryField(field)
		}
	}
	fields = append(fields, "id")
	if scored && kind == "chunk" {
		fields = append(fields, "pagerank_fea")
	}
	fields = append(fields, hiddenSortFields...)
	output := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}
		if _, exists := columns[field]; exists && !containsString(output, field) {
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
	fulltextLimit := fullTextLimit(plan, limit)
	args := &sqlArgs{}
	cte, err := e.fullTextCTE(tableName, output, filterSQL, filterArgs, plan.text, fulltextLimit, args)
	if err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf("SELECT * FROM (%s) AS ranked ORDER BY _score DESC LIMIT %d OFFSET %d", cte, limit, offset)
	rows, err := e.queryRows(ctx, query, args.values...)
	if err != nil {
		return nil, 0, fmt.Errorf("vastbase: fulltext search %s: %w", tableName, err)
	}
	chunks := decodeRows(rows, kind)
	return chunks, int64(len(chunks)), nil
}

// fullTextCTE builds the per-column bm25 scan: a single field stays a plain
// WHERE predicate, multiple fields run one @~@ scan per column UNION ALL'd
// and deduplicated with DISTINCT ON so a row matching several fields is kept
// once with its best score.
func (e *Engine) fullTextCTE(tableName string, output []string, filterSQL string, filterArgs []interface{}, text *types.MatchTextExpr, fulltextLimit int, args *sqlArgs) (string, error) {
	fields, weights := parseFullTextFields(text)
	if len(fields) == 0 {
		return "", fmt.Errorf("vastbase: fulltext expression carries no fields")
	}
	fieldsExpr := selectFieldsSQL(output)
	query := fullTextQuery(text)
	minimumShouldMatch := formatMinimumShouldMatch(text)

	if len(fields) == 1 {
		filterPart := args.embedFilter(filterSQL, filterArgs)
		operand := fmt.Sprintf("%s @<PARAM:MINIMUM_SHOULD_MATCH=%s PARAM:BOOST=%s>@", query, minimumShouldMatch, weights[0])
		operandPlaceholder := args.add(operand)
		return fmt.Sprintf("SELECT %s, bm25_score AS _score FROM (SELECT %s, bm25_score() AS bm25_score FROM %s WHERE (%s) AND (%s @~@ $%d) ORDER BY bm25_score DESC LIMIT %d) AS ranked",
			fieldsExpr, fieldsExpr, quoteIdent(tableName), filterPart, quoteIdent(fields[0]), operandPlaceholder, fulltextLimit), nil
	}

	perColumnLimit := max(fulltextLimit*2, 1)
	branches := make([]string, 0, len(fields))
	for i, field := range fields {
		filterPart := args.embedFilter(filterSQL, filterArgs)
		operand := fmt.Sprintf("%s @<PARAM:MINIMUM_SHOULD_MATCH=%s PARAM:BOOST=%s>@", query, minimumShouldMatch, weights[i])
		operandPlaceholder := args.add(operand)
		branches = append(branches, fmt.Sprintf("(SELECT %s, bm25_score() AS bm25_score FROM %s WHERE (%s) AND (%s @~@ $%d) ORDER BY bm25_score DESC LIMIT %d)",
			fieldsExpr, quoteIdent(tableName), filterPart, quoteIdent(field), operandPlaceholder, perColumnLimit))
	}
	return fmt.Sprintf(`SELECT %s, "SCORE" AS _score FROM (SELECT DISTINCT ON (id) %s, bm25_score AS "SCORE" FROM (%s) AS unioned ORDER BY id, "SCORE" DESC) AS deduped ORDER BY _score DESC LIMIT %d`,
		fieldsExpr, fieldsExpr, strings.Join(branches, " UNION ALL "), fulltextLimit), nil
}

// fullTextLimit scales the bm25 recall up to the fusion topn: the text expr's
// own topn (100, fixed by FulltextQueryer) starves the weighted sum when the
// request topk is large.
func fullTextLimit(plan searchPlan, limit int) int {
	fulltextLimit := positiveOr(plan.text.TopN, limit)
	if plan.fusion != nil {
		fulltextLimit = max(fulltextLimit, positiveOr(plan.fusion.TopN, limit))
	}
	return fulltextLimit
}

// searchVector runs the dense path over the expression's q_N_vec column,
// ordered by distance-derived similarity.
func (e *Engine) searchVector(ctx context.Context, tableName, kind string, output []string, filterSQL string, filterArgs []interface{}, offset, limit int, dense *types.MatchDenseExpr) ([]map[string]interface{}, int64, error) {
	if err := validateVectorExpr(dense); err != nil {
		return nil, 0, err
	}
	vector, err := encodeVector(dense.EmbeddingData)
	if err != nil {
		return nil, 0, err
	}
	args := &sqlArgs{}
	cte, err := e.vectorCTE(tableName, output, filterSQL, filterArgs, dense, positiveOr(dense.TopN, limit), denseSimilarity(dense), vector, args)
	if err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf("SELECT * FROM (%s) AS candidates ORDER BY _score DESC LIMIT %d OFFSET %d", cte, limit, offset)
	rows, err := e.queryRows(ctx, query, args.values...)
	if err != nil {
		return nil, 0, fmt.Errorf("vastbase: vector search %s: %w", tableName, err)
	}
	chunks := decodeRows(rows, kind)
	return chunks, int64(len(chunks)), nil
}

// vectorCTE builds the pure KNN scan. The distance expression never enters an
// inner WHERE and the inner ORDER BY is the bare column: either would make the
// planner fall back to a sequential scan and bypass the graph_index (HNSW).
// The similarity threshold is applied outside the KNN scan.
func (e *Engine) vectorCTE(tableName string, output []string, filterSQL string, filterArgs []interface{}, dense *types.MatchDenseExpr, topn int, threshold float64, vector string, args *sqlArgs) (string, error) {
	if err := validateVectorExpr(dense); err != nil {
		return "", err
	}
	filterPart := args.embedFilter(filterSQL, filterArgs)
	first := args.add(vector)
	second := args.add(vector)
	column := quoteIdent(dense.VectorColumnName)
	scoreExpr := fmt.Sprintf("(1 - (%s %s $%d))", column, e.distanceOp(), first)
	inner := fmt.Sprintf("SELECT %s, %s AS _score FROM %s WHERE %s ORDER BY %s %s $%d LIMIT %d",
		selectFieldsSQL(output), scoreExpr, quoteIdent(tableName), filterPart, column, e.distanceOp(), second, topn)
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
	fulltextSQL, err := e.fullTextCTE(tableName, output, filterSQL, filterArgs, plan.text, fullTextLimit(plan, limit), args)
	if err != nil {
		return nil, 0, err
	}
	vectorSQL, err := e.vectorCTE(tableName, output, filterSQL, filterArgs, plan.dense, positiveOr(plan.dense.TopN, limit), denseSimilarity(plan.dense), vector, args)
	if err != nil {
		return nil, 0, err
	}

	// Normalize the bm25 score by this table's maximum to [0,1] before the
	// weighted sum: raw bm25 fused directly with the [0,1] cosine similarity
	// lets any fulltext hit outrank every vector-only row. The divisor falls
	// back to 1 when fulltext is empty or all-zero so vector-only rows keep a
	// numeric score.
	vectorWeight := fusionVectorWeight(plan.fusion)
	textWeight := 1 - vectorWeight
	fused := fmt.Sprintf("(COALESCE(a._score, 0) / COALESCE(NULLIF((SELECT MAX(_score) FROM filter_fulltext), 0), 1) * %s + COALESCE(b._score, 0) * %s)",
		formatFloat(textWeight), formatFloat(vectorWeight))
	projection := make([]string, 0, len(output)+1)
	for _, field := range output {
		projection = append(projection, fmt.Sprintf("COALESCE(a.%s, b.%s) AS %s", quoteIdent(field), quoteIdent(field), quoteIdent(field)))
	}
	projection = append(projection, fused+" AS _score")
	query := fmt.Sprintf("WITH filter_fulltext AS (%s), filter_vector AS (%s) SELECT %s FROM filter_fulltext a FULL OUTER JOIN filter_vector b ON a.id = b.id ORDER BY _score DESC LIMIT %d OFFSET %d",
		fulltextSQL, vectorSQL, strings.Join(projection, ", "), minPositive(limit, plan.fusion.TopN), offset)
	rows, err := e.queryRows(ctx, query, args.values...)
	if err != nil {
		return nil, 0, fmt.Errorf("vastbase: fusion search %s: %w", tableName, err)
	}
	chunks := decodeRows(rows, kind)
	return chunks, int64(len(chunks)), nil
}

// searchMemoryFullText matches memory content with the built-in tsvector
// machinery ('simple' dictionary), the path memory tables use since they carry
// no full-text index; ts_rank is naturally ~[0,1].
func (e *Engine) searchMemoryFullText(ctx context.Context, tableName, kind string, output []string, filterSQL string, filterArgs []interface{}, offset, limit int, text *types.MatchTextExpr) ([]map[string]interface{}, int64, error) {
	tsquery := memoryTSQuery(text)
	if tsquery == "" {
		return []map[string]interface{}{}, 0, nil
	}
	args := &sqlArgs{}
	filterPart := args.embedFilter(filterSQL, filterArgs)
	match := fmt.Sprintf("to_tsvector('simple', COALESCE(content_ltks, '')) @@ to_tsquery('simple', $%d)", args.add(tsquery))
	score := fmt.Sprintf("ts_rank(to_tsvector('simple', COALESCE(content_ltks, '')), to_tsquery('simple', $%d))", args.add(tsquery))
	query := fmt.Sprintf("SELECT %s, %s AS _score FROM %s WHERE %s AND %s ORDER BY _score DESC LIMIT %d OFFSET %d",
		selectFieldsSQL(output), score, quoteIdent(tableName), filterPart, match, limit, offset)
	rows, err := e.queryRows(ctx, query, args.values...)
	if err != nil {
		return nil, 0, fmt.Errorf("vastbase: memory fulltext search %s: %w", tableName, err)
	}
	chunks := decodeRows(rows, kind)
	return chunks, int64(len(chunks)), nil
}

// searchMemoryFusion computes both signals on the same candidate rows (the
// Python memory connector contract): text-matching rows are scored with
// ts_rank * textWeight + similarity * vectorWeight, then filtered by the
// vector threshold. No bm25 normalization — ts_rank is already ~[0,1].
func (e *Engine) searchMemoryFusion(ctx context.Context, tableName, kind string, output []string, filterSQL string, filterArgs []interface{}, offset, limit int, plan searchPlan) ([]map[string]interface{}, int64, error) {
	if err := validateVectorExpr(plan.dense); err != nil {
		return nil, 0, err
	}
	tsquery := memoryTSQuery(plan.text)
	if tsquery == "" {
		return []map[string]interface{}{}, 0, nil
	}
	vector, err := encodeVector(plan.dense.EmbeddingData)
	if err != nil {
		return nil, 0, err
	}
	vectorWeight := fusionVectorWeight(plan.fusion)
	textWeight := 1 - vectorWeight
	threshold := denseSimilarity(plan.dense)
	column := quoteIdent(plan.dense.VectorColumnName)
	numCandidates := positiveOr(plan.dense.TopN, limit) + positiveOr(plan.text.TopN, limit)

	args := &sqlArgs{}
	filterPart := args.embedFilter(filterSQL, filterArgs)
	match := fmt.Sprintf("to_tsvector('simple', COALESCE(content_ltks, '')) @@ to_tsquery('simple', $%d)", args.add(tsquery))
	score := fmt.Sprintf("(ts_rank(to_tsvector('simple', COALESCE(content_ltks, '')), to_tsquery('simple', $%d)) * %s + (1 - (%s %s $%d)) * %s)",
		args.add(tsquery), formatFloat(textWeight), column, e.distanceOp(), args.add(vector), formatFloat(vectorWeight))
	inner := fmt.Sprintf("SELECT %s, %s AS _score FROM %s WHERE %s AND %s LIMIT %d",
		selectFieldsSQL(output), score, quoteIdent(tableName), filterPart, match, numCandidates)
	outerVector := fmt.Sprintf("(1 - (%s %s $%d))", column, e.distanceOp(), args.add(vector))
	thresholdPlaceholder := args.add(threshold)
	query := fmt.Sprintf("SELECT * FROM (%s) AS candidates WHERE %s >= $%d ORDER BY _score DESC LIMIT %d OFFSET %d",
		inner, outerVector, thresholdPlaceholder, limit, offset)
	rows, err := e.queryRows(ctx, query, args.values...)
	if err != nil {
		return nil, 0, fmt.Errorf("vastbase: memory fusion search %s: %w", tableName, err)
	}
	chunks := decodeRows(rows, kind)
	return chunks, int64(len(chunks)), nil
}

// searchFilterOnly serves requests with no text or vector expression: one
// COUNT for the total plus one filtered, ordered select for the page.
func (e *Engine) searchFilterOnly(ctx context.Context, tableName, kind string, output []string, filterSQL string, filterArgs []interface{}, req *types.SearchRequest, offset, limit int) ([]map[string]interface{}, int64, error) {
	identifier := quoteIdent(identifierColumn(kind))
	var count int64
	if err := e.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(%s) FROM %s WHERE %s", identifier, quoteIdent(tableName), filterSQL),
		filterArgs...).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("vastbase: count %s: %w", tableName, err)
	}
	if count == 0 {
		return []map[string]interface{}{}, 0, nil
	}
	columns, err := e.listTableColumns(ctx, tableName)
	if err != nil {
		return nil, 0, err
	}
	orderSQL, err := buildOrderBy(req.OrderBy, kind, columns)
	if err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s%s LIMIT %d OFFSET %d",
		selectFieldsSQL(output), quoteIdent(tableName), filterSQL, orderSQL, limit, offset)
	rows, err := e.queryRows(ctx, query, filterArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("vastbase: filter search %s: %w", tableName, err)
	}
	return decodeRows(rows, kind), count, nil
}

// buildSearchOutput helper: memory content_embed maps to whatever vector
// column the table actually carries.
func memoryVectorColumn(columns map[string]columnMeta) string {
	best := ""
	for column := range columns {
		if vectorDimRegex.MatchString(column) && (best == "" || column < best) {
			best = column
		}
	}
	return best
}

// buildOrderBy renders the ORDER BY clause, mapping memory field names to
// columns and rejecting sort fields the table does not carry.
func buildOrderBy(orderBy *types.OrderByExpr, kind string, columns map[string]columnMeta) (string, error) {
	if orderBy == nil || len(orderBy.Fields) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(orderBy.Fields))
	for _, order := range orderBy.Fields {
		column := order.Field
		if kind == "memory" {
			column = mapMemoryField(column)
		}
		if _, exists := columns[column]; !exists {
			return "", fmt.Errorf("vastbase: unknown order field: %s", order.Field)
		}
		expression := quoteIdent(column)
		if integerArrayColumns[column] {
			expression = fmt.Sprintf("(SELECT AVG(element) FROM unnest(%s) AS element)", expression)
		}
		direction := "ASC"
		if order.Type == types.SortDesc {
			direction = "DESC"
		}
		parts = append(parts, expression+" "+direction)
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

// formatMinimumShouldMatch renders the minimum_should_match option in the
// percent form the SQL full-text rank formula expects.
func formatMinimumShouldMatch(text *types.MatchTextExpr) string {
	if text.ExtraOptions == nil {
		return "0%"
	}
	switch value := text.ExtraOptions["minimum_should_match"].(type) {
	case float64:
		return common.FormatMinimumShouldMatchPercent(value)
	case float32:
		return common.FormatMinimumShouldMatchPercent(float64(value))
	case int:
		return common.FormatMinimumShouldMatchPercent(float64(value))
	case string:
		if value != "" {
			return value
		}
	}
	return "0%"
}

// memoryTSQuery builds the 'simple' to_tsquery operand: the original query
// words AND-joined, with tsquery operator characters stripped per word.
func memoryTSQuery(text *types.MatchTextExpr) string {
	query := ""
	if text.ExtraOptions != nil {
		query = strings.TrimSpace(stringValue(text.ExtraOptions["original_query"]))
	}
	if query == "" {
		query = boostSuffixRegex.ReplaceAllString(text.MatchingText, "")
	}
	words := strings.FieldsFunc(query, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
	operands := make([]string, 0, len(words))
	for _, word := range words {
		word = strings.Map(func(r rune) rune {
			switch r {
			case '&', '|', '!', '(', ')', ':', '*', '\\', '\'', '"':
				return -1
			}
			return r
		}, word)
		if word != "" {
			operands = append(operands, word)
		}
	}
	return strings.Join(operands, " & ")
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

// validateVectorExpr rejects empty, non-float, or malformed vector
// expressions before any SQL is built from them.
func validateVectorExpr(dense *types.MatchDenseExpr) error {
	if dense == nil || len(dense.EmbeddingData) == 0 {
		return fmt.Errorf("vector expression is empty")
	}
	if dense.EmbeddingDataType != "" && dense.EmbeddingDataType != "float" {
		return fmt.Errorf("embedding data type %q is not float", dense.EmbeddingDataType)
	}
	if !vectorDimRegex.MatchString(dense.VectorColumnName) {
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

// fusionVectorWeight reads the vector side of a "text,vector" fusion weight
// pair, defaulting to an even split on any malformed input.
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

// sortScoredChunks orders scored chunks by their ranking key, descending.
func sortScoredChunks(chunks []map[string]interface{}) {
	sort.SliceStable(chunks, func(i, j int) bool {
		return scoredChunkSortKey(chunks[i]) > scoredChunkSortKey(chunks[j])
	})
}

// scoredChunkSortKey is a chunk's merged ranking key: _score plus
// pagerank_fea.
func scoredChunkSortKey(chunk map[string]interface{}) float64 {
	score, _ := numberToFloat(chunk["_score"])
	pagerank, _ := numberToFloat(chunk["pagerank_fea"])
	return score + pagerank
}

// decodeRows decodes each physical row into logical field names.
func decodeRows(rows []map[string]interface{}, kind string) []map[string]interface{} {
	result := make([]map[string]interface{}, len(rows))
	for i, row := range rows {
		result[i] = decodeLogicalRow(row, kind)
	}
	return result
}

// mergeSearchChunks sorts the merged multi-table result — by ranking key when
// scored, otherwise by the request's ORDER BY — then applies the page window.
func mergeSearchChunks(chunks []map[string]interface{}, req *types.SearchRequest, scored bool) []map[string]interface{} {
	if scored {
		sortScoredChunks(chunks)
	} else if req.OrderBy != nil && len(req.OrderBy.Fields) > 0 {
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
	}
	offset := min(max(req.Offset, 0), len(chunks))
	limit := positiveOr(req.Limit, 30)
	end := min(offset+limit, len(chunks))
	return chunks[offset:end]
}

// compareSearchValues orders two sort-field values, with nil sorting first;
// zero means equal or unordered.
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

// formatFloat renders a float as a minimal SQL literal.
func formatFloat(value float64) string { return strconv.FormatFloat(value, 'g', -1, 64) }

// minPositive returns the smaller positive value, falling back to the other
// one when either is non-positive.
func minPositive(first, second int) int {
	if first <= 0 {
		return second
	}
	if second <= 0 || first < second {
		return first
	}
	return second
}

// positiveOr returns value when positive, otherwise fallback.
func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

// uniqueStrings deduplicates while preserving first-seen order.
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

// globalSearchCandidateLimit is the candidate range a merged search fetches
// from every table: the page offset plus limit (default page size 30).
func globalSearchCandidateLimit(req *types.SearchRequest) int {
	return max(req.Offset, 0) + positiveOr(req.Limit, 30)
}

// expandSearchPlan raises the text and dense TopNs to at least the candidate
// limit so merged searches do not under-fetch from any table.
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

// searchHiddenSortFields lists ORDER BY fields missing from SelectFields, so
// merged SQL can still sort by them before the page window drops rows.
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
