package pipeline

import (
	"testing"
)

// A canvas saved before TableChunker had column params must still accept the
// new keys: the schema-derived whitelist keeps them even though the DSL
// bakes neither.
func TestCleanComponentParamsKeepsTableChunkerColumnParams(t *testing.T) {
	dslJSON := []byte(`{"components":{"TableChunker:FastFoxesJump":{"obj":{"component_name":"TableChunker","params":{"outputs":{}}}}}}`)
	cleaned := CleanComponentParams(dslJSON, map[string]interface{}{
		"TableChunker:FastFoxesJump": map[string]any{
			"column_mode":  "manual",
			"column_roles": map[string]any{"金额": "metadata"},
		},
	})
	got, ok := cleaned["TableChunker:FastFoxesJump"].(map[string]any)
	if !ok {
		t.Fatalf("TableChunker node missing from cleaned: %#v", cleaned)
	}
	if got["column_mode"] != "manual" {
		t.Errorf("column_mode dropped: %#v", got)
	}
	roles, ok := got["column_roles"].(map[string]any)
	if !ok || roles["金额"] != "metadata" {
		t.Errorf("column_roles dropped: %#v", got["column_roles"])
	}
}
