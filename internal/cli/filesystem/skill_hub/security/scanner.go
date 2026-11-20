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
	"path/filepath"
	"regexp"
	"strings"
)

// Finding represents a security issue found during scanning
type Finding struct {
	PatternID   string // Rule ID
	Severity    string // critical | high | medium | low
	Category    string // exfiltration | injection | destructive | persistence | network | obfuscation
	File        string // File path where found
	Line        int    // Line number
	Match       string // The matched text
	Description string // Human-readable description
}

// ScanResult represents the result of a security scan
type ScanResult struct {
	SkillName  string
	Source     string
	TrustLevel string // builtin | trusted | community
	Verdict    string // safe | caution | dangerous
	Findings   []Finding
}

// Scanner performs security scans on skill content
type Scanner struct {
	patterns []ThreatPattern
}

// NewScanner creates a new security scanner
func NewScanner() *Scanner {
	return &Scanner{
		patterns: ThreatPatterns,
	}
}

// ScanSkill scans skill files for security threats
func (s *Scanner) ScanSkill(skillName, source, trustLevel string, files map[string][]byte) *ScanResult {
	var allFindings []Finding

	for filename, content := range files {
		findings := s.scanFile(filename, string(content))
		allFindings = append(allFindings, findings...)
	}

	verdict := s.determineVerdict(allFindings)

	return &ScanResult{
		SkillName:  skillName,
		Source:     source,
		TrustLevel: trustLevel,
		Verdict:    verdict,
		Findings:   allFindings,
	}
}

// scanFile scans a single file for threats
func (s *Scanner) scanFile(filename, content string) []Finding {
	var findings []Finding
	lines := strings.Split(content, "\n")
	// A commented-out example such as "# Example: rm -rf $TARGET" is not a
	// command, but a destructive-rm finding makes the skill uninstallable for
	// community sources, so match against the code-only view of each line.
	masked := make([]string, len(lines))
	for i, line := range lines {
		if isProseFile(filename) {
			masked[i] = line
			continue
		}
		masked[i] = maskShellComments(line)
	}

	for _, pattern := range s.patterns {
		re, err := regexp.Compile("(?i:" + pattern.Pattern + ")")
		if err != nil {
			continue
		}

		for i, line := range masked {
			loc := re.FindStringIndex(line)
			if loc == nil {
				continue
			}
			findings = append(findings, Finding{
				PatternID:   pattern.PatternID,
				Severity:    pattern.Severity,
				Category:    pattern.Category,
				File:        filename,
				Line:        i + 1,
				Match:       strings.TrimSpace(lines[i][loc[0]:loc[1]]),
				Description: pattern.Description,
			})
		}
	}

	return findings
}

// maskShellComments blanks the text a POSIX shell treats as a comment without
// changing the line length, so match offsets stay valid for the original line.
// A '#' opens a comment only where a word may start and only outside quotes;
// '#' inside a word (a#b), in an expansion ($#) or in a quoted string is data.
func maskShellComments(line string) string {
	var masked []byte
	inComment := false
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inComment {
			if masked == nil {
				masked = []byte(line)
			}
			masked[i] = ' '
			continue
		}
		if quote != 0 {
			if c == '\\' && quote == '"' && i+1 < len(line) {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\\':
			if i+1 < len(line) {
				i++
			}
		case '\'', '"':
			quote = c
		case '#':
			if i == 0 || isShellWordBreak(line[i-1]) {
				inComment = true
				if masked == nil {
					masked = []byte(line)
				}
				masked[i] = ' '
			}
		}
	}
	if masked == nil {
		return line
	}
	return string(masked)
}

func isShellWordBreak(c byte) bool {
	switch c {
	case ' ', '\t', ';', '&', '|', '(', ')', '<', '>':
		return true
	}
	return false
}

// Markdown is prose the assistant reads as instructions, so a line such as
// "# ignore previous instructions" is content, not a comment. Only files where
// '#' introduces something the shell would skip are masked.
func isProseFile(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".md", ".markdown", ".mdx", ".txt", ".rst", ".adoc":
		return true
	}
	return false
}

// determineVerdict determines the overall verdict based on findings
func (s *Scanner) determineVerdict(findings []Finding) string {
	if len(findings) == 0 {
		return "safe"
	}

	hasCritical := false
	hasHigh := false

	for _, f := range findings {
		if f.Severity == "critical" {
			hasCritical = true
		} else if f.Severity == "high" {
			hasHigh = true
		}
	}

	if hasCritical {
		return "dangerous"
	}
	if hasHigh {
		return "caution"
	}
	return "caution"
}

// HasCriticalChecks if any finding is critical severity
func (r *ScanResult) HasCritical() bool {
	for _, f := range r.Findings {
		if f.Severity == "critical" {
			return true
		}
	}
	return false
}

// CountBySeverity counts findings by severity level
func (r *ScanResult) CountBySeverity(severity string) int {
	count := 0
	for _, f := range r.Findings {
		if f.Severity == severity {
			count++
		}
	}
	return count
}
