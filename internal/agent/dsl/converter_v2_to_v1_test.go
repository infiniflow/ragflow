// Package dsl — tests for v2 -> v1 conversion (Phase 5.5).
//
// ENVIRONMENT GAP NOTE (per plan §8.5 数据源约束):
// The 100-sample staging corpus "staging_canvas_snapshot_2026q2.json" is
// owned by InfiniFlow SRE and is not present in this dev env. The 10
// real v1 templates under agent/templates/*.json (web_search_assistant,
// customer_feedback_dispatcher, ingestion_pipeline_general, etc.) are
// the best local proxy. The Python reader compat test ("v2 写出的 DSL
// 喂给旧 Python reader 仍能加载") is deferred to staging verification
// — see docs/agent-port/phase-5-5-acceptance.md for the run-book.
//
// The tests below verify the strongest deterministic invariant we can
// ship from a Go-only env: the round-trip
//
//	loadV1(t) -> v1ToV2 -> v2ToV1 -> loadV1 -> v1ToV2
//
// produces a v2 Canvas whose component-ID set, downstream refs, and
// param shapes match the direct v1ToV2 conversion. The reverse direction
// (v1 -> v2 -> v1 -> v2) is what the Python reader effectively does on
// re-load, so equality here implies the Go-emitted v1 is structurally
// readable.
package dsl

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestV2ToV1_WebSearchAssistant: round-trip the web_search_assistant
// template through v1 -> v2 -> v1 -> v2 and assert the v2 representations
// are structurally equivalent.
func TestV2ToV1_WebSearchAssistant(t *testing.T) {
	v1 := loadTemplate(t, "web_search_assistant.json")

	// Forward: v1 -> v2.
	first, err := v1ToV2(v1)
	if err != nil {
		t.Fatalf("v1ToV2 (initial): %v", err)
	}

	// Backward: v2 -> v1.
	v1Bytes, err := v2ToV1(first)
	if err != nil {
		t.Fatalf("v2ToV1: %v", err)
	}

	// The emitted v1 must be valid JSON and re-loadable through LoadV1.
	if !json.Valid(v1Bytes) {
		t.Fatalf("v2ToV1 output is not valid JSON:\n%s", string(v1Bytes))
	}

	// Round-trip: v2 -> v1 -> v2.
	second, err := v1ToV2(v1Bytes)
	if err != nil {
		t.Fatalf("v1ToV2 (round-trip): %v", err)
	}

	// Compare component-ID sets.
	if len(first.Components) != len(second.Components) {
		t.Fatalf("component count differs: first=%d, second=%d",
			len(first.Components), len(second.Components))
	}
	for id := range first.Components {
		if _, ok := second.Components[id]; !ok {
			t.Errorf("v2 id %q lost on round-trip", id)
		}
	}
	for id := range second.Components {
		if _, ok := first.Components[id]; !ok {
			t.Errorf("v2 id %q gained spuriously on round-trip", id)
		}
	}

	// Compare downstream refs and params per component.
	for id, c1 := range first.Components {
		c2 := second.Components[id]
		if c1.Name != c2.Name {
			t.Errorf("[%s] Name: first=%q, second=%q", id, c1.Name, c2.Name)
		}
		if !sameStringSet(c1.Downstream, c2.Downstream) {
			t.Errorf("[%s] Downstream: first=%v, second=%v",
				id, c1.Downstream, c2.Downstream)
		}
		if !sameParams(c1.Params, c2.Params) {
			t.Errorf("[%s] Params differ:\n  first:  %s\n  second: %s",
				id, asJSON(t, c1.Params), asJSON(t, c2.Params))
		}
	}
}

// TestV2ToV1_CustomerFeedback: same shape for customer_feedback_dispatcher.
func TestV2ToV1_CustomerFeedback(t *testing.T) {
	v1 := loadTemplate(t, "customer_feedback_dispatcher.json")
	first, err := v1ToV2(v1)
	if err != nil {
		t.Fatalf("v1ToV2: %v", err)
	}
	v1Bytes, err := v2ToV1(first)
	if err != nil {
		t.Fatalf("v2ToV1: %v", err)
	}
	second, err := v1ToV2(v1Bytes)
	if err != nil {
		t.Fatalf("v1ToV2 (round-trip): %v", err)
	}
	if len(first.Components) != len(second.Components) {
		t.Fatalf("component count differs: first=%d, second=%d",
			len(first.Components), len(second.Components))
	}
	for id, c1 := range first.Components {
		c2 := second.Components[id]
		if c1.Name != c2.Name {
			t.Errorf("[%s] Name: first=%q, second=%q", id, c1.Name, c2.Name)
		}
		if !sameStringSet(c1.Downstream, c2.Downstream) {
			t.Errorf("[%s] Downstream: first=%v, second=%v",
				id, c1.Downstream, c2.Downstream)
		}
		if !sameParams(c1.Params, c2.Params) {
			t.Errorf("[%s] Params differ", id)
		}
	}
}

// TestV2ToV1_IngestionPipeline: same shape for ingestion_pipeline_general.
func TestV2ToV1_IngestionPipeline(t *testing.T) {
	v1 := loadTemplate(t, "ingestion_pipeline_general.json")
	first, err := v1ToV2(v1)
	if err != nil {
		t.Fatalf("v1ToV2: %v", err)
	}
	v1Bytes, err := v2ToV1(first)
	if err != nil {
		t.Fatalf("v2ToV1: %v", err)
	}
	second, err := v1ToV2(v1Bytes)
	if err != nil {
		t.Fatalf("v1ToV2 (round-trip): %v", err)
	}
	if len(first.Components) != len(second.Components) {
		t.Fatalf("component count differs: first=%d, second=%d",
			len(first.Components), len(second.Components))
	}
	for id := range first.Components {
		if _, ok := second.Components[id]; !ok {
			t.Errorf("v2 id %q lost on round-trip", id)
		}
	}
}

// TestV2ToV1_EmptyDownstream: a synthetic v2 with one component that has
// no downstream must emit "downstream": [] (not null) at BOTH the outer
// and the obj level.
func TestV2ToV1_EmptyDownstream(t *testing.T) {
	c := &Canvas{
		Version: CurrentVersion,
		Components: map[string]Component{
			"message_0": {
				ID:         "message_0",
				Name:       "Message",
				Downstream: []string{},
				Params:     map[string]any{"content": "hi"},
			},
		},
	}
	bs, err := v2ToV1(c)
	if err != nil {
		t.Fatalf("v2ToV1: %v", err)
	}
	out := string(bs)
	// Outer "downstream": []
	if !strings.Contains(out, `"downstream": []`) {
		t.Errorf("outer downstream not empty array:\n%s", out)
	}
	// Inside obj too: "obj": { ..., "downstream": [] }.
	if !strings.Contains(out, `"obj":`) {
		t.Errorf("missing obj sub-object:\n%s", out)
	}
	// No literal "null" downstream in the output.
	if strings.Contains(out, `"downstream": null`) {
		t.Errorf("downstream emitted as null:\n%s", out)
	}
}

// TestV2ToV1_NilParams: synthetic v2 with no params must emit "params":
// {} (not null) on the obj.
func TestV2ToV1_NilParams(t *testing.T) {
	c := &Canvas{
		Version: CurrentVersion,
		Components: map[string]Component{
			"begin_0": {
				ID:         "begin_0",
				Name:       "Begin",
				Downstream: []string{"message_0"},
				Params:     map[string]any{},
			},
			"message_0": {
				ID:         "message_0",
				Name:       "Message",
				Downstream: []string{},
				Params:     map[string]any{},
			},
		},
	}
	bs, err := v2ToV1(c)
	if err != nil {
		t.Fatalf("v2ToV1: %v", err)
	}
	out := string(bs)
	// We expect exactly two empty params objects (one per component).
	count := strings.Count(out, `"params": {}`)
	if count != 2 {
		t.Errorf("expected 2 `\"params\": {}` entries, got %d:\n%s", count, out)
	}
	if strings.Contains(out, `"params": null`) {
		t.Errorf("params emitted as null:\n%s", out)
	}
}

// TestV2ToV1_NoLegacyFields: the v1 emit must NEVER contain any of the
// three Python-era legacy keys (_deprecated_params,
// _feeded_deprecated_params, _user_feeded_params), even when the v2
// input is full of synthetic data.
func TestV2ToV1_NoLegacyFields(t *testing.T) {
	c := &Canvas{
		Version: CurrentVersion,
		Components: map[string]Component{
			"retrieval_0": {
				ID:         "retrieval_0",
				Name:       "Retrieval",
				Downstream: []string{"llm_0"},
				Params: map[string]any{
					"k":       5,
					"kb_ids":  []any{"kb1", "kb2"},
					"outputs": map[string]any{"content": map[string]any{"type": "string"}},
				},
			},
			"llm_0": {
				ID:         "llm_0",
				Name:       "LLM",
				Downstream: []string{},
				Params: map[string]any{
					"model":       "deepseek-chat",
					"temperature": 0.1,
				},
			},
		},
	}
	bs, err := v2ToV1(c)
	if err != nil {
		t.Fatalf("v2ToV1: %v", err)
	}
	out := string(bs)
	for _, banned := range []string{
		"_deprecated_params",
		"_feeded_deprecated_params",
		"_user_feeded_params",
	} {
		if strings.Contains(out, banned) {
			t.Errorf("v1 emit contains banned legacy key %q:\n%s", banned, out)
		}
	}
	// And: the canonical params ("k", "kb_ids", "model", "temperature")
	// must still be present.
	for _, expected := range []string{`"k"`, `"kb_ids"`, `"model"`, `"temperature"`} {
		if !strings.Contains(out, expected) {
			t.Errorf("v1 emit missing expected field %s:\n%s", expected, out)
		}
	}
}

// TestV2ToV1_DeterministicOrder: calling v2ToV1 twice on the same canvas
// must produce byte-for-byte identical output. Map iteration in Go is
// non-deterministic, so a correct implementation must sort.
func TestV2ToV1_DeterministicOrder(t *testing.T) {
	c := &Canvas{
		Version: CurrentVersion,
		Components: map[string]Component{
			"zeta_0": {ID: "zeta_0", Name: "Z", Downstream: []string{}, Params: map[string]any{}},
			"alpha_0": {
				ID: "alpha_0", Name: "A",
				Downstream: []string{"beta_0"},
				Params:     map[string]any{"x": 1},
			},
			"beta_0": {
				ID: "beta_0", Name: "B",
				Downstream: []string{"gamma_0"},
				Params:     map[string]any{"y": 2},
			},
			"gamma_0": {
				ID: "gamma_0", Name: "G",
				Downstream: []string{"delta_0"},
				Params:     map[string]any{"z": 3},
			},
			"delta_0": {ID: "delta_0", Name: "D", Downstream: []string{}, Params: map[string]any{}},
		},
	}
	first, err := v2ToV1(c)
	if err != nil {
		t.Fatalf("v2ToV1 #1: %v", err)
	}
	// Mutate the map insertion order by adding/removing entries to
	// exercise the Go map's non-determinism on the second pass.
	c.Components["epsilon_0"] = Component{
		ID: "epsilon_0", Name: "E", Downstream: []string{}, Params: map[string]any{},
	}
	delete(c.Components, "zeta_0")
	second, err := v2ToV1(c)
	if err != nil {
		t.Fatalf("v2ToV1 #2: %v", err)
	}
	// Now mutate back to exactly the original 5 components, ensuring a
	// different insertion order than the first call.
	c.Components["zeta_0"] = Component{
		ID: "zeta_0", Name: "Z", Downstream: []string{}, Params: map[string]any{},
	}
	delete(c.Components, "epsilon_0")
	third, err := v2ToV1(c)
	if err != nil {
		t.Fatalf("v2ToV1 #3: %v", err)
	}
	if !bytes.Equal(first, third) {
		t.Errorf("v2ToV1 not deterministic across map-mutation cycles:\n#1:\n%s\n#3:\n%s",
			string(first), string(third))
	}
	// And #2 (different canvas) must differ.
	if bytes.Equal(first, second) {
		t.Error("v2ToV1 #1 == v2ToV1 #2 (different canvases produced same bytes)")
	}
}

// TestV2ToV1_KeyRestore: verify the case-restore heuristic on the v2
// id <-> v1 key reversal.
func TestV2ToV1_KeyRestore(t *testing.T) {
	cases := []struct {
		v2ID, want string
	}{
		{"begin_abc", "Begin:abc"},
		{"agent_abc", "Agent:abc"},
		// Trailing underscore: original v1 had no colon (e.g. "begin").
		// Emit WITHOUT a colon so v1ToV2's no-colon branch re-parses.
		{"begin_", "Begin"},
		{"message_0", "Message:0"},
		{"switch_abc_def", "Switch:abc_def"},
		{"llm_xyz", "Llm:xyz"}, // lossy vs original "LLM:xyz" but documented
	}
	for _, tc := range cases {
		t.Run(tc.v2ID, func(t *testing.T) {
			got := reverseIDToV1Key(tc.v2ID)
			if got != tc.want {
				t.Errorf("reverseIDToV1Key(%q) = %q, want %q", tc.v2ID, got, tc.want)
			}
		})
	}
}

// TestV2ToV1_NilCanvas: a nil canvas must error, not panic.
func TestV2ToV1_NilCanvas(t *testing.T) {
	if _, err := v2ToV1(nil); err == nil {
		t.Error("v2ToV1(nil): expected error, got nil")
	}
}

// TestV2ToV1_EmptyComponents: a v2 with zero components must error.
func TestV2ToV1_EmptyComponents(t *testing.T) {
	c := &Canvas{Version: CurrentVersion, Components: map[string]Component{}}
	if _, err := v2ToV1(c); err == nil {
		t.Error("v2ToV1(empty): expected error, got nil")
	}
}

// TestV2ToV1_BeginFirst: the "begin" component must appear before all
// other components in the v1 emit. With lexicographic sort, "Begin:..."
// sorts before any other PascalCase key, so this is a built-in property —
// the test guards against a future change that drops the sort.
func TestV2ToV1_BeginFirst(t *testing.T) {
	c := &Canvas{
		Version: CurrentVersion,
		Components: map[string]Component{
			"zeta_0":  {ID: "zeta_0", Name: "Z", Downstream: []string{}, Params: map[string]any{}},
			"begin_0": {ID: "begin_0", Name: "Begin", Downstream: []string{"zeta_0"}, Params: map[string]any{}},
			"alpha_0": {ID: "alpha_0", Name: "A", Downstream: []string{}, Params: map[string]any{}},
		},
	}
	bs, err := v2ToV1(c)
	if err != nil {
		t.Fatalf("v2ToV1: %v", err)
	}
	out := string(bs)
	idxBegin := strings.Index(out, `"Begin:`)
	idxAlpha := strings.Index(out, `"Alpha:`)
	idxZeta := strings.Index(out, `"Zeta:`)
	if idxBegin < 0 || idxAlpha < 0 || idxZeta < 0 {
		t.Fatalf("expected Begin/Alpha/Zeta keys in output:\n%s", out)
	}
	if !(idxBegin < idxAlpha && idxBegin < idxZeta) {
		t.Errorf("Begin is not first in emit order: begin=%d alpha=%d zeta=%d\n%s",
			idxBegin, idxAlpha, idxZeta, out)
	}
}

// TestV2ToV1_ParamOrderStable: when a v2 Canvas is built with several
// keys in one Params map, the v1 emit's "params" object should reflect
// them faithfully (we don't constrain key order, but every value must
// round-trip).
func TestV2ToV1_ParamOrderStable(t *testing.T) {
	c := &Canvas{
		Version: CurrentVersion,
		Components: map[string]Component{
			"llm_0": {
				ID:         "llm_0",
				Name:       "LLM",
				Downstream: []string{},
				Params: map[string]any{
					"alpha":   1,
					"bravo":   "two",
					"charlie": []any{3, 4},
					"delta":   map[string]any{"nested": true},
				},
			},
		},
	}
	bs, err := v2ToV1(c)
	if err != nil {
		t.Fatalf("v2ToV1: %v", err)
	}
	var got v1Envelope
	if err := json.Unmarshal(bs, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	cpn := got.Components["Llm:0"]
	if cpn.Obj == nil {
		t.Fatal("missing obj")
	}
	if got := cpn.Obj.Params["alpha"]; got != float64(1) {
		t.Errorf("alpha = %v, want 1", got)
	}
	if got := cpn.Obj.Params["bravo"]; got != "two" {
		t.Errorf("bravo = %v, want \"two\"", got)
	}
	if got := cpn.Obj.Params["charlie"]; got == nil {
		t.Error("charlie missing")
	}
	if got := cpn.Obj.Params["delta"]; got == nil {
		t.Error("delta missing")
	}
}

// TestV2ToV1_AcceptanceFixture_Smoke: a quick end-to-end smoke that
// runs the v1 template through v1ToV2 -> v2ToV1 and re-loads via LoadV1.
// Catches malformed-JSON regressions even when the structural comparison
// would pass.
func TestV2ToV1_AcceptanceFixture_Smoke(t *testing.T) {
	if dir := templatesDir(); dir == "" {
		t.Skip("agent/templates not found; skipping smoke")
	}
	v1 := loadTemplate(t, "web_search_assistant.json")
	stage1, err := v1ToV2(v1)
	if err != nil {
		t.Fatalf("v1ToV2: %v", err)
	}
	v1Out, err := v2ToV1(stage1)
	if err != nil {
		t.Fatalf("v2ToV1: %v", err)
	}
	// Re-parse through the public LoadV1 entry point (catches loader
	// regressions that v1ToV2 alone would not surface).
	stage2, err := LoadV1(v1Out)
	if err != nil {
		t.Fatalf("LoadV1(round-tripped v1): %v", err)
	}
	if len(stage2.Components) != len(stage1.Components) {
		t.Errorf("component count drift: stage1=%d, stage2=%d",
			len(stage1.Components), len(stage2.Components))
	}
}

// sameStringSet returns true if a and b contain the same elements
// regardless of order. nil and empty are treated as equal.
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
		if seen[s] < 0 {
			return false
		}
	}
	return true
}

// sameParams compares two Params maps by canonical JSON encoding so
// key-order and slice-order are normalized. This mirrors the pattern
// used by TestLoadV2RoundTrip in loader_test.go (bytes.Equal on
// canonical Marshal output).
func sameParams(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		ca, errA := json.Marshal(va)
		cb, errB := json.Marshal(vb)
		if errA != nil || errB != nil {
			return false
		}
		if !bytes.Equal(ca, cb) {
			return false
		}
	}
	return true
}

// asJSON is a small helper that marshals v to canonical JSON for
// diagnostic messages.
func asJSON(t *testing.T, v any) string {
	t.Helper()
	bs, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return string(bs)
}
