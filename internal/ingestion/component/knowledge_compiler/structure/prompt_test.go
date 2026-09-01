package structure

import (
	"strings"
	"testing"
)

func TestRenderTypeFieldsExtraFields(t *testing.T) {
	ent := map[string]any{
		"fields": []any{
			map[string]any{"type": "title"},
			map[string]any{"type": "fact"},
		},
		"output_fields": []any{map[string]any{
			"name":        "evidence",
			"type":        "list",
			"required":    false,
			"description": "verbatim source sentences supporting this claim",
			"shape":       `[{"quote": "<verbatim source sentence>", "chunk_id": "<source chunk id>"}]`,
		}},
	}

	lines, skel := renderTypeFields(configFields(ent), "English", "entity", configOutputFields(ent))

	// The skeleton is what fixes the model's output shape. A key described only
	// in a type's rule text but absent here is silently dropped by the model —
	// which is exactly how the first evidence attempt produced zero evidence.
	if !strings.Contains(skel, `"evidence"`) {
		t.Fatalf("evidence missing from skeleton: %s", skel)
	}
	if !strings.Contains(skel, `"quote"`) || !strings.Contains(skel, `"chunk_id"`) {
		t.Fatalf("evidence shape missing quote/chunk_id: %s", skel)
	}
	// The declared shape must render verbatim, not as a generic placeholder.
	if strings.Contains(skel, `"evidence": [...]`) {
		t.Fatalf("shape not honoured, fell back to placeholder: %s", skel)
	}
	if !strings.Contains(lines, "- evidence (list, optional, omit when not applicable)") {
		t.Fatalf("evidence not described in field list: %s", lines)
	}
	// Fixed keys must survive.
	for _, key := range []string{`"type"`, `"name"`, `"description"`, `"source_chunk_ids"`} {
		if !strings.Contains(skel, key) {
			t.Fatalf("fixed key %s dropped from skeleton: %s", key, skel)
		}
	}
}

func TestRenderTypeFieldsExtraFieldsAbsent(t *testing.T) {
	// Templates that declare no output_fields must keep the original skeleton,
	// with no stray separator.
	ent := map[string]any{"fields": []any{map[string]any{"type": "title"}}}
	_, skel := renderTypeFields(configFields(ent), "English", "entity", configOutputFields(ent))
	// Note: the word "evidence" also appears in the fixed description
	// placeholder, so match the key with its quotes, not the bare word.
	if strings.Contains(skel, `"evidence"`) {
		t.Fatalf("skeleton should not carry an evidence key: %s", skel)
	}
	if strings.Contains(skel, ", }") {
		t.Fatalf("dangling separator in skeleton: %s", skel)
	}
}

func TestRenderTypeFieldsFallsBackToPlaceholder(t *testing.T) {
	// Without a `shape`, the declared type picks a sensible placeholder.
	ent := map[string]any{
		"fields": []any{map[string]any{"type": "fact"}},
		"output_fields": []any{
			map[string]any{"name": "evidence", "type": "list"},
			map[string]any{"name": "score", "type": "float"},
			map[string]any{"name": "flag", "type": "bool"},
		},
	}
	_, skel := renderTypeFields(configFields(ent), "English", "entity", configOutputFields(ent))
	for _, want := range []string{`"evidence": [...]`, `"score": <float>`, `"flag": <true|false>`} {
		if !strings.Contains(skel, want) {
			t.Fatalf("expected %s in skeleton: %s", want, skel)
		}
	}
}

func TestRenderTypeFieldsRelationUnaffected(t *testing.T) {
	rel := map[string]any{"fields": []any{map[string]any{"type": "include"}}}
	_, skel := renderTypeFields(configFields(rel), "English", "relation", configOutputFields(rel))
	if !strings.Contains(skel, `"source"`) || !strings.Contains(skel, `"target"`) {
		t.Fatalf("relation skeleton lost its shape: %s", skel)
	}
	if strings.Contains(skel, `"evidence"`) {
		t.Fatalf("relation skeleton should not carry an evidence key: %s", skel)
	}
}
