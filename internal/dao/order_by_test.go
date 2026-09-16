package dao

import "testing"

func TestOrderClauseRendersAllowedTerms(t *testing.T) {
	allowed := map[string]struct{}{"name": {}, "create_time": {}, "size": {}}

	cases := []struct {
		name  string
		terms []OrderTerm
		want  string
	}{
		{
			name:  "one allowed column ascending",
			terms: []OrderTerm{{Column: "name"}},
			want:  "name ASC",
		},
		{
			name:  "one allowed column descending",
			terms: []OrderTerm{{Column: "name", Desc: true}},
			want:  "name DESC",
		},
		{
			name:  "several terms keep their order and direction",
			terms: []OrderTerm{{Column: "name"}, {Column: "size", Desc: true}, {Column: "create_time"}},
			want:  "name ASC, size DESC, create_time ASC",
		},
		{
			name:  "a term outside the allowlist is dropped and the rest survive",
			terms: []OrderTerm{{Column: "name"}, {Column: "password", Desc: true}},
			want:  "name ASC",
		},
		{
			name:  "no surviving term falls back, keeping the first requested direction",
			terms: []OrderTerm{{Column: "password", Desc: true}},
			want:  "create_time DESC",
		},
		{
			name:  "no terms at all falls back ascending",
			terms: nil,
			want:  "create_time ASC",
		},
		{
			name:  "an injected expression is not a column name, so it is dropped",
			terms: []OrderTerm{{Column: "name; DROP TABLE file"}},
			want:  "create_time ASC",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := orderClause(allowed, tc.terms, defaultOrderColumn); got != tc.want {
				t.Fatalf("orderClause = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestQualifiedOrderClauseNamesTheTableOnEveryColumn(t *testing.T) {
	allowed := map[string]struct{}{"name": {}, "create_time": {}}

	got := qualifiedOrderClause("knowledgebase", allowed,
		[]OrderTerm{{Column: "name", Desc: true}, {Column: "create_time"}}, defaultOrderColumn)
	want := "knowledgebase.name DESC, knowledgebase.create_time ASC"
	if got != want {
		t.Fatalf("qualifiedOrderClause = %q, want %q", got, want)
	}

	// The fallback is a column too, so it has to carry the qualifier or a join
	// would reject it as ambiguous.
	got = qualifiedOrderClause("knowledgebase", allowed, []OrderTerm{{Column: "password"}}, defaultOrderColumn)
	want = "knowledgebase.create_time ASC"
	if got != want {
		t.Fatalf("qualifiedOrderClause fallback = %q, want %q", got, want)
	}
}

// Every entity binds its own allowlist, so a column one entity exposes must not
// become orderable on another.
func TestEntityAllowlistsDoNotLeakAcrossEntities(t *testing.T) {
	if got := searchOrderClause("size", false); got != "create_time ASC" {
		t.Fatalf("search accepted a file column: %q", got)
	}
	if got := fileOrderClause("size", false); got != "size ASC" {
		t.Fatalf("file rejected its own column: %q", got)
	}
	if got := userCanvasOrderClause("doc_num", true); got != "create_time DESC" {
		t.Fatalf("user canvas accepted a knowledge base column: %q", got)
	}
}

// The conversation list reached ORDER BY through string concatenation with no
// allowlist at all, so these cases cover the column names it now refuses as well
// as the empty value its callers still send.
func TestChatSessionOrderClauseGuardsTheConversationList(t *testing.T) {
	cases := []struct {
		name    string
		orderby string
		desc    bool
		want    string
	}{
		{name: "a column the list rows expose", orderby: "name", desc: true, want: "name DESC"},
		{name: "empty keeps the previous default", orderby: "", want: "create_time ASC"},
		{name: "empty keeps the requested direction", orderby: "", desc: true, want: "create_time DESC"},
		{name: "a column of another entity", orderby: "size", want: "create_time ASC"},
		{name: "an injected expression", orderby: "name; DROP TABLE conversation", desc: true, want: "create_time DESC"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chatSessionOrderClause(tc.orderby, tc.desc); got != tc.want {
				t.Fatalf("chatSessionOrderClause(%q, %v) = %q, want %q", tc.orderby, tc.desc, got, tc.want)
			}
		})
	}
}

// The template group list already refused unknown names before the rule moved
// here, so these cases pin the set it accepted rather than the wider set its row
// would allow.
func TestCompilationTemplateGroupOrderClauseKeepsItsAcceptedColumns(t *testing.T) {
	for _, column := range []string{"name", "scope", "create_time", "update_time"} {
		if got := compilationTemplateGroupOrderClause(column, false); got != column+" ASC" {
			t.Fatalf("template group rejected %q: %q", column, got)
		}
	}
	for _, column := range []string{"id", "tenant_id", "description", "status", "create_date"} {
		if got := compilationTemplateGroupOrderClause(column, true); got != "create_time DESC" {
			t.Fatalf("template group accepted %q: %q", column, got)
		}
	}
}
