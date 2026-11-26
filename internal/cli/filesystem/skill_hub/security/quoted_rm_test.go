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
	"strings"
	"testing"
	"time"
)

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
		`rm -rf "a\;$TARGET"`,
		`rm -rf "a\&$T"`,
		`rm -rf "a#b $T"`,
		`rm -rf "a; $T"#note`,
		`$(rm -rf "a; $T")`,
		`echo && rm -rf "a; $T"`,
		`FOO=1 rm -rf "a; $T"`,
		`sudo -n rm -rf "a; $T"`,
		`sudo -u root rm -rf "a; $T"`,
		`sudo -uroot rm -rf "a; $T"`,
		`sudo --user=root rm -rf "a; $T"`,
		`xargs -n 1 rm -rf "a; $T"`,
		`bin/rm -rf "a; $T"`,
		`./build/bin/rm -rf "a; $T"`,
		`LABEL="a b" rm -rf "a; $T"`,
		`if true; then rm -rf "a; $T"; fi`,
		`for f in a b; do rm -rf "a; $T"; done`,
		`while true; do rm -rf "a; $T"; done`,
		`{ rm -rf "a; $T"; }`,
		`f() { rm -rf "a; $T"; }`,
		`echo <(rm -rf "a; $T")`,
		`echo >(rm -rf "a; $T")`,
		`env IGNORE=<(printf ok) rm -rf "p;/../victim$SUFFIX"`,
		`$(case x in x) rm -rf "p;/../victim$SUFFIX";; esac)`,
		`case $x in a) rm -rf "a; $T";; b) rm -rf "b; $T";; esac`,
		`case $x in (a) rm -rf "a; $T";; esac`,
		`env -u root rm -rf "a; $T"`,
		`rm -r "a; $T" > "log; $T"`,
		`env -S rm -rf "a; $T"`,
		`env -S 'rm' -rf "a; $T"`,
		`env -S 'sudo -u root rm' -rf "a; $T"`,
		`command -p rm -rf "a; $T"`,
		`printf 'x\n' | xargs -I -n rm -rf "a; $T"`,
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
		`# rm -rf "a; $T"`,
		`echo 'rm -rf "a; $T"'`,
		`echo "rm -rf \"a; $T\""`,
		`echo "rm -rf a; $T"`,
		`echo rm -rf "a; $T"`,
		`echo -n rm -rf "a; $T"`,
		`sudo -u root echo rm -rf "a; $T"`,
		`echo then rm -rf "a; $T"`,
		`echo do rm -rf "a; $T"`,
		`/usr/bin/tr.rm -rf "a; $T"`,
		`./build.rm -rf "a; $T"`,
		`sudo -u root transform -rf "a; $T"`,
		`rmdir "a; $T"`,
		`rm -fd "a; $T"`,
		`echo { rm -rf "a; $T"`,
		`echo $(printf ok) rm -rf "a; $T"`,
		`echo \) rm -rf "a; $T"`,
		`echo <(printf ok) rm -rf "a; $T"`,
		`echo >(printf ok) rm -rf "a; $T"`,
		`env IGNORE=<(printf ok) echo rm -rf "a; $T"`,
		`sudo $(printf ok) rm -rf "a; $T"`,
		`echo $(case $x in (a) echo x;; esac) rm -rf "a; $T"`,
		`env -u rm printf %s -r "label; $T"`,
		`rm -r < "input; $T"`,
		`echo $(case x in (x) printf ok;; y) printf ok;; esac) rm -r "label; $T"`,
		`echo $(case x in x) printf %s esac;; y) printf ok;; esac) rm -r "label; $T"`,
		`env -S 'rm -rf "a; b"'`,
		`env -S 'echo rm -rf "a; $T"'`,
		`env -S 'rm -rf "a; $T"'`,
		`env -S'rm -rf "a; $T"'`,
		`env --split-string 'rm -rf "a; $T"'`,
		`env --split-string='rm -rf "a; $T"'`,
		`env -S 'sudo -u root rm -rf "a; $T"'`,
		`env --split-string 'printf %s "a; $T"'`,
		`command -v rm -rf "a; $T"`,
		`command -V rm -rf "a; $T"`,
		`command -pv rm -rf "a; $T"`,
		`xargs -n 1 echo rm -rf "a; $T"`,
		`xargs -I -n echo rm -rf "a; $T"`,
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

func TestScanSkillJoinsBackslashContinuations(t *testing.T) {
	cases := []string{
		"#!/bin/sh\nrm -rf \\\n  \"build; $HOME\"\n",
		"#!/bin/sh\nsudo -u root rm -rf \\\n  \"build; $HOME\"\n",
		"#!/bin/sh\nrm -rf \"build;\\\n$HOME\"\n",
	}
	for _, content := range cases {
		t.Run(content, func(t *testing.T) {
			findings := scanContent(content)
			if !hasPattern(findings, "rm_quoted_expansion") {
				t.Fatalf("%q did not raise rm_quoted_expansion: %v", content, findings)
			}
			for _, finding := range findings {
				if finding.PatternID == "rm_quoted_expansion" && finding.Line != 2 {
					t.Fatalf("%q reported line %d, want the logical line's first physical line 2", content, finding.Line)
				}
			}
		})
	}
}

func TestScanSkillKeepsCommentAfterContinuation(t *testing.T) {
	// The shell removes the first `\<newline>`, so the `#` follows a blank and
	// opens a comment. The backslash before the second newline sits inside that
	// comment and stays literal: the newline ends the comment, so rm still runs
	// on line 3 and must be found there.
	content := "echo \\\n# c \\\nrm -rf \"p;/../victim$SUFFIX\"\n"
	findings := scanContent(content)
	if !hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("missing quoted expansion on the continued command: %v", findings)
	}
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" && finding.Line != 3 {
			t.Fatalf("reported line %d, want physical line 3", finding.Line)
		}
	}
}

func TestScanSkillJoinsContinuationAfterApostropheInDoubleQuotes(t *testing.T) {
	// An apostrophe is literal inside a double-quoted span: it must not open
	// single-quote mode and swallow the backslash-newline that follows.
	content := "#!/bin/sh\nrm -rf \"it's;\\\n$HOME\"\n"
	findings := scanContent(content)
	if !hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("missing quoted expansion: %v", findings)
	}
}

func TestScanSkillKeepsSingleQuotedBackslashLiteral(t *testing.T) {
	// Inside single quotes a backslash does not continue the line, so the
	// operand stays split across two physical lines and hides nothing.
	content := "#!/bin/sh\nrm -rf 'build;\\\n$HOME'\n"
	for _, finding := range scanContent(content) {
		if finding.PatternID == "rm_quoted_expansion" {
			t.Fatalf("single-quoted backslash raised rm_quoted_expansion: %v", finding)
		}
	}
}

func TestScanSkillKeepsCommentNewline(t *testing.T) {
	// A comment ends at its physical newline, so the backslash at its end
	// continues nothing and the next line still runs.
	content := "#!/bin/sh\n# note \\\nrm -rf \"p;/../victim$SUFFIX\"\n"
	findings := scanContent(content)
	if !hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("missing quoted expansion after a comment: %v", findings)
	}
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" && finding.Line != 3 {
			t.Fatalf("reported line %d, want the physical line 3", finding.Line)
		}
	}
}

func TestScanSkillSkipsHeredocBodies(t *testing.T) {
	// The body of a quoted here-document is literal input to cat, not a
	// command the shell runs; neither its rm example nor a real command after
	// the delimiter may be confused with it.
	content := "#!/bin/sh\ncat <<'DOC' > notes.txt\nrm -rf \"build; $HOME\"\nDOC\nrm -rf \"build; $HOME\"\n"
	findings := scanContent(content)
	var got []int
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" {
			got = append(got, finding.Line)
		}
	}
	if len(got) != 1 || got[0] != 5 {
		t.Fatalf("rm_quoted_expansion lines = %v, want only the command on line 5", got)
	}
}

func TestScanSkillKeepsQuotedStringAcrossPhysicalLines(t *testing.T) {
	// A single-quoted printf format spans two physical lines; the rm text
	// inside it is data, while a real command after the closing quote still
	// runs and must be found.
	content := "#!/bin/sh\nprintf 'name: %s\nrm -rf \"a; $T\"\n' x && rm -rf \"b; $T\"\n"
	if findings := scanContent(content); !hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("missing the command after the closing quote: %v", findings)
	}
	multiline := "#!/bin/sh\nprintf 'name: %s\nrm -rf \"a; $T\"\n' x\n"
	for _, finding := range scanContent(multiline) {
		if finding.PatternID == "rm_quoted_expansion" {
			t.Fatalf("quoted format text raised rm_quoted_expansion: %v", finding)
		}
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
	return scanContent("#!/bin/sh\n" + line + "\n")
}

func scanContent(content string) []Finding {
	return NewScanner().ScanSkill("skill", "github", "community", map[string][]byte{
		"install.sh": []byte(content),
	}).Findings
}

func TestScanSkillScalesWithManyCandidateCommands(t *testing.T) {
	// Every rm here is an argument of echo, so the pass must reject each one
	// from the words of its own command instead of rescanning the growing line
	// for every candidate.
	line := strings.Repeat(`echo rm -rf "a; $T"; `, 120000)
	start := time.Now()
	for _, finding := range scanLine(line) {
		if finding.PatternID == "rm_quoted_expansion" {
			t.Fatalf("argument rm raised rm_quoted_expansion: %v", finding)
		}
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("scanning %d candidate commands took %v", 120000, elapsed)
	}
}
