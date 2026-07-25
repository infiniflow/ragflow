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

package serenedb

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/engine/types"
)

const (
	defaultPageSize   = 30
	defaultBranchTopN = 200
)

// pagerankExpr folds the dataset pagerank into a scored search, matching the ES
// fusion math (score + pagerank_fea/100).
const pagerankExpr = "COALESCE(" + pagerankField + ", 0) / 100.0"

type parsedMatch struct {
	textQuery    string
	textTopN     int
	vectorData   []float64
	vectorTopN   int
	vecThreshold float64
	vectorWeight float64
	hasText      bool
	hasVector    bool
}

// Search runs the fulltext, vector, hybrid-fusion, or filter-only query implied
// by the request's match expressions against the tenant table(s), scoping to
// req.KbIDs with a kb_id filter. Because all datasets share one table, BM25 is
// scored over the whole tenant corpus.
func (e *serenedbEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	types.LogSearchRequest("serenedb", req)

	pm := parseMatchExprs(req.MatchExprs)
	outputFields := resolveOutputFields(req.SelectFields)
	fieldsExpr := strings.Join(outputFields, ", ")

	filters := searchFilters(req.Filter, req.KbIDs, pm.hasText || pm.hasVector)
	where := filtersExpr(filters)

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultPageSize
	}
	scored := pm.hasText || pm.hasVector

	result := &types.SearchResult{Chunks: []map[string]interface{}{}}
	for _, tableName := range req.IndexNames {
		exists, err := e.tableExists(ctx, tableName)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		var query string
		switch {
		case pm.hasText && pm.hasVector:
			query = buildFusionSQL(tableName, fieldsExpr, outputFields, where, pm, offset, limit)
		case pm.hasText:
			query = buildFulltextSQL(tableName, fieldsExpr, where, pm.textQuery, branchLimit(pm.textTopN, limit), offset)
		case pm.hasVector:
			query = buildVectorSQL(tableName, fieldsExpr, where, pm, branchLimit(pm.vectorTopN, limit), offset)
		default:
			total, err := e.countRows(ctx, tableName, where)
			if err != nil {
				return nil, err
			}
			result.Total += total
			query = buildFilterSQL(tableName, fieldsExpr, where, req.OrderBy, limit, offset)
		}
		rows, err := e.queryMaps(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("serenedb: search %s: %w", tableName, err)
		}
		result.Chunks = append(result.Chunks, rows...)
	}

	if scored && len(result.Chunks) > 1 {
		sortByScore(result.Chunks)
	}
	if limit > 0 && len(result.Chunks) > limit {
		result.Chunks = result.Chunks[:limit]
	}
	if result.Total == 0 {
		result.Total = int64(len(result.Chunks))
	}
	return result, nil
}

// resolveOutputFields keeps id and pagerank in the projection and drops the
// synthetic _score and any unknown fields.
func resolveOutputFields(selectFields []string) []string {
	var fields []string
	seen := map[string]struct{}{}
	add := func(f string) {
		if _, ok := seen[f]; ok {
			return
		}
		seen[f] = struct{}{}
		fields = append(fields, f)
	}
	add("id")
	src := selectFields
	useAll := len(src) == 0
	for _, f := range src {
		if f == "*" {
			useAll = true
		}
	}
	if useAll {
		for _, c := range columnOrder {
			add(c)
		}
	} else {
		for _, f := range src {
			if f == "_score" || f == "*" {
				continue
			}
			if isKnownColumn(f) {
				add(f)
			}
		}
	}
	add(pagerankField)
	return fields
}

// searchFilters builds the SQL predicates for a search. kb_id scopes the query
// to the requested datasets within the shared tenant table, and scored queries
// default to available_int=1 when the caller did not set it.
func searchFilters(filter map[string]interface{}, kbIDs []string, scored bool) []string {
	cond := map[string]interface{}{}
	for k, v := range filter {
		cond[k] = v
	}
	if kbs := stringSlice(kbIDs); len(kbs) > 0 {
		cond["kb_id"] = kbs
	}
	if scored {
		_, hasAvail := cond["available_int"]
		_, hasStatus := cond["status"]
		if !hasAvail && !hasStatus {
			cond["available_int"] = 1
		}
	}
	return buildFilters(cond)
}

// branchLimit is the row cap for a single-mode query.
func branchLimit(topN, limit int) int {
	if limit > 0 {
		return limit
	}
	if topN > 0 {
		return topN
	}
	return defaultPageSize
}

// buildFulltextSQL scores the single lexical column with BM25 plus pagerank.
func buildFulltextSQL(tableName, fieldsExpr, where, textQuery string, limit, offset int) string {
	idx := indexRelation(tableName)
	match := fmt.Sprintf("%s @@ %s", lexScoredCol, escapeLiteral(textQuery))
	return fmt.Sprintf(
		"SELECT %s, BM25(%s.tableoid) + %s AS _score FROM %s WHERE %s AND (%s) "+
			"ORDER BY _score DESC LIMIT %d OFFSET %d",
		fieldsExpr, idx, pagerankExpr, idx, where, match, limit, offset)
}

// buildVectorSQL runs the ANN scan on the normalized shadow column. The
// similarity threshold goes straight in the WHERE (relies on SereneDB 26.07.4).
func buildVectorSQL(tableName, fieldsExpr, where string, pm parsedMatch, limit, offset int) string {
	idx := indexRelation(tableName)
	vecN := normColumn(len(pm.vectorData))
	qv := vectorLiteral(pm.vectorData)
	sim := fmt.Sprintf("-(%s <#> %s)", vecN, qv)
	return fmt.Sprintf(
		"SELECT %s, %s + %s AS _score FROM %s WHERE %s AND %s >= %s "+
			"ORDER BY %s <#> %s LIMIT %d OFFSET %d",
		fieldsExpr, sim, pagerankExpr, idx, where, sim, formatFloat(pm.vecThreshold),
		vecN, qv, limit, offset)
}

// buildFusionSQL is the one-statement hybrid over a single tenant table: the
// BM25 branch normalized against the whole-table max with a window function,
// FULL OUTER JOINed with the ANN branch, weighted-summed with pagerank. Because
// the table holds the whole tenant corpus, the BM25 normalization is global.
func buildFusionSQL(tableName, fieldsExpr string, outputFields []string, where string, pm parsedMatch, offset, limit int) string {
	idx := indexRelation(tableName)
	vecN := normColumn(len(pm.vectorData))
	qv := vectorLiteral(pm.vectorData)
	match := fmt.Sprintf("%s @@ %s", lexScoredCol, escapeLiteral(pm.textQuery))
	lexN := pm.textTopN
	if lexN <= 0 {
		lexN = defaultBranchTopN
	}
	vN := pm.vectorTopN
	if vN <= 0 {
		vN = defaultBranchTopN
	}
	n := limit
	if n <= 0 {
		n = lexN + vN
	}
	prefixed := make([]string, len(outputFields))
	for i, f := range outputFields {
		prefixed[i] = "t." + f
	}
	vw := pm.vectorWeight
	return fmt.Sprintf(`WITH lex AS (
    SELECT id, BM25(%s.tableoid) AS s
    FROM %s WHERE %s AND (%s)
    ORDER BY s DESC LIMIT %d),
lexn AS (SELECT id, s / NULLIF(MAX(s) OVER (), 0) AS sn FROM lex),
vec AS (
    SELECT id, -(%s <#> %s) AS sim
    FROM %s WHERE %s AND -(%s <#> %s) >= %s
    ORDER BY %s <#> %s LIMIT %d),
fused AS (
    SELECT COALESCE(l.id, v.id) AS id,
           COALESCE(l.sn, 0) * %s + COALESCE(v.sim, 0) * %s AS fs
    FROM lexn l FULL OUTER JOIN vec v ON l.id = v.id)
SELECT %s, f.fs + COALESCE(t.%s, 0) / 100.0 AS _score
FROM fused f JOIN %s t ON t.id = f.id
ORDER BY _score DESC LIMIT %d OFFSET %d`,
		idx, idx, where, match, lexN,
		vecN, qv, idx, where, vecN, qv, formatFloat(pm.vecThreshold), vecN, qv, vN,
		formatWeight(1.0-vw), formatWeight(vw),
		strings.Join(prefixed, ", "), pagerankField, tableName, n, offset)
}

// buildFilterSQL is the metadata/browse path: no scoring, optional ordering.
func buildFilterSQL(tableName, fieldsExpr, where string, orderBy *types.OrderByExpr, limit, offset int) string {
	var order string
	if orderBy != nil && len(orderBy.Fields) > 0 {
		var parts []string
		for _, f := range orderBy.Fields {
			if _, known := columnDDL[f.Field]; !known {
				continue
			}
			dir := "ASC"
			if f.Type == types.SortDesc {
				dir = "DESC"
			}
			parts = append(parts, fmt.Sprintf("%s %s", f.Field, dir))
		}
		if len(parts) > 0 {
			order = " ORDER BY " + strings.Join(parts, ", ")
		}
	}
	return fmt.Sprintf("SELECT %s FROM %s WHERE %s%s LIMIT %d OFFSET %d",
		fieldsExpr, tableName, where, order, limit, offset)
}

func (e *serenedbEngine) countRows(ctx context.Context, tableName, where string) (int64, error) {
	rows, err := e.queryMaps(ctx, fmt.Sprintf("SELECT count(*) AS c FROM %s WHERE %s", tableName, where))
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return toInt64(rows[0]["c"]), nil
}

// parseMatchExprs extracts the text query, dense vector, and fusion weight from
// the ordered match expressions.
func parseMatchExprs(exprs []interface{}) parsedMatch {
	pm := parsedMatch{vectorWeight: 0.5}
	for _, m := range exprs {
		switch expr := m.(type) {
		case string:
			if expr != "" {
				pm.textQuery = stripESQuery(expr)
				pm.hasText = true
			}
		case *types.MatchTextExpr:
			raw := expr.MatchingText
			if raw == "" && expr.ExtraOptions != nil {
				if oq, ok := expr.ExtraOptions["original_query"].(string); ok {
					raw = oq
				}
			}
			if raw != "" {
				pm.textQuery = stripESQuery(raw)
				pm.textTopN = expr.TopN
				pm.hasText = true
			}
		case *types.MatchDenseExpr:
			if len(expr.EmbeddingData) > 0 {
				pm.vectorData = expr.EmbeddingData
				pm.vectorTopN = expr.TopN
				pm.vecThreshold = denseThreshold(expr.ExtraOptions)
				pm.hasVector = true
			}
		case *types.FusionExpr:
			if w, ok := fusionVectorWeight(expr.FusionParams); ok {
				pm.vectorWeight = w
			}
		}
	}
	return pm
}

func denseThreshold(opts map[string]interface{}) float64 {
	if opts == nil {
		return 0.0
	}
	switch v := opts["similarity"].(type) {
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	if s, ok := opts["threshold"].(string); ok {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	return 0.0
}

// fusionVectorWeight reads the vector weight (second element of the weights
// pair) from the fusion params.
func fusionVectorWeight(params map[string]interface{}) (float64, bool) {
	if params == nil {
		return 0, false
	}
	w, ok := params["weights"].(string)
	if !ok {
		return 0, false
	}
	parts := strings.Split(w, ",")
	if len(parts) < 2 {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// formatWeight renders a fusion weight without float-subtraction noise
// (e.g. 1.0 - 0.95 renders as 0.05, not 0.050000000000000044). Weights are
// low-precision by nature, so rounding to 1e-6 is exact enough.
func formatWeight(f float64) string {
	return strconv.FormatFloat(math.Round(f*1e6)/1e6, 'f', -1, 64)
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	}
	return 0
}

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}

func sortByScore(chunks []map[string]interface{}) {
	sort.SliceStable(chunks, func(i, j int) bool {
		return toFloat64(chunks[i]["_score"]) > toFloat64(chunks[j]["_score"])
	})
}

// GetChunkIDs returns the ids of the given chunks in order.
func (e *serenedbEngine) GetChunkIDs(chunks []map[string]interface{}) []string {
	ids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if id, ok := c["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// GetScores maps chunk id to its recovered score, reading the structure
// KNNScores produces.
func (e *serenedbEngine) GetScores(searchResult map[string]interface{}) map[string]float64 {
	scores := map[string]float64{}
	hits, ok := searchResult["hits"].(map[string]interface{})
	if !ok {
		return scores
	}
	hitList, ok := hits["hits"].([]interface{})
	if !ok {
		return scores
	}
	for _, h := range hitList {
		hit, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		id, ok := hit["_id"].(string)
		if !ok {
			continue
		}
		scores[id] = toFloat64(hit["_score"])
	}
	return scores
}

// KNNScores repackages the per-chunk _score into the hits structure GetScores
// consumes.
func (e *serenedbEngine) KNNScores(ctx context.Context, chunks []map[string]interface{}, queryVector []float64, topK int) (map[string]interface{}, error) {
	if len(chunks) == 0 {
		return nil, nil
	}
	hits := make([]interface{}, 0, len(chunks))
	for _, c := range chunks {
		id, _ := c["id"].(string)
		hits = append(hits, map[string]interface{}{"_id": id, "_score": toFloat64(c["_score"])})
	}
	return map[string]interface{}{"hits": map[string]interface{}{"hits": hits}}, nil
}

// GetFields returns the requested fields per chunk id, omitting nil values.
func (e *serenedbEngine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	out := map[string]map[string]interface{}{}
	if len(chunks) == 0 || len(fields) == 0 {
		return out
	}
	for _, c := range chunks {
		id, ok := c["id"].(string)
		if !ok {
			continue
		}
		row := map[string]interface{}{}
		for _, f := range fields {
			if v, ok := c[f]; ok && v != nil {
				row[f] = v
			}
		}
		out[id] = row
	}
	return out
}

// GetAggregation counts distinct values of a field across chunks, ordered by
// count descending.
func (e *serenedbEngine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	counts := map[string]int{}
	for _, c := range chunks {
		// Aggregation-style chunks carry an explicit value/count.
		if val, ok := c["value"]; ok {
			if s, ok := val.(string); ok {
				counts[s] += toInt(c["count"])
				continue
			}
		}
		v, ok := c[fieldName]
		if !ok {
			continue
		}
		for _, item := range asList(v) {
			if s, ok := item.(string); ok {
				if strings.TrimSpace(s) != "" {
					counts[s]++
				}
			}
		}
	}
	out := make([]map[string]interface{}, 0, len(counts))
	for k, n := range counts {
		out = append(out, map[string]interface{}{"key": k, "count": n})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i]["count"].(int) > out[j]["count"].(int)
	})
	return out
}

var nonWordBoundary = regexp.MustCompile(`</em>\s*<em>`)

// GetHighlight emphasizes keyword hits in the stored text, client-side.
func (e *serenedbEngine) GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string {
	ans := map[string]string{}
	if len(chunks) == 0 || len(keywords) == 0 {
		return ans
	}
	var pats []*regexp.Regexp
	for _, k := range keywords {
		if k == "" {
			continue
		}
		pats = append(pats, regexp.MustCompile(`(?i)(^|\W)(`+regexp.QuoteMeta(k)+`)(\W|$)`))
	}
	for _, c := range chunks {
		id, ok := c["id"].(string)
		if !ok {
			continue
		}
		txt, ok := c[fieldName].(string)
		if !ok || txt == "" {
			continue
		}
		marked := txt
		for _, p := range pats {
			marked = p.ReplaceAllString(marked, "$1<em>$2</em>$3")
		}
		if strings.Contains(marked, "<em>") {
			ans[id] = nonWordBoundary.ReplaceAllString(marked, " ")
		}
	}
	return ans
}

func asList(v interface{}) []interface{} {
	switch val := v.(type) {
	case []interface{}:
		return val
	case []string:
		out := make([]interface{}, len(val))
		for i, s := range val {
			out[i] = s
		}
		return out
	default:
		return []interface{}{v}
	}
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}
