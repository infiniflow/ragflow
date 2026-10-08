package types

import "testing"

func TestCompilationKindPrefersCanonicalValue(t *testing.T) {
	for _, test := range []struct {
		row  map[string]any
		want string
	}{
		{map[string]any{"compile_kwd": "graph"}, "graph"},
		{map[string]any{"compile_kwd": "timeline", "compilation_template_kind_kwd": "knowledge_graph"}, "graph"},
		{map[string]any{"compile_kwd": "list", "compilation_template_kind_kwd": "page_index"}, "page_index"},
		{map[string]any{"compile_kwd": "hypergraph"}, "graph"},
		{map[string]any{"compile_kwd": []string{"mindmap"}}, "mind_map"},
		{map[string]any{"compile_kwd": "wiki_section"}, "wiki"},
	} {
		if got := CompilationKind(test.row); got != test.want {
			t.Errorf("CompilationKind(%v) = %q, want %q", test.row, got, test.want)
		}
	}
}

func TestCompilationFilterReadsMixedGenerations(t *testing.T) {
	for _, test := range []struct {
		name        string
		filter, row map[string]any
		want        bool
	}{
		{"new graph", map[string]any{"compile_kwd": "graph", "kb_id": "kb1"}, map[string]any{"compile_kwd": "graph", "kb_id": "kb1"}, true},
		{"old graph", map[string]any{"compile_kwd": "graph"}, map[string]any{"compile_kwd": "hypergraph", "compilation_template_kind_kwd": "knowledge_graph"}, true},
		{"old inferred graph", map[string]any{"compile_kwd": "graph"}, map[string]any{"compile_kwd": "hypergraph"}, true},
		{"old page index inferred list", map[string]any{"compile_kwd": "page_index"}, map[string]any{"compile_kwd": "list", "compilation_template_kind_kwd": "page_index"}, true},
		{"no category mixing", map[string]any{"compile_kwd": "graph"}, map[string]any{"compile_kwd": "list", "compilation_template_kind_kwd": "page_index"}, false},
		{"canonical row has no old stamp", map[string]any{"compilation_template_kind_kwd": "timeline"}, map[string]any{"compile_kwd": "graph"}, false},
		{"old stamp preserves original kind", map[string]any{"compile_kwd": "graph"}, map[string]any{"compile_kwd": "timeline", "compilation_template_kind_kwd": "knowledge_graph"}, true},
		{"old subtype is not original kind", map[string]any{"compile_kwd": "timeline"}, map[string]any{"compile_kwd": "timeline", "compilation_template_kind_kwd": "knowledge_graph"}, false},
		{"dataset stays scoped", map[string]any{"compile_kwd": "graph", "kb_id": "kb1"}, map[string]any{"compile_kwd": "graph", "kb_id": "kb2"}, false},
		{"new wiki page", map[string]any{"compile_kwd": "wiki_page"}, map[string]any{"compile_kwd": "wiki", "type_kwd": "wiki_page"}, true},
		{"old wiki page", map[string]any{"type_kwd": "wiki_page"}, map[string]any{"compile_kwd": "wiki_page"}, true},
		{"section excluded", map[string]any{"compile_kwd": "wiki_page"}, map[string]any{"compile_kwd": "wiki", "type_kwd": "wiki_section"}, false},
		{"state excluded", map[string]any{"compile_kwd": "wiki_page"}, map[string]any{"compile_kwd": "wiki", "type_kwd": "wiki_map_active"}, false},
		{"new navigation", map[string]any{"compile_kwd": "dataset_nav"}, map[string]any{"compile_kwd": "page_index", "type_kwd": "nav_doc"}, true},
		{"structure excluded", map[string]any{"compile_kwd": "dataset_nav"}, map[string]any{"compile_kwd": "page_index", "type_kwd": "entity"}, false},
		{"new category", map[string]any{"page_type_kwd": "concept"}, map[string]any{"compile_kwd": "wiki", "entity_type_kwd": "concept"}, true},
		{"old category", map[string]any{"page_type_kwd": "concept"}, map[string]any{"compile_kwd": "wiki_page", "page_type_kwd": "concept"}, true},
		{"category wins", map[string]any{"page_type_kwd": "entity"}, map[string]any{"compile_kwd": "wiki", "entity_type_kwd": "concept", "page_type_kwd": "entity"}, false},
		{"mind map type reads old value", map[string]any{"entity_type_kwd": "mind_map"}, map[string]any{"entity_type_kwd": "mindmap"}, true},
		{"old mind map type reads new value", map[string]any{"entity_type_kwd": []string{"mindmap"}}, map[string]any{"entity_type_kwd": "mind_map"}, true},
		{"mind map type excludes other types", map[string]any{"entity_type_kwd": []any{"mind_map"}}, map[string]any{"entity_type_kwd": "person"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := matchesCompilationFilter(test.row, CompilationFilter(test.filter)); got != test.want {
				t.Fatalf("matched = %v, want %v", got, test.want)
			}
		})
	}
}

func matchesCompilationFilter(row, filter map[string]any) bool {
	for field, value := range filter {
		switch field {
		case "and":
			for _, child := range FilterClauses(value) {
				if !matchesCompilationFilter(row, child) {
					return false
				}
			}
		case "or":
			matched := false
			for _, child := range FilterClauses(value) {
				matched = matched || matchesCompilationFilter(row, child)
			}
			if !matched {
				return false
			}
		case "must_not":
			if matchesCompilationFilter(row, value.(map[string]any)) {
				return false
			}
		case "exists":
			if compilationString(row[value.(string)]) == "" {
				return false
			}
		default:
			matched := false
			for _, actual := range compilationValues(row[field]) {
				for _, expected := range compilationValues(value) {
					matched = matched || actual == expected
				}
			}
			if !matched {
				return false
			}
		}
	}
	return true
}
