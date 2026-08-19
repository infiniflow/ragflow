package pipeline

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestExtractAllComponentParams(t *testing.T) {
	// Minimal DSL with all component types (general template pattern).
	dsl := map[string]any{
		"components": map[string]any{
			"File": map[string]any{
				"obj": map[string]any{
					"component_name": "File",
					"params":         map[string]any{},
				},
			},
			"Parser:HipSignsRhyme": map[string]any{
				"obj": map[string]any{
					"component_name": "Parser",
					"params": map[string]any{
						"outputs": map[string]any{"html": map[string]any{"type": "string"}},
						"setups": map[string]any{
							"pdf":         map[string]any{"parse_method": "deepdoc"},
							"spreadsheet": map[string]any{"parse_method": "deepdoc"},
						},
					},
				},
			},
			"Tokenizer:LegalReadersDecide": map[string]any{
				"obj": map[string]any{
					"component_name": "Tokenizer",
					"params": map[string]any{
						"fields":               "text",
						"filename_embd_weight": 0.1,
						"search_method":        []any{"embedding", "full_text"},
						"outputs":              map[string]any{},
					},
				},
			},
		},
	}
	dslJSON, err := json.Marshal(dsl)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	schemas, err := ExtractAllComponentParams(dslJSON)
	if err != nil {
		t.Fatalf("ExtractAllComponentParams: %v", err)
	}

	// Build a lookup by cpnID for assertion.
	byCPN := make(map[string]ComponentParamsSchema, len(schemas))
	for _, s := range schemas {
		if _, dup := byCPN[s.CpnID]; dup {
			t.Errorf("duplicate cpnID %q in result", s.CpnID)
		}
		byCPN[s.CpnID] = s
	}

	// --- File ---
	file, ok := byCPN["File"]
	if !ok {
		t.Fatal("missing component: File")
	}
	if file.ComponentName != "File" {
		t.Errorf("File.ComponentName = %q, want %q", file.ComponentName, "File")
	}
	if len(file.ParamsDefaults) != 0 {
		t.Errorf("File.ParamsDefaults should be empty, got %v", file.ParamsDefaults)
	}

	// --- Parser ---
	parser, ok := byCPN["Parser:HipSignsRhyme"]
	if !ok {
		t.Fatal("missing component: Parser:HipSignsRhyme")
	}
	if parser.ComponentName != "Parser" {
		t.Errorf("Parser.ComponentName = %q, want %q", parser.ComponentName, "Parser")
	}
	// ParamsDefaults must contain "setups", must NOT contain "outputs".
	if _, ok := parser.ParamsDefaults["setups"]; !ok {
		t.Errorf("Parser.ParamsDefaults missing key %q", "setups")
	}
	if _, ok := parser.ParamsDefaults["outputs"]; ok {
		t.Errorf("Parser.ParamsDefaults should NOT contain %q (excluded)", "outputs")
	}
	if setups, ok := parser.ParamsDefaults["setups"].(map[string]any); !ok {
		t.Errorf("Parser.ParamsDefaults[\"setups\"] is not a map: %T", parser.ParamsDefaults["setups"])
	} else {
		if _, ok := setups["pdf"]; !ok {
			t.Errorf("Parser setups missing key %q", "pdf")
		}
		if _, ok := setups["spreadsheet"]; !ok {
			t.Errorf("Parser setups missing key %q", "spreadsheet")
		}
	}

	// --- Tokenizer ---
	tokenizer, ok := byCPN["Tokenizer:LegalReadersDecide"]
	if !ok {
		t.Fatal("missing component: Tokenizer:LegalReadersDecide")
	}
	if tokenizer.ComponentName != "Tokenizer" {
		t.Errorf("Tokenizer.ComponentName = %q, want %q", tokenizer.ComponentName, "Tokenizer")
	}
	// ParamsDefaults must contain "fields", "filename_embd_weight", "search_method".
	if _, ok := tokenizer.ParamsDefaults["fields"]; !ok {
		t.Errorf("Tokenizer.ParamsDefaults missing key %q", "fields")
	}
	if _, ok := tokenizer.ParamsDefaults["filename_embd_weight"]; !ok {
		t.Errorf("Tokenizer.ParamsDefaults missing key %q", "filename_embd_weight")
	}
	if _, ok := tokenizer.ParamsDefaults["search_method"]; !ok {
		t.Errorf("Tokenizer.ParamsDefaults missing key %q", "search_method")
	}
	if _, ok := tokenizer.ParamsDefaults["outputs"]; ok {
		t.Errorf("Tokenizer.ParamsDefaults should NOT contain %q (excluded)", "outputs")
	}

	// --- No extra/leaked ---
	if len(schemas) != 3 {
		t.Errorf("expected 3 components, got %d: %v", len(schemas), componentIDs(schemas))
	}
}

func TestExtractAllComponentParams_EmptyDSL(t *testing.T) {
	dsl := map[string]any{
		"components": map[string]any{},
	}
	dslJSON, _ := json.Marshal(dsl)
	schemas, err := ExtractAllComponentParams(dslJSON)
	if err != nil {
		t.Fatalf("ExtractAllComponentParams: %v", err)
	}
	if len(schemas) != 0 {
		t.Errorf("expected 0 components, got %d", len(schemas))
	}
}

func TestExtractAllComponentParams_InvalidJSON(t *testing.T) {
	_, err := ExtractAllComponentParams([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestExtractAllComponentParams_MissingComponents(t *testing.T) {
	dsl := map[string]any{"globals": map[string]any{}}
	dslJSON, _ := json.Marshal(dsl)
	_, err := ExtractAllComponentParams(dslJSON)
	if err == nil {
		t.Error("expected error for DSL missing components")
	}
}

func componentIDs(schemas []ComponentParamsSchema) []string {
	ids := make([]string, len(schemas))
	for i, s := range schemas {
		ids[i] = s.CpnID
	}
	slices.Sort(ids)
	return ids
}

func TestExtractAllComponentParams_AllBuiltinTemplates(t *testing.T) {
	entries, err := fs.ReadDir(builtinTemplateFS, "template")
	if err != nil {
		t.Fatalf("read embedded template dir: %v", err)
	}

	var tested int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, builtinTemplatePrefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		tested++
		raw, err := fs.ReadFile(builtinTemplateFS, filepath.Join("template", name))
		if err != nil {
			t.Errorf("%s: read: %v", name, err)
			continue
		}

		// The template file wraps the DSL in {"dsl": {...}}.
		var wrapper struct {
			DSL json.RawMessage `json:"dsl"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			t.Errorf("%s: unmarshal wrapper: %v", name, err)
			continue
		}
		if len(wrapper.DSL) == 0 {
			t.Errorf("%s: missing dsl key", name)
			continue
		}

		schemas, err := ExtractAllComponentParams(wrapper.DSL)
		if err != nil {
			t.Errorf("%s: ExtractAllComponentParams: %v", name, err)
			continue
		}
		if len(schemas) == 0 {
			t.Errorf("%s: expected at least 1 component, got 0", name)
			continue
		}

		// Every schema must have a non-empty CpnID and ComponentName.
		for _, s := range schemas {
			if s.CpnID == "" {
				t.Errorf("%s: component %q has empty CpnID", name, s.ComponentName)
			}
			if s.ComponentName == "" {
				t.Errorf("%s: cpnID %q has empty ComponentName", name, s.CpnID)
			}
			if s.ParamsDefaults == nil {
				t.Errorf("%s: cpnID %q has nil ParamsDefaults", name, s.CpnID)
			}
			// "outputs" must never leak into ParamsDefaults.
			if _, ok := s.ParamsDefaults["outputs"]; ok {
				t.Errorf("%s: cpnID %q ParamsDefaults contains excluded key %q", name, s.CpnID, "outputs")
			}
		}

		t.Logf("%s: %d components: %v", name, len(schemas), componentIDs(schemas))
	}
	if tested == 0 {
		t.Fatal("no builtin templates found")
	}
	t.Logf("tested %d builtin templates", tested)
}
