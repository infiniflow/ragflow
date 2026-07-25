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
	"fmt"
	"regexp"
)

// pagerankField is folded into every scored search and is always selected.
const pagerankField = "pagerank_fea"

// docMetaPrefix marks the per-tenant metadata tables. A datasetID-scoped
// delete must not touch these, and inserts route to the metadata path.
const docMetaPrefix = "ragflow_doc_meta_"

// dictionaryName is the text-search dictionary the inverted index uses.
// frequency and norm are what make BM25() score at all: without frequency the
// scorer silently returns 0.0 for every row.
const dictionaryName = "rf_scored_delim"

const dictionaryDDL = "CREATE TEXT SEARCH DICTIONARY IF NOT EXISTS " + dictionaryName +
	" (template = 'delimiter', delimiter = ' ', frequency = true, position = true, norm = true)"

// vectorColumnPattern matches the ES vector field name, e.g. q_1024_vec.
var vectorColumnPattern = regexp.MustCompile(`^q_(\d+)_vec$`)

// identifierPattern is the shape a table name (derived from a tenant index
// name) must have before it is interpolated into DDL/DML. Table names are not
// parameterizable, so they are validated instead.
var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validIdentifier(name string) bool {
	return identifierPattern.MatchString(name)
}

// Column groups keep the ES mapping names verbatim so the read path needs no
// renames. The write path adapts Go values to SQL; the read path decodes JSON
// columns back to structured values.
var (
	textColumns = []string{
		"docnm_kwd", "doc_type_kwd", "title_tks", "title_sm_tks", "content_with_weight",
		"content_ltks", "content_sm_ltks", "important_tks", "question_tks", "create_time",
		"img_id", "knowledge_graph_kwd", "entity_kwd", "entity_type_kwd", "from_entity_kwd",
		"to_entity_kwd", "removed_kwd", "raptor_kwd", "group_id", "mom_id", "n_hop_with_weight",
	}
	arrayColumns = []string{"important_kwd", "question_kwd", "tag_kwd", "source_id", "entities_kwd"}
	intColumns   = []string{"pagerank_fea", "available_int", "weight_int", "raptor_layer_int", "_order_id"}
	floatColumns = []string{"create_timestamp_flt", "weight_flt", "rank_flt"}
	jsonColumns  = []string{
		"tag_feas", "position_int", "page_num_int", "top_int", "chunk_data",
		"metadata", "extra", "meta_fields",
	}
)

// columnDDL maps every stored column to its SQL type. columnOrder preserves a
// stable CREATE TABLE order (Go maps do not).
var (
	columnDDL   = map[string]string{}
	columnOrder []string
	arraySet    = toSet(arrayColumns)
	jsonSet     = toSet(jsonColumns)
)

// ftsColumns are the text columns the inverted index carries.
var ftsColumns = []string{"title_tks", "important_tks", "question_tks", "content_ltks"}

// lexScoredCol is the single column the scored lexical branch matches.
// ORDER BY BM25() over a multi-column @@ OR returns an empty set, so per-field
// boosts must be summed in application code, never as a SQL-level OR.
// content_ltks is the dominant field and is what the parity eval scored on.
const lexScoredCol = "content_ltks"

// docMetaColumnOrder / docMetaDDL define the per-tenant metadata table.
var docMetaColumnOrder = []string{"id", "kb_id", "meta_fields"}

var docMetaDDL = map[string]string{
	"id":          "VARCHAR PRIMARY KEY",
	"kb_id":       "VARCHAR",
	"meta_fields": "JSON",
}

// columnDefaults are applied on insert when the caller omits them.
var columnDefaults = map[string]interface{}{
	"available_int": 1,
	"removed_kwd":   "N",
	"_order_id":     0,
}

func init() {
	columnOrder = append(columnOrder, "id", "kb_id", "doc_id")
	columnDDL["id"] = "VARCHAR PRIMARY KEY"
	columnDDL["kb_id"] = "VARCHAR"
	columnDDL["doc_id"] = "VARCHAR"
	add := func(cols []string, typ string) {
		for _, c := range cols {
			if _, seen := columnDDL[c]; seen {
				continue
			}
			columnDDL[c] = typ
			columnOrder = append(columnOrder, c)
		}
	}
	add(textColumns, "TEXT")
	add(arrayColumns, "VARCHAR[]")
	add(intColumns, "INTEGER")
	add(floatColumns, "DOUBLE PRECISION")
	add(jsonColumns, "JSON")
}

func toSet(cols []string) map[string]struct{} {
	s := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		s[c] = struct{}{}
	}
	return s
}

// isKnownColumn reports whether a field is a stored column or a vector column.
func isKnownColumn(name string) bool {
	if _, ok := columnDDL[name]; ok {
		return true
	}
	return vectorColumnPattern.MatchString(name)
}

// normColumn is the L2-normalized shadow of a vector column. ip on the unit
// column is exact cosine and is what the IVF index quantizes.
func normColumn(vectorSize int) string {
	return fmt.Sprintf("q_%d_vec_n", vectorSize)
}

func rawVectorColumn(vectorSize int) string {
	return fmt.Sprintf("q_%d_vec", vectorSize)
}

// indexRelation is the inverted index name for a table.
func indexRelation(tableName string) string {
	return "idx_" + tableName
}

// chunkTableName returns the tenant's chunk table. All of a tenant's datasets
// share one table (the Elasticsearch/OceanBase model), with kb_id as a filter
// column, so BM25 statistics are computed over the whole tenant corpus rather
// than per dataset. baseName is already the tenant index name.
func chunkTableName(baseName string) string {
	return baseName
}

// buildMetadataTableName returns the per-tenant metadata table name.
func buildMetadataTableName(tenantID string) string {
	return docMetaPrefix + tenantID
}
