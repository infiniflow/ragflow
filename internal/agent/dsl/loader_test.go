// Package dsl — tests for the version auto-detection loader.
package dsl

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestDetectVersion_V1(t *testing.T) {
	v1 := []byte(`{
		"components": {
			"Agent:SmartSchoolsCross": {
				"downstream": ["Message:ShaggyRingsCrash"],
				"obj": {"component_name": "Agent", "params": {}}
			}
		}
	}`)
	v, err := DetectVersion(v1)
	if err != nil {
		t.Fatalf("DetectVersion: %v", err)
	}
	if v != V1 {
		t.Errorf("version = %s, want v1", v)
	}
}

func TestDetectVersion_V2(t *testing.T) {
	v2 := []byte(`{
		"version": 2,
		"components": {
			"begin_0": {
				"id": "begin_0",
				"name": "Begin",
				"downstream": ["llm_0"],
				"params": {}
			},
			"llm_0": {
				"id": "llm_0",
				"name": "LLM",
				"downstream": [],
				"params": {"model": "deepseek-chat"}
			}
		}
	}`)
	v, err := DetectVersion(v2)
	if err != nil {
		t.Fatalf("DetectVersion: %v", err)
	}
	if v != V2 {
		t.Errorf("version = %s, want v2", v)
	}
}

func TestDetectVersion_Unknown(t *testing.T) {
	// `version` is set to 99 (not 2), and `components` is empty. Probe 1
	// fails the V2 check, probe 2 sees an empty components map and
	// rejects the payload.
	bogus := []byte(`{"version": 99, "components": {}}`)
	if _, err := DetectVersion(bogus); err == nil {
		t.Error("expected error for unknown version, got nil")
	}

	// No `version` field at all and no `components` map: rejected.
	noComponents := []byte(`{"something": "else"}`)
	if _, err := DetectVersion(noComponents); err == nil {
		t.Error("expected error for no components, got nil")
	}

	// Garbage JSON: rejected at the very first decode.
	notJSON := []byte(`not json at all`)
	if _, err := DetectVersion(notJSON); err == nil {
		t.Error("expected error for non-JSON, got nil")
	}

	// `version: 2` with empty components is still a v2 envelope; the
	// emptiness is a load-time validation concern, not a detection
	// concern. DetectVersion must say V2; LoadV2 must then fail Validate.
	v2Empty := []byte(`{"version": 2, "components": {}}`)
	v, err := DetectVersion(v2Empty)
	if err != nil {
		t.Errorf("DetectVersion(version=2, components=empty): unexpected err = %v", err)
	}
	if v != V2 {
		t.Errorf("DetectVersion(version=2): v = %s, want v2", v)
	}
	if _, err := LoadV2(v2Empty); err == nil {
		t.Error("LoadV2 should reject empty Components, got nil")
	}
}

func TestLoadV1FullChain(t *testing.T) {
	// Use a real template (skips if templates dir missing).
	v1 := loadTemplate(t, "web_search_assistant.json")
	c, err := LoadV1(v1)
	if err != nil {
		t.Fatalf("LoadV1: %v", err)
	}
	if c.Version != CurrentVersion {
		t.Fatalf("Version = %d, want %d", c.Version, CurrentVersion)
	}
	if len(c.Components) == 0 {
		t.Fatal("expected non-empty Components")
	}
	// All downstream refs must resolve.
	for id, cpn := range c.Components {
		for _, ds := range cpn.Downstream {
			if _, ok := c.Components[ds]; !ok {
				t.Errorf("component %q downstream %q does not exist", id, ds)
			}
		}
	}
}

func TestLoadV2RoundTrip(t *testing.T) {
	// Build a v2 Canvas in memory, serialize it, parse it back, and
	// verify field-for-field equality.
	src := &Canvas{
		Version: CurrentVersion,
		Components: map[string]Component{
			"begin_0": {
				ID:         "begin_0",
				Name:       "Begin",
				Downstream: []string{"llm_0", "retrieval_0"},
				Params:     map[string]any{"query": "{{sys.query}}"},
			},
			"llm_0": {
				ID:         "llm_0",
				Name:       "LLM",
				Downstream: []string{"message_0"},
				Params: map[string]any{
					"model":       "deepseek-chat",
					"temperature": 0.1,
					"prompts": []any{
						map[string]any{
							"role":    "user",
							"content": "Hi",
						},
					},
				},
			},
			"retrieval_0": {
				ID:         "retrieval_0",
				Name:       "Retrieval",
				Downstream: []string{"message_0"},
				Params:     map[string]any{"k": 5},
			},
			"message_0": {
				ID:         "message_0",
				Name:       "Message",
				Downstream: []string{},
				Params:     map[string]any{},
			},
		},
	}

	bs, err := src.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := LoadV2(bs)
	if err != nil {
		t.Fatalf("LoadV2: %v", err)
	}
	if got.Version != src.Version {
		t.Errorf("Version = %d, want %d", got.Version, src.Version)
	}
	if len(got.Components) != len(src.Components) {
		t.Fatalf("len(Components) = %d, want %d", len(got.Components), len(src.Components))
	}
	for id, want := range src.Components {
		got := got.Components[id]
		if got.ID != want.ID {
			t.Errorf("[%s] ID = %q, want %q", id, got.ID, want.ID)
		}
		if got.Name != want.Name {
			t.Errorf("[%s] Name = %q, want %q", id, got.Name, want.Name)
		}
		if len(got.Downstream) != len(want.Downstream) {
			t.Errorf("[%s] len(Downstream) = %d, want %d", id, len(got.Downstream), len(want.Downstream))
			continue
		}
		for i, ds := range want.Downstream {
			if got.Downstream[i] != ds {
				t.Errorf("[%s] Downstream[%d] = %q, want %q", id, i, got.Downstream[i], ds)
			}
		}
		// Params is map[string]any; compare as canonical JSON.
		wantJSON, _ := json.Marshal(want.Params)
		gotJSON, _ := json.Marshal(got.Params)
		if !bytes.Equal(wantJSON, gotJSON) {
			t.Errorf("[%s] Params mismatch: got %s, want %s", id, gotJSON, wantJSON)
		}
	}
}

func TestLoad_AutoDetect(t *testing.T) {
	// A v1 payload fed to Load() should auto-convert.
	v1 := []byte(`{
		"components": {
			"Begin:AutoOne": {
				"downstream": ["Message:AutoTwo"],
				"obj": {"component_name": "Begin", "params": {}}
			},
			"Message:AutoTwo": {
				"obj": {"component_name": "Message", "params": {}}
			}
		}
	}`)
	c, err := Load(v1)
	if err != nil {
		t.Fatalf("Load(v1): %v", err)
	}
	if c.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", c.Version, CurrentVersion)
	}
	if _, ok := c.Components["begin_autoone"]; !ok {
		t.Error("expected v2 id begin_autoone")
	}
}

func TestDecodeReader(t *testing.T) {
	v2 := []byte(`{
		"version": 2,
		"components": {
			"begin_0": {
				"id": "begin_0",
				"name": "Begin",
				"downstream": [],
				"params": {}
			}
		}
	}`)
	c, err := DecodeReader(io.NopCloser(bytes.NewReader(v2)))
	if err != nil {
		t.Fatalf("DecodeReader: %v", err)
	}
	if _, ok := c.Components["begin_0"]; !ok {
		t.Error("expected begin_0 in components")
	}
}

func TestValidate_RejectsDangling(t *testing.T) {
	bad := &Canvas{
		Version: CurrentVersion,
		Components: map[string]Component{
			"a": {ID: "a", Name: "A", Downstream: []string{"zzz"}, Params: map[string]any{}},
		},
	}
	if err := bad.Validate(); err == nil {
		t.Error("expected error for dangling downstream ref, got nil")
	}
}

func TestValidate_RejectsEmptyName(t *testing.T) {
	bad := &Canvas{
		Version: CurrentVersion,
		Components: map[string]Component{
			"a": {ID: "a", Name: "", Downstream: []string{}, Params: map[string]any{}},
		},
	}
	if err := bad.Validate(); err == nil {
		t.Error("expected error for empty Name, got nil")
	}
}

func TestValidate_RejectsColonInID(t *testing.T) {
	bad := &Canvas{
		Version: CurrentVersion,
		Components: map[string]Component{
			"Agent:BadId": {ID: "Agent:BadId", Name: "Agent", Downstream: []string{}, Params: map[string]any{}},
		},
	}
	if err := bad.Validate(); err == nil {
		t.Error("expected error for v1-style colon id, got nil")
	}
}

// loadComplexFixture reads the comprehensive v1 DSL fixture from
// testdata/complex_v1.json, next to this test file. Unlike the production
// templates under agent/templates/*.json, this fixture is stored as the
// raw v1 `{"components": {...}}` envelope (no `dsl:` wrapper), so it
// round-trips through LoadV1 / Load byte-for-byte.
func loadComplexFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/complex_v1.json")
	if err != nil {
		t.Fatalf("read testdata/complex_v1.json: %v", err)
	}
	return raw
}

func TestLoadV1ComplexFixture(t *testing.T) {
	// Round-trip the full fixture through the v1->v2 converter and
	// assert the resulting Canvas mirrors what the Python v1 reader
	// would build at runtime. The fixture deliberately exercises
	// control-flow (Categorize, Switch, Iteration/IterationItem,
	// Loop/LoopItem) and the data-plane surfaces (Begin, Agent, LLM,
	// Retrieval, VariableAggregator, VariableAssigner, CodeExec,
	// Invoke, UserFillup, Message).
	v1 := loadComplexFixture(t)
	c, err := LoadV1(v1)
	if err != nil {
		t.Fatalf("LoadV1(complex_v1.json): %v", err)
	}
	if c.Version != CurrentVersion {
		t.Fatalf("Version = %d, want %d", c.Version, CurrentVersion)
	}
	// 23 components in the fixture, modelling a "multi-source
	// research copilot" workflow:
	//   Begin (1) -> Categorize(2) -> Switch(3) routes between
	//     - UserFillup (4) -> Message(clarify)
	//     - Message(decline) terminal
	//     - Agent(decompose) (5) -> Loop(over sub-questions)
	//       -> LoopItem (body) -> Retrieval -> LLM(score) ->
	//       Categorize(filter) -> Switch(filter) -> VariableAggregator
	//       -> Iteration(re-rank) -> IterationItem ->
	//       VariableAggregator(ranked) -> LLM(synthesize) ->
	//       CodeExec(validate) -> Switch(decide, with back-edge
	//       retry) -> VariableAssigner -> Message(final) /
	//       Message(fallback).
	const wantComponents = 23
	if got := len(c.Components); got != wantComponents {
		t.Fatalf("len(Components) = %d, want %d", got, wantComponents)
	}

	// Component-name histogram: every control-flow surface must be
	// present and at the right multiplicity. Failing this catches
	// fixture drift (someone rebalances the graph) and converter
	// regressions that drop a node silently.
	byName := map[string]int{}
	for _, cpn := range c.Components {
		byName[cpn.Name]++
	}
	wantNames := map[string]int{
		"Begin":              1,
		"Categorize":         2, // intent + chunk-relevance filter
		"Switch":             3, // intent route + filter + validate-decide
		"UserFillup":         1,
		"Agent":              1, // decomposes the user question
		"LLM":                2, // score + synthesize
		"Retrieval":          1,
		"Iteration":          1, // re-rank the aggregated chunks
		"IterationItem":      1,
		"VariableAggregator": 2, // within-loop evidence + post-iter ranked
		"VariableAssigner":   1, // persists the final answer to session
		"Loop":               1, // iterates over sub-questions (max 5)
		"LoopItem":           1,
		"CodeExec":           1, // validates citation completeness
		"Message":            4, // decline / clarify / fallback / final
	}
	for name, want := range wantNames {
		if got := byName[name]; got != want {
			t.Errorf("components by name: %q = %d, want %d", name, got, want)
		}
	}

	// v2 ids follow the canonical lowercased "<name>_<uuid>" shape;
	// the converter drops the v1 "<Name>:<UUID>" colon convention
	// and lowercases both halves (see converter_v1_to_v2.go).
	for id, cpn := range c.Components {
		if strings.Contains(id, ":") {
			t.Errorf("v2 id %q still contains ':'", id)
		}
		if id != cpn.ID {
			t.Errorf("key %q != Component.ID %q", id, cpn.ID)
		}
		if cpn.Name == "" {
			t.Errorf("component %q has empty Name", id)
		}
		for _, ds := range cpn.Downstream {
			if _, ok := c.Components[ds]; !ok {
				t.Errorf("component %q downstream %q does not exist (have %v)", id, ds, sortedIDs(c.Components))
			}
		}
	}

	// Spot-check the v2 ids the runtime will actually look up. These
	// are the bridge between the v1 "<Name>:<UUID>" keys and the v2
	// "<name>_<uuid>" lowercase ids. If any of these go missing,
	// either the fixture was edited or the key-remap rule changed.
	wantIDs := []string{
		"begin_7d83abf3b4f611efa3c40242ac120002",
		"categorize_11111111aaaa0001aaaa0001aaaa0001",
		"categorize_11111111aaaa0002aaaa0002aaaa0002",
		"switch_22222222bbbb0001bbbb0001bbbb0001",
		"switch_22222222bbbb0002bbbb0002bbbb0002",
		"switch_22222222bbbb0003bbbb0003bbbb0003",
		"userfillup_cccccccc00010001cccc00010001cccc0001",
		"agent_33333333cccc0001cccc0001cccc0001",
		"loop_dddddddd0001000100010001000100010001",
		"loopitem_eeeeeeee00010001000100010001bbbb0001",
		"retrieval_555555550001000100010001eeee0001",
		"llm_444444440001000100010001dddd0001",
		"llm_444444440002000200020002dddd0002",
		"variableaggregator_666666660001000100010001ffff0001",
		"variableaggregator_666666660002000200020002ffff0002",
		"iteration_7777777700010001000100010001aaaa0001",
		"iterationitem_8888888800010001000100010001bbbb0001",
		"codeexec_bbbbbbbb0001000100010001cccc0001",
		"variableassigner_9999999900010001000100010001cccc0001",
		"message_00000000eeee0001eeee0001eeee0001",
		"message_00000000eeee0002eeee0002eeee0002",
		"message_00000000eeee0003eeee0003eeee0003",
		"message_00000000eeee0004eeee0004eeee0004",
	}
	for _, id := range wantIDs {
		if _, ok := c.Components[id]; !ok {
			t.Errorf("expected v2 id %q in converted canvas", id)
		}
	}

	// Topological shape: Begin has at least one downstream (proves
	// the graph has a real entry point) and at least one Message has
	// empty downstream (proves the graph has a real sink). The
	// per-node downstream check above catches dangling refs, so we
	// don't need a full BFS — Iteration/Loop's body items are linked
	// via implicit parent_id at runtime, not via downstream edges.
	beginCpn, ok := c.Components["begin_7d83abf3b4f611efa3c40242ac120002"]
	if !ok {
		t.Fatal("begin component missing from converted canvas")
	}
	if len(beginCpn.Downstream) == 0 {
		t.Error("Begin has no downstream; the graph has no entry edge")
	}
	sinkCount := 0
	for _, cpn := range c.Components {
		if len(cpn.Downstream) == 0 {
			sinkCount++
		}
	}
	if sinkCount < 1 {
		t.Error("no terminal components (every node has a downstream); graph is infinite")
	}

	// The same payload must also auto-detect through the generic
	// Load() entry point (DetectVersion says v1 -> LoadV1).
	c2, err := Load(v1)
	if err != nil {
		t.Fatalf("Load(v1) auto-detect: %v", err)
	}
	if c2.Version != CurrentVersion {
		t.Errorf("Load auto-detect Version = %d, want %d", c2.Version, CurrentVersion)
	}
	if len(c2.Components) != len(c.Components) {
		t.Errorf("Load auto-detect produced %d components, LoadV1 produced %d",
			len(c2.Components), len(c.Components))
	}
}

// sortedIDs returns the canvas component IDs in lexicographic order.
// Used in test failure messages so the dump is deterministic.
func sortedIDs(m map[string]Component) []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	// Avoid pulling in sort in tests that don't already need it; the
	// caller only needs a stable list, and the v1 fixture uses
	// hand-named UUIDs that sort lexically fine on their own. A simple
	// bubble sort is plenty for the <=30 IDs these messages contain.
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j-1] > ids[j]; j-- {
			ids[j-1], ids[j] = ids[j], ids[j-1]
		}
	}
	return ids
}
