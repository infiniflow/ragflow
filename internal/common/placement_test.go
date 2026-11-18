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

package common

import (
	"os"
	"strings"
	"testing"
)

// allowedCommonFiles is the explicit placement allowlist for internal/common.
// internal/common is a leaf package imported across the whole tree; admitting a
// file here must be a deliberate decision, not a default. To add a file, append
// it here AND record the rationale in doc.go. Files that carry a single
// consumer or application-specific semantics belong in their owning package.
// This list is a ratchet: entries are removed only via a reviewed relocation.
var allowedCommonFiles = map[string]bool{
	"allow_private_host.go":          true,
	"app_name.go":                    true,
	"component_identity.go":          true,
	"component_names.go":             true,
	"connector_credentials.go":       true,
	"constants.go":                   true,
	"deep_copy.go":                   true,
	"doc.go":                         true,
	"environments.go":                true,
	"error_code.go":                  true,
	"format.go":                      true,
	"http.go":                        true,
	"idenfication.go":                true,
	"ingest_log.go":                  true,
	"ingestor_tier_community.go":     true,
	"json_types.go":                  true,
	"libm_cgo.go":                    true,
	"libm_purego.go":                 true,
	"logger.go":                      true,
	"metadata_utils.go":              true,
	"model.go":                       true,
	"model_assets.go":                true,
	"multimodal.go":                  true,
	"name_limit.go":                  true,
	"number.go":                      true,
	"pagination.go":                  true,
	"parser_config.go":               true,
	"password.go":                    true,
	"reference_metadata.go":          true,
	"response.go":                    true,
	"retry.go":                       true,
	"status_message.go":              true,
	"string_utils.go":                true,
	"stub.go":                        true,
	"subject_for_tier_community.go":  true,
	"task.go":                        true,
	"task_default_community.go":      true,
	"task_pull_timeout_community.go": true,
	"think.go":                       true,
	"time.go":                        true,
	"timer.go":                       true,
	"token.go":                       true,
	"token_budget.go":                true,
	"trusted_proxies.go":             true,
	"type_conversion.go":             true,
	"vector_column.go":               true,
	"version.go":                     true,
}

// relocatedOutOfCommon lists files whose ownership was moved to a more
// specific package. They must never reappear in internal/common.
var relocatedOutOfCommon = []string{
	"kg_query_rewrite.go",
	"smtp_config.go",
}

func isAllowedCommonFile(name string) bool {
	return allowedCommonFiles[name]
}

// commonProductionGoFiles returns the production Go files in the current
// package directory (the test binary runs with cwd set to internal/common).
// Test files and edition overlay files are excluded: overlays belong to a
// separate tree and cannot be registered in this package's allowlist.
func commonProductionGoFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read common dir: %v", err)
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_ee.go") {
			continue
		}
		files = append(files, name)
	}
	return files
}

// TestCommonFilesAreExplicitlyAllowed is the core guard: every production file
// physically in internal/common must be an explicit allowlist entry. A file
// dropped into common without registration fails here.
func TestCommonFilesAreExplicitlyAllowed(t *testing.T) {
	files := commonProductionGoFiles(t)
	if len(files) == 0 {
		t.Fatal("no production Go files found in internal/common; guard would be vacuous")
	}
	for _, f := range files {
		if !isAllowedCommonFile(f) {
			t.Errorf("common file %q is not in allowedCommonFiles: register it with a rationale in doc.go or move it to its owning package", f)
		}
	}
}

// TestAllowlistRejectsRelocatedAndGhostFiles is adversarial: the allowlist
// must not contain files we deliberately relocated, nor an arbitrary new name.
func TestAllowlistRejectsRelocatedAndGhostFiles(t *testing.T) {
	for _, f := range relocatedOutOfCommon {
		if isAllowedCommonFile(f) {
			t.Errorf("relocated file %q must not be allowlisted in common", f)
		}
	}
	const ghost = "scratch_helper.go"
	if isAllowedCommonFile(ghost) {
		t.Errorf("ghost file %q must not be allowlisted in common", ghost)
	}
}

// TestAllowlistHasNoGhostEntries is adversarial in the other direction: every
// allowlist entry must correspond to a real file, so stale entries cannot hide
// a vacuous allowlist.
func TestAllowlistHasNoGhostEntries(t *testing.T) {
	physical := make(map[string]bool)
	for _, f := range commonProductionGoFiles(t) {
		physical[f] = true
	}
	for f := range allowedCommonFiles {
		if !physical[f] {
			t.Errorf("allowlisted file %q does not exist in internal/common", f)
		}
	}
}

// TestEditionOverlayFilesAreIgnoredByGuard reproduces a tree where an
// edition-specific overlay file is physically present in internal/common.
// Such a file is owned by a separate tree; the guard must not demand an
// allowlist entry for a file this tree cannot register.
func TestEditionOverlayFilesAreIgnoredByGuard(t *testing.T) {
	const probe = "zz_guard_probe_ee.go"
	if _, err := os.Stat(probe); err == nil {
		t.Fatalf("probe path %q already exists; refusing to overwrite it", probe)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat probe path: %v", err)
	}
	if err := os.WriteFile(probe, []byte("package common\n"), 0o644); err != nil {
		t.Fatalf("write probe file: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Remove(probe); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove probe file: %v", err)
		}
	})

	files := commonProductionGoFiles(t)
	for _, f := range files {
		if f == probe {
			t.Errorf("guard treated overlay file %q as a production common file; it is owned by a separate tree and must be excluded", probe)
		}
	}

	// Distinguishing condition: filtering must not drop genuine production files.
	if len(files) != len(allowedCommonFiles) {
		t.Errorf("guard returned %d files but allowlist has %d entries; filtering must not drop genuine production files", len(files), len(allowedCommonFiles))
	}
}

// TestDocRecordsInvertedSeamExceptions guards the documented rationale for the
// two low-fan-in files that remain in common as dependency-inversion seams.
// Without an explicit note, a future reader would treat them as rule violations.
func TestDocRecordsInvertedSeamExceptions(t *testing.T) {
	doc, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatalf("read doc.go: %v", err)
	}
	text := string(doc)
	for _, name := range []string{"allow_private_host.go", "model_assets.go"} {
		if !strings.Contains(text, name) {
			t.Errorf("doc.go must record why %s remains in common as a dependency-inversion seam", name)
		}
	}
}
