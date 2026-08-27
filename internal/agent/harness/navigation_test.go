package harness

import (
	"strings"
	"testing"
)

// TestRenderStructureEntities asserts the prompt outline is compact and includes
// entity name/type/description.
func TestRenderStructureEntities(t *testing.T) {
	entities := []structureEntity{
		{Name: "Rocket", Type: "concept", Description: "a propulsion device"},
		{Name: "Engine", Type: "part"},
	}
	out := renderStructureEntities(entities)
	if out == "" {
		t.Fatal("expected rendered outline")
	}
	if !containsAll(out, "Rocket", "Engine", "concept") {
		t.Errorf("rendered outline missing entities: %q", out)
	}
}

// TestNavigateStructure_NoScope asserts empty doc scope yields empty chunks
// without calling the model.
func TestNavigateStructure_NoScope(t *testing.T) {
	out, err := NavigateStructure(t.Context(), "t1", toolOntologyNavigate, structureNavArgs{
		Topic: "rocket", DocScope: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"chunks":[]}` {
		t.Errorf("no-scope result = %q, want empty chunks", out)
	}
}

// TestNormalizeKind mirrors the API's kind normalization.
func TestNormalizeKind(t *testing.T) {
	cases := []struct {
		row  map[string]interface{}
		want string
	}{
		{map[string]interface{}{"compile_kwd": "raptor_graph"}, "raptor"},
		{map[string]interface{}{"compilation_template_kind_kwd": "page_index"}, "timeline"},
		{map[string]interface{}{"compilation_template_kind_kwd": "knowledge_graph"}, "timeline"},
		{map[string]interface{}{"compilation_template_kind_kwd": "mindmap"}, "mindmap"},
		{map[string]interface{}{"compile_kwd": "tree"}, "tree"},
	}
	for _, c := range cases {
		if got := normalizeKind(c.row); got != c.want {
			t.Errorf("normalizeKind(%v) = %q, want %q", c.row, got, c.want)
		}
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
