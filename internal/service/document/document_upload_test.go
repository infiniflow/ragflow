package document

import (
	"strings"
	"testing"

	"ragflow/internal/entity"
)

// TestCloneParserConfigForDocument verifies that cloneParserConfigForDocument must
// deep-copy the nested maps a database load hands out, so writing a component
// entry onto one document's config cannot leak into the dataset's config or a
// sibling document's config.
func TestCloneParserConfigForDocument(t *testing.T) {
	if clone := cloneParserConfigForDocument(nil); clone == nil || len(clone) != 0 {
		t.Fatalf("nil config must clone to an empty object, got %#v", clone)
	}

	original := entity.JSONMap{
		"Parser:HipSignsRhyme": map[string]interface{}{
			"spreadsheet": map[string]interface{}{"output_format": "html"},
		},
		"topn": 10,
	}

	clone := cloneParserConfigForDocument(original)

	nested, ok := clone["Parser:HipSignsRhyme"].(map[string]interface{})
	if !ok {
		t.Fatalf("clone lost the component entry: %#v", clone["Parser:HipSignsRhyme"])
	}
	ss, ok := nested["spreadsheet"].(map[string]interface{})
	if !ok {
		t.Fatalf("clone lost the spreadsheet setup: %#v", nested["spreadsheet"])
	}
	ss["output_format"] = "json"
	clone["topn"] = 42

	origNested := original["Parser:HipSignsRhyme"].(map[string]interface{})["spreadsheet"].(map[string]interface{})
	if origNested["output_format"] != "html" {
		t.Fatalf("original nested map was mutated: %#v", origNested)
	}
	if original["topn"] != 10 {
		t.Fatalf("original top-level map was aliased: topn=%#v", original["topn"])
	}
}

func TestNormalizeWebDocumentName(t *testing.T) {
	pdfBlob := []byte("%PDF-1.4 fake")
	htmlBlob := []byte("<html><body>hi</body></html>")
	cases := []struct {
		name, filename, ct string
		blob               []byte
		want               string
	}{
		{"pdf detected by blob", "report", "application/octet-stream", pdfBlob, "report.pdf"},
		{"html detected by blob", "page", "application/octet-stream", htmlBlob, "page.html"},
		{"dot stripped by utility, no type hint", "image.png", "application/octet-stream", []byte("plain"), "imagepng"},
		{"no hint, no extension", "doc", "application/json", []byte("{}"), "doc"},
	}
	for _, c := range cases {
		if got := normalizeWebDocumentName(c.filename, c.ct, c.blob); got != c.want {
			t.Errorf("%s: normalizeWebDocumentName = %q, want %q", c.name, got, c.want)
		}
	}
}
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
			"column_mode":     entity.TableModeAuto,
			"column_roles":    map[string]interface{}{"旧列": entity.TableRoleMetadata},
			"enable_children": true,
		},
		"Tokenizer:SomeNode": map[string]interface{}{"chunk_token_size": 256},
	}

	out, err := applyColumnOverride(config, columnOverride(entity.TableModeManual, map[string]string{"金额": "metadata"}))
	if err != nil {
		t.Fatalf("override refused: %v", err)
	}
	node, ok := out["TableChunker:TableOnlyNode"].(map[string]interface{})
	if !ok {
		t.Fatalf("node lost: %v", out)
	}
	if node["column_mode"] != entity.TableModeManual {
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
		"TableChunker:Alpha": map[string]interface{}{"column_mode": entity.TableModeAuto},
		"TableChunker:Beta":  map[string]interface{}{"column_mode": entity.TableModeAuto},
	}

	_, err := applyColumnOverride(config, columnOverride(entity.TableModeManual, nil))
	if err == nil {
		t.Fatal("an ambiguous override was accepted")
	}
	if !strings.Contains(err.Error(), "several TableChunker nodes") {
		t.Errorf("error does not explain the ambiguity: %v", err)
	}

	// Naming one of them is unambiguous.
	out, err := applyColumnOverride(config, map[string]interface{}{
		"TableChunker:Beta": map[string]interface{}{"column_mode": entity.TableModeManual},
	})
	if err != nil {
		t.Fatalf("explicit node refused: %v", err)
	}
	if out["TableChunker:Beta"].(map[string]interface{})["column_mode"] != entity.TableModeManual {
		t.Errorf("named node not updated: %v", out)
	}
	if out["TableChunker:Alpha"].(map[string]interface{})["column_mode"] != entity.TableModeAuto {
		t.Errorf("the other node changed: %v", out)
	}
}

func TestApplyColumnOverrideWithoutAnyNode(t *testing.T) {
	config := entity.JSONMap{"Tokenizer:SomeNode": map[string]interface{}{"chunk_token_size": 256}}
	_, err := applyColumnOverride(config, columnOverride(entity.TableModeManual, nil))
	if err == nil || !strings.Contains(err.Error(), "no TableChunker node") {
		t.Errorf("error = %v, want the missing-node case named", err)
	}
}

func TestApplyColumnOverrideRefusesBadValues(t *testing.T) {
	config := entity.JSONMap{
		"TableChunker:Only": map[string]interface{}{"column_mode": entity.TableModeAuto},
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
	config := entity.JSONMap{"TableChunker:Only": map[string]interface{}{"column_mode": entity.TableModeManual}}
	out, err := applyColumnOverride(config, nil)
	if err != nil {
		t.Fatalf("empty override: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("config changed: %v", out)
	}
}
