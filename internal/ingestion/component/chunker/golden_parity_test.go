//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package chunker

// Cross-language golden parity: Python is the reference implementation, and
// every case under testdata/parity/cases is run through both sides.
// testdata/parity/golden holds what Python produced; this test asserts the Go
// port reproduces it.
//
// Unit tier (no build tag) on purpose. The chunker package builds and passes
// under CGO_ENABLED=0: the only native-backed file is pdfcrop_cgo.go, and its
// cropImageChunks returns the chunks untouched when the PDF engine is nil —
// exactly what happens here, since the harness passes no *gorm.DB and so no
// engine is ever resolved. Running without CGO therefore costs no coverage;
// the single field it cannot populate is ChunkDoc.Image (a base64 preview).
//
// Regenerate the golden files with:
//
//	python internal/ingestion/component/chunker/tool-py/capture_golden.py --all

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"

	"github.com/google/go-cmp/cmp"
)

const (
	parityCasesDir  = "testdata/parity/cases"
	parityGoldenDir = "testdata/parity/golden"
	goSnapshotDir   = "testdata/parity/go_snapshot"
	knownDiffsPath  = "testdata/parity/known_diffs.json"
	captureCmd      = "python internal/ingestion/component/chunker/tool-py/capture_golden.py --all"
	kindExtraFields = "extra_fields"
	kindChunkCount  = "chunk_count"
	kindChunkText   = "chunk_text"
)

// update regenerates the Go-side snapshots for known-diff cases (see
// compareChunkCountKnownDiff). It is a test-only flag, so it is declared here
// at package scope and consumed by `go test -update`.
var update = flag.Bool("update", false, "regenerate go_snapshot files for known-diff cases")

// parityCase is one input fixture, shared verbatim by both languages.
//
// Sharing is possible because the Python upstream models declare
// populate_by_name=True with the short aliases ("json", "markdown", "text",
// "html") that the Go schema tags already use, so neither side needs a
// translation layer. Param keys line up the same way: the Python attribute
// names equal the Go json tags.
type parityCase struct {
	ID        string         `json:"id"`
	Component string         `json:"component"`
	Notes     string         `json:"notes"`
	Param     map[string]any `json:"param"`
	Input     map[string]any `json:"input"`
}

// goldenResult is what capture_golden.py recorded for a case.
type goldenResult struct {
	CaseID string           `json:"case_id"`
	Chunks []map[string]any `json:"chunks"`
	Error  string           `json:"error"`
}

// diffRule is one entry in known_diffs.json: an accepted, documented
// divergence from the Python baseline.
//
// The registry exists so that a difference is either fixed or written down —
// never silently tolerated. Anything not covered by a rule fails the test.
type diffRule struct {
	ID           string   `json:"id"`
	Tag          string   `json:"tag"`
	Kind         string   `json:"kind"`
	AppliesTo    []string `json:"applies_to"`
	Fields       []string `json:"fields"`
	OwnerFixSide string   `json:"owner_fix_side"`
	Tracking     string   `json:"tracking"`
	Reason       string   `json:"reason"`
	Permanent    bool     `json:"permanent"`
}

func (r diffRule) matches(caseID string) bool {
	for _, pattern := range r.AppliesTo {
		if ok, err := path.Match(pattern, caseID); err == nil && ok {
			return true
		}
	}
	return false
}

func loadKnownDiffs(t *testing.T) []diffRule {
	t.Helper()
	raw, err := os.ReadFile(knownDiffsPath)
	if err != nil {
		t.Fatalf("read %s: %v", knownDiffsPath, err)
	}
	var registry struct {
		Version int        `json:"version"`
		Rules   []diffRule `json:"rules"`
	}
	if err := json.Unmarshal(raw, &registry); err != nil {
		t.Fatalf("parse %s: %v", knownDiffsPath, err)
	}
	return registry.Rules
}

// allowedExtraFields collects the Go-only chunk keys declared for this case.
func allowedExtraFields(rules []diffRule, caseID string) map[string]string {
	allowed := make(map[string]string)
	for _, rule := range rules {
		if rule.Kind != kindExtraFields || !rule.matches(caseID) {
			continue
		}
		for _, field := range rule.Fields {
			allowed[field] = rule.ID
		}
	}
	return allowed
}

func TestChunkerGoldenParity(t *testing.T) {
	entries, err := os.ReadDir(parityCasesDir)
	if err != nil {
		t.Fatalf("read cases dir: %v", err)
	}
	rules := loadKnownDiffs(t)
	var ran int
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		ran++
		name := strings.TrimSuffix(entry.Name(), ".json")
		t.Run(name, func(t *testing.T) {
			tc := loadCase(t, filepath.Join(parityCasesDir, entry.Name()))
			if tc.ID != name {
				t.Fatalf("case id %q does not match filename stem %q", tc.ID, name)
			}
			want := loadGolden(t, tc.ID)
			got := invokeChunker(t, tc)
			allowedExtra := allowedExtraFields(rules, tc.ID)
			if rule := matchedRatchetRule(rules, tc.ID); rule != nil {
				compareRatchetedKnownDiff(t, tc.ID, want.Chunks, got, rule, allowedExtra, extraFieldsToStrip(rules))
				return
			}
			compareChunks(t, want.Chunks, got, allowedExtra)
		})
	}
	if ran == 0 {
		t.Fatalf("no cases found under %s", parityCasesDir)
	}
}

func loadCase(t *testing.T, path string) parityCase {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read case: %v", err)
	}
	var tc parityCase
	if err := json.Unmarshal(raw, &tc); err != nil {
		t.Fatalf("parse case %s: %v", path, err)
	}
	return tc
}

func loadGolden(t *testing.T, caseID string) goldenResult {
	t.Helper()
	path := filepath.Join(parityGoldenDir, caseID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		// Never skip on a missing baseline: an absent golden file means the
		// case is unverified, which is a failure, not a pass.
		t.Fatalf("missing golden file %s — capture it with:\n\t%s\n(%v)", path, captureCmd, err)
	}
	var g goldenResult
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("parse golden %s: %v", path, err)
	}
	if g.Error != "" {
		t.Fatalf("golden records a Python-side error for this case: %s", g.Error)
	}
	return g
}

// invokeChunker runs the component through the registry, which is the
// production path: it wraps every chunker in imageUploadDecorator. That
// decorator is what writes the Go-only "id" field, and with no kb_id in
// context it drops raw image bytes instead of reaching for MinIO.
func invokeChunker(t *testing.T, tc parityCase) []map[string]any {
	t.Helper()
	factory, _, _, ok := runtime.DefaultRegistry.Lookup(tc.Component)
	if !ok || factory == nil {
		t.Fatalf("component %q is not registered", tc.Component)
	}
	comp, err := factory(tc.Component, tc.Param)
	if err != nil {
		t.Fatalf("construct %s: %v", tc.Component, err)
	}
	out, err := comp.Invoke(context.Background(), nil, tc.Input)
	if err != nil {
		t.Fatalf("invoke %s: %v", tc.Component, err)
	}
	if msg, ok := out["_ERROR"].(string); ok && msg != "" {
		t.Fatalf("Go returned _ERROR while Python succeeded: %s", msg)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	return chunks
}

// compareChunks asserts the Go chunks match the Python baseline.
//
// Field handling is deliberately asymmetric, because the two directions mean
// different things:
//
//   - a key Python emits that Go does not is always a defect — Go dropped
//     data, so it is reported unconditionally;
//   - a key Go emits that Python does not must be declared in known_diffs.json,
//     so a new Go-only field has to be justified rather than silently tolerated.
//
// Deriving both sets per case rather than hardcoding an ignore list matters:
// the Go-only field set is not constant across components (TokenChunker emits
// ck_type/tk_nums/id, TitleChunker emits only id), so a fixed list would both
// over- and under-match.
func compareChunks(t *testing.T, want, got []map[string]any, allowedExtra map[string]string) {
	t.Helper()
	for _, problem := range chunkDiffs(want, got, allowedExtra) {
		t.Error(problem)
	}
}

// chunkDiffs returns every difference between the Python baseline and the Go
// output as human-readable messages; an empty result means the two sides agree.
//
// It is shared by the strict path and the known-diff ratchet so that "are these
// still diverging?" is answered by the exact same comparison that would fail the
// strict path — otherwise a rule could stay enrolled after its divergence was
// fixed, or hide a second, undeclared difference behind the first.
func chunkDiffs(want, got []map[string]any, allowedExtra map[string]string) []string {
	if len(want) != len(got) {
		// Report counts only. Diffing dozens of chunks that are misaligned by
		// one index produces pages of noise that hide the actual cause.
		return []string{fmt.Sprintf("chunk count: python=%d go=%d\npython texts: %s\ngo texts: %s",
			len(want), len(got), previewTexts(want), previewTexts(got))}
	}
	var problems []string
	for i := range want {
		pyChunk, goChunk := want[i], got[i]
		if missing := keysMissing(pyChunk, goChunk); len(missing) > 0 {
			problems = append(problems, fmt.Sprintf("chunk[%d]: Go is missing key(s) %v that Python emits", i, missing))
		}
		var undeclared []string
		for _, key := range keysMissing(goChunk, pyChunk) {
			if _, ok := allowedExtra[key]; !ok {
				undeclared = append(undeclared, key)
			}
		}
		if len(undeclared) > 0 {
			problems = append(problems, fmt.Sprintf("chunk[%d]: Go emits key(s) %v that Python does not, and no rule in %s declares them.\n"+
				"Either drop the field in Go or add an extra_fields rule explaining why it must stay.",
				i, undeclared, knownDiffsPath))
		}
		for _, key := range sortedKeys(pyChunk) {
			goVal, ok := goChunk[key]
			if !ok {
				continue // already reported as missing
			}
			if diff := cmp.Diff(normalizeValue(pyChunk[key]), normalizeValue(goVal)); diff != "" {
				problems = append(problems, fmt.Sprintf("chunk[%d][%q] mismatch (-python +go):\n%s", i, key, diff))
			}
		}
	}
	return problems
}

// matchedRatchetRule returns the first snapshot-ratcheted known-diff rule that
// applies to this case, or nil if the case is compared strictly.
func matchedRatchetRule(rules []diffRule, caseID string) *diffRule {
	for i := range rules {
		r := &rules[i]
		if r.Kind != kindChunkCount && r.Kind != kindChunkText {
			continue
		}
		if !r.matches(caseID) {
			continue
		}
		return r
	}
	return nil
}

// compareRatchetedKnownDiff ratchets a documented output divergence.
//
// Unlike the strict compareChunks path, this case is recorded as a known diff,
// so the goal is not to match Python but to (1) keep diverging and (2) stay
// stable on the Go side. If the two sides ever agree again, the divergence is
// resolved and the rule must be removed; if Go's output drifts from its own
// recorded snapshot, a Go-side change broke something the registry was pinning.
// Either way the test fails loudly instead of silently passing.
//
// The rule's kind is also verified against the shape of the surviving
// divergence, so a case that changes class (same chunk count but different
// text, or vice versa) forces a re-read of the rule instead of quietly
// re-pinning under a description that no longer fits.
func compareRatchetedKnownDiff(t *testing.T, caseID string, want, got []map[string]any, rule *diffRule, allowedExtra map[string]string, strip map[string]bool) {
	t.Helper()
	if len(chunkDiffs(want, got, allowedExtra)) == 0 {
		t.Fatalf("divergence resolved: Go output now matches the Python baseline (%d chunks). "+
			"Remove rule %q from %s and delete its snapshot — the known diff is gone.",
			len(got), rule.ID, knownDiffsPath)
	}
	switch rule.Kind {
	case kindChunkCount:
		if len(got) == len(want) {
			t.Fatalf("rule %q is kind %q, but both sides now emit %d chunks — only the content differs. "+
				"Reclassify it as %q in %s and rewrite its reason.",
				rule.ID, kindChunkCount, len(got), kindChunkText, knownDiffsPath)
		}
	case kindChunkText:
		if len(got) != len(want) {
			t.Fatalf("rule %q is kind %q, but chunk counts now differ (python=%d go=%d). "+
				"Reclassify it as %q in %s and rewrite its reason.",
				rule.ID, kindChunkText, len(want), len(got), kindChunkCount, knownDiffsPath)
		}
	}

	snapPath := filepath.Join(goSnapshotDir, caseID+".json")
	if *update {
		if err := os.MkdirAll(goSnapshotDir, 0o755); err != nil {
			t.Fatalf("create %s: %v", goSnapshotDir, err)
		}
		if err := os.WriteFile(snapPath, []byte(serializeChunks(caseID, got, strip)), 0o644); err != nil {
			t.Fatalf("write snapshot %s: %v", snapPath, err)
		}
		t.Logf("updated go snapshot %s (%d chunks)", snapPath, len(got))
		return
	}

	wantSnap, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatalf("missing go snapshot %s — generate it with `go test -update` (%v)", snapPath, err)
	}
	if gotSnap := serializeChunks(caseID, got, strip); gotSnap != string(wantSnap) {
		t.Fatalf("Go output drifted from snapshot %s.\n"+
			"Either a Go-side change altered this known-diff behaviour (investigate), "+
			"or run `go test -update` to accept the new snapshot.", snapPath)
	}
}

// extraFieldsToStrip collects every field declared as a Go-only extra field in
// known_diffs.json, so the ratchet snapshot can omit them (see serializeChunks).
func extraFieldsToStrip(rules []diffRule) map[string]bool {
	strip := make(map[string]bool)
	for _, r := range rules {
		if r.Kind != kindExtraFields {
			continue
		}
		for _, f := range r.Fields {
			strip[f] = true
		}
	}
	return strip
}

// serializeChunks renders Go chunks into a deterministic, human-diffable form
// used for go_snapshot files. Go's json encoder sorts map keys, so two runs
// with equal content produce byte-identical output.
//
// The Go-only decorative fields declared in known_diffs.json (id, ck_type,
// tk_nums) are stripped first: `id` is non-deterministic (derived from a
// per-run doc id), and `ck_type`/`tk_nums` are Go-internal metadata already
// covered by the extra_fields allow-list. Snapshotting them would make the
// ratchet non-reproducible across runs, defeating its purpose.
func serializeChunks(caseID string, chunks []map[string]any, strip map[string]bool) string {
	cleaned := make([]map[string]any, len(chunks))
	for i, ck := range chunks {
		c := make(map[string]any, len(ck))
		for k, v := range ck {
			if strip[k] {
				continue
			}
			c[k] = v
		}
		cleaned[i] = c
	}
	out := struct {
		CaseID string           `json:"case_id"`
		Chunks []map[string]any `json:"chunks"`
	}{CaseID: caseID, Chunks: cleaned}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		// Should be impossible for these inputs; surface it rather than hide.
		panic(fmt.Sprintf("serializeChunks: %v", err))
	}
	return string(raw) + "\n"
}

// TestKnownDiffRegistryWellFormed keeps known_diffs.json from rotting into a
// junk drawer: every rule must be classifiable, justified, and point at a real
// case file. Without this gate, rules accumulate with no owner and no reason.
func TestKnownDiffRegistryWellFormed(t *testing.T) {
	rules := loadKnownDiffs(t)
	if len(rules) == 0 {
		t.Fatal("known_diffs.json has no rules; the registry must not be empty")
	}
	caseIDs := listCaseIDs(t)
	validTags := map[string]bool{
		"python_bug":     true,
		"go_bug":         true,
		"go_intentional": true,
		"unresolved":     true,
	}
	for _, r := range rules {
		if r.ID == "" {
			t.Error("rule with empty id")
		}
		if !validTags[r.Tag] {
			t.Errorf("rule %q: tag %q not in %v", r.ID, r.Tag, validTags)
		}
		if r.Kind != kindExtraFields && r.Kind != kindChunkCount && r.Kind != kindChunkText {
			t.Errorf("rule %q: kind %q is not supported by the parity harness", r.ID, r.Kind)
		}
		if r.Reason == "" {
			t.Errorf("rule %q: reason must not be empty", r.ID)
		}
		// Either a tracking issue is recorded or the divergence is permanent.
		if r.Tracking == "" && !r.Permanent {
			t.Errorf("rule %q: must set tracking (issue link) or permanent:true", r.ID)
		}
		if r.Tracking != "" && r.Permanent {
			t.Errorf("rule %q: tracking and permanent:true are mutually exclusive", r.ID)
		}
		if !r.Permanent && r.OwnerFixSide == "" {
			t.Errorf("rule %q: non-permanent rule must set owner_fix_side", r.ID)
		}
		for _, pattern := range r.AppliesTo {
			if !patternMatchesAny(caseIDs, pattern) {
				t.Errorf("rule %q: applies_to pattern %q matches no case file under %s", r.ID, pattern, parityCasesDir)
			}
		}
	}
}

func listCaseIDs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(parityCasesDir)
	if err != nil {
		t.Fatalf("read cases dir: %v", err)
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(e.Name(), ".json"))
	}
	return ids
}

func patternMatchesAny(ids []string, pattern string) bool {
	for _, id := range ids {
		if pattern == id {
			return true
		}
		if ok, err := path.Match(pattern, id); err == nil && ok {
			return true
		}
	}
	return false
}

// normalizeValue round-trips through encoding/json so numbers decoded from the
// golden file (always float64) and numbers produced natively by Go (int, int64)
// compare by value rather than by Go type.
func normalizeValue(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return v
	}
	return out
}

func keysMissing(from, in map[string]any) []string {
	var out []string
	for key := range from {
		if _, ok := in[key]; !ok {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// previewTexts renders chunk texts compactly so a count mismatch shows where
// the two sides diverged without dumping whole documents.
func previewTexts(chunks []map[string]any) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, ck := range chunks {
		if i > 0 {
			b.WriteString(", ")
		}
		if i == 6 {
			b.WriteString("...")
			break
		}
		text, _ := ck["text"].(string)
		if len(text) > 40 {
			text = text[:40] + "..."
		}
		b.WriteString(strconv.Quote(text))
	}
	b.WriteByte(']')
	return b.String()
}
