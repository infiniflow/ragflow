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
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/common"

	"github.com/lib/pq"
	"go.uber.org/zap"
)

const defaultVectorSize = 1024

// chunkTableDDL returns the statements that create a chunk table, its scored
// dictionary, and the hybrid inverted index (text columns + normalized vector
// shadow). Pure so it can be asserted in tests.
func chunkTableDDL(tableName string, vectorSize int) []string {
	if vectorSize <= 0 {
		vectorSize = defaultVectorSize
	}
	cols := make([]string, 0, len(columnOrder)+2)
	for _, c := range columnOrder {
		cols = append(cols, fmt.Sprintf("%s %s", c, columnDDL[c]))
	}
	vec, vecN := rawVectorColumn(vectorSize), normColumn(vectorSize)
	cols = append(cols,
		fmt.Sprintf("%s FLOAT[%d]", vec, vectorSize),
		fmt.Sprintf("%s FLOAT[%d]", vecN, vectorSize))

	fts := make([]string, 0, len(ftsColumns))
	for _, c := range ftsColumns {
		fts = append(fts, fmt.Sprintf("%s %s", c, dictionaryName))
	}
	return []string{
		fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", tableName, strings.Join(cols, ", ")),
		dictionaryDDL,
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s "+
			"USING inverted (id, %s, %s ivf (metric = 'ip', quant = 'sq8')) "+
			"WITH (optimize_top_k = 'bm25(1.2, 0.75)')",
			indexRelation(tableName), tableName, strings.Join(fts, ", "), vecN),
	}
}

// CreateChunkStore ensures the tenant chunk table and its indexes exist. All
// datasets in the tenant share this table, so it is created once and is a
// no-op for later datasets.
func (e *serenedbEngine) CreateChunkStore(ctx context.Context, baseName, datasetID string, vectorSize int, parserID string) error {
	tableName := chunkTableName(baseName)
	for _, stmt := range chunkTableDDL(tableName, vectorSize) {
		if err := e.exec(ctx, stmt); err != nil {
			return fmt.Errorf("serenedb: create chunk store %s: %w", tableName, err)
		}
	}
	common.Info("SereneDB created chunk store", zap.String("table", tableName))
	return nil
}

// DropChunkStore removes a dataset from the tenant table. Because the table is
// shared, a dataset drop deletes that dataset's rows; only a whole-tenant drop
// (empty datasetID) drops the table itself.
func (e *serenedbEngine) DropChunkStore(ctx context.Context, baseName, datasetID string) error {
	tableName := chunkTableName(baseName)
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if datasetID != "" {
		return e.exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE kb_id = $1", tableName), datasetID)
	}
	if err := e.exec(ctx, fmt.Sprintf("DROP INDEX IF EXISTS %s", indexRelation(tableName))); err != nil {
		return err
	}
	return e.exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
}

// ChunkStoreExists reports whether the tenant table exists.
func (e *serenedbEngine) ChunkStoreExists(ctx context.Context, baseName, datasetID string) (bool, error) {
	return e.tableExists(ctx, chunkTableName(baseName))
}

// toFloatSlice coerces a chunk vector value to []float64.
func toFloatSlice(v interface{}) ([]float64, bool) {
	switch val := v.(type) {
	case []float64:
		return val, true
	case []float32:
		out := make([]float64, len(val))
		for i, f := range val {
			out[i] = float64(f)
		}
		return out, true
	case []interface{}:
		out := make([]float64, 0, len(val))
		for _, x := range val {
			switch n := x.(type) {
			case float64:
				out = append(out, n)
			case float32:
				out = append(out, float64(n))
			case int:
				out = append(out, float64(n))
			case json.Number:
				f, _ := n.Float64()
				out = append(out, f)
			default:
				return nil, false
			}
		}
		return out, true
	default:
		return nil, false
	}
}

// prepareChunkRow flattens a chunk into ordered (columns, values) for insert.
// Vector columns get their L2-normalized shadow; unknown fields collapse into
// the `extra` JSON column; JSON columns are serialized. ES field names are
// preserved verbatim.
func prepareChunkRow(chunk map[string]interface{}, defaultKbID string) ([]string, []interface{}) {
	d := map[string]interface{}{}
	extra := map[string]interface{}{}
	vecCols := map[string][]float64{}

	for k, v := range chunk {
		if m := vectorColumnPattern.FindStringSubmatch(k); m != nil {
			if vec, ok := toFloatSlice(v); ok {
				vecCols[k] = vec
			}
			continue
		}
		if _, known := columnDDL[k]; !known {
			extra[k] = v
			continue
		}
		if k == "kb_id" {
			if list, ok := v.([]interface{}); ok && len(list) > 0 {
				v = list[0]
			} else if list, ok := v.([]string); ok && len(list) > 0 {
				v = list[0]
			}
		}
		if isJSONColumn(k) {
			if _, ok := v.(string); !ok {
				b, _ := json.Marshal(v)
				v = string(b)
			}
		}
		d[k] = v
	}

	if len(extra) > 0 {
		merged := map[string]interface{}{}
		if s, ok := d["extra"].(string); ok && s != "" {
			_ = json.Unmarshal([]byte(s), &merged)
		}
		for k, v := range extra {
			merged[k] = v
		}
		b, _ := json.Marshal(merged)
		d["extra"] = string(b)
	}
	// The table is shared across datasets, so kb_id identifies the row's
	// dataset. Honour an explicit kb_id, otherwise stamp the target dataset.
	if defaultKbID != "" {
		if _, ok := d["kb_id"]; !ok {
			d["kb_id"] = defaultKbID
		}
	}
	for k, dv := range columnDefaults {
		if _, ok := d[k]; !ok {
			d[k] = dv
		}
	}

	cols := make([]string, 0, len(d)+2*len(vecCols))
	for c := range d {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	vals := make([]interface{}, 0, len(cols)+2*len(vecCols))
	for _, c := range cols {
		if isArrayColumn(c) {
			// VARCHAR[] columns must be bound through pq.Array; database/sql
			// cannot convert a bare Go slice.
			vals = append(vals, pq.Array(toStringArray(d[c])))
		} else {
			vals = append(vals, d[c])
		}
	}
	vnames := make([]string, 0, len(vecCols))
	for vc := range vecCols {
		vnames = append(vnames, vc)
	}
	sort.Strings(vnames)
	for _, vc := range vnames {
		size, _ := strconv.Atoi(vectorColumnPattern.FindStringSubmatch(vc)[1])
		cols = append(cols, vc, normColumn(size))
		vals = append(vals, pq.Array(vecCols[vc]), pq.Array(l2Normalize(vecCols[vc])))
	}
	return cols, vals
}

// InsertChunks upserts chunks by id. Rows are grouped by identical column set
// and inserted in one multi-row statement per group. Returns an empty slice on
// success (ids are the caller's own).
func (e *serenedbEngine) InsertChunks(ctx context.Context, chunks []map[string]interface{}, baseName, datasetID string) ([]string, error) {
	if len(chunks) == 0 {
		return []string{}, nil
	}
	tableName := chunkTableName(baseName)
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return nil, err
	}
	if !exists {
		size := 0
		for k := range chunks[0] {
			if m := vectorColumnPattern.FindStringSubmatch(k); m != nil {
				size, _ = strconv.Atoi(m[1])
			}
		}
		if err := e.CreateChunkStore(ctx, baseName, datasetID, size, ""); err != nil {
			return nil, err
		}
	}

	type group struct {
		cols []string
		rows [][]interface{}
	}
	groups := map[string]*group{}
	for _, chunk := range chunks {
		cols, vals := prepareChunkRow(chunk, datasetID)
		key := strings.Join(cols, ",")
		g := groups[key]
		if g == nil {
			g = &group{cols: cols}
			groups[key] = g
		}
		g.rows = append(g.rows, vals)
	}

	for _, g := range groups {
		query, args := buildUpsert(tableName, g.cols, g.rows)
		if err := e.exec(ctx, query, args...); err != nil {
			return nil, fmt.Errorf("serenedb: insert into %s: %w", tableName, err)
		}
	}
	return []string{}, nil
}

// buildUpsert renders a multi-row INSERT ... ON CONFLICT (id) DO UPDATE for one
// column set. Pure so it can be asserted in tests.
func buildUpsert(tableName string, cols []string, rows [][]interface{}) (string, []interface{}) {
	var args []interface{}
	valueGroups := make([]string, 0, len(rows))
	ph := 1
	for _, row := range rows {
		placeholders := make([]string, len(cols))
		for i := range cols {
			placeholders[i] = "$" + strconv.Itoa(ph)
			ph++
			args = append(args, row[i])
		}
		valueGroups = append(valueGroups, "("+strings.Join(placeholders, ", ")+")")
	}
	updates := make([]string, 0, len(cols))
	for _, c := range cols {
		if c == "id" {
			continue
		}
		updates = append(updates, fmt.Sprintf("%s = EXCLUDED.%s", c, c))
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s ON CONFLICT (id) DO UPDATE SET %s",
		tableName, strings.Join(cols, ", "), strings.Join(valueGroups, ", "), strings.Join(updates, ", "))
	return query, args
}

// UpdateChunks applies field updates to rows matching condition. add/remove on
// array columns map to list_append/array_remove; other fields are set directly.
func (e *serenedbEngine) UpdateChunks(ctx context.Context, condition, newValue map[string]interface{}, baseName, datasetID string) error {
	tableName := chunkTableName(baseName)
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("serenedb: table %s does not exist", tableName)
	}
	cond := copyCondition(condition)
	cond["kb_id"] = datasetID
	if unknown := unrecognizedFilterKeys(cond); len(unknown) > 0 {
		return fmt.Errorf("serenedb: refusing to update %s with unrecognized filter keys %v", tableName, unknown)
	}
	filters := buildFilters(cond)
	if len(filters) == 0 {
		return fmt.Errorf("serenedb: refusing to update %s without a filter", tableName)
	}
	sets := buildUpdateSets(newValue)
	if len(sets) == 0 {
		return nil
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", tableName, strings.Join(sets, ", "), strings.Join(filters, " AND "))
	return e.exec(ctx, query)
}

// buildUpdateSets renders the SET clause fragments. Pure for tests.
func buildUpdateSets(newValue map[string]interface{}) []string {
	var sets []string
	for k, v := range newValue {
		switch k {
		case "remove":
			items := map[string]interface{}{}
			if s, ok := v.(string); ok {
				items[s] = nil
			} else if m, ok := v.(map[string]interface{}); ok {
				items = m
			}
			for kk, vv := range items {
				if _, known := columnDDL[kk]; !known {
					continue
				}
				if vv == nil {
					sets = append(sets, fmt.Sprintf("%s = NULL", kk))
				} else if isArrayColumn(kk) {
					sets = append(sets, fmt.Sprintf("%s = array_remove(%s, %s)", kk, kk, escapeLiteral(vv)))
				}
			}
		case "add":
			if m, ok := v.(map[string]interface{}); ok {
				for kk, vv := range m {
					if isArrayColumn(kk) {
						sets = append(sets, fmt.Sprintf("%s = list_append(%s, %s)", kk, kk, escapeLiteral(vv)))
					}
				}
			}
		default:
			if isJSONColumn(k) {
				if s, ok := v.(string); ok {
					sets = append(sets, fmt.Sprintf("%s = %s", k, escapeLiteral(s)))
				} else {
					b, _ := json.Marshal(v)
					sets = append(sets, fmt.Sprintf("%s = %s", k, escapeLiteral(string(b))))
				}
			} else if _, known := columnDDL[k]; known {
				sets = append(sets, fmt.Sprintf("%s = %s", k, escapeLiteral(v)))
			}
		}
	}
	sort.Strings(sets)
	return sets
}

// DeleteChunks removes rows matching condition and returns the count. A missing
// table is not an error.
func (e *serenedbEngine) DeleteChunks(ctx context.Context, condition map[string]interface{}, baseName, datasetID string) (int64, error) {
	tableName := chunkTableName(baseName)
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	cond := copyCondition(condition)
	cond["kb_id"] = datasetID
	if unknown := unrecognizedFilterKeys(cond); len(unknown) > 0 {
		return 0, fmt.Errorf("serenedb: refusing to delete from %s with unrecognized filter keys %v", tableName, unknown)
	}
	filters := buildFilters(cond)
	if len(filters) == 0 {
		return 0, nil
	}
	where := strings.Join(filters, " AND ")
	res, err := e.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s", tableName, where))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// GetChunk looks a chunk up by id in the tenant table, optionally scoped to
// the caller's datasets.
func (e *serenedbEngine) GetChunk(ctx context.Context, baseName, chunkID string, datasetIDs []string) (interface{}, error) {
	tableName := chunkTableName(baseName)
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = $1", tableName)
	if kbs := stringSlice(datasetIDs); len(kbs) > 0 {
		if kb := buildFilters(map[string]interface{}{"kb_id": kbs}); len(kb) > 0 {
			query += " AND " + kb[0]
		}
	}
	rows, err := e.queryMaps(ctx, query, chunkID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// stringSlice drops empty ids so an all-blank dataset list yields no kb filter.
func stringSlice(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// toStringArray coerces an array-column value into the []string that pq.Array
// binds as a VARCHAR[].
func toStringArray(v interface{}) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		out := make([]string, 0, len(val))
		for _, x := range val {
			if s, ok := x.(string); ok {
				out = append(out, s)
			} else {
				out = append(out, fmt.Sprintf("%v", x))
			}
		}
		return out
	case nil:
		return nil
	case string:
		return []string{val}
	default:
		return []string{fmt.Sprintf("%v", val)}
	}
}

func copyCondition(condition map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(condition)+1)
	for k, v := range condition {
		out[k] = v
	}
	return out
}
