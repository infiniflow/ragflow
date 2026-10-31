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

package security

import (
	"regexp"
	"testing"
)

// scanFile skips a pattern whose expression does not compile, so a typo in a
// rule removes it from detection without any signal. Lock every rule to a
// compilable expression.
func TestThreatPatternsCompile(t *testing.T) {
	for _, pattern := range ThreatPatterns {
		if _, err := regexp.Compile("(?i:" + pattern.Pattern + ")"); err != nil {
			t.Errorf("pattern %s does not compile: %v", pattern.PatternID, err)
		}
	}
}

func TestScanSkillDetectsRecursiveRmOfExpandedTarget(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"short flag", "rm -r $TARGET"},
		{"bundled flags", "rm -rf ${TARGET}"},
		{"long flag", "rm --recursive $TARGET"},
		{"recursive flag after another option", `rm -f -r "$TARGET"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewScanner().ScanSkill("skill", "github", "community", map[string][]byte{
				"install.sh": []byte("#!/bin/sh\n" + tt.line + "\n"),
			})
			for _, finding := range result.Findings {
				if finding.PatternID == "rm_recursive_dangerous" {
					return
				}
			}
			t.Fatalf("%q did not raise rm_recursive_dangerous: %v", tt.line, result.Findings)
		})
	}
}

// The removal target must end at a shell command separator, otherwise a
// variable expanded by a later command in the same line reads as an expanded
// removal target and blocks an otherwise safe skill.
func TestScanSkillIgnoresExpansionAfterRemovalCommand(t *testing.T) {
	line := `rm -rf build && echo "$USER"`
	result := NewScanner().ScanSkill("skill", "github", "community", map[string][]byte{
		"install.sh": []byte("#!/bin/sh\n" + line + "\n"),
	})
	for _, finding := range result.Findings {
		if finding.PatternID == "rm_recursive_dangerous" {
			t.Fatalf("%q raised rm_recursive_dangerous: %v", line, finding)
		}
	}
}
