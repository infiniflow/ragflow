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
	"os/exec"
	"strings"
	"testing"
)

// TestTierTestsAreSingleLaneGuarded pins the build-system behavior, not source
// text: both tier tests must be compiled in the default build and excluded
// from the multi-lane build. Asking the go tool for TestGoFiles under each tag
// keeps the guard correct if the constraint is reformulated (e.g. a longer
// build expression) and avoids brittle relative-path source reads.
func TestTierTestsAreSingleLaneGuarded(t *testing.T) {
	const tierTest = "ingestor_tier_test.go"
	for _, pkg := range []string{".", "./../engine/nats"} {
		t.Run(pkg, func(t *testing.T) {
			defaultFiles := listedTestFiles(t, pkg)
			if !strings.Contains(defaultFiles, tierTest) {
				t.Fatalf("%s: %s missing from default build; got %q", pkg, tierTest, defaultFiles)
			}
			otherFiles := listedTestFilesWithTag(t, pkg, "ee")
			if strings.Contains(otherFiles, tierTest) {
				t.Fatalf("%s: %s must be excluded from the multi-lane build; got %q", pkg, tierTest, otherFiles)
			}
		})
	}
}

func listedTestFiles(t *testing.T, pkg string) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-f", "{{.TestGoFiles}}", pkg)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list %s: %v", pkg, err)
	}
	return string(out)
}

func listedTestFilesWithTag(t *testing.T, pkg, tag string) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-tags", tag, "-f", "{{.TestGoFiles}}", pkg)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -tags %s %s: %v", tag, pkg, err)
	}
	return string(out)
}
