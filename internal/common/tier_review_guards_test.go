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

// TestTierTestsAreSingleLaneGuarded pins the structural property that the two
// tier tests live only in the single-lane build. Both assert that extra lane
// names are rejected, which is only true where exactly one lane exists; in a
// build that defines more lanes those assertions would fail spuriously. Every
// sibling single-lane test file already carries this constraint, so leaving
// these two out makes the file set inconsistent and breaks those builds.
func TestTierTestsAreSingleLaneGuarded(t *testing.T) {
	for _, rel := range []string{
		"ingestor_tier_test.go",
		"../engine/nats/ingestor_tier_test.go",
	} {
		t.Run(rel, func(t *testing.T) {
			content, err := os.ReadFile(rel)
			if err != nil {
				t.Fatalf("read %s: %v", rel, err)
			}
			if !strings.Contains(string(content), "//go:build !ee") {
				t.Fatalf("%s is missing the single-lane build constraint", rel)
			}
		})
	}
}

// TestIngestorTierArgCommentDoesNotAdvertiseUnsupportedValues guards that the
// ServerArgs.IngestorTier doc comment does not enumerate lane names this build
// refuses to accept. An unqualified "high/medium/low" list invites operators
// to pass a value that is rejected at startup.
func TestIngestorTierArgCommentDoesNotAdvertiseUnsupportedValues(t *testing.T) {
	content, err := os.ReadFile("../server/bootstrap/args.go")
	if err != nil {
		t.Fatalf("read args.go: %v", err)
	}
	if strings.Contains(string(content), "(high/medium/low)") {
		t.Fatal("IngestorTier comment still advertises unsupported high/medium/low values")
	}
}
