// Package dsl — loader test for the production v1 DSL examples.
//
// These fixtures are mirrored from agent/test/dsl_examples/*.json so
// the Go port has a self-contained set of representative v1 payloads
// to exercise against. The Python side ships the originals (and may
// grow them); this directory is the Go side's frozen copy for unit
// tests so the Go port is buildable on its own.
//
// The test walks every fixture through the four DSL entry points
// (DetectVersion, LoadV1, Load auto-detect, DecodeReader) and asserts
// the converted v2 Canvas is structurally sound in each case. The
// four entry points differ only in how the bytes reach the
// parser/detector — by covering them all from one table we catch
// any drift in detection rules or converter behaviour without
// fragmenting coverage into per-entry-point subtests.
//
// Each example covers a distinct topology:
//
//	retrieval_and_generate              — Begin → Retrieval → LLM → Message
//	tavily_and_generate                 — Begin → TavilySearch → LLM → Message
//	categorize_and_agent_with_tavily    — Begin → Categorize → {Agent(Tavily) | Message}
//	retrieval_categorize_and_generate   — Begin → Categorize → {Retrieval → Agent → Message | Message}
//	iteration                           — Begin → Agent → Iteration → {IterationItem → TavilySearch → Agent}
//	exesql                              — Begin → Answer ↔ ExeSQL (cycle, kept in fixtures as a stress test)
//	headhunter_zh                       — multi-Categorize, multi-Generate, Answer+Message loop
package dsl

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// v1Examples lists every fixture under testdata/v1_examples. Adding
// a new file to the directory automatically extends coverage.
var v1Examples = []string{
	"categorize_and_agent_with_tavily.json",
	"exesql.json",
	"headhunter_zh.json",
	"iteration.json",
	"retrieval_and_generate.json",
	"retrieval_categorize_and_generate.json",
	"tavily_and_generate.json",
}

// readV1Example reads a v1 fixture from testdata/v1_examples. Tests
// call t.Skip if the file is missing so the package still builds in
// environments where the fixtures have not been vendored.
func readV1Example(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "v1_examples", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("v1 fixture %s not readable: %v", path, err)
	}
	return raw
}

// TestV1Examples is the single canary for the v1 fixture set. It
// drives every fixture through the four DSL entry points and asserts
// the resulting v2 Canvas is structurally sound in each case. Failing
// here means either the directory contents drifted, a new fixture
// doesn't match the v1 envelope, or the converter lost something on
// its way to v2.
func TestV1Examples(t *testing.T) {
	// Directory canary: the on-disk list must match v1Examples. This
	// guards against the suite silently passing because new files
	// were added without updating v1Examples, or vice versa.
	entries, err := os.ReadDir(filepath.Join("testdata", "v1_examples"))
	if err != nil {
		t.Fatalf("testdata/v1_examples not accessible: %v", err)
	}
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			got = append(got, e.Name())
		}
	}
	sort.Strings(got)
	want := append([]string(nil), v1Examples...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("v1_examples count = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("v1_examples[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	for _, name := range v1Examples {
		t.Run(name, func(t *testing.T) {
			raw := readV1Example(t, name)

			// DetectVersion must report V1.
			v, err := DetectVersion(raw)
			if err != nil {
				t.Fatalf("DetectVersion: %v", err)
			}
			if v != V1 {
				t.Fatalf("DetectVersion = %s, want v1", v)
			}

			// LoadV1 (explicit v1) and Load (auto-detect) must both
			// produce a valid v2 Canvas. Comparing the two
			// additionally proves the auto-detect path routed to
			// LoadV1 (the canvas shapes must match).
			explicit, err := LoadV1(raw)
			if err != nil {
				t.Fatalf("LoadV1: %v", err)
			}
			assertConvertedCanvasOK(t, name, explicit)

			auto, err := Load(raw)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			assertConvertedCanvasOK(t, name, auto)
			if len(auto.Components) != len(explicit.Components) {
				t.Errorf("Load and LoadV1 disagree on component count: %d vs %d",
					len(auto.Components), len(explicit.Components))
			}

			// DecodeReader (io.Reader path) must produce the same
			// v2 Canvas up to component count.
			decoded, err := DecodeReader(strings.NewReader(string(raw)))
			if err != nil {
				t.Fatalf("DecodeReader: %v", err)
			}
			if decoded.Version != CurrentVersion {
				t.Errorf("DecodeReader Version = %d, want %d", decoded.Version, CurrentVersion)
			}
			if len(decoded.Components) != len(explicit.Components) {
				t.Errorf("DecodeReader produced %d components, LoadV1 produced %d",
					len(decoded.Components), len(explicit.Components))
			}
		})
	}
}

// assertConvertedCanvasOK verifies a v1-converted Canvas is a valid v2
// graph: non-empty components, non-empty names, no colon in ids, and
// every Downstream ref resolves. This is the load-time check that
// protects against drift in either the fixture or the converter.
func assertConvertedCanvasOK(t *testing.T, name string, c *Canvas) {
	t.Helper()
	if c.Version != CurrentVersion {
		t.Errorf("[%s] Version = %d, want %d", name, c.Version, CurrentVersion)
	}
	if len(c.Components) == 0 {
		t.Fatalf("[%s] Components is empty", name)
	}
	byName := map[string]int{}
	for id, cpn := range c.Components {
		if strings.Contains(id, ":") {
			t.Errorf("[%s] v2 id %q still contains ':'", name, id)
		}
		if id != cpn.ID && cpn.ID != "" {
			t.Errorf("[%s] key %q != Component.ID %q", name, id, cpn.ID)
		}
		if cpn.Name == "" {
			t.Errorf("[%s] component %q has empty Name", name, id)
		}
		for _, ds := range cpn.Downstream {
			if _, ok := c.Components[ds]; !ok {
				t.Errorf("[%s] component %q downstream %q does not exist", name, id, ds)
			}
		}
		byName[cpn.Name]++
	}
	// Begin must be present in every fixture (they all start from a
	// user query). Catches the case where someone renames Begin in a
	// fixture and silently breaks every orchestrator that looks it up.
	if byName["Begin"] == 0 {
		t.Errorf("[%s] no Begin component found in converted canvas (have %v)", name, byName)
	}
	// We deliberately do NOT require at least one terminal node
	// here: the v1 fixtures include Answer↔ExeSQL and Answer↔Message
	// cycles (e.g. exesql.json, headhunter_zh.json) where every
	// component has a non-empty downstream by design — the cycle
	// represents the agent waiting for the next user turn. Cycle
	// detection is a runtime concern, not a load-time one, and lives
	// in the scheduler.
	//
	// Run the v2 Validate as the final gate. This catches anything
	// the per-component loop above did not (e.g. Version mismatch).
	if err := c.Validate(); err != nil {
		t.Errorf("[%s] Validate: %v", name, err)
	}
}
