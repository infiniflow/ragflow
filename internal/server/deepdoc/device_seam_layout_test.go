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

package deepdoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// defaultOnlySeamTests assert the CPU-only build contract (GPU requests are
// rejected, hooks are no-ops). An accelerator-capable build has the opposite
// contract and must not compile these assertions.
var defaultOnlySeamTests = []string{
	"TestParseDeviceGPURequestsReportEnterpriseEdition",
	"TestParseDeviceInvalidValueReportsInvalid",
	"TestDefaultBuildHooksAreNoOps",
}

// sharedSeamTests hold in every build and must stay in a file compiled in all
// editions (no !ee guard).
var sharedSeamTests = []string{
	"TestParseDeviceAcceptsCPU",
	"TestDeviceConfigString",
}

type seamTestFile struct {
	name  string
	tag   string
	funcs map[string]bool
}

// scanSeamTestFiles reads device-seam test sources from disk regardless of the
// active build tag, so this meta-test can verify the layout of files that are
// excluded from the current edition.
func scanSeamTestFiles(t *testing.T) []seamTestFile {
	t.Helper()
	matches, err := filepath.Glob("device_seam*_test.go")
	if err != nil {
		t.Fatalf("glob device seam tests: %v", err)
	}
	var out []seamTestFile
	for _, name := range matches {
		if name == "device_seam_layout_test.go" {
			continue // this meta-test itself
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		tag := ""
		funcs := map[string]bool{}
		for _, line := range strings.Split(string(raw), "\n") {
			if tag == "" && strings.HasPrefix(line, "//go:build") {
				tag = line
			}
			if strings.HasPrefix(line, "func Test") {
				rest := strings.TrimPrefix(line, "func ")
				if i := strings.Index(rest, "("); i > 0 {
					funcs[rest[:i]] = true
				}
			}
		}
		out = append(out, seamTestFile{name: name, tag: tag, funcs: funcs})
	}
	return out
}

func excludesAccelerator(tag string) bool { return strings.Contains(tag, "!ee") }

// TestDeviceSeamTestBuildTagLayout guards that default-only device assertions
// live in a file excluded from accelerator builds. Without this separation a
// correct accelerator implementation fails these tests (it accepts GPU
// requests), breaking that build's test suite.
func TestDeviceSeamTestBuildTagLayout(t *testing.T) {
	files := scanSeamTestFiles(t)
	if len(files) == 0 {
		t.Fatal("no device_seam test files found")
	}

	for _, want := range defaultOnlySeamTests {
		foundInGuarded := false
		for _, f := range files {
			if !f.funcs[want] {
				continue
			}
			if !excludesAccelerator(f.tag) {
				t.Errorf("%s is in %s (tag %q) compiled by accelerator builds; it must live in a !ee-guarded file", want, f.name, f.tag)
			} else {
				foundInGuarded = true
			}
		}
		if !foundInGuarded {
			t.Errorf("%s not found in any !ee-guarded test file", want)
		}
	}

	for _, want := range sharedSeamTests {
		found := false
		for _, f := range files {
			if f.funcs[want] && !excludesAccelerator(f.tag) {
				found = true
			}
		}
		if !found {
			t.Errorf("shared test %s must remain in a file compiled in all builds (no !ee guard)", want)
		}
	}
}
