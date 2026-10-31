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

import "testing"

func TestScanSkillDetectsRmQuotedExpansion(t *testing.T) {
	lines := []string{
		`rm -rf "prefix; $TARGET"`,
		`rm -r "p;/../victim$SUFFIX"`,
		`rm --recursive --force "a|$T"`,
		`rm -R 'skip' "a;$T"`,
		`rm -rf "a" "b; $T"`,
		`sudo rm -rf "a; $T"`,
		`rm -rf -- "a; $T"`,
		`rm --rec "a; $T"`,
		`/bin/rm -rf "a; $T"`,
		`rm -rf -v "a; $T"`,
		`rm -rf "prefix; "$TARGET`,
		`rm -rf "prefix; \"part$TARGET"`,
		`rm -rf "a#b $T"`,
		`rm -rf "a; $T"#note`,
	}

	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			if !hasPattern(scanLine(line), "rm_quoted_expansion") {
				t.Fatalf("%q did not raise rm_quoted_expansion: %v", line, scanLine(line))
			}
		})
	}
}

func TestScanSkillIgnoresRmQuotedExpansion(t *testing.T) {
	lines := []string{
		`rm -rf "a; b"`,
		`rm -rf build && echo "$USER"`,
		`rm -rf "a" && echo "$USER"`,
		`rm -rf "a; b" && echo $USER`,
		`transform -rf "a; $T"`,
		`rm -rf 'a; $T'`,
		`rm -rf 'a; ' $T`,
		`rm -rf "$TARGET"`,
		`rm -rf "a $T"`,
		`rm -rf "a; b" $T`,
		`rm -rf "prefix; \$TARGET"`,
		`rm -rf "a; b" # $T`,
		`echo "rm -rf a; $T"`,
		`rmdir "a; $T"`,
		`rm -fd "a; $T"`,
	}

	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			for _, finding := range scanLine(line) {
				if finding.PatternID == "rm_quoted_expansion" {
					t.Fatalf("%q raised rm_quoted_expansion: %v", line, finding)
				}
			}
		})
	}
}

func hasPattern(findings []Finding, id string) bool {
	for _, finding := range findings {
		if finding.PatternID == id {
			return true
		}
	}
	return false
}

func scanLine(line string) []Finding {
	return NewScanner().ScanSkill("skill", "github", "community", map[string][]byte{
		"install.sh": []byte("#!/bin/sh\n" + line + "\n"),
	}).Findings
}
