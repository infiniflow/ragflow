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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// preprocessSQL — mirrors infinity_conn_base.py:788-789.
// -----------------------------------------------------------------------------

func TestPreprocessSQL_WhitespaceAndBackticks(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"a  b", "a b"},
		{"a   b   c", "a b c"},
		{"a`b`c", "a b c"},
		{"a `` b", "a b"},
		// The regex collapses ALL runs of spaces/backticks — including
		// leading and trailing whitespace. Trimming is a separate step
		// in RunSQL (strings.TrimSpace before the preprocessing pass).
		{"  leading and trailing  ", " leading and trailing "},
	}
	for _, c := range cases {
		if got := preprocessSQL(c.in); got != c.want {
			t.Errorf("preprocessSQL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPreprocessSQL_StripsPercent(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"count > 0 %", "count > 0 "},
		{"100% match", "100 match"},
		{"%%%", ""},
	}
	for _, c := range cases {
		if got := preprocessSQL(c.in); got != c.want {
			t.Errorf("preprocessSQL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPreprocessSQL_Combined(t *testing.T) {
	in := "SELECT   docnm_kwd  FROM  `ragflow_t1`  WHERE  count  >  0  %"
	got := preprocessSQL(in)
	want := "SELECT docnm_kwd FROM ragflow_t1 WHERE count > 0 "
	if got != want {
		t.Errorf("preprocessSQL(%q) = %q, want %q", in, got, want)
	}
}

// -----------------------------------------------------------------------------
// rewriteFieldAliases — mirrors infinity_conn_base.py:809-830.
// -----------------------------------------------------------------------------

func TestRewriteFieldAliases_SelectClause(t *testing.T) {
	aliases := map[string]string{
		"docnm_kwd":    "docnm",
		"title_tks":    "docnm",
		"title_sm_tks": "docnm",
		"content_ltks": "content",
	}
	in := "select docnm_kwd, title_tks, content_ltks from ragflow_t1"
	got := rewriteFieldAliases(in, aliases)
	want := "select docnm, docnm, content from ragflow_t1"
	if got != want {
		t.Errorf("rewriteFieldAliases(%q) = %q, want %q", in, got, want)
	}
}

func TestRewriteFieldAliases_WhereClause(t *testing.T) {
	aliases := map[string]string{
		"docnm_kwd": "docnm",
	}
	in := "select doc_id from ragflow_t1 where docnm_kwd = 'foo'"
	got := rewriteFieldAliases(in, aliases)
	want := "select doc_id from ragflow_t1 where docnm = 'foo'"
	if got != want {
		t.Errorf("rewriteFieldAliases(%q) = %q, want %q", in, got, want)
	}
}

func TestRewriteFieldAliases_OrderGroupHaving(t *testing.T) {
	aliases := map[string]string{
		"docnm_kwd":     "docnm",
		"important_kwd": "important_keywords",
	}
	in := "select doc_id from ragflow_t1 order by docnm_kwd group by important_kwd having important_kwd > 0"
	got := rewriteFieldAliases(in, aliases)
	want := "select doc_id from ragflow_t1 order by docnm group by important_keywords having important_keywords > 0"
	if got != want {
		t.Errorf("rewriteFieldAliases(%q) = %q, want %q", in, got, want)
	}
}

func TestRewriteFieldAliases_EmptyMapIsNoop(t *testing.T) {
	in := "select docnm_kwd from ragflow_t1"
	if got := rewriteFieldAliases(in, map[string]string{}); got != in {
		t.Errorf("empty alias map should not modify SQL; got %q", got)
	}
}

func TestRewriteFieldAliases_WordBoundaryProtected(t *testing.T) {
	// "title" is an alias; "title_sm_tks" should NOT match because
	// word boundary is enforced.
	aliases := map[string]string{
		"title": "docnm",
	}
	in := "select title_sm_tks from ragflow_t1"
	got := rewriteFieldAliases(in, aliases)
	// "title" inside "title_sm_tks" should NOT be rewritten.
	want := "select title_sm_tks from ragflow_t1"
	if got != want {
		t.Errorf("rewriteFieldAliases(%q) = %q, want %q (title_sm_tks must NOT be touched)", in, got, want)
	}
}

func TestRewriteFieldAliases_NoAliasMatchLeavesSQLAlone(t *testing.T) {
	aliases := map[string]string{
		"docnm_kwd": "docnm",
	}
	in := "select content_with_weight from ragflow_t1"
	got := rewriteFieldAliases(in, aliases)
	if got != in {
		t.Errorf("unrelated SQL should be unchanged; got %q", got)
	}
}

// -----------------------------------------------------------------------------
// parsePsqlTable — mirrors infinity_conn_base.py:894-934.
// -----------------------------------------------------------------------------

func TestParsePsqlTable_StandardOutput(t *testing.T) {
	// Sample psql table output for `select 1 as a, 2 as b;`
	out := ` a | b
---+---
 1 | 2
(1 row)`

	res := parsePsqlTable(out)
	wantCols := []string{"a", "b"}
	if !reflect.DeepEqual(res.Columns, wantCols) {
		t.Errorf("columns: got %v, want %v", res.Columns, wantCols)
	}
	wantRows := [][]string{{"1", "2"}}
	if !reflect.DeepEqual(res.Rows, wantRows) {
		t.Errorf("rows: got %v, want %v", res.Rows, wantRows)
	}
}

func TestParsePsqlTable_EmptyOutput(t *testing.T) {
	res := parsePsqlTable("")
	if len(res.Columns) != 0 || len(res.Rows) != 0 {
		t.Errorf("empty output should yield (0 cols, 0 rows); got %+v", res)
	}
}

func TestParsePsqlTable_NoSeparatorLine(t *testing.T) {
	// Some psql configurations skip the separator line; the parser
	// should still recover (data starts at line 1 in that case).
	out := "a | b\n1 | 2"
	res := parsePsqlTable(out)
	if len(res.Rows) != 1 {
		t.Errorf("rows: got %d, want 1", len(res.Rows))
	}
}

func TestParsePsqlTable_MultipleRowsAndRowCountFooter(t *testing.T) {
	out := ` id | name
----+------
  1 | foo
  2 | bar
(2 rows)`
	res := parsePsqlTable(out)
	wantCols := []string{"id", "name"}
	if !reflect.DeepEqual(res.Columns, wantCols) {
		t.Errorf("columns: got %v, want %v", res.Columns, wantCols)
	}
	if len(res.Rows) != 2 {
		t.Errorf("rows: got %d, want 2", len(res.Rows))
	}
	if res.Rows[0][0] != "1" || res.Rows[0][1] != "foo" {
		t.Errorf("row[0]: got %v, want [1 foo]", res.Rows[0])
	}
	if res.Rows[1][0] != "2" || res.Rows[1][1] != "bar" {
		t.Errorf("row[1]: got %v, want [2 bar]", res.Rows[1])
	}
}

func TestParsePsqlTable_PadsAndTruncatesRows(t *testing.T) {
	// Row with fewer cells → pad with empty strings.
	// Row with more cells → truncate.
	out := ` a | b | c
---+---+---
 1 | 2
 1 | 2 | 3 | 4
(2 rows)`
	res := parsePsqlTable(out)
	if len(res.Rows) != 2 {
		t.Fatalf("rows: got %d, want 2", len(res.Rows))
	}
	// First row: ["1", "2", ""] (padded)
	if !reflect.DeepEqual(res.Rows[0], []string{"1", "2", ""}) {
		t.Errorf("padded row: got %v, want [1 2 ]", res.Rows[0])
	}
	// Second row: ["1", "2", "3"] (truncated)
	if !reflect.DeepEqual(res.Rows[1], []string{"1", "2", "3"}) {
		t.Errorf("truncated row: got %v, want [1 2 3]", res.Rows[1])
	}
}

func TestParsePsqlTable_SkipsRowCountFooter(t *testing.T) {
	out := " a \n---\n 1 \n(1 row)"
	res := parsePsqlTable(out)
	if len(res.Rows) != 1 {
		t.Errorf("row count footer should be skipped; got %d rows", len(res.Rows))
	}
}

// -----------------------------------------------------------------------------
// toRowMaps — chunk-shape conversion.
// -----------------------------------------------------------------------------

func TestToRowMaps_EmptyResultsReturnsNil(t *testing.T) {
	if rows := toRowMaps(nil); rows != nil {
		t.Errorf("nil result: got %v, want nil", rows)
	}
	if rows := toRowMaps(&psqlResult{}); rows != nil {
		t.Errorf("empty result: got %v, want nil", rows)
	}
}

func TestToRowMaps_ConvertsToRowMaps(t *testing.T) {
	res := &psqlResult{
		Columns: []string{"id", "name"},
		Rows: [][]string{
			{"1", "foo"},
			{"2", "bar"},
		},
	}
	got := toRowMaps(res)
	want := []map[string]interface{}{
		{"id": "1", "name": "foo"},
		{"id": "2", "name": "bar"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toRowMaps: got %v, want %v", got, want)
	}
}

// -----------------------------------------------------------------------------
// resolvePsqlHostPort — mirrors infinity_conn_base.py:838-858.
// -----------------------------------------------------------------------------

func TestResolvePsqlHostPort_DefaultsWhenConfigEmpty(t *testing.T) {
	host, port := resolvePsqlHostPort("", 0)
	if host != defaultPsqlHost {
		t.Errorf("host: got %q, want %q", host, defaultPsqlHost)
	}
	if port != defaultPsqlPort {
		t.Errorf("port: got %q, want %q", port, defaultPsqlPort)
	}
}

func TestResolvePsqlHostPort_OverridesFromConfig(t *testing.T) {
	host, port := resolvePsqlHostPort("10.0.0.1:23817", 5433)
	if host != "10.0.0.1" {
		t.Errorf("host: got %q, want 10.0.0.1", host)
	}
	if port != "5433" {
		t.Errorf("port: got %q, want 5433", port)
	}
}

func TestResolvePsqlHostPort_EmptyHostInURIFallsBackToDefault(t *testing.T) {
	// ":23817" parses via strings.Cut to ("", "23817") — the empty
	// host doesn't override the default, matching Python's
	// `re.search(r"host=(\S+)", ...)` which only matches a non-empty
	// value.
	host, port := resolvePsqlHostPort(":23817", 5432)
	if host != defaultPsqlHost {
		t.Errorf("host: got %q, want default %q (empty host in URI should not override)", host, defaultPsqlHost)
	}
	if port != "5432" {
		t.Errorf("port: got %q, want 5432", port)
	}
}

func TestLoadFieldMapping_ParsesAliases(t *testing.T) {
	// Write a temporary mapping file.
	dir := t.TempDir()
	contents := `{
		"docnm": {"type": "varchar", "comment": "docnm_kwd, title_tks, title_sm_tks"},
		"content": {"type": "varchar", "comment": "content_with_weight, content_ltks"},
		"plain": {"type": "varchar"}
	}`

	// Set RAG_PROJECT_BASE to the temp dir's parent so loadFieldMapping
	// finds the file at <base>/conf/<filename>.
	t.Setenv("RAG_PROJECT_BASE", dir)

	// Need to create conf/ subdir.
	if err := os.MkdirAll(filepath.Join(dir, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "conf", "test_mapping.json"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write conf/mapping: %v", err)
	}

	a2a, r2a, err := loadFieldMapping("test_mapping.json")
	if err != nil {
		t.Fatalf("loadFieldMapping: %v", err)
	}

	// alias → actual
	expectedAliases := map[string]string{
		"docnm_kwd":           "docnm",
		"title_tks":           "docnm",
		"title_sm_tks":        "docnm",
		"content_with_weight": "content",
		"content_ltks":        "content",
	}
	if !reflect.DeepEqual(a2a, expectedAliases) {
		t.Errorf("aliasToActual: got %v, want %v", a2a, expectedAliases)
	}

	// actual → first alias (mirrors Python at line 807)
	if r2a["docnm"] != "docnm_kwd" {
		t.Errorf("actualToFirstAlias[docnm]: got %q, want docnm_kwd", r2a["docnm"])
	}
	if r2a["content"] != "content_with_weight" {
		t.Errorf("actualToFirstAlias[content]: got %q, want content_with_weight", r2a["content"])
	}
	// "plain" has no comment, so it shouldn't appear in the reverse map.
	if _, ok := r2a["plain"]; ok {
		t.Errorf("actualToFirstAlias should not include fields without comments")
	}
}

func TestLoadFieldMapping_EmptyNameDefaultsToInfinityMappingJSON(t *testing.T) {
	// Ensure the test runs in an isolated project base so any repo file
	// named "infinity_mapping.json" doesn't get picked up.
	dir := t.TempDir()
	os.Setenv("RAG_PROJECT_BASE", dir)
	defer os.Unsetenv("RAG_PROJECT_BASE")

	// Empty name → defaults to "infinity_mapping.json" (line 145).
	// We just verify the function doesn't panic and the file-not-found
	// path is taken silently.
	t.Setenv("RAG_PROJECT_BASE", t.TempDir())

	a2a, r2a, err := loadFieldMapping("")
	if err != nil {
		t.Fatalf("empty name: %v", err)
	}
	if len(a2a) == 0 || len(r2a) == 0 {
		t.Errorf("empty name + no file should yield empty maps; got a2a=%v r2a=%v", a2a, r2a)
	}
}

func TestBuildFilterFromCondition_UnconstrainedFilter(t *testing.T) {
	clmns := map[string]struct {
		Type    string
		Default interface{}
	}{
		"id": {"Varchar", ""},
	}
	// empty condition yields "1=1"
	if got := buildFilterFromCondition(map[string]interface{}{}, clmns); got != "1=1" {
		t.Errorf("empty condition: got %q, want '1=1'", got)
	}
	// condition with nil or empty string values yields "1=1"
	cond := map[string]interface{}{
		"source_id": "",
		"nil_field": nil,
	}
	if got := buildFilterFromCondition(cond, clmns); got != "1=1" {
		t.Errorf("non-empty condition with blank values: got %q, want '1=1'", got)
	}
}

// TestBuildFilterFromCondition_StringSliceIDPreservesScope pins the shape the
// document availability switch relies on: the doc-service caller of
// UpdateChunks (updateDocumentChunkAvailability) passes a typed []string id
// list, and Infinity must render it as an IN clause. Dropping it leaves whatever
// other clauses the caller passed (none, for that path), i.e. an update scoped
// to the whole dataset table.
func TestBuildFilterFromCondition_StringSliceIDPreservesScope(t *testing.T) {
	clmns := map[string]struct {
		Type    string
		Default interface{}
	}{
		"id": {"Varchar", ""},
	}
	got := buildFilterFromCondition(map[string]interface{}{"id": []string{"chunk-a", "chunk-b"}}, clmns)
	if got != "id IN ('chunk-a', 'chunk-b')" {
		t.Errorf("condition []string id: got %q, want %q", got, "id IN ('chunk-a', 'chunk-b')")
	}
	// An empty list contributes no clause, so the caller is left with an
	// unconstrained filter — the case UpdateChunks/DeleteChunks refuse.
	if got := buildFilterFromCondition(map[string]interface{}{"id": []string{}}, clmns); got != "1=1" {
		t.Errorf("empty []string id: got %q, want '1=1'", got)
	}
}

// TestEquivalentConditionToStrSkipsBlankKeywordValues pins that a blank entry in
// a keyword (filter_fulltext) condition is dropped instead of being rendered as
// filter_fulltext('<col>', ”): Infinity rejects that with
//
//	3052 Trying to match:  on fields: <col> failed
//
// and fails the whole statement, which is exactly what GetWikiGraph hit when an
// entity row carried no slug.
func TestEquivalentConditionToStrSkipsBlankKeywordValues(t *testing.T) {
	got := equivalentConditionToStr(map[string]interface{}{
		"from_kwd": []string{"", "  ", "topic/出师表"},
	}, nil)
	want := `(filter_fulltext('from_kwd', 'topic/出师表'))`
	if got != want {
		t.Errorf("blank keyword filter = %q, want %q", got, want)
	}

	// All entries blank: the column contributes no clause at all.
	if got := equivalentConditionToStr(map[string]interface{}{"from_kwd": []string{"", ""}}, nil); got != "" {
		t.Errorf("all-blank keyword filter = %q, want no clause", got)
	}
}

// maxParenDepth returns the deepest nesting of parentheses in an expression.
func maxParenDepth(expr string) int {
	depth, max := 0, 0
	for _, r := range expr {
		switch r {
		case '(':
			depth++
			if depth > max {
				max = depth
			}
		case ')':
			depth--
		}
	}
	return max
}

// TestJoinBalanced pins the depth-bounded boolean rendering. Infinity parses the
// filter string into a Thrift expression tree, so a left-deep chain of N terms
// nests N levels deep: the wiki contribution read's 22 OR'ed filter_fulltext
// clauses made Infinity abort the connection ("TProtocolException: Exceeded depth
// limit"), which surfaced as 7018/EOF. A balanced tree must keep every term and
// only log2(N) nesting.
func TestJoinBalanced(t *testing.T) {
	if got := joinBalanced(nil, " OR "); got != "" {
		t.Errorf("joinBalanced(nil) = %q, want empty", got)
	}
	if got := joinBalanced([]string{"a"}, " OR "); got != "(a)" {
		t.Errorf("single term = %q, want %q", got, "(a)")
	}
	if got := joinBalanced([]string{"a", "b"}, " OR "); got != "(a OR b)" {
		t.Errorf("two terms = %q, want %q", got, "(a OR b)")
	}

	parts := make([]string, 0, 22)
	for i := 0; i < 22; i++ {
		parts = append(parts, fmt.Sprintf("f%02d", i))
	}
	expr := joinBalanced(parts, " or ")
	for _, p := range parts {
		if !strings.Contains(expr, p) {
			t.Fatalf("expression dropped %s: %s", p, expr)
		}
	}
	// Paren depth of the rendered string mirrors the depth of the expression tree
	// the SDK builds from it; a flat chain would be sent N levels deep.
	if maxParenDepth(expr) > 6 {
		t.Errorf("balanced depth = %d, want <= ceil(log2(22))+1 = 6: %s", maxParenDepth(expr), expr)
	}
}

// TestEquivalentConditionToStrBalancesLongKeywordLists pins that the keyword
// branch uses the balanced join: a 22-element slug list is exactly the request
// that killed the connection in production.
func TestEquivalentConditionToStrBalancesLongKeywordLists(t *testing.T) {
	values := make([]string, 0, 22)
	for i := 0; i < 22; i++ {
		values = append(values, fmt.Sprintf("entity/person/p%d", i))
	}
	got := equivalentConditionToStr(map[string]interface{}{"slug_kwd": values}, nil)
	for _, v := range values {
		if !strings.Contains(got, "filter_fulltext('slug_kwd', '"+v+"')") {
			t.Fatalf("condition dropped %s: %s", v, got)
		}
	}
	if depth := maxParenDepth(got); depth > 6 {
		t.Errorf("paren depth = %d, want <= 6 for 22 values: %s", depth, got)
	}
}

// TestIsConnectionLevelError pins which failures mark a pooled connection as
// unusable: a cut-short reply desyncs the Thrift stream, and the next caller on
// that socket is the one the server rejects with "Exceeded depth limit" (seen as
// 7018/EOF). Semantic Infinity errors must NOT drop the connection.
func TestIsConnectionLevelError(t *testing.T) {
	drop := []string{
		"InfinityException(7018, Failed to execute query: EOF)",
		"InfinityException(6003, Connection is dead, removed from pool)",
		"read tcp 127.0.0.1:33924->127.0.0.1:23817: read: connection reset by peer",
		"write tcp: broken pipe",
		"dial tcp 127.0.0.1:23817: i/o timeout",
		"context deadline exceeded",
		"TProtocolException: Exceeded depth limit",
	}
	for _, msg := range drop {
		if !isConnectionLevelError(errors.New(msg)) {
			t.Errorf("isConnectionLevelError(%q) = false, want true", msg)
		}
	}
	keep := []string{
		"",
		"InfinityException(3052, Failed to execute query: Trying to match:  on fields: from_kwd failed)",
		"InfinityException(3013, Failed to execute query: Fail to bind the expression: q_)",
		"InfinityException(7001, Table not found)",
	}
	for _, msg := range keep {
		var err error
		if msg != "" {
			err = errors.New(msg)
		}
		if isConnectionLevelError(err) {
			t.Errorf("isConnectionLevelError(%q) = true, want false", msg)
		}
	}
}

// TestRowFieldNames pins the insert-failure log payload: sorted column names of
// the first row (values are never logged — a vector column would flood the log).
func TestRowFieldNames(t *testing.T) {
	got := rowFieldNames([]map[string]interface{}{{"title_kwd": "a", "available_int": 0, "q_1024_vec": []float64{1}}})
	want := []string{"available_int", "q_1024_vec", "title_kwd"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rowFieldNames = %#v, want %#v", got, want)
	}
	if got := rowFieldNames(nil); got != nil {
		t.Errorf("rowFieldNames(nil) = %#v, want nil", got)
	}
}

// TestIsEmptyEngineCell pins the row-materialization guard: Infinity can return a
// null cell (untyped nil or a nil slice) for a projection it found no row for, and
// such a cell must not turn into a row of its own — GetWikiGraph used to read an
// empty slug off that phantom row and poison the relation filter.
func TestIsEmptyEngineCell(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
		want  bool
	}{
		{"untyped nil", nil, true},
		{"nil slice", []uint8(nil), true},
		{"nil []interface{}", []interface{}(nil), true},
		{"empty slice", []interface{}{}, false},
		{"empty string", "", false},
		{"value", "doc-1", false},
	}
	for _, c := range cases {
		if got := isEmptyEngineCell(c.value); got != c.want {
			t.Errorf("isEmptyEngineCell(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBuildFilterFromConditionJSONListMembership(t *testing.T) {
	jsonColumns := map[string]struct {
		Type    string
		Default interface{}
	}{
		"source_doc_ids": {Type: "Json", Default: "[]"},
	}
	condition := map[string]interface{}{"source_doc_ids": []string{"doc-1", "doc-2"}}
	want := `(json_contains(source_doc_ids, '"doc-1"') OR json_contains(source_doc_ids, '"doc-2"'))`
	if got := buildFilterFromCondition(condition, jsonColumns); got != want {
		t.Errorf("JSON-list condition = %q, want %q", got, want)
	}
	if got := equivalentConditionToStr(condition, jsonColumns); got != want {
		t.Errorf("JSON-list search condition = %q, want %q", got, want)
	}
}

func TestBuildFilterFromConditionLegacyJSONListMembership(t *testing.T) {
	legacyColumns := map[string]struct {
		Type    string
		Default interface{}
	}{
		"source_doc_ids": {Type: "Varchar", Default: ""},
	}
	condition := map[string]interface{}{"source_doc_ids": []string{"doc-1"}}
	want := `(filter_fulltext('source_doc_ids', 'doc-1'))`
	if got := buildFilterFromCondition(condition, legacyColumns); got != want {
		t.Errorf("legacy JSON-list condition = %q, want %q", got, want)
	}
	if got := equivalentConditionToStr(condition, legacyColumns); got != want {
		t.Errorf("legacy JSON-list search condition = %q, want %q", got, want)
	}
}

func TestBuildFilterFromConditionMissingJSONListColumnNeverMatches(t *testing.T) {
	condition := map[string]interface{}{"source_doc_ids": []string{"doc-1"}}
	want := `(1=0)`
	if got := buildFilterFromCondition(condition, map[string]struct {
		Type    string
		Default interface{}
	}{}); got != want {
		t.Errorf("missing JSON-list column condition = %q, want %q", got, want)
	}
}

// TestKeywordFilterConditionExactForWhitespaceValues pins the fix that makes a
// *_kwd filter with a multi-token value match at all. Infinity tokenizes a
// filter_fulltext() query, so a whitespace-bearing value matched NOTHING (nav's
// parent_kwd child lookup, cluster updates and cleanup all no-op'd); the ES
// columns are keywords and match exactly.
func TestKeywordFilterConditionExactForWhitespaceValues(t *testing.T) {
	cases := []struct {
		name, field, value, want string
	}{
		{"cluster name", "parent_kwd", "Imperial Memorial and Governance Advice 59cbfbef",
			`parent_kwd = 'Imperial Memorial and Governance Advice 59cbfbef'`},
		{"CJK cluster name with suffix", "title_kwd", "忠臣进谏与托孤遗志 27577543",
			`title_kwd = '忠臣进谏与托孤遗志 27577543'`},
		{"doc leaf label", "title_kwd", "出师表中的忠诚与北伐决心 1234",
			`title_kwd = '出师表中的忠诚与北伐决心 1234'`},
		// Single-token values keep the full-text rendering: legacy ###-joined
		// varchar columns still rely on token matching.
		{"single token", "type_kwd", "nav_cluster", `filter_fulltext('type_kwd', 'nav_cluster')`},
		{"root parent", "parent_kwd", "root", `filter_fulltext('parent_kwd', 'root')`},
		// Any Unicode space counts: a full-width space tokenizes like an ASCII
		// one and would hit the same silent no-match.
		{"ideographic space", "title_kwd", "忠臣进谏　与托孤遗志",
			`title_kwd = '忠臣进谏　与托孤遗志'`},
		// Fields convertMatchingField maps to a full-text index reference are NOT
		// columns: `= ` on the reference makes Infinity reject the whole statement,
		// so a multi-token value is phrased through the index instead (that index
		// is what gives tag/toc the membership match ES gets from its array).
		{"tag multi-token", "tag_kwd", "machine learning",
			`filter_fulltext('tag_kwd@ft_tag_kwd_whitespace__', '"machine learning"')`},
		{"toc multi-token", "toc_kwd", "第 一章",
			`filter_fulltext('toc_kwd@ft_toc_kwd_whitespace__', '"第 一章"')`},
		{"tag single token", "tag_kwd", "nlp",
			`filter_fulltext('tag_kwd@ft_tag_kwd_whitespace__', 'nlp')`},
		// A value that cannot be phrased compares the raw column, which is at
		// least addressable (the index reference is not).
		{"tag with a quote", "tag_kwd", `say "hi" now`,
			`tag_kwd = 'say "hi" now'`},
		// A *_kwd field with no index variant stays on the raw column.
		{"source_id", "source_id", "a b", `source_id = 'a b'`},
		// A quote stays escaped in both renderings.
		{"quoted multi-token", "title_kwd", "O'Brien cluster", `title_kwd = 'O''Brien cluster'`},
		{"quoted single token", "type_kwd", "o'brien", `filter_fulltext('type_kwd', 'o''brien')`},
	}
	for _, c := range cases {
		if got := keywordFilterCondition(c.field, c.value); got != c.want {
			t.Errorf("%s: keywordFilterCondition(%q, %q) = %q, want %q",
				c.name, c.field, c.value, got, c.want)
		}
	}
}

// TestKeywordFilterRenderingBothPaths pins that the search path
// (equivalentConditionToStr) and the update/delete path
// (buildFilterFromCondition) render a cluster-name filter identically: a drift
// would write a doc's parent_kwd while the cluster's own update matched nothing.
func TestKeywordFilterRenderingBothPaths(t *testing.T) {
	const name = "Imperial Memorial and Governance Advice 59cbfbef"

	wantParent := `(parent_kwd = '` + name + `')`
	parentCond := map[string]interface{}{"parent_kwd": []string{name}}
	if got := equivalentConditionToStr(parentCond, nil); got != wantParent {
		t.Errorf("search path parent_kwd = %q, want %q", got, wantParent)
	}
	if got := buildFilterFromCondition(parentCond, nil); got != wantParent {
		t.Errorf("update/delete path parent_kwd = %q, want %q", got, wantParent)
	}

	// The cluster's own filter (merge description, append doc, cleanup) uses
	// title_kwd; ListClusters keeps filtering parent_kwd='root', which stays on
	// the full-text path because it is a single token.
	wantTitle := `(title_kwd = '` + name + `')`
	titleCond := map[string]interface{}{"title_kwd": []string{name}}
	if got := equivalentConditionToStr(titleCond, nil); got != wantTitle {
		t.Errorf("search path title_kwd = %q, want %q", got, wantTitle)
	}
	if got := buildFilterFromCondition(titleCond, nil); got != wantTitle {
		t.Errorf("update/delete path title_kwd = %q, want %q", got, wantTitle)
	}
	if got := equivalentConditionToStr(map[string]interface{}{"parent_kwd": []string{"root"}}, nil); got !=
		`(filter_fulltext('parent_kwd', 'root'))` {
		t.Errorf("root clusters must keep the single-token rendering, got %q", got)
	}

	// Blank entries are dropped by BOTH paths. Infinity rejects an empty
	// full-text query ("Trying to match:  on fields: <column> failed", 3052) and
	// fails the whole statement — including UpdateChunks/DeleteChunks, which the
	// update path feeds.
	blankMixed := map[string]interface{}{"from_kwd": []string{"", "  ", "x"}}
	wantMixed := `(filter_fulltext('from_kwd', 'x'))`
	if got := equivalentConditionToStr(blankMixed, nil); got != wantMixed {
		t.Errorf("search path with blank entries = %q, want %q", got, wantMixed)
	}
	if got := buildFilterFromCondition(blankMixed, nil); got != wantMixed {
		t.Errorf("update/delete path with blank entries = %q, want %q", got, wantMixed)
	}

	// Every entry blank: the search path contributes no clause, while the update
	// path yields "1=1" so the caller's unconstrained-statement guard refuses it
	// instead of running an unscoped update or delete.
	allBlank := map[string]interface{}{"from_kwd": []string{"", "  "}}
	if got := equivalentConditionToStr(allBlank, nil); got != "" {
		t.Errorf("search path with only blank entries = %q, want no clause", got)
	}
	if got := buildFilterFromCondition(allBlank, nil); got != "1=1" {
		t.Errorf("update/delete path with only blank entries = %q, want '1=1'", got)
	}
}
