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
	"os"
	"path/filepath"
	"reflect"
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

// -----------------------------------------------------------------------------
// loadFieldMapping — mirrors infinity_conn_base.py:793-807.
// -----------------------------------------------------------------------------

func TestLoadFieldMapping_MissingFileReturnsEmpty(t *testing.T) {
	// Use a name that doesn't exist; the function should silently
	// return empty maps (matching Python's `os.path.exists` guard).
	a2a, r2a, err := loadFieldMapping("nonexistent_mapping_xyz.json")
	if err != nil {
		t.Fatalf("missing file should be a no-op, got error: %v", err)
	}
	if len(a2a) != 0 || len(r2a) != 0 {
		t.Errorf("missing file should yield empty maps; got a2a=%v r2a=%v", a2a, r2a)
	}
}

func TestLoadFieldMapping_ParsesAliases(t *testing.T) {
	// Write a temporary mapping file.
	dir := t.TempDir()
	mappingPath := filepath.Join(dir, "test_mapping.json")
	contents := `{
		"docnm": {"type": "varchar", "comment": "docnm_kwd, title_tks, title_sm_tks"},
		"content": {"type": "varchar", "comment": "content_with_weight, content_ltks"},
		"plain": {"type": "varchar"}
	}`
	if err := os.WriteFile(mappingPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("write mapping: %v", err)
	}

	// Set RAG_PROJECT_BASE to the temp dir's parent so loadFieldMapping
	// finds the file at <base>/conf/<filename>.
	os.Setenv("RAG_PROJECT_BASE", dir)
	defer os.Unsetenv("RAG_PROJECT_BASE")

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
	// Empty name → defaults to "infinity_mapping.json" (line 145).
	// We just verify the function doesn't panic and the file-not-found
	// path is taken silently.
	a2a, r2a, err := loadFieldMapping("")
	if err != nil {
		t.Fatalf("empty name: %v", err)
	}
	if len(a2a) != 0 || len(r2a) != 0 {
		t.Errorf("empty name + no file should yield empty maps; got a2a=%v r2a=%v", a2a, r2a)
	}
}
