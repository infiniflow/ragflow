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
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var esBoostRe = regexp.MustCompile(`\^[0-9.]+`)
var esSyntaxRe = regexp.MustCompile(`["()~*?:+\-]|\bAND\b|\bOR\b|\bNOT\b`)

// escapeLiteral renders a Go value as a SQL literal for the templated search
// statements (filters, aggregation). Parameterized placeholders are used on
// the write path; the read path templates values that are engine-internal.
func escapeLiteral(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return "NULL"
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case string:
		return "'" + strings.ReplaceAll(val, "'", "''") + "'"
	case []string, []interface{}, map[string]interface{}:
		b, _ := json.Marshal(val)
		return "'" + strings.ReplaceAll(string(b), "'", "''") + "'"
	default:
		return "'" + strings.ReplaceAll(fmt.Sprintf("%v", val), "'", "''") + "'"
	}
}

// asString coerces a filter value to its scalar string form when possible.
func asString(v interface{}) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	default:
		return "", false
	}
}

// toStringSlice normalizes a filter value into a slice of scalars.
func toStringSlice(v interface{}) []interface{} {
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

// buildFilters translates a RAGFlow condition map into SQL predicates. kb_id is
// scoped by table name (like the other engines), so callers strip it before
// calling. Array columns filter with list_contains; exists/must_not map to
// IS NULL / IS NOT NULL.
// neverMatch is a predicate that matches no rows. An empty IN list (or empty
// array-contains) reduces to it, and it keeps a DELETE/UPDATE from widening
// scope when the caller asked to match "nothing".
const neverMatch = "1 = 0"

// recognizedFilterKey reports whether buildFilters emits a predicate for a key.
// Callers that must not silently widen scope (DeleteChunks/UpdateChunks/
// DeleteMetadata) reject conditions carrying unrecognized keys.
func recognizedFilterKey(k string) bool {
	return k == "exists" || k == "must_not" || isArrayColumn(k) || isKnownColumn(k)
}

// unrecognizedFilterKeys returns any condition keys buildFilters would drop.
func unrecognizedFilterKeys(condition map[string]interface{}) []string {
	var unknown []string
	for k := range condition {
		if !recognizedFilterKey(k) {
			unknown = append(unknown, k)
		}
	}
	return unknown
}

func inList(column string, vals []interface{}) string {
	if len(vals) == 0 {
		return neverMatch
	}
	parts := make([]string, 0, len(vals))
	for _, x := range vals {
		parts = append(parts, escapeLiteral(x))
	}
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(parts, ", "))
}

func buildFilters(condition map[string]interface{}) []string {
	var filters []string
	for k, v := range condition {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		switch {
		case k == "exists":
			if col, ok := asString(v); ok {
				if _, known := columnDDL[col]; known {
					filters = append(filters, col+" IS NOT NULL")
				}
			}
		case k == "must_not":
			if mn, ok := v.(map[string]interface{}); ok {
				if ex, ok := mn["exists"]; ok {
					if col, ok := asString(ex); ok {
						if _, known := columnDDL[col]; known {
							filters = append(filters, col+" IS NULL")
						}
					}
				}
			}
		case isArrayColumn(k):
			vals := toStringSlice(v)
			if len(vals) == 0 {
				filters = append(filters, neverMatch)
				continue
			}
			ors := make([]string, 0, len(vals))
			for _, x := range vals {
				ors = append(ors, fmt.Sprintf("list_contains(%s, %s)", k, escapeLiteral(x)))
			}
			filters = append(filters, "("+strings.Join(ors, " OR ")+")")
		case isKnownColumn(k):
			switch list := v.(type) {
			case []interface{}:
				filters = append(filters, inList(k, list))
			case []string:
				filters = append(filters, inList(k, toStringSlice(list)))
			default:
				filters = append(filters, fmt.Sprintf("%s = %s", k, escapeLiteral(v)))
			}
		}
	}
	return filters
}

func isArrayColumn(name string) bool {
	_, ok := arraySet[name]
	return ok
}

func isJSONColumn(name string) bool {
	_, ok := jsonSet[name]
	return ok
}

// filtersExpr joins predicates, defaulting to TRUE when there are none.
func filtersExpr(filters []string) string {
	if len(filters) == 0 {
		return "TRUE"
	}
	return strings.Join(filters, " AND ")
}

// stripESQuery reduces RAGFlow's tokenized, ^-weighted query_string to a plain
// space-separated token bag for @@. The stored *_ltks columns are tokenized
// the same way, so @@ must see those tokens, not the raw human question.
func stripESQuery(matchingText string) string {
	txt := esBoostRe.ReplaceAllString(matchingText, " ")
	txt = esSyntaxRe.ReplaceAllString(txt, " ")
	seen := map[string]struct{}{}
	var out []string
	for t := range strings.FieldsSeq(txt) {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return strings.Join(out, " ")
}

// l2Normalize returns the unit vector; ip on the unit column is exact cosine.
func l2Normalize(vec []float64) []float64 {
	var s float64
	for _, v := range vec {
		s += v * v
	}
	s = math.Sqrt(s)
	if s == 0 {
		return vec
	}
	out := make([]float64, len(vec))
	for i, v := range vec {
		out[i] = v / s
	}
	return out
}

// vectorLiteral renders a normalized query vector as a typed SQL array literal.
func vectorLiteral(vec []float64) string {
	norm := l2Normalize(vec)
	parts := make([]string, len(norm))
	for i, v := range norm {
		parts[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return fmt.Sprintf("ARRAY[%s]::FLOAT[%d]", strings.Join(parts, ","), len(norm))
}

// parsePgArray decodes a PostgreSQL text-array literal (e.g. {a,"b,c"}) into a
// slice. lib/pq returns array columns as this literal when scanned dynamically.
func parsePgArray(literal string) []string {
	if len(literal) < 2 || literal[0] != '{' || literal[len(literal)-1] != '}' {
		return nil
	}
	body := literal[1 : len(literal)-1]
	if body == "" {
		return []string{}
	}
	var out []string
	var buf strings.Builder
	inQuote := false
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c == '"':
			if inQuote && i+1 < len(body) && body[i+1] == '"' {
				buf.WriteByte('"')
				i++
				continue
			}
			inQuote = !inQuote
		case c == '\\' && i+1 < len(body):
			buf.WriteByte(body[i+1])
			i++
		case c == ',' && !inQuote:
			out = append(out, buf.String())
			buf.Reset()
		default:
			buf.WriteByte(c)
		}
	}
	out = append(out, buf.String())
	return out
}

// decodeValue turns a raw database/sql scan value into the entity value for a
// column: JSON columns are parsed, array columns are split, scalars pass
// through. ES field names are stored verbatim so no renaming is needed.
func decodeValue(column string, raw interface{}) interface{} {
	if raw == nil {
		return nil
	}
	asBytes := func() (string, bool) {
		switch b := raw.(type) {
		case []byte:
			return string(b), true
		case string:
			return b, true
		}
		return "", false
	}
	if isJSONColumn(column) {
		if s, ok := asBytes(); ok {
			var v interface{}
			if err := json.Unmarshal([]byte(s), &v); err == nil {
				return v
			}
			return s
		}
	}
	if isArrayColumn(column) {
		if s, ok := asBytes(); ok {
			return parsePgArray(s)
		}
	}
	if s, ok := raw.([]byte); ok {
		return string(s)
	}
	return raw
}
