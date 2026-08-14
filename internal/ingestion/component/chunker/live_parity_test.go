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

//go:build manual

package chunker

// Live Go<->Python chunker parity.
//
// Unlike golden_parity_test.go (which compares Go against committed Python
// golden fixtures), this test invokes the Python reference chunker as a
// subprocess at test time and diffs its fresh output against the Go port. The
// benefits are exactly what a live check buys you: no golden file can go
// stale, and a single command re-verifies every case against the current
// Python. The cost is a runtime dependency on the .venv Python environment
// (ragflow deps + cl100k table), so this is a manual-tier test only — it
// never runs in the default `go test ./...` CI path.
//
// Python side: tool-py/live_chunk.py <case_id> prints the case's chunks as
// JSON, reusing capture_golden.run_case so the shape matches the golden
// fixtures. It aborts (non-zero) if the tokenizer is dead, which turns a
// silently-collapsed baseline into a hard failure instead of a false pass.
//
// Comparison logic is shared with the golden harness: chunkDiffs reports every
// difference, allowedExtraFields tolerates the Go-only id/ck_type/tk_nums
// fields, and known_diffs.json is consulted so documented divergences are
// reported (not failed) rather than surprising the reader.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// findRepoRoot walks up from this source file to the directory holding go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 12; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("repo root (go.mod) not found walking up from test file")
	return ""
}

// venvPython returns the path to the ragflow .venv interpreter, or skips the
// test when it is unavailable (the live parity check needs that environment).
func venvPython(t *testing.T) string {
	t.Helper()
	root := findRepoRoot(t)
	p := filepath.Join(root, ".venv", "bin", "python")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("venv python not found at %s — live Go->Python parity needs the .venv environment; skipping", p)
	}
	return p
}

// pythonChunksLive runs the Python reference chunker on one case and returns
// its chunks. A non-zero Python exit (including a dead tokenizer) fails the
// test rather than comparing against a poisoned baseline.
func pythonChunksLive(t *testing.T, caseID string) []map[string]any {
	t.Helper()
	root := findRepoRoot(t)
	py := venvPython(t)
	script := filepath.Join(root, "internal", "ingestion", "component", "chunker", "tool-py", "live_chunk.py")

	cmd := exec.Command(py, script, caseID)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PYTHONPATH="+root, "PYTHONUNBUFFERED=1")

	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("python chunker failed for %s: %v\nstderr:\n%s", caseID, err, string(ee.Stderr))
		}
		t.Fatalf("python chunker failed for %s: %v", caseID, err)
	}

	var result struct {
		CaseID string           `json:"case_id"`
		Chunks []map[string]any `json:"chunks"`
		Error  string           `json:"error"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("parse python output for %s: %v\nraw:\n%s", caseID, err, string(out))
	}
	if result.Error != "" {
		t.Fatalf("python returned error for %s: %s", caseID, result.Error)
	}
	return result.Chunks
}

func TestChunkerLiveParity(t *testing.T) {
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
			want := pythonChunksLive(t, tc.ID) // Python reference, live
			got := invokeChunker(t, tc)        // Go port
			allowedExtra := allowedExtraFields(rules, tc.ID)
			if rule := matchedRatchetRule(rules, tc.ID); rule != nil {
				// Documented divergence: Python is the truth and Go diverges by
				// design (a known go_bug). A live mismatch is expected; a live
				// match means the bug was fixed and the rule should be dropped.
				if diffs := chunkDiffs(want, got, allowedExtra); len(diffs) == 0 {
					t.Logf("known-diff %s RESOLVED live — remove rule from known_diffs.json", rule.ID)
				} else {
					t.Logf("known-diff %s still diverges live (expected): %d diff(s)", rule.ID, len(diffs))
				}
				return
			}
			for _, problem := range chunkDiffs(want, got, allowedExtra) {
				t.Errorf("%s", problem)
			}
		})
	}
	if ran == 0 {
		t.Fatalf("no cases found under %s", parityCasesDir)
	}
}
