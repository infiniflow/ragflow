//
//  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
	"fmt"
	"testing"
)

func TestGetRAGFlowVersion(t *testing.T) {
	version := GetRAGFlowVersion()
	fmt.Printf("RAGFlow Version: %s\n", version)
	if version == "" {
		t.Error("GetRAGFlowVersion returned empty string")
	}
	if version == "unknown" {
		t.Log("Warning: GetRAGFlowVersion returned 'unknown', VERSION file not found and git command failed")
	}
}

func TestGetClosestTagAndCount(t *testing.T) {
	version := getClosestTagAndCount()
	fmt.Printf("Git Version: %s\n", version)
	// This test just prints the version, no strict assertion
}

func TestIsOlderReleaseThan(t *testing.T) {
	cases := []struct {
		name      string
		code      string
		target    string
		wantOlder bool
		wantCmp   bool
	}{
		{name: "older patch", code: "v0.26.0", target: "v0.27.1", wantOlder: true, wantCmp: true},
		{name: "older minor", code: "v0.26.9", target: "v0.27.1", wantOlder: true, wantCmp: true},
		{name: "older major", code: "v0.27.1", target: "v1.0.0", wantOlder: true, wantCmp: true},
		{name: "equal", code: "v0.27.1", target: "v0.27.1", wantOlder: false, wantCmp: true},
		{name: "newer", code: "v0.28.0", target: "v0.27.1", wantOlder: false, wantCmp: true},
		// git describe reports commits after a tag as v0.27.1-14-g<sha>; that is
		// newer code, not an older release.
		{name: "commits after tag", code: "v0.27.1-14-g1a2b3c4", target: "v0.27.1", wantOlder: false, wantCmp: true},
		{name: "commits after older tag", code: "v0.27.0-3-g1a2b3c4", target: "v0.27.1", wantOlder: true, wantCmp: true},
		{name: "missing v prefix", code: "0.26.0", target: "v0.27.1", wantOlder: true, wantCmp: true},
		{name: "unknown code", code: "unknown", target: "v0.27.1", wantOlder: false, wantCmp: false},
		{name: "empty code", code: "", target: "v0.27.1", wantOlder: false, wantCmp: false},
		{name: "empty target", code: "v0.27.1", target: "", wantOlder: false, wantCmp: false},
		{name: "invalid code", code: "not-a-version", target: "v0.27.1", wantOlder: false, wantCmp: false},
		{name: "invalid target", code: "v0.27.1", target: "not-a-version", wantOlder: false, wantCmp: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			older, comparable := IsOlderReleaseThan(tc.code, tc.target)
			if older != tc.wantOlder || comparable != tc.wantCmp {
				t.Fatalf("IsOlderReleaseThan(%q, %q) = (%v, %v), want (%v, %v)",
					tc.code, tc.target, older, comparable, tc.wantOlder, tc.wantCmp)
			}
		})
	}
}
