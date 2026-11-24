// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handler

import (
	"os"
	"strings"
	"testing"
)

// TestDocAnalyzerTestsScopedToDefaultBuild guards the build separation of the
// doc-analyzer unit tests. One assertion pins the exact set of backends the
// default build contributes, so these tests must not run in a build that
// supplies additional backends. The two files share test helpers, so both
// must carry the same constraint or that build fails to compile.
//
// This guard deliberately has no build constraint of its own, so it verifies
// the constraint on both test files in every build.
func TestDocAnalyzerTestsScopedToDefaultBuild(t *testing.T) {
	const constraint = "//go:build !ee"
	for _, name := range []string{
		"system_doc_analyzers_test.go",
		"system_doc_analyzers_auth_test.go",
	} {
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			text := string(src)

			tagIdx := strings.Index(text, constraint)
			if tagIdx < 0 {
				t.Fatalf("%s: missing %q build constraint", name, constraint)
			}

			pkgIdx := strings.Index(text, "\npackage handler")
			if pkgIdx < 0 {
				t.Fatalf("%s: cannot locate package clause", name)
			}
			if tagIdx > pkgIdx {
				t.Fatalf("%s: build constraint must precede the package clause", name)
			}

			// Go honors a build constraint only when a blank line separates it
			// from the package clause.
			lineEnd := strings.IndexByte(text[tagIdx:], '\n')
			if lineEnd < 0 {
				t.Fatalf("%s: build constraint line is not terminated", name)
			}
			if !strings.HasPrefix(text[tagIdx+lineEnd+1:], "\n") {
				t.Fatalf("%s: build constraint must be followed by a blank line", name)
			}
		})
	}
}
