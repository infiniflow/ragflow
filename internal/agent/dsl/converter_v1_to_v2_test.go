// Package dsl — tests for v1 -> v2 conversion.
//
// These tests load real v1 templates shipped under agent/templates/*.json
// plus a few synthetic fixtures covering edge cases.
package dsl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// templatesDir is the on-disk location of the v1 DSL templates used as
// fixtures. Tests skip (rather than fail) if the directory is missing so
// the package can be built in environments where the Python side is
// pruned.
func templatesDir() string {
	// Walk up from the test file's working directory to the repo root.
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "agent", "templates")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func loadTemplate(t *testing.T, name string) []byte {
	t.Helper()
	dir := templatesDir()
	if dir == "" {
		t.Skip("agent/templates not found; skipping v1 fixture")
	}
	// Templates are wrapped in {"id": ..., "title": ..., "dsl": {...}}.
	// The v1 converter expects the raw "dsl" object, so we extract it.
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Skipf("template %s not readable: %v", name, err)
	}
	var wrapped struct {
		DSL json.RawMessage `json:"dsl"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		t.Fatalf("template %s: parse wrapper: %v", name, err)
	}
	if len(wrapped.DSL) == 0 {
		t.Fatalf("template %s: missing dsl field", name)
	}
	return wrapped.DSL
}

func TestV1ToV2_WebSearchAssistant(t *testing.T) {
	v1 := loadTemplate(t, "web_search_assistant.json")
	c, err := v1ToV2(v1)
	if err != nil {
		t.Fatalf("v1ToV2(web_search_assistant): %v", err)
	}
	if got, want := c.Version, CurrentVersion; got != want {
		t.Fatalf("Version = %d, want %d", got, want)
	}
	if len(c.Components) < 5 {
		t.Fatalf("expected >=5 components, got %d", len(c.Components))
	}
	// All v2 IDs must follow name_uuid (lowercased) and not contain ":".
	for id, cpn := range c.Components {
		if strings.Contains(id, ":") {
			t.Errorf("v2 id %q still contains ':'", id)
		}
		if cpn.ID != id {
			t.Errorf("key %q != Component.ID %q", id, cpn.ID)
		}
		if cpn.Name == "" {
			t.Errorf("component %q has empty Name", id)
		}
		// Downstream refs must resolve inside the same canvas.
		for _, ds := range cpn.Downstream {
			if _, ok := c.Components[ds]; !ok {
				t.Errorf("component %q downstream %q does not exist", id, ds)
			}
		}
	}
	// Validate the result end-to-end.
	if err := c.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
	// Spot-check a known component by its v1 key.
	// web_search_assistant.json contains an "Agent:SmartSchoolsCross" entry;
	// after conversion its v2 id is "agent_smartschoolscross".
	const wantID = "agent_smartschoolscross"
	cpn, ok := c.Components[wantID]
	if !ok {
		t.Fatalf("expected v2 id %q; not found (have %d components)", wantID, len(c.Components))
	}
	if cpn.Name != "Agent" {
		t.Errorf("Name = %q, want %q", cpn.Name, "Agent")
	}
}

func TestV1ToV2_CustomerFeedback(t *testing.T) {
	v1 := loadTemplate(t, "customer_feedback_dispatcher.json")
	c, err := v1ToV2(v1)
	if err != nil {
		t.Fatalf("v1ToV2(customer_feedback_dispatcher): %v", err)
	}
	if c.Version != CurrentVersion {
		t.Fatalf("Version = %d, want %d", c.Version, CurrentVersion)
	}
	if len(c.Components) == 0 {
		t.Fatal("expected non-empty Components")
	}
	// Component names must be preserved as PascalCase class names
	// (e.g. "Categorize", "LLM", "Message").
	seen := map[string]bool{}
	for id, cpn := range c.Components {
		if seen[cpn.Name] {
			continue
		}
		seen[cpn.Name] = true
		if cpn.Name == "" {
			t.Errorf("component %q has empty Name", id)
		}
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestV1ToV2_EdgeNoObj(t *testing.T) {
	// v1 component with no `obj` sub-object: downstream preserved,
	// Params becomes an empty map.
	v1 := []byte(`{
		"components": {
			"Begin:NoObjOne": {
				"downstream": ["Message:NoObjTwo"]
			},
			"Message:NoObjTwo": {
				"obj": {
					"component_name": "Message",
					"params": {}
				}
			}
		}
	}`)
	c, err := v1ToV2(v1)
	if err != nil {
		t.Fatalf("v1ToV2: %v", err)
	}
	begin, ok := c.Components["begin_noobjone"]
	if !ok {
		t.Fatalf("expected v2 id %q", "begin_noobjone")
	}
	if begin.Name != "Begin" {
		t.Errorf("Name = %q, want %q", begin.Name, "Begin")
	}
	if len(begin.Downstream) != 1 || begin.Downstream[0] != "message_noobjtwo" {
		t.Errorf("Downstream = %v, want [message_noobjtwo]", begin.Downstream)
	}
	if begin.Params == nil {
		t.Error("Params should be non-nil empty map (not nil)")
	}
	if len(begin.Params) != 0 {
		t.Errorf("Params should be empty, got %v", begin.Params)
	}
}

func TestV1ToV2_StripLegacy(t *testing.T) {
	// v1 component with the three deprecated param sets: they must NOT
	// appear anywhere on the v2 output.
	v1 := []byte(`{
		"components": {
			"Retrieval:LegacyOne": {
				"downstream": [],
				"obj": {
					"component_name": "Retrieval",
					"params": {"k": 5},
					"_feeded_deprecated_params": {"old_k": 5},
					"_deprecated_params":        {"removed": true},
					"_user_feeded_params":       {"k": 7}
				},
				"_deprecated_params": {"top": 99}
			}
		}
	}`)
	c, err := v1ToV2(v1)
	if err != nil {
		t.Fatalf("v1ToV2: %v", err)
	}
	cpn, ok := c.Components["retrieval_legacyone"]
	if !ok {
		t.Fatalf("expected v2 id %q", "retrieval_legacyone")
	}
	// Round-trip through JSON and grep for the legacy keys.
	bs, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, banned := range []string{
		"_feeded_deprecated_params",
		"_deprecated_params",
		"_user_feeded_params",
	} {
		if strings.Contains(string(bs), banned) {
			t.Errorf("v2 JSON still contains legacy key %q: %s", banned, string(bs))
		}
	}
	// And the kept param "k" must equal 5 (the canonical v1 value, not
	// the user_feeded override).
	if got, ok := cpn.Params["k"]; !ok {
		t.Error("Params[k] missing")
	} else if got != float64(5) {
		// JSON numbers decode as float64.
		t.Errorf("Params[k] = %v, want 5", got)
	}
}

func TestV1ToV2_CustomHeaderPreserved(t *testing.T) {
	// `custom_header` injection inside params must be preserved as-is.
	v1 := []byte(`{
		"components": {
			"HTTP:CustomHeaderOne": {
				"downstream": [],
				"obj": {
					"component_name": "HTTP",
					"params": {
						"url": "https://example.com",
						"custom_header": {"X-Trace": "abc"}
					}
				}
			}
		}
	}`)
	c, err := v1ToV2(v1)
	if err != nil {
		t.Fatalf("v1ToV2: %v", err)
	}
	cpn := c.Components["http_customheaderone"]
	if cpn.Params["custom_header"] == nil {
		t.Fatal("custom_header was stripped on v1->v2")
	}
}

func TestV1ToV2_EmptyDownstream(t *testing.T) {
	// v1 component with no `downstream` key: should produce empty slice.
	v1 := []byte(`{
		"components": {
			"End:TerminalOne": {
				"obj": {
					"component_name": "End",
					"params": {}
				}
			}
		}
	}`)
	c, err := v1ToV2(v1)
	if err != nil {
		t.Fatalf("v1ToV2: %v", err)
	}
	cpn := c.Components["end_terminalone"]
	if cpn.Downstream == nil {
		t.Error("Downstream should be empty slice, not nil")
	}
	if len(cpn.Downstream) != 0 {
		t.Errorf("Downstream should be empty, got %v", cpn.Downstream)
	}
}

func TestConvertKey(t *testing.T) {
	cases := []struct {
		old, newID, name string
		wantErr          bool
	}{
		{"Agent:SmartSchoolsCross", "agent_smartschoolscross", "Agent", false},
		{"Begin:NoObjOne", "begin_noobjone", "Begin", false},
		{"Message:NoObjTwo", "message_noobjtwo", "Message", false},
		{"LLM:LLM_Foo", "llm_llm_foo", "LLM", false},
		{":", "", "", true},
		{"NoColon", "nocolon_", "NoColon", false},
	}
	for _, tc := range cases {
		t.Run(tc.old, func(t *testing.T) {
			id, name, err := convertKey(tc.old)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if id != tc.newID {
				t.Errorf("id = %q, want %q", id, tc.newID)
			}
			if name != tc.name {
				t.Errorf("name = %q, want %q", name, tc.name)
			}
		})
	}
}
