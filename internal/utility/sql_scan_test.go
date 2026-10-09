package utility

import (
	"strings"
	"testing"
)

func mustScan(t *testing.T, sql string) []SQLToken {
	t.Helper()
	tokens, err := SQLScan(sql)
	if err != nil {
		t.Fatalf("Scan(%q): %v", sql, err)
	}
	return tokens
}

// literalOf returns the single string literal in a scanned statement.
func literalOf(t *testing.T, sql string) string {
	t.Helper()
	var found []string
	for _, tok := range mustScan(t, sql) {
		if tok.Kind == SQLString {
			found = append(found, tok.Value)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected one literal in %q, found %v", sql, found)
	}
	return found[0]
}

func TestScanKeepsLiteralBytes(t *testing.T) {
	cases := map[string]string{
		"semicolon":         "SELECT a FROM t WHERE b = 'x;y'",
		"keywords inside":   "SELECT a FROM t WHERE b = 'from union where'",
		"percent wildcards": "SELECT a FROM t WHERE b LIKE '%Al%'",
		"non ascii":         "SELECT a FROM t WHERE b = '华东'",
		"backticks inside":  "SELECT a FROM t WHERE b = 'a`b'",
		"comment marker":    "SELECT a FROM t WHERE b = '-- not a comment'",
	}
	want := map[string]string{
		"semicolon":         "x;y",
		"keywords inside":   "from union where",
		"percent wildcards": "%Al%",
		"non ascii":         "华东",
		"backticks inside":  "a`b",
		"comment marker":    "-- not a comment",
	}
	for name, sql := range cases {
		t.Run(name, func(t *testing.T) {
			if got := literalOf(t, sql); got != want[name] {
				t.Errorf("literal = %q, want %q", got, want[name])
			}
		})
	}
}

// MySQL reads "x" as a string and standard SQL reads it as a name. The scanner
// takes the standard reading so a caller can check it as an identifier: the
// range it injects is rendered with the quoting character the engine expects,
// so what runs is the statement that was checked either way.
func TestScanReadsDoubleQuotesAsNames(t *testing.T) {
	tokens := mustScan(t, `SELECT a FROM t WHERE b = "x"`)
	last := tokens[len(tokens)-1]
	if last.Kind != SQLQuoted || last.Name != "x" {
		t.Errorf("got %v", last)
	}
	if got := SQLRender(tokens, '`'); !strings.Contains(got, "`x`") {
		t.Errorf("mysql render = %q", got)
	}
}

func TestScanUnescapesDoubledQuote(t *testing.T) {
	if got := literalOf(t, `SELECT a FROM t WHERE b = 'it''s'`); got != "it's" {
		t.Errorf("literal = %q, want %q", got, "it's")
	}
}

func TestScanRefusesAmbiguousInput(t *testing.T) {
	cases := map[string]string{
		"line comment":        "SELECT a -- b\nFROM t",
		"block comment":       "SELECT /* a */ b FROM t",
		"hash comment":        "SELECT a # b\nFROM t",
		"unterminated":        "SELECT a FROM t WHERE b = 'x",
		"unquoted identifier": `SELECT a FROM t WHERE b = "x`,
		"backslash escape":    `SELECT a FROM t WHERE b = 'x\'`,
		"control char":        "SELECT a FROM t WHERE b = 'x\ny'",
		"placeholder":         "SELECT a FROM t WHERE b = ?",
		"at variable":         "SELECT @x",
		"dollar quote":        "SELECT a$$b$$",
		"number then word":    "SELECT 1abc FROM t",
		"empty quoted":        "SELECT \"\" FROM t",
		"blank":               "   ",
	}
	for name, sql := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := SQLScan(sql); err == nil {
				t.Errorf("Scan(%q) succeeded, want an error", sql)
			}
		})
	}
}

func TestScanSeparatesKeywordsFromLiterals(t *testing.T) {
	tokens := mustScan(t, "select DOCNM, count (*) from T where a<=>b and c>=1.5")
	kinds := make([]SQLKind, 0, len(tokens))
	for _, tok := range tokens {
		kinds = append(kinds, tok.Kind)
	}
	// select DOCNM , count ( * ) from T where a <=> b and c >= 1.5
	want := []SQLKind{SQLWord, SQLWord, SQLPunct, SQLWord, SQLPunct, SQLPunct, SQLPunct, SQLWord, SQLWord, SQLWord, SQLWord, SQLPunct, SQLWord, SQLWord, SQLWord, SQLPunct, SQLNumber}
	if len(kinds) != len(want) {
		t.Fatalf("tokens = %v, want %d", kinds, len(want))
	}
	for i, kind := range want {
		if kinds[i] != kind {
			t.Errorf("token %d = %v, want kind %d", i, tokens[i], kind)
		}
	}
	if !tokens[1].IsWord("docnm") {
		t.Errorf("an identifier compares case-folded, got %q / %q", tokens[1].Name, tokens[1].Lower)
	}
	if tokens[len(tokens)-1].Kind != SQLNumber {
		t.Error("1.5 must be one number token")
	}
}

func TestRenderQuotesPerDialect(t *testing.T) {
	tokens := mustScan(t, "SELECT `地区`, \"other\" FROM t WHERE x = 'a b'")
	if got, want := SQLRender(tokens, '"'), `SELECT "地区", "other" FROM t WHERE x = 'a b'`; got != want {
		t.Errorf("postgres render = %q, want %q", got, want)
	}
	if got, want := SQLRender(tokens, '`'), "SELECT `地区`, `other` FROM t WHERE x = 'a b'"; got != want {
		t.Errorf("mysql render = %q, want %q", got, want)
	}
	// A literal's own spaces survive: whitespace is collapsed between tokens,
	// never inside a caller's value.
	if got := SQLRender(mustScan(t, "select a from t where b = 'x  y'"), 0); got != "select a from t where b = 'x  y'" {
		t.Errorf("verbatim render = %q", got)
	}
}

func TestQuoteLiteralEscapes(t *testing.T) {
	got, err := SQLQuoteLiteral("it's")
	if err != nil {
		t.Fatal(err)
	}
	if got != "'it''s'" {
		t.Errorf("QuoteLiteral = %q", got)
	}
	tokens := mustScan(t, got)
	if len(tokens) != 1 || tokens[0].Value != "it's" {
		t.Errorf("a quoted literal must scan back: %v", tokens)
	}
	if _, err := SQLQuoteLiteral("bad\x00"); err == nil {
		t.Error("control character accepted")
	}
}

func mustShape(t *testing.T, sql string) *SQLStatementShape {
	t.Helper()
	shape, err := SQLSplitSelect(mustScan(t, sql))
	if err != nil {
		t.Fatalf("SplitSelect(%q): %v", sql, err)
	}
	return shape
}

func TestSplitSelectCoversEveryClause(t *testing.T) {
	shape := mustShape(t, `select doc_id, count (*) as n
		from ragflow_t_1 where available_int = 1 and table_row_int = 1
		group by doc_id having sum (weight_flt) > 2 order by doc_id desc limit 10 offset 5`)
	c := shape.Clauses
	if got := SQLRender(c.Select, 0); got != "doc_id, count ( * ) as n" {
		t.Errorf("select list = %q", got)
	}
	if got := SQLRender(c.From, 0); got != "ragflow_t_1" {
		t.Errorf("from = %q", got)
	}
	if got := SQLRender(c.Where, 0); !strings.HasPrefix(got, "available_int = 1") {
		t.Errorf("where = %q", got)
	}
	if got := SQLRender(c.GroupBy, 0); got != "doc_id" {
		t.Errorf("group by = %q", got)
	}
	if got := SQLRender(c.Having, 0); got != "sum ( weight_flt ) > 2" {
		t.Errorf("having = %q", got)
	}
	if got := SQLRender(c.OrderBy, 0); got != "doc_id desc" {
		t.Errorf("order by = %q", got)
	}
	if got := SQLRender(c.Limit, 0); got != "10" {
		t.Errorf("limit = %q", got)
	}
	if got := SQLRender(c.Offset, 0); got != "5" {
		t.Errorf("offset = %q", got)
	}
	if !shape.Aggregating {
		t.Error("count() must mark the statement aggregate")
	}
}

func TestSplitSelectRefusesTwoQueries(t *testing.T) {
	cases := map[string]string{
		"not select":        "update t set a = 1",
		"leading with":      "with x as (select 1) select * from x",
		"union":             "select a from t union select b from u",
		"join":              "select a from t join u on t.id = u.id",
		"subquery":          "select a from t where b in (select c from u)",
		"nested select":     "select (select b from u) from t",
		"doubly nested":     "select a from t where b in ((select c from u))",
		"select into":       "select a into outfile '/tmp/x' from t",
		"two from":          "select a from t from u",
		"where before from": "select a where b = 1 from t",
		"no from":           "select 1",
		"unbalanced":        "select a from t where (b = 1",
		"extra close":       "select a from t) where b = 1",
		"offset first":      "select a from t offset 5",
		"limit twice":       "select a from t limit 1 limit 2",
		"empty select":      "select from t",
	}
	for name, sql := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := SQLSplitSelect(mustScan(t, sql)); err == nil {
				t.Errorf("SplitSelect(%q) succeeded, want an error", sql)
			}
		})
	}
}

func TestSplitSelectAllowsParenthesisedConditions(t *testing.T) {
	shape := mustShape(t, "select a from t where (x = 1 or y = 2) and (z in ('a', 'b'))")
	if got := SQLRender(shape.Clauses.Where, 0); !strings.Contains(got, "( x = 1 or y = 2 )") {
		t.Errorf("where = %q", got)
	}
	if shape.Aggregating {
		t.Error("no aggregate here")
	}
}

func TestTableReference(t *testing.T) {
	cases := map[string]string{
		"ragflow_t_1":    "ragflow_t_1",
		"db.ragflow_t_1": "db.ragflow_t_1",
		"\"Ragflow_T\"":  "ragflow_t",
	}
	for in, want := range cases {
		got, err := SQLTableReference(mustScan(t, "select a from "+in)[3:])
		if err != nil {
			t.Errorf("TableReference(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("TableReference(%q) = %q, want %q", in, got, want)
		}
	}
	for _, in := range []string{
		"t, u", "t as x", "t x", "t join u", "t (", "t ;", "t.1", "t order 1",
	} {
		tokens := mustScan(t, "select a from "+in)[3:]
		if got, err := SQLTableReference(tokens); err == nil {
			t.Errorf("TableReference(%q) = %q, want an error", in, got)
		}
	}
}

func TestCallArguments(t *testing.T) {
	tokens := mustScan(t, "select cast (json_extract_string (chunk_data, '$.c_1') as integer) from t")
	cast := -1
	for i, tok := range tokens {
		if tok.IsWord("cast") {
			cast = i
		}
	}
	args, next, err := SQLCallArguments(tokens, cast)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 1 {
		t.Fatalf("CAST takes one argument group, got %d: %v", len(args), args)
	}
	if got := SQLRender(args[0], 0); got != "json_extract_string ( chunk_data, '$.c_1' ) as integer" {
		t.Errorf("cast argument = %q", got)
	}
	if tokens[next].Lower != "from" {
		t.Errorf("next = %d reads as %v", next, tokens[next])
	}

	jsonTokens := args[0]
	name := -1
	for i, tok := range jsonTokens {
		if tok.IsWord("json_extract_string") {
			name = i
		}
	}
	inner, _, err := SQLCallArguments(jsonTokens, name)
	if err != nil {
		t.Fatal(err)
	}
	if len(inner) != 2 || SQLRender(inner[0], 0) != "chunk_data" || inner[1][0].Value != "$.c_1" {
		t.Errorf("json call args = %v", inner)
	}
}
