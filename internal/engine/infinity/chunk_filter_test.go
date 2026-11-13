package infinity

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/engine/types"
)

func TestShouldDefaultAvailableFilter(t *testing.T) {
	if !shouldDefaultAvailableFilter(&types.SearchRequest{}, false) {
		t.Fatal("ordinary retrieval must default to available_int=1")
	}
	if shouldDefaultAvailableFilter(&types.SearchRequest{IncludeUnavailable: true}, false) {
		t.Fatal("management list must not add the retrieval availability default")
	}
	if shouldDefaultAvailableFilter(&types.SearchRequest{}, true) {
		t.Fatal("skill indexes do not use chunk availability")
	}
}

// TestListFilterConditionKeepsDocumentScope pins the condition the management
// chunk list hands to Infinity (doc_id + optional chunk ids + "no compiled rows").
// A filter-only search that reaches the engine with an empty condition degenerates
// into a scan of the whole table, which is what makes the chunk panel render other
// documents' rows — so every clause must survive the conversion, and a scope that
// carries no ids must not turn into a dangling clause.
func TestListFilterConditionKeepsDocumentScope(t *testing.T) {
	const (
		docID   = "d7c44b97f2544a3e99a8ca698e25ab0f"
		chunkID = "6877433186c7e860"
	)
	cases := []struct {
		name     string
		filter   map[string]interface{}
		contains []string
		excludes []string
	}{
		{
			name: "document scope only",
			filter: map[string]interface{}{
				"doc_id":   docID,
				"must_not": map[string]interface{}{"exists": "compile_kwd"},
			},
			contains: []string{fmt.Sprintf("doc_id='%s'", docID), "NOT (compile_kwd!='')"},
		},
		{
			name: "document scope plus one chunk id",
			filter: map[string]interface{}{
				"doc_id":   docID,
				"must_not": map[string]interface{}{"exists": "compile_kwd"},
				"id":       []string{chunkID},
			},
			contains: []string{
				fmt.Sprintf("doc_id='%s'", docID),
				"NOT (compile_kwd!='')",
				fmt.Sprintf("id='%s'", chunkID),
			},
		},
		{
			name: "document scope plus several chunk ids",
			filter: map[string]interface{}{
				"doc_id":   docID,
				"must_not": map[string]interface{}{"exists": "compile_kwd"},
				"id":       []string{chunkID, "a1b2c3d4e5f60718"},
			},
			contains: []string{
				fmt.Sprintf("doc_id='%s'", docID),
				"NOT (compile_kwd!='')",
				fmt.Sprintf("id IN ('%s', 'a1b2c3d4e5f60718')", chunkID),
			},
		},
		{
			// A negation on its own is still a condition: it must not collapse to
			// an empty string and turn the query into a table scan.
			name:     "negation without a document scope",
			filter:   map[string]interface{}{"must_not": map[string]interface{}{"exists": "compile_kwd"}},
			contains: []string{"NOT (compile_kwd!='')"},
		},
		{
			// An empty id list means "no chunk filter", not "match id ''": the
			// condition must stay exactly document scope + negation.
			name: "empty chunk id list adds no clause",
			filter: map[string]interface{}{
				"doc_id":   docID,
				"must_not": map[string]interface{}{"exists": "compile_kwd"},
				"id":       []string{},
			},
			contains: []string{fmt.Sprintf("doc_id='%s'", docID), "NOT (compile_kwd!='')"},
			excludes: []string{"AND id", "id IN"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			condition := equivalentConditionToStr(tc.filter, nil)
			if condition == "" {
				t.Fatal("filter-only chunk list must not reach the engine without a condition")
			}
			for _, want := range tc.contains {
				if !strings.Contains(condition, want) {
					t.Errorf("condition %q is missing %q", condition, want)
				}
			}
			for _, unwanted := range tc.excludes {
				if strings.Contains(condition, unwanted) {
					t.Errorf("condition %q must not contain %q", condition, unwanted)
				}
			}
		})
	}
}

// buildRegexpFilter must keep the caller's document scope, drop kb_id (the
// per-dataset table name already scopes the query), and append the regex()
// predicate with the case-insensitive inline flag on the content column.
func TestBuildRegexpFilterScopesAndMatchesContent(t *testing.T) {
	got := buildRegexpFilter(map[string]interface{}{
		"available_int": 1,
		"doc_id":        []string{"d1", "d2"},
		"must_not":      map[string]interface{}{"exists": "compile_kwd"},
		"kb_id":         "kb-must-not-appear",
	}, `foo.*bar`)

	for _, want := range []string{
		"regex(content, '(?i)foo.*bar')",
		"available_int=1",
		"doc_id IN ('d1', 'd2')",
		"NOT (compile_kwd!='')",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("buildRegexpFilter() = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "kb_id") {
		t.Errorf("buildRegexpFilter() = %q, must not carry kb_id into the per-dataset table filter", got)
	}
}

// A pattern quoted in SQL-ish filter syntax must double its single quotes, or
// the SDK's expression parser would read a truncated literal.
func TestBuildRegexpFilterEscapesSingleQuotes(t *testing.T) {
	got := buildRegexpFilter(nil, `it's a match`)
	if want := `regex(content, '(?i)it''s a match')`; got != want {
		t.Errorf("buildRegexpFilter() = %q, want %q", got, want)
	}
}

// A regexp search from the agent's grep tools carries only a pattern and no
// scope filter; the predicate itself must still be well-formed.
func TestBuildRegexpFilterWithoutScope(t *testing.T) {
	if got, want := buildRegexpFilter(nil, `.*`), `regex(content, '(?i).*')`; got != want {
		t.Errorf("buildRegexpFilter() = %q, want %q", got, want)
	}
}

// An unscoped regexp search must fail rather than scan the bare ragflow_<tenant> table:
// chunk tables are per dataset, so an empty KbIDs would come back as an error-free empty
// result. The check runs before the client is touched, so a zero Engine pins it.
func TestSearchByRegexpRejectsEmptyScope(t *testing.T) {
	eng := &Engine{}
	for _, kbIDs := range [][]string{nil, {}, {""}, {"", "  "}} {
		_, err := eng.SearchByRegexp(context.Background(), &types.RegexpSearchRequest{
			TenantID: "tenant1",
			KbIDs:    kbIDs,
			Pattern:  "foo.*bar",
		})
		if err == nil {
			t.Fatalf("SearchByRegexp(KbIDs=%v) must reject an empty dataset scope", kbIDs)
		}
		if !strings.Contains(err.Error(), "at least one dataset id") {
			t.Errorf("SearchByRegexp(KbIDs=%v) error = %v, want it to name the missing dataset scope", kbIDs, err)
		}
	}
}
