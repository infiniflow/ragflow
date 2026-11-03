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

package service

import (
	"strings"
	"testing"

	"ragflow/internal/entity"
)

const (
	scopeAllowedKB = "11111111-1111-1111-1111-111111111111"
	scopeVictimKB  = "22222222-2222-2222-2222-222222222222"
)

func mustContainScope(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q in rewritten SQL:\n%s", want, got)
	}
}

// TestAddKBFilterNeutralizesModelSuppliedKB checks the core invariant: the
// authorized filter is always conjoined, so a model-written kb_id for another
// dataset becomes a contradiction that returns no rows.
func TestAddKBFilterNeutralizesModelSuppliedKB(t *testing.T) {
	sql := "SELECT doc_id, docnm_kwd FROM ragflow_tenant_b WHERE kb_id = '" + scopeVictimKB + "'"
	got, ok := addKBFilter(sql, "elasticsearch", []*entity.Knowledgebase{{ID: scopeAllowedKB}})
	if !ok {
		t.Fatalf("expected the statement to be safely scoped, got ok=false")
	}
	mustContainScope(t, got, "kb_id = '"+scopeAllowedKB+"'")
	mustContainScope(t, got, "WHERE (kb_id = '"+scopeVictimKB+"')")
	mustContainScope(t, got, ") AND (kb_id = '"+scopeAllowedKB+"')")
}

// TestAddKBFilterPrecedence checks that an OR in the model's predicate cannot
// escape the injected scope.
func TestAddKBFilterPrecedence(t *testing.T) {
	got, ok := addKBFilter("SELECT doc_id FROM t WHERE title = 'x' OR 1=1", "elasticsearch",
		[]*entity.Knowledgebase{{ID: scopeAllowedKB}})
	if !ok {
		t.Fatalf("expected ok=true")
	}
	mustContainScope(t, got, "WHERE (title = 'x' OR 1=1) AND (kb_id = '"+scopeAllowedKB+"')")
}

// TestAddKBFilterNoWhere checks that a statement without WHERE gets the scope
// inserted before trailing clauses.
func TestAddKBFilterNoWhere(t *testing.T) {
	got, ok := addKBFilter("SELECT doc_id FROM t ORDER BY x", "elasticsearch",
		[]*entity.Knowledgebase{{ID: scopeAllowedKB}})
	if !ok {
		t.Fatalf("expected ok=true")
	}
	mustContainScope(t, got, "WHERE kb_id = '"+scopeAllowedKB+"' ORDER BY x")
}

// TestAddKBFilterTrailingClauses checks the predicate ends before GROUP BY so
// the injected scope does not land inside the grouping clause.
func TestAddKBFilterTrailingClauses(t *testing.T) {
	got, ok := addKBFilter("SELECT doc_id FROM t WHERE x=1 GROUP BY g ORDER BY o LIMIT 5", "elasticsearch",
		[]*entity.Knowledgebase{{ID: scopeAllowedKB}})
	if !ok {
		t.Fatalf("expected ok=true")
	}
	mustContainScope(t, got, "WHERE (x=1) AND (kb_id = '"+scopeAllowedKB+"') GROUP BY g ORDER BY o LIMIT 5")
}

// TestAddKBFilterSubqueryAndCTE checks only the top-level predicate is wrapped.
func TestAddKBFilterSubqueryAndCTE(t *testing.T) {
	got, ok := addKBFilter("SELECT doc_id FROM t WHERE a IN (SELECT b FROM u WHERE c=1)", "elasticsearch",
		[]*entity.Knowledgebase{{ID: scopeAllowedKB}})
	if !ok {
		t.Fatalf("expected ok=true")
	}
	mustContainScope(t, got, "WHERE (a IN (SELECT b FROM u WHERE c=1)) AND (kb_id = '"+scopeAllowedKB+"')")

	got, ok = addKBFilter("WITH x AS (SELECT doc_id FROM t WHERE y=1) SELECT doc_id FROM x WHERE z=2", "elasticsearch",
		[]*entity.Knowledgebase{{ID: scopeAllowedKB}})
	if !ok {
		t.Fatalf("expected ok=true")
	}
	mustContainScope(t, got, "WHERE (z=2) AND (kb_id = '"+scopeAllowedKB+"')")
	mustContainScope(t, got, "(SELECT doc_id FROM t WHERE y=1)")
}

// TestAddKBFilterIgnoresKeywordsInsideLiterals checks a quoted keyword cannot
// be mistaken for the real clause.
func TestAddKBFilterIgnoresKeywordsInsideLiterals(t *testing.T) {
	got, ok := addKBFilter("SELECT doc_id FROM t WHERE title = 'where kb_id ='", "elasticsearch",
		[]*entity.Knowledgebase{{ID: scopeAllowedKB}})
	if !ok {
		t.Fatalf("expected ok=true")
	}
	mustContainScope(t, got, "WHERE (title = 'where kb_id =') AND (kb_id = '"+scopeAllowedKB+"')")

	got, ok = addKBFilter("SELECT doc_id FROM t /* where */ WHERE a=1 -- kb_id =", "elasticsearch",
		[]*entity.Knowledgebase{{ID: scopeAllowedKB}})
	if !ok {
		t.Fatalf("expected ok=true")
	}
	// The line comment stays inside the wrapper and is terminated by a newline
	// so the conjoined filter is not swallowed by it.
	if !strings.Contains(got, "WHERE (a=1 -- kb_id =\n) AND (kb_id = '"+scopeAllowedKB+"')") {
		t.Fatalf("trailing line comment must stay inside the wrapper, got:\n%s", got)
	}
}

// TestAddKBFilterMultiKB checks multiple authorized datasets are ORed inside
// the conjoined filter.
func TestAddKBFilterMultiKB(t *testing.T) {
	got, ok := addKBFilter("SELECT doc_id FROM t WHERE title='x'", "elasticsearch",
		[]*entity.Knowledgebase{{ID: scopeAllowedKB}, {ID: scopeVictimKB}})
	if !ok {
		t.Fatalf("expected ok=true")
	}
	mustContainScope(t, got, "AND ((kb_id = '"+scopeAllowedKB+"' OR kb_id = '"+scopeVictimKB+"'))")
}

// TestAddKBFilterRejectsUnscopableShapes checks fail-closed behavior: shapes
// where the injected predicate cannot cover every arm must be refused.
func TestAddKBFilterRejectsUnscopableShapes(t *testing.T) {
	kbs := []*entity.Knowledgebase{{ID: scopeAllowedKB}}
	for _, sql := range []string{
		"SELECT doc_id FROM t1 UNION SELECT doc_id FROM t2",
		"SELECT doc_id FROM t1 INTERSECT SELECT doc_id FROM t2",
		"SELECT doc_id FROM t1 EXCEPT SELECT doc_id FROM t2",
		"SELECT doc_id FROM t WHERE title = 'unterminated",
		"SELECT doc_id FROM t WHERE a=1 /* unterminated",
		// Unbalanced parentheses would let the predicate escape the wrapping
		// parens: `WHERE (1=1) OR (1=1) AND (...)`, which is true for every row.
		"SELECT doc_id FROM t WHERE 1=1) OR (1=1",
		"SELECT doc_id FROM t WHERE (1=1",
		"SELECT doc_id FROM t WHERE ((a=1)",
	} {
		if got, ok := addKBFilter(sql, "elasticsearch", kbs); ok {
			t.Fatalf("SECURITY: unscopable SQL was accepted: %q -> %q", sql, got)
		}
	}
}

func TestAddKBFilterRejectsAmbiguousBackslashQuotes(t *testing.T) {
	kbs := []*entity.Knowledgebase{{ID: scopeAllowedKB}}
	// The newline ends the server's -- comment. If the scanner mistakes \' for
	// an escaped quote, its wrapper makes the first OR arm unscoped.
	sql := "SELECT doc_id FROM t WHERE a = 'x\\' OR 1=1) OR (1=1 --'\n AND b = 'y'"
	for _, engineName := range []string{"elasticsearch", "opensearch", "vastbase", "serenedb"} {
		if got, ok := addKBFilter(sql, engineName, kbs); ok {
			t.Fatalf("SECURITY: %s accepted ambiguous quote: %q", engineName, got)
		}
	}
	for _, engineName := range []string{"oceanbase", "seekdb"} {
		for _, sql := range []string{
			"SELECT doc_id FROM t WHERE a = 'x\\'y'",
			"SELECT doc_id FROM t WHERE a = \"x\\\"y\"",
			"SELECT doc_id FROM t WHERE a = 'x\\' OR 1=1) OR (1=1 -- '\n AND b = 'y'",
		} {
			if got, ok := addKBFilter(sql, engineName, kbs); ok {
				t.Fatalf("%s accepted quote with mode-dependent escape: %q", engineName, got)
			}
		}
	}

	// PostgreSQL E-strings have explicit escape semantics. Doubled quotes have
	// the same boundary with or without MySQL's NO_BACKSLASH_ESCAPES mode.
	for _, tc := range []struct {
		engineName string
		sql        string
	}{
		{"vastbase", "SELECT doc_id FROM t WHERE a = E'x\\'y'"},
		{"serenedb", "SELECT doc_id FROM t WHERE a = E'x\\'y'"},
		{"oceanbase", "SELECT doc_id FROM t WHERE a = 'x''y'"},
		{"seekdb", "SELECT doc_id FROM t WHERE a = 'x''y'"},
	} {
		got, ok := addKBFilter(tc.sql, tc.engineName, kbs)
		if !ok {
			t.Fatalf("%s rejected valid quoted literal: %q", tc.engineName, got)
		}
		mustContainScope(t, got, "AND (kb_id = '"+scopeAllowedKB+"')")
	}
}

// TestAddKBFilterRejectsMySQLCommentSyntax checks the comment syntaxes OceanBase
// inherits from MySQL, which the "--"/"/* */" scan does not cover:
//   - "#" runs to end of line and would swallow the appended filter.
//   - "/*! ... */" is executable, so it can hide a set operation from the scan.
//
// Both are refused for the OceanBase family and left alone for other engines.
func TestAddKBFilterRejectsMySQLCommentSyntax(t *testing.T) {
	kbs := []*entity.Knowledgebase{{ID: scopeAllowedKB}}
	shapes := []string{
		"SELECT doc_id FROM t # note",
		"SELECT doc_id FROM t WHERE a=1 # note",
		"SELECT doc_id FROM t /*! UNION SELECT doc_id FROM t */",
		"SELECT doc_id FROM t WHERE a=1 /*!50000 OR 1=1 */",
		"SELECT doc_id FROM t WHERE 1--1) OR (1=1",
		"SELECT doc_id FROM t WHERE 1--",
	}
	for _, engineName := range []string{"oceanbase", "seekdb"} {
		for _, sql := range shapes {
			if got, ok := addKBFilter(sql, engineName, kbs); ok {
				t.Fatalf("SECURITY: %s accepted MySQL-only comment syntax: %q -> %q", engineName, sql, got)
			}
		}
	}
	if got, ok := addKBFilter("SELECT doc_id FROM t WHERE a=1 -- note", "oceanbase", kbs); !ok {
		t.Fatalf("oceanbase rejected a valid line comment: %q", got)
	}

	// PostgreSQL-family engines treat "#" as an operator, so the same text is
	// not a comment there and must keep working.
	if got, ok := addKBFilter("SELECT doc_id FROM t WHERE a # b", "vastbase", kbs); !ok {
		t.Fatalf("vastbase rejected '#' operator: %q", got)
	}
}

// TestAddKBFilterSkipsDollarQuotedLiterals checks PostgreSQL dollar-quoted
// strings are treated as literals. Otherwise a clause keyword inside one is
// mistaken for the end of the predicate and the injected filter lands inside
// the string, where the server treats it as data.
func TestAddKBFilterSkipsDollarQuotedLiterals(t *testing.T) {
	kbs := []*entity.Knowledgebase{{ID: scopeAllowedKB}}

	// The bypass from the review. With $$ recognised as a literal, the text
	// inside is data, so the trailing ")" leaves an unbalanced parenthesis and
	// the statement is refused instead of being rewritten into valid SQL.
	if got, ok := addKBFilter("SELECT doc_id FROM t WHERE title = $$ORDER BY($$) OR 1=1", "vastbase", kbs); ok {
		t.Fatalf("SECURITY: unbalanced statement made valid by the rewrite: %q", got)
	}

	// The valid variant must be scoped, not split: the clause keyword inside
	// the literal must not be taken as the end of the predicate.
	got, ok := addKBFilter("SELECT doc_id FROM t WHERE title = $$ORDER BY$$ OR 1=1", "vastbase", kbs)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if !strings.Contains(got, "WHERE (title = $$ORDER BY$$ OR 1=1) AND (kb_id = '"+scopeAllowedKB+"')") {
		t.Fatalf("dollar-quoted literal was split by the wrapper, got:\n%s", got)
	}

	// Tagged dollar quotes.
	got, ok = addKBFilter("SELECT doc_id FROM t WHERE title = $tag$ ORDER BY $tag$ OR 1=1", "serenedb", kbs)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if !strings.Contains(got, "WHERE (title = $tag$ ORDER BY $tag$ OR 1=1) AND (kb_id = '"+scopeAllowedKB+"')") {
		t.Fatalf("tagged dollar quote was split, got:\n%s", got)
	}

	// An unterminated dollar quote must fail closed.
	if got, ok := addKBFilter("SELECT doc_id FROM t WHERE title = $$unterminated", "vastbase", kbs); ok {
		t.Fatalf("SECURITY: unterminated dollar quote accepted: %q", got)
	}

	// "$1" is a bind placeholder, not a dollar quote; the scan must not treat
	// it as one and swallow the rest of the statement.
	if got, ok := addKBFilter("SELECT doc_id FROM t WHERE a = $1 AND b = 2", "vastbase", kbs); !ok {
		t.Fatalf("placeholder rejected: %q", got)
	} else if !strings.Contains(got, "AND (kb_id = '"+scopeAllowedKB+"')") {
		t.Fatalf("filter missing for placeholder query:\n%s", got)
	}
}

func TestAddKBFilterNonASCIIDollarQuoteTags(t *testing.T) {
	kbs := []*entity.Knowledgebase{{ID: scopeAllowedKB}}
	for _, engineName := range []string{"vastbase", "serenedb"} {
		if got, ok := addKBFilter("SELECT doc_id FROM t WHERE title = $é$ ORDER BY ($é$ OR 1=1)", engineName, kbs); ok {
			t.Fatalf("%s accepted an unbalanced predicate: %q", engineName, got)
		}
		got, ok := addKBFilter("SELECT doc_id FROM t WHERE title = $é$ ORDER BY ($é$ OR 1=1", engineName, kbs)
		if !ok {
			t.Fatalf("%s rejected a balanced dollar quote: %q", engineName, got)
		}
		mustContainScope(t, got, "WHERE (title = $é$ ORDER BY ($é$ OR 1=1) AND (kb_id = '"+scopeAllowedKB+"')")
	}
}

func TestAddKBFilterNonASCIIIdentifiersDoNotOpenDollarQuotes(t *testing.T) {
	kbs := []*entity.Knowledgebase{{ID: scopeAllowedKB}}
	sql := "SELECT doc_id FROM t WHERE é$x$ = 1 OR 1=1) OR (1=1 AND b$x$ = 2"
	for _, engineName := range []string{"vastbase", "serenedb"} {
		if got, ok := addKBFilter(sql, engineName, kbs); ok {
			t.Fatalf("%s accepted an unbalanced predicate: %q", engineName, got)
		}
	}
}

// The engine removes backticks after filtering, which can turn a quoted
// identifier into SQL syntax that escapes the added predicate.
func TestAddKBFilterRejectsBackticks(t *testing.T) {
	kbs := []*entity.Knowledgebase{{ID: scopeAllowedKB}}
	for _, engineName := range []string{"elasticsearch", "opensearch", "oceanbase", "seekdb"} {
		if got, ok := addKBFilter("SELECT doc_id FROM t WHERE `order by` = 1", engineName, kbs); ok {
			t.Fatalf("%s accepted backticks removed by its engine: %q", engineName, got)
		}
	}
	if got, ok := addKBFilter("SELECT doc_id FROM t WHERE title = 'a`b'", "vastbase", kbs); !ok {
		t.Fatalf("vastbase rejected a backtick inside a string: %q", got)
	}
}

func TestAddKBFilterScopesAfterElasticsearchPercentRemoval(t *testing.T) {
	kbs := []*entity.Knowledgebase{{ID: scopeAllowedKB}}
	for _, engineName := range []string{"elasticsearch", "opensearch"} {
		got, ok := addKBFilter("SELECT doc_id FROM t -%- ORDER BY doc_id", engineName, kbs)
		if !ok || !strings.Contains(got, "-- ORDER BY doc_id\nWHERE kb_id = '"+scopeAllowedKB+"'") {
			t.Fatalf("%s line comment did not keep the filter live: %q", engineName, got)
		}
		got, ok = addKBFilter("SELECT doc_id FROM t /%* ORDER BY doc_id *%/", engineName, kbs)
		if !ok || !strings.Contains(got, "/* ORDER BY doc_id */ WHERE kb_id = '"+scopeAllowedKB+"'") {
			t.Fatalf("%s block comment did not keep the filter live: %q", engineName, got)
		}
	}
}

// A nested block comment can keep the added WHERE inside the server's comment
// even though the scanner sees ORDER BY after the inner comment closes.
func TestAddKBFilterRejectsNestedBlockCommentsOutsidePostgres(t *testing.T) {
	kbs := []*entity.Knowledgebase{{ID: scopeAllowedKB}}
	sql := "SELECT doc_id FROM t /* outer /* inner */ ORDER BY x */"
	for _, engineName := range []string{"elasticsearch", "opensearch", "oceanbase", "seekdb"} {
		if got, ok := addKBFilter(sql, engineName, kbs); ok {
			t.Fatalf("%s accepted nested block comment: %q", engineName, got)
		}
	}
}

// TestAddKBFilterSupportsNestedBlockComments checks PostgreSQL nested block
// comments are consumed exactly; the old scan stopped at the first "*/" and
// then read the comment body as SQL.
func TestAddKBFilterSupportsNestedBlockComments(t *testing.T) {
	kbs := []*entity.Knowledgebase{{ID: scopeAllowedKB}}
	got, ok := addKBFilter("SELECT doc_id FROM t /* a /* b */ c */ WHERE x=1", "vastbase", kbs)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if !strings.Contains(got, "WHERE (x=1) AND (kb_id = '"+scopeAllowedKB+"')") {
		t.Fatalf("nested block comment mishandled, got:\n%s", got)
	}
}

// TestAddKBFilterWrapsPredicateAcrossLineComments checks a `--` comment cannot
// cut the predicate in two. Everything from the model's predicate must stay
// inside the wrapper, including any OR that follows on the next line, so the
// authorization filter (ANDed outside) always constrains the whole predicate.
func TestAddKBFilterWrapsPredicateAcrossLineComments(t *testing.T) {
	kbs := []*entity.Knowledgebase{{ID: scopeAllowedKB}}

	// The review bypass: OR after the comment used to escape the wrapper.
	// Wrapped output is `WHERE (1=1 -- note\nOR 1=1) AND (kb_id = 'A')`, so the
	// trailing OR can no longer make the predicate always true.
	got, ok := addKBFilter("SELECT doc_id FROM t WHERE 1=1 -- note\nOR 1=1", "elasticsearch", kbs)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if !strings.Contains(got, "-- note\nOR 1=1) AND (kb_id = '"+scopeAllowedKB+"')") {
		t.Fatalf("OR after the comment escaped the wrapper, got:\n%s", got)
	}

	// No WHERE: a comment before ORDER BY must not swallow the injected filter.
	got, ok = addKBFilter("SELECT doc_id FROM t -- note\nORDER BY x", "elasticsearch", kbs)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if !strings.Contains(got, "WHERE kb_id = '"+scopeAllowedKB+"'") {
		t.Fatalf("filter missing, got:\n%s", got)
	}
	if strings.Index(got, "WHERE kb_id") < strings.Index(got, "-- note") {
		t.Fatalf("filter injected inside the comment, got:\n%s", got)
	}

	// Comment between the predicate and LIMIT: the filter must still apply.
	got, ok = addKBFilter("SELECT doc_id FROM t WHERE 1=1 --\nLIMIT 5", "elasticsearch", kbs)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if !strings.Contains(got, ") AND (kb_id = '"+scopeAllowedKB+"')") {
		t.Fatalf("filter not conjoined after the wrapped predicate, got:\n%s", got)
	}
	if strings.Index(got, scopeAllowedKB) < strings.Index(got, "--") {
		t.Fatalf("filter landed inside the comment, got:\n%s", got)
	}

	// A block comment in the middle of the predicate stays inside the wrapper.
	got, ok = addKBFilter("SELECT doc_id FROM t WHERE a=1 /* x */ OR b=2 GROUP BY g", "elasticsearch", kbs)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if !strings.Contains(got, "WHERE (a=1 /* x */ OR b=2) AND (kb_id = '"+scopeAllowedKB+"') GROUP BY g") {
		t.Fatalf("block comment handling changed, got:\n%s", got)
	}
}

// TestAddKBFilterRejectsInvalidKBID checks a non-UUID id fails closed instead
// of producing an unfiltered query.
func TestAddKBFilterRejectsInvalidKBID(t *testing.T) {
	if got, ok := addKBFilter("SELECT doc_id FROM t", "elasticsearch",
		[]*entity.Knowledgebase{{ID: "kb-private"}}); ok {
		t.Fatalf("SECURITY: invalid kb id accepted: %q", got)
	}
}

// TestAddKBFilterInfinityNoop pins that Infinity keeps relying on its
// per-dataset table name, as before.
func TestAddKBFilterInfinityNoop(t *testing.T) {
	const sql = "SELECT doc_id FROM ragflow_t_kb"
	got, ok := addKBFilter(sql, "infinity", []*entity.Knowledgebase{{ID: scopeAllowedKB}})
	if !ok || got != sql {
		t.Fatalf("infinity must stay a no-op, got %q ok=%v", got, ok)
	}
}

// TestBuildChunkFetchSQLInheritsScope checks the aggregate secondary fetch
// builds on the already-scoped SQL, so it does not re-introduce a
// model-written kb_id. useSQL passes the filtered sqlText here.
func TestBuildChunkFetchSQLInheritsScope(t *testing.T) {
	scoped, ok := addKBFilter(
		"SELECT COUNT(*) FROM ragflow_tenant_a WHERE kb_id = '"+scopeVictimKB+"'",
		"elasticsearch", []*entity.Knowledgebase{{ID: scopeAllowedKB}})
	if !ok {
		t.Fatalf("expected the input to be scoped")
	}
	got, found := buildChunkFetchSQL(scoped, "ragflow_tenant_a", "docnm_kwd", false)
	if !found {
		t.Fatalf("expected a secondary fetch SQL")
	}
	mustContainScope(t, got, "kb_id = '"+scopeAllowedKB+"'")
	if !strings.Contains(got, "AND (kb_id = '"+scopeAllowedKB+"')") {
		t.Fatalf("secondary fetch lost the authorized filter:\n%s", got)
	}
}
