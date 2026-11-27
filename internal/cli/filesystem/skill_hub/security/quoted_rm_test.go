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

func scanNamed(filename, content string) []Finding {
	return NewScanner().ScanSkill("skill", "github", "community", map[string][]byte{
		filename: []byte(content),
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

// A substitution in a redirection or as a process substitution does not end
// the simple command, so the operand written after it still belongs to rm.
func TestScanSkillDetectsRmOperandAfterASubstitution(t *testing.T) {
	lines := []string{
		`rm -r >$(true) "label; $T"`,
		`rm -r 2>$(true) "label; $T"`,
		`rm -r <(printf ok) "label; $T"`,
		`rm -r >$(printf ok) "label; $T"`,
		`$(printf ok); rm -r "label; $T"`,
	}
	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			if !hasPattern(scanLine(line), "rm_quoted_expansion") {
				t.Fatalf("%q did not raise rm_quoted_expansion: %v", line, scanLine(line))
			}
		})
	}
}

// A dollar sign that starts no expansion is literal text, and env rejects a
// split string with one before running anything. An env split string also
// treats a word-initial `#` as a comment, and a literal `>` from the split
// value is an argument rather than a shell redirection.
func TestScanSkillIgnoresLiteralDollarAndInvalidEnvSplit(t *testing.T) {
	lines := []string{
		`rm -r "label; $"`,
		`rm -r "label; "$`,
		`rm -r "label; \$T"`,
		`env -S 'rm -rf $T' "a; $T"`,
		`env -S 'rm # -r' "a; $T"`,
		`env -S 'rm -f' "a; $T"`,
	}
	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			if findings := scanLine(line); hasPattern(findings, "rm_quoted_expansion") {
				t.Fatalf("%q raised rm_quoted_expansion: %v", line, findings)
			}
		})
	}

	positives := []string{
		`rm -r "label; $T"`,
		`rm -r "label; $?"`,
		`env -S 'rm -rf ${T}' "a; $T"`,
		`env -S 'rm -rf >' "a; $T"`,
	}
	for _, line := range positives {
		t.Run(line, func(t *testing.T) {
			if !hasPattern(scanLine(line), "rm_quoted_expansion") {
				t.Fatalf("%q did not raise rm_quoted_expansion: %v", line, scanLine(line))
			}
		})
	}
}

// A command substitution that spans physical lines keeps the outer command
// whole: rm after the closing `)` is still an argument of echo.
func TestScanSkillKeepsSubstitutionAcrossPhysicalLines(t *testing.T) {
	content := "#!/bin/sh\necho $(\n  printf ok\n) rm -r \"label; $T\"\n"
	if findings := scanContent(content); hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("rm inside the outer command raised rm_quoted_expansion: %v", findings)
	}
	real := "#!/bin/sh\necho $(\n  printf ok\n); rm -r \"label; $T\"\n"
	if findings := scanContent(real); !hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("the command after the substitution was not reported: %v", findings)
	}
}

// An unquoted here-document still runs command substitutions while it
// expands, so an rm inside `$(...)` is executable even though the rest of the
// body is data.
func TestScanSkillInspectsSubstitutionsInUnquotedHeredoc(t *testing.T) {
	content := "#!/bin/sh\ncat <<EOF\n$(rm -rf \"x; $T\")\nEOF\n"
	findings := scanContent(content)
	if !hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("substitution in the here-document body was not inspected: %v", findings)
	}
	quoted := "#!/bin/sh\ncat <<'EOF'\n$(rm -rf \"x; $T\")\nEOF\n"
	if findings := scanContent(quoted); hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("quoted here-document body raised rm_quoted_expansion: %v", findings)
	}
	// Plain body text is not a command even when the delimiter is unquoted.
	prose := "#!/bin/sh\ncat <<EOF\nrm -rf \"x; $T\"\nEOF\n"
	if findings := scanContent(prose); hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("plain here-document text raised rm_quoted_expansion: %v", findings)
	}
}

// `<<<` is a here-string with no body, so the next line is code.
func TestScanSkillKeepsCodeAfterHereString(t *testing.T) {
	content := "#!/bin/sh\ncat <<< foo\nrm -rf \"a; $T\"\n"
	findings := scanContent(content)
	found := false
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" {
			found = true
			if finding.Line != 3 {
				t.Fatalf("reported line %d, want 3", finding.Line)
			}
		}
	}
	if !found {
		t.Fatalf("command after a here-string was skipped: %v", findings)
	}
}

// An unquoted delimiter has its `\`-newline pairs removed before the shell
// compares it, so the terminator can be split over two physical lines.
func TestScanSkillJoinsContinuedHeredocDelimiter(t *testing.T) {
	content := "#!/bin/sh\ncat <<DOC\nbody\nDO\\\nC\nrm -rf \"a; $T\"\n"
	findings := scanContent(content)
	found := false
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" {
			found = true
			if finding.Line != 6 {
				t.Fatalf("reported line %d, want 6", finding.Line)
			}
		}
	}
	if !found {
		t.Fatalf("command after the continued delimiter was skipped: %v", findings)
	}
}

// A quoted empty delimiter is still a delimiter word: the body ends at the
// first blank line, and the command after it runs.
func TestScanSkillAcceptsQuotedEmptyHeredocDelimiter(t *testing.T) {
	content := "#!/bin/sh\ncat <<''\nrm -rf \"a; $T\"\n\nrm -rf \"b; $T\"\n"
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

// A comment line inside an open substitution ends at the physical newline: the
// command on the next line is still scanned instead of being masked.
func TestScanSkillScansLineAfterCommentInsideSubstitution(t *testing.T) {
	content := "#!/bin/sh\necho $(\n  # note\n  rm -rf \"x; $T\"\n)\n"
	findings := scanContent(content)
	found := false
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" {
			found = true
			if finding.Line != 4 {
				t.Fatalf("reported line %d, want 4", finding.Line)
			}
		}
	}
	if !found {
		t.Fatalf("command after a comment inside the substitution was skipped: %v", findings)
	}

	real := "#!/bin/sh\necho $( # note\n); rm -rf \"label; $T\"\n"
	if findings := scanContent(real); !hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("command after the substitution was not reported: %v", findings)
	}
}

// A finding reports the physical line the offending command is written on, not
// the line its compound command started on.
func TestScanSkillReportsTheStageLineOfAPipeline(t *testing.T) {
	content := "#!/bin/sh\necho start |\n  rm -rf \"a; $T\"\n"
	findings := scanContent(content)
	found := false
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" {
			found = true
			if finding.Line != 3 {
				t.Fatalf("reported line %d, want the second stage's line 3", finding.Line)
			}
		}
	}
	if !found {
		t.Fatalf("second pipeline stage was not reported: %v", findings)
	}
}

// A comment inside a substitution ends at the newline. The outer command keeps
// running: text after the closing parenthesis still belongs to it, while a
// separate command after `;` runs on its own.
func TestScanSkillKeepsOuterCommandAfterCommentInsideSubstitution(t *testing.T) {
	negative := "#!/bin/sh\necho $(printf ok # note\n) rm -rf \"label; $T\"\n"
	if findings := scanContent(negative); hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("argument rm after the substitution raised rm_quoted_expansion: %v", findings)
	}
	positive := "#!/bin/sh\necho $(printf ok # note\n); rm -rf \"label; $T\"\n"
	if findings := scanContent(positive); !hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("the command after the closed substitution was not reported: %v", findings)
	}
}

// A newline between two complete commands inside a substitution separates
// them, so the second one runs.
func TestScanSkillSeparatesCommandsInsideASubstitution(t *testing.T) {
	content := "#!/bin/sh\necho $(\nprintf ok\nrm -rf \"a; $T\"\n)\n"
	findings := scanContent(content)
	found := false
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" {
			found = true
			if finding.Line != 4 {
				t.Fatalf("reported line %d, want 4", finding.Line)
			}
		}
	}
	if !found {
		t.Fatalf("second command inside the substitution was treated as an argument: %v", findings)
	}
}

// Prose parentheses are punctuation, not shell syntax: a Markdown file keeps
// its physical lines, and each destructive line reports its own number.
func TestScanSkillKeepsProseOnPhysicalLines(t *testing.T) {
	content := "Example (\nrm -rf \"a; $T\"\nrm -rf \"b; $T\"\n"
	findings := scanNamed("SKILL.md", content)
	var got []int
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" {
			got = append(got, finding.Line)
		}
	}
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("rm_quoted_expansion lines = %v, want 2 and 3", got)
	}
}

// A here-document body starts at the newline that ends its header, even inside
// an open subshell, so the commands after the delimiter are still scanned.
func TestScanSkillDrainsHeredocInsideSubshell(t *testing.T) {
	content := "#!/bin/sh\n(\ncat <<'DOC'\nrm -rf \"x; $T\"\nDOC\nrm -rf \"a; $T\"\n)\n"
	findings := scanContent(content)
	var got []int
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" {
			got = append(got, finding.Line)
		}
	}
	if len(got) != 1 || got[0] != 6 {
		t.Fatalf("rm_quoted_expansion lines = %v, want only line 6", got)
	}
}

// An escaped pipe is an argument to the previous word, not a pipeline
// operator, so the next physical line is a command of its own.
func TestScanSkillKeepsNextLineAfterEscapedPipe(t *testing.T) {
	content := "#!/bin/sh\nprintf '%s\\n' \\|\nrm -rf \"a; $T\"\n"
	findings := scanContent(content)
	found := false
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" {
			found = true
			if finding.Line != 3 {
				t.Fatalf("reported line %d, want 3", finding.Line)
			}
		}
	}
	if !found {
		t.Fatalf("command after an escaped pipe was merged into the printf arguments: %v", findings)
	}
}

// An even trailing backslash run in an unquoted here-document body is an
// escaped backslash, not a line continuation, so the delimiter still closes
// the body.
func TestScanSkillKeepsEvenBackslashInHeredocBody(t *testing.T) {
	content := "#!/bin/sh\ncat <<DOC\nx\\\\\nDOC\nrm -rf \"a; $T\"\n"
	findings := scanContent(content)
	var got []int
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" {
			got = append(got, finding.Line)
		}
	}
	if len(got) != 1 || got[0] != 5 {
		t.Fatalf("rm_quoted_expansion lines = %v, want only line 5", got)
	}
}

// Inside a double-quoted delimiter only a backslash before `$`, a backtick,
// `"`, `\` or a newline is removed; before any other character it is literal.
func TestScanSkillKeepsLiteralBackslashInDoubleQuotedDelimiter(t *testing.T) {
	content := "#!/bin/sh\ncat <<\"A\\q\"\nrm -rf \"x; $T\"\nA\\q\nrm -rf \"a; $T\"\n"
	findings := scanContent(content)
	var got []int
	for _, finding := range findings {
		if finding.PatternID == "rm_quoted_expansion" {
			got = append(got, finding.Line)
		}
	}
	if len(got) != 1 || got[0] != 5 {
		t.Fatalf("rm_quoted_expansion lines = %v, want only line 5", got)
	}
}

// Deeply nested substitutions are bounded like the env -S rescan: the scan
// finishes and the operand after the substitution is still inspected.
func TestScanSkillBoundsNestedSubstitutions(t *testing.T) {
	line := "rm -r >$(" + strings.Repeat("$(", 64) + "true" + strings.Repeat(")", 65) + " \"label; $T\""
	start := time.Now()
	if findings := scanLine(line); !hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("operand after the nested substitution was not reported: %v", findings)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("nested substitution scan took %v", elapsed)
	}
}

// A substitution is one word of the outer command: separators inside its body
// do not end the outer scan before the rm.
func TestScanSkillKeepsNestedSeparatorsOutOfTheOuterScan(t *testing.T) {
	lines := []string{
		`rm -r >$(printf ok; true) "label; $T"`,
		`rm -r <(printf ok; true) "label; $T"`,
		`env IGNORE=<(printf ok; true) rm -r "label; $T"`,
	}
	for _, line := range lines {
		if findings := scanLine(line); !hasPattern(findings, "rm_quoted_expansion") {
			t.Fatalf("%q: operand after the substitution was missed: %v", line, findings)
		}
	}
}

// Sibling substitutions in one command must not retokenize the growing prefix
// for each opener.
func TestScanSkillScalesWithManySiblingSubstitutions(t *testing.T) {
	line := "env " + strings.Repeat("A=<(true) ", 20000) + "rm -r \"label; $T\""
	start := time.Now()
	if findings := scanLine(line); !hasPattern(findings, "rm_quoted_expansion") {
		t.Fatalf("operand after many sibling substitutions was not reported")
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("scanning 20000 sibling substitutions took %v", elapsed)
	}
}
