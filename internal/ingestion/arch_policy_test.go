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

package ingestion

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// convergenceBaselineFile is the checked-in snapshot of converged service
// files. It is the ratchet baseline: entries are only ever appended.
const convergenceBaselineFile = "internal/ingestion/arch_baseline.txt"

// maxGlobalAccessWhitelistEntries bounds the exception channel in arch_test.go.
const maxGlobalAccessWhitelistEntries = 3

// convergedServiceFiles is the explicit set of internal/service production
// files whose dependencies are fully injected. The guard scans exactly these
// files (plus all of internal/ingestion). Registering a file here is what
// subjects it to the no-global-singleton rule; keep it in sync with
// convergenceBaselineFile. Adding an entry is how a migration PR converges a
// module; removing one requires an explicit, reviewed decision.
var convergedServiceFiles = map[string]bool{
	"internal/service/memory.go":                 true,
	"internal/service/memory_message_service.go": true,
	"internal/service/memory_extractor.go":       true,
	"internal/service/memory_saver_adapter.go":   true,
}

// scansServiceFile reports whether a service-relative path is in the explicit
// converged set. Both walkers in arch_test.go use it so file selection has a
// single source of truth.
func scansServiceFile(rel string) bool {
	return convergedServiceFiles[rel]
}

// archSkipReason reports why a file is skipped by the guard walkers, or ""
// when the file is scanned. Both walkers in arch_test.go use it.
func archSkipReason(rel, scanRoot string) string {
	switch {
	case strings.HasSuffix(rel, "_test.go"):
		return "test file"
	case globalAccessWhitelist[rel] != "":
		return "whitelisted"
	case scanRoot == "internal/service" && !scansServiceFile(rel):
		return "unconverged service file"
	}
	return ""
}

// parseConvergenceBaseline parses the checked-in baseline (one path per line;
// blank lines and # comments ignored).
func parseConvergenceBaseline(content string) map[string]bool {
	files := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		rel := strings.TrimSpace(line)
		if rel == "" || strings.HasPrefix(rel, "#") {
			continue
		}
		files[rel] = true
	}
	return files
}

func TestConvergenceListIsExplicitAndNonEmpty(t *testing.T) {
	if len(convergedServiceFiles) == 0 {
		t.Fatal("convergedServiceFiles is empty; the arch guard would scan no service files")
	}
}

func TestScansServiceFileUsesExplicitList(t *testing.T) {
	// Adversarial: a listed path whose name does not start with memory must
	// still be scanned - this is what distinguishes the explicit list from
	// the old string-prefix filter.
	const listedNonMemory = "internal/service/user.go"
	convergedServiceFiles[listedNonMemory] = true
	defer delete(convergedServiceFiles, listedNonMemory)
	if !scansServiceFile(listedNonMemory) {
		t.Fatalf("listed file %q must be scanned regardless of its name", listedNonMemory)
	}

	// Adversarial: a memory-prefixed path that is NOT in the list must be
	// skipped - the old prefix filter would have scanned it.
	const unlistedMemory = "internal/service/memory_future.go"
	delete(convergedServiceFiles, unlistedMemory)
	if scansServiceFile(unlistedMemory) {
		t.Fatalf("unlisted file %q must be skipped", unlistedMemory)
	}
}

func TestConvergenceRatchetMatchesBaseline(t *testing.T) {
	root := findModuleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, convergenceBaselineFile))
	if err != nil {
		t.Fatalf("read convergence baseline %s: %v", convergenceBaselineFile, err)
	}
	baseline := parseConvergenceBaseline(string(data))
	if !reflect.DeepEqual(baseline, convergedServiceFiles) {
		t.Fatalf("convergence baseline %v does not match convergedServiceFiles %v", baseline, convergedServiceFiles)
	}

	// Falsifiability: a baseline carrying a path that is not registered must
	// never compare equal to the live list.
	withGhost := parseConvergenceBaseline("internal/service/memory.go\ninternal/service/ghost.go\n")
	if reflect.DeepEqual(withGhost, convergedServiceFiles) {
		t.Fatal("a baseline with an unregistered ghost path must not match the list")
	}
}

func TestConvergenceListPathsAreProductionGoFiles(t *testing.T) {
	if len(convergedServiceFiles) == 0 {
		t.Fatal("convergedServiceFiles is empty; path validation would pass vacuously")
	}
	root := findModuleRoot(t)
	for rel := range convergedServiceFiles {
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			t.Errorf("%q is not a production .go file", rel)
		}
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("listed file %q does not exist: %v", rel, err)
		}
	}
}

func TestArchSkipReasonUsesExplicitList(t *testing.T) {
	cases := []struct {
		name, rel, scanRoot, want string
	}{
		{"converged service file is scanned", "internal/service/memory.go", "internal/service", ""},
		{"non-converged non-memory file is skipped", "internal/service/user.go", "internal/service", "unconverged service file"},
		{"non-converged memory-named file is skipped", "internal/service/memory_future.go", "internal/service", "unconverged service file"},
		{"ingestion file is scanned", "internal/ingestion/foo.go", "internal/ingestion", ""},
		{"test file is skipped", "internal/ingestion/foo_test.go", "internal/ingestion", "test file"},
		{"whitelisted file is skipped", "internal/ingestion/chunkcache/chunkcache.go", "internal/ingestion", "whitelisted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := archSkipReason(tc.rel, tc.scanRoot); got != tc.want {
				t.Fatalf("archSkipReason(%q, %q) = %q, want %q", tc.rel, tc.scanRoot, got, tc.want)
			}
		})
	}
}

func TestWhitelistDoesNotGrow(t *testing.T) {
	if len(globalAccessWhitelist) > maxGlobalAccessWhitelistEntries {
		t.Fatalf("global access whitelist grew to %d entries (max %d)", len(globalAccessWhitelist), maxGlobalAccessWhitelistEntries)
	}
}
