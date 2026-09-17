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

// These tests exercise the deterministic SQL builders and value codecs. They
// need no live SereneDB, matching the pure-unit-test style of the infinity
// engine, so they run under build.sh --test.
package serenedb

import (
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"
)

func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("expected substring %q in:\n%s", want, got)
	}
}

func mustNotContain(t *testing.T, got, want string) {
	t.Helper()
	if strings.Contains(got, want) {
		t.Errorf("did not expect substring %q in:\n%s", want, got)
	}
}

func TestEscapeLiteral(t *testing.T) {
	cases := map[string]struct {
		in   interface{}
		want string
	}{
		"nil":    {nil, "NULL"},
		"bool":   {true, "true"},
		"int":    {7, "7"},
		"float":  {1.5, "1.5"},
		"string": {"a'b", "'a''b'"},
		"list":   {[]string{"x", "y"}, `'["x","y"]'`},
	}
	for name, c := range cases {
		if got := escapeLiteral(c.in); got != c.want {
			t.Errorf("%s: escapeLiteral(%v) = %q, want %q", name, c.in, got, c.want)
		}
	}
}

func TestBuildFiltersArrayUsesListContains(t *testing.T) {
	got := buildFilters(map[string]interface{}{"tag_kwd": []interface{}{"a", "b"}})
	if len(got) != 1 {
		t.Fatalf("want 1 filter, got %v", got)
	}
	mustContain(t, got[0], "list_contains(tag_kwd, 'a')")
	mustContain(t, got[0], " OR ")
	mustContain(t, got[0], "list_contains(tag_kwd, 'b')")
}

func TestBuildFiltersScalarAndIn(t *testing.T) {
	scalar := buildFilters(map[string]interface{}{"doc_id": "d1"})
	if len(scalar) != 1 || scalar[0] != "doc_id = 'd1'" {
		t.Errorf("scalar filter = %v", scalar)
	}
	in := buildFilters(map[string]interface{}{"doc_id": []interface{}{"a", "b"}})
	if len(in) != 1 || in[0] != "doc_id IN ('a', 'b')" {
		t.Errorf("IN filter = %v", in)
	}
}

func TestBuildFiltersExists(t *testing.T) {
	ex := buildFilters(map[string]interface{}{"exists": "img_id"})
	if len(ex) != 1 || ex[0] != "img_id IS NOT NULL" {
		t.Errorf("exists = %v", ex)
	}
	mn := buildFilters(map[string]interface{}{"must_not": map[string]interface{}{"exists": "img_id"}})
	if len(mn) != 1 || mn[0] != "img_id IS NULL" {
		t.Errorf("must_not exists = %v", mn)
	}
}

func TestBuildFiltersSkipsUnknownAndEmpty(t *testing.T) {
	got := buildFilters(map[string]interface{}{"not_a_column": "x", "doc_id": ""})
	if len(got) != 0 {
		t.Errorf("expected no filters, got %v", got)
	}
}

func TestStripESQuery(t *testing.T) {
	got := stripESQuery(`(auto^0.5) (ptr^0.4) "auto _"^0.9 auto`)
	// boosts and punctuation removed; tokens de-duplicated preserving order.
	if got != "auto ptr _" {
		t.Errorf("stripESQuery = %q", got)
	}
}

func TestL2NormalizeUnitLength(t *testing.T) {
	out := l2Normalize([]float64{3, 4})
	if !reflect.DeepEqual(out, []float64{0.6, 0.8}) {
		t.Errorf("l2Normalize = %v", out)
	}
	if got := l2Normalize([]float64{0, 0}); !reflect.DeepEqual(got, []float64{0, 0}) {
		t.Errorf("zero vector should pass through, got %v", got)
	}
}

func TestVectorLiteralTyped(t *testing.T) {
	got := vectorLiteral([]float64{3, 4})
	mustContain(t, got, "ARRAY[0.6,0.8]::FLOAT[2]")
}

func TestParsePgArray(t *testing.T) {
	cases := map[string]struct {
		in   string
		want []string
	}{
		"simple": {"{a,b,c}", []string{"a", "b", "c"}},
		"empty":  {"{}", []string{}},
		"quoted": {`{"a,b","c"}`, []string{"a,b", "c"}},
		"nonarr": {"plain", nil},
	}
	for name, c := range cases {
		if got := parsePgArray(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: parsePgArray(%q) = %v, want %v", name, c.in, got, c.want)
		}
	}
}

func TestDecodeValue(t *testing.T) {
	if got := decodeValue("tag_kwd", []byte("{a,b}")); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("array decode = %v", got)
	}
	got := decodeValue("position_int", []byte(`{"p":1}`))
	m, ok := got.(map[string]interface{})
	if !ok || m["p"].(float64) != 1 {
		t.Errorf("json decode = %v", got)
	}
	if got := decodeValue("doc_id", []byte("d1")); got != "d1" {
		t.Errorf("scalar decode = %v", got)
	}
	if got := decodeValue("doc_id", nil); got != nil {
		t.Errorf("nil decode = %v", got)
	}
}

func TestChunkTableDDLLandmines(t *testing.T) {
	stmts := chunkTableDDL("ragflow_t1_kb1", 1024)
	joined := strings.Join(stmts, "\n")
	// dictionary must declare frequency+norm or BM25() silently scores 0.
	mustContain(t, joined, "frequency = true")
	mustContain(t, joined, "norm = true")
	// normalized shadow column indexed with ip/sq8.
	mustContain(t, joined, "q_1024_vec_n FLOAT[1024]")
	mustContain(t, joined, "q_1024_vec_n ivf (metric = 'ip', quant = 'sq8')")
	mustContain(t, joined, "optimize_top_k = 'bm25(1.2, 0.75)'")
	mustContain(t, joined, "CREATE TABLE IF NOT EXISTS ragflow_t1_kb1")
}

func TestBuildFulltextSQLSingleColumn(t *testing.T) {
	sql := buildFulltextSQL("t", "id, content_ltks", "TRUE", "hello world", 10, 0)
	mustContain(t, sql, "content_ltks @@ 'hello world'")
	mustContain(t, sql, "BM25(idx_t.tableoid)")
	mustContain(t, sql, "ORDER BY _score DESC LIMIT 10 OFFSET 0")
	// The scored branch must match one column only, never an OR across fields.
	mustNotContain(t, sql, "title_tks @@")
}

func TestBuildVectorSQLThresholdInWhere(t *testing.T) {
	pm := parsedMatch{vectorData: []float64{3, 4}, vecThreshold: 0.2}
	sql := buildVectorSQL("t", "id", "TRUE", pm, 10, 5)
	mustContain(t, sql, "-(q_2_vec_n <#> ARRAY[0.6,0.8]::FLOAT[2]) >= 0.2")
	mustContain(t, sql, "ORDER BY q_2_vec_n <#> ARRAY[0.6,0.8]::FLOAT[2] LIMIT 10 OFFSET 5")
}

func TestBuildFusionSQLShape(t *testing.T) {
	pm := parsedMatch{
		textQuery: "q", vectorData: []float64{3, 4},
		textTopN: 200, vectorTopN: 200, vectorWeight: 0.95,
	}
	sql := buildFusionSQL("t", "id, content_ltks", []string{"id", "content_ltks"}, "TRUE", pm, 0, 30)
	mustContain(t, sql, "s / NULLIF(MAX(s) OVER (), 0) AS sn") // window-fn normalization over the whole tenant table
	mustContain(t, sql, "FULL OUTER JOIN vec v ON l.id = v.id")
	mustContain(t, sql, "COALESCE(l.sn, 0) * 0.05 + COALESCE(v.sim, 0) * 0.95 AS fs")
	mustContain(t, sql, "f.fs + COALESCE(t.pagerank_fea, 0) / 100.0 AS _score")
	mustContain(t, sql, "ORDER BY _score DESC")
	mustContain(t, sql, "content_ltks @@ 'q'")
	mustContain(t, sql, "t.id, t.content_ltks") // output fields prefixed with t.
}

func TestBuildUpsert(t *testing.T) {
	q, args := buildUpsert("t", []string{"id", "doc_id"}, [][]interface{}{{"1", "d1"}, {"2", "d2"}})
	mustContain(t, q, "INSERT INTO t (id, doc_id) VALUES ($1, $2), ($3, $4)")
	mustContain(t, q, "ON CONFLICT (id) DO UPDATE SET doc_id = EXCLUDED.doc_id")
	mustNotContain(t, q, "id = EXCLUDED.id") // never update the conflict key
	if !reflect.DeepEqual(args, []interface{}{"1", "d1", "2", "d2"}) {
		t.Errorf("args = %v", args)
	}
}

func TestBuildUpdateSetsAddRemove(t *testing.T) {
	sets := buildUpdateSets(map[string]interface{}{
		"add":    map[string]interface{}{"tag_kwd": "x"},
		"remove": map[string]interface{}{"tag_kwd": "y"},
	})
	joined := strings.Join(sets, " | ")
	mustContain(t, joined, "tag_kwd = list_append(tag_kwd, 'x')")
	mustContain(t, joined, "tag_kwd = array_remove(tag_kwd, 'y')")
}

func TestBuildUpdateSetsRejectsUnknownColumns(t *testing.T) {
	// Every branch must whitelist the identifier; an unknown key (including a
	// remove-to-NULL, which is the only branch that emits a bare identifier)
	// must never reach the SQL.
	sets := buildUpdateSets(map[string]interface{}{
		"remove":      map[string]interface{}{"evil = 1; DROP TABLE t; --": nil, "tag_kwd": nil},
		"add":         map[string]interface{}{"not_a_column": "x"},
		"bogus_field": "v",
		"1=1; DROP":   "v",
	})
	joined := strings.Join(sets, " | ")
	mustContain(t, joined, "tag_kwd = NULL")
	mustNotContain(t, joined, "DROP")
	mustNotContain(t, joined, "not_a_column")
	mustNotContain(t, joined, "bogus_field")
}

func TestChunkTableNameIsTenantScoped(t *testing.T) {
	// All datasets share the tenant table; datasetID never enters the name.
	if got := chunkTableName("ragflow_t1"); got != "ragflow_t1" {
		t.Errorf("chunkTableName = %q, want ragflow_t1", got)
	}
}

func TestResolveOutputFields(t *testing.T) {
	got := resolveOutputFields([]string{"content_ltks", "_score", "bogus_field"})
	// id first, _score and unknown dropped, pagerank appended.
	if got[0] != "id" {
		t.Errorf("id must be first: %v", got)
	}
	joined := strings.Join(got, ",")
	mustContain(t, joined, "content_ltks")
	mustContain(t, joined, pagerankField)
	mustNotContain(t, joined, "_score")
	mustNotContain(t, joined, "bogus_field")
}

func TestSearchFiltersScoping(t *testing.T) {
	// KbIDs become an IN predicate over the shared tenant table.
	scored := searchFilters(map[string]interface{}{}, []string{"kb1", "kb2"}, true)
	joined := strings.Join(scored, " AND ")
	mustContain(t, joined, "kb_id IN ('kb1', 'kb2')")
	mustContain(t, joined, "available_int = 1")
	// No KbIDs and no match expr -> no predicates.
	if got := searchFilters(map[string]interface{}{}, nil, false); len(got) != 0 {
		t.Errorf("expected no filters, got %v", got)
	}
	// A scored query with no available_int/status defaults available_int=1.
	one := searchFilters(map[string]interface{}{}, nil, true)
	if len(one) != 1 || one[0] != "available_int = 1" {
		t.Errorf("scored default = %v", one)
	}
	// Blank dataset ids are dropped, so no empty IN () is emitted.
	if got := searchFilters(map[string]interface{}{}, []string{""}, false); len(got) != 0 {
		t.Errorf("blank kb ids should yield no filter, got %v", got)
	}
}

func TestFusionVectorWeight(t *testing.T) {
	w, ok := fusionVectorWeight(map[string]interface{}{"weights": "0.05,0.95"})
	if !ok || w != 0.95 {
		t.Errorf("weight = %v, ok = %v", w, ok)
	}
	if _, ok := fusionVectorWeight(map[string]interface{}{}); ok {
		t.Error("missing weights should not parse")
	}
}

func TestParseMatchExprs(t *testing.T) {
	pm := parseMatchExprs([]interface{}{
		&types.MatchTextExpr{MatchingText: "auto^1", TopN: 50},
		&types.MatchDenseExpr{EmbeddingData: []float64{1, 2}, TopN: 60, ExtraOptions: map[string]interface{}{"similarity": 0.3}},
		&types.FusionExpr{FusionParams: map[string]interface{}{"weights": "0.1,0.9"}},
	})
	if !pm.hasText || !pm.hasVector {
		t.Fatalf("expected text+vector, got %+v", pm)
	}
	if pm.textQuery != "auto" || pm.textTopN != 50 {
		t.Errorf("text parse = %q/%d", pm.textQuery, pm.textTopN)
	}
	if pm.vecThreshold != 0.3 || pm.vectorTopN != 60 {
		t.Errorf("vector parse = %v/%d", pm.vecThreshold, pm.vectorTopN)
	}
	if pm.vectorWeight != 0.9 {
		t.Errorf("fusion weight = %v", pm.vectorWeight)
	}
}

func TestBoundRunSQL(t *testing.T) {
	if got := boundRunSQL("SELECT * FROM t;", 1024); got != "SELECT * FROM t LIMIT 1024" {
		t.Errorf("bound = %q", got)
	}
	if got := boundRunSQL("SELECT * FROM t LIMIT 5", 1024); got != "SELECT * FROM t LIMIT 5" {
		t.Errorf("existing limit must be kept: %q", got)
	}
	if got := boundRunSQL("UPDATE t SET x=1", 1024); got != "UPDATE t SET x=1" {
		t.Errorf("non-select must be untouched: %q", got)
	}
}

func TestMetadataTableDDL(t *testing.T) {
	stmts := metadataTableDDL("ragflow_doc_meta_t1")
	joined := strings.Join(stmts, "\n")
	mustContain(t, joined, "CREATE TABLE IF NOT EXISTS ragflow_doc_meta_t1")
	mustContain(t, joined, "meta_fields JSON")
	mustContain(t, joined, "id VARCHAR PRIMARY KEY")
}

func TestGetScoresFromKNNStructure(t *testing.T) {
	e := &serenedbEngine{}
	knn := map[string]interface{}{"hits": map[string]interface{}{"hits": []interface{}{
		map[string]interface{}{"_id": "a", "_score": 1.5},
		map[string]interface{}{"_id": "b", "_score": 0.0},
	}}}
	got := e.GetScores(knn)
	if got["a"] != 1.5 || got["b"] != 0.0 {
		t.Errorf("GetScores = %v", got)
	}
}

func TestBuildFiltersEmptyListNeverMatches(t *testing.T) {
	// An empty IN / array list must not emit invalid `col IN ()`; it reduces to
	// a never-match predicate so a DELETE/UPDATE cannot widen scope.
	if got := buildFilters(map[string]interface{}{"doc_id": []interface{}{}}); len(got) != 1 || got[0] != neverMatch {
		t.Errorf("empty IN filter = %v", got)
	}
	if got := buildFilters(map[string]interface{}{"tag_kwd": []interface{}{}}); len(got) != 1 || got[0] != neverMatch {
		t.Errorf("empty array filter = %v", got)
	}
}

func TestUnrecognizedFilterKeys(t *testing.T) {
	unknown := unrecognizedFilterKeys(map[string]interface{}{
		"doc_id": "x", "exists": "img_id", "must_not": nil, "tag_kwd": "t", "bogus": "y",
	})
	if len(unknown) != 1 || unknown[0] != "bogus" {
		t.Errorf("unrecognized keys = %v, want [bogus]", unknown)
	}
}

func TestValidIdentifier(t *testing.T) {
	for _, ok := range []string{"ragflow_t1", "ragflow_doc_meta_abc", "_x"} {
		if !validIdentifier(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"a; DROP TABLE t", "1abc", "a b", "a-b", "", "t';--"} {
		if validIdentifier(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestRedactDSN(t *testing.T) {
	mustContain(t, redactDSN("host=h user=u password=secret sslmode=disable"), "password=***")
	mustNotContain(t, redactDSN("host=h password=secret"), "secret")
	// URL form (SERENEDB_DSN)
	got := redactDSN("postgresql://u:secret@host:7890/db")
	mustContain(t, got, "://u:***@host")
	mustNotContain(t, got, "secret")
}

func TestQuoteDSNValue(t *testing.T) {
	if got := quoteDSNValue("p'a ss"); got != `'p\'a ss'` {
		t.Errorf("quoteDSNValue = %q", got)
	}
	if got := quoteDSNValue(`a\b`); got != `'a\\b'` {
		t.Errorf("quoteDSNValue backslash = %q", got)
	}
}

func TestBuildDSNSSLMode(t *testing.T) {
	t.Setenv("SERENEDB_DSN", "") // force the config path, not the env override
	def := buildDSN(config.SereneDBConfig{Host: "h", Port: 7890, User: "u"})
	mustContain(t, def, "sslmode='disable'") // safe default preserved
	enc := buildDSN(config.SereneDBConfig{Host: "h", User: "u", SSLMode: "require"})
	mustContain(t, enc, "sslmode='require'")
}
