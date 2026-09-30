package document

import (
	"strings"
	"testing"

	"ragflow/internal/entity"
	ingestiontable "ragflow/internal/ingestion/table"
)

func columnOverride(mode string, roles map[string]string) map[string]interface{} {
	wire := make(map[string]interface{}, len(roles))
	for key, role := range roles {
		wire[key] = role
	}
	return map[string]interface{}{
		"TableChunker:DatasetNode": map[string]interface{}{
			"column_mode":  mode,
			"column_roles": wire,
		},
	}
}

// TestApplyColumnOverrideMapsToTheSoleNode: uploading a spreadsheet into a
// dataset configured with another template switches the document to the table
// pipeline, whose TableChunker node has a different id. The request cannot be
// expected to know that id.
func TestApplyColumnOverrideMapsToTheSoleNode(t *testing.T) {
	config := entity.JSONMap{
		"TableChunker:TableOnlyNode": map[string]interface{}{
			"column_mode":     ingestiontable.ModeAuto,
			"column_roles":    map[string]interface{}{"旧列": ingestiontable.RoleMetadata},
			"enable_children": true,
		},
		"Tokenizer:SomeNode": map[string]interface{}{"chunk_token_size": 256},
	}

	out, err := applyColumnOverride(config, columnOverride(ingestiontable.ModeManual, map[string]string{"金额": "metadata"}))
	if err != nil {
		t.Fatalf("override refused: %v", err)
	}
	node, ok := out["TableChunker:TableOnlyNode"].(map[string]interface{})
	if !ok {
		t.Fatalf("node lost: %v", out)
	}
	if node["column_mode"] != ingestiontable.ModeManual {
		t.Errorf("column_mode = %v", node["column_mode"])
	}
	// A submitted role map replaces the dataset's, so a stale role cannot
	// survive a re-upload that dropped it.
	roles, _ := node["column_roles"].(map[string]string)
	if _, stale := roles["旧列"]; stale {
		t.Errorf("a role the override did not carry survived: %v", roles)
	}
	if node["enable_children"] != true {
		t.Errorf("an unrelated node parameter was lost: %v", node)
	}
	if _, stray := out["TableChunker:DatasetNode"]; stray {
		t.Errorf("the override created a node the document does not run: %v", out)
	}
	if tok := out["Tokenizer:SomeNode"].(map[string]interface{})["chunk_token_size"]; tok != 256 {
		t.Errorf("another node changed: %v", out)
	}
}

// TestApplyColumnOverrideRequiresAnExactNodeWhenSeveralExist: with more than one
// TableChunker in the canvas there is no defensible guess about which rows the
// roles belong to, and writing to the wrong one would silently index the wrong
// configuration.
func TestApplyColumnOverrideRequiresAnExactNodeWhenSeveralExist(t *testing.T) {
	config := entity.JSONMap{
		"TableChunker:Alpha": map[string]interface{}{"column_mode": ingestiontable.ModeAuto},
		"TableChunker:Beta":  map[string]interface{}{"column_mode": ingestiontable.ModeAuto},
	}

	_, err := applyColumnOverride(config, columnOverride(ingestiontable.ModeManual, nil))
	if err == nil {
		t.Fatal("an ambiguous override was accepted")
	}
	if !strings.Contains(err.Error(), "several TableChunker nodes") {
		t.Errorf("error does not explain the ambiguity: %v", err)
	}

	// Naming one of them is unambiguous.
	out, err := applyColumnOverride(config, map[string]interface{}{
		"TableChunker:Beta": map[string]interface{}{"column_mode": ingestiontable.ModeManual},
	})
	if err != nil {
		t.Fatalf("explicit node refused: %v", err)
	}
	if out["TableChunker:Beta"].(map[string]interface{})["column_mode"] != ingestiontable.ModeManual {
		t.Errorf("named node not updated: %v", out)
	}
	if out["TableChunker:Alpha"].(map[string]interface{})["column_mode"] != ingestiontable.ModeAuto {
		t.Errorf("the other node changed: %v", out)
	}
}

func TestApplyColumnOverrideWithoutAnyNode(t *testing.T) {
	config := entity.JSONMap{"Tokenizer:SomeNode": map[string]interface{}{"chunk_token_size": 256}}
	_, err := applyColumnOverride(config, columnOverride(ingestiontable.ModeManual, nil))
	if err == nil || !strings.Contains(err.Error(), "no TableChunker node") {
		t.Errorf("error = %v, want the missing-node case named", err)
	}
}

func TestApplyColumnOverrideRefusesBadValues(t *testing.T) {
	config := entity.JSONMap{
		"TableChunker:Only": map[string]interface{}{"column_mode": ingestiontable.ModeAuto},
	}
	cases := []struct {
		name     string
		override map[string]interface{}
	}{
		{"unknown mode", map[string]interface{}{"TableChunker:Only": map[string]interface{}{"column_mode": "assist"}}},
		{"unknown role", map[string]interface{}{"TableChunker:Only": map[string]interface{}{"column_roles": map[string]interface{}{"金额": "keyword"}}}},
		{"roles not an object", map[string]interface{}{"TableChunker:Only": map[string]interface{}{"column_roles": "金额"}}},
		{"node value not an object", map[string]interface{}{"TableChunker:Only": "manual"}},
		{"non-node key", map[string]interface{}{"table_column_mode": "manual"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := applyColumnOverride(config, c.override); err == nil {
				t.Errorf("accepted %v", c.override)
			}
		})
	}
}

func TestApplyColumnOverrideWithoutOverrideIsIdentity(t *testing.T) {
	config := entity.JSONMap{"TableChunker:Only": map[string]interface{}{"column_mode": ingestiontable.ModeManual}}
	out, err := applyColumnOverride(config, nil)
	if err != nil {
		t.Fatalf("empty override: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("config changed: %v", out)
	}
}
