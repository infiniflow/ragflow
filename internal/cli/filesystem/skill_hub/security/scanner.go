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

	for _, pattern := range s.patterns {
		re, err := regexp.Compile("(?i:" + pattern.Pattern + ")")
		if err != nil {
			continue
		}

		for i, line := range lines {
			if matches := re.FindString(line); matches != "" {
				findings = append(findings, Finding{
					PatternID:   pattern.PatternID,
					Severity:    pattern.Severity,
					Category:    pattern.Category,
					File:        filename,
					Line:        i + 1,
					Match:       strings.TrimSpace(matches),
					Description: pattern.Description,
				})
			}
		}
	}

	for i, line := range lines {
		if operand, ok := scanQuotedRmExpansion(line); ok {
			findings = append(findings, Finding{
				PatternID:   "rm_quoted_expansion",
				Severity:    "high",
				Category:    "destructive",
				File:        filename,
				Line:        i + 1,
				Match:       strings.TrimSpace(operand),
				Description: "recursive rm of a quoted operand with a separator before an expansion",
			})
		}
	}

	return findings
}

// scanQuotedRmExpansion is a quote-aware pass that stays out of
// ThreatPatterns: a regular expression over the raw line cannot track quote
// and escape state across an operand. The destructive-rm rules stop an
// unquoted operand at a separator, so a separator inside a double-quoted
// operand hides an expansion that follows it. The operand can also continue
// after the closing quote with more quoted or unquoted fragments, and `\"`
// does not end the quoted span. Return the operand that carries the hidden
// expansion.
func scanQuotedRmExpansion(line string) (string, bool) {
	code := shellCodeMask(line)
	for i := 0; i+2 <= len(line); i++ {
		if !code[i] || !code[i+1] {
			continue
		}
		if line[i:i+2] != "rm" || (i > 0 && isShellWordByte(line[i-1])) {
			continue
		}
		if i+2 < len(line) && line[i+2] != ' ' && line[i+2] != '\t' {
			continue
		}

		recursive, endOfOptions := false, false
		var operands []string
		for _, word := range shellWords(line[i+2:]) {
			if !endOfOptions && word == "--" {
				endOfOptions = true
				continue
			}
			if !endOfOptions && len(word) > 1 && word[0] == '-' {
				if isRecursiveFlag(word) {
					recursive = true
				}
				continue
			}
			operands = append(operands, word)
		}
		if !recursive {
			continue
		}
		for _, operand := range operands {
			if operandHidesExpansion(operand) {
				return operand, true
			}
		}
	}
	return "", false
}

func isShellWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isRecursiveFlag(word string) bool {
	if strings.HasPrefix(word, "--") {
		name := word[2:]
		return name != "" && !strings.ContainsRune(name, '=') && strings.HasPrefix("recursive", name)
	}
	for i := 1; i < len(word); i++ {
		if word[i] == 'r' || word[i] == 'R' {
			return true
		}
	}
	return false
}

// shellCodeMask marks the characters that belong to unquoted code. A command
// name inside a quoted string or after a comment is not a command.
func shellCodeMask(line string) []bool {
	mask := make([]bool, len(line))
	inSingle, inDouble, escaped := false, false, false
	atWordStart := true

	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case escaped:
			escaped = false
			mask[i] = !inSingle && !inDouble
			atWordStart = false
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case inDouble:
			switch c {
			case '\\':
				escaped = true
			case '"':
				inDouble = false
			}
		default:
			switch c {
			case '\\':
				escaped = true
				mask[i] = true
				atWordStart = false
			case '\'':
				inSingle = true
				atWordStart = false
			case '"':
				inDouble = true
				atWordStart = false
			case '#':
				if atWordStart {
					return mask
				}
				mask[i] = true
				atWordStart = false
			case ' ', '\t':
				mask[i] = true
				atWordStart = true
			case ';', '&', '|':
				mask[i] = true
				atWordStart = true
			default:
				mask[i] = true
				atWordStart = false
			}
		}
	}
	return mask
}

// shellWords splits one simple command into words. Quoted spans are kept
// whole, a backslash hides the next character outside single quotes, an
// unquoted separator ends the command, and an unquoted `#` at a word start
// opens a comment.
func shellWords(s string) []string {
	var words []string
	var word strings.Builder
	inWord := false
	flush := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}

	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case ' ', '\t', '\r':
			flush()
		case ';', '&', '|', '\n':
			flush()
			return words
		case '#':
			if !inWord {
				return words
			}
			inWord = true
			word.WriteByte(c)
		case '\'':
			inWord = true
			word.WriteByte(c)
			j := i + 1
			for j < len(s) && s[j] != '\'' {
				word.WriteByte(s[j])
				j++
			}
			if j < len(s) {
				word.WriteByte(s[j])
			}
			i = j
		case '"':
			inWord = true
			word.WriteByte(c)
			j := i + 1
			for j < len(s) {
				if s[j] == '\\' && j+1 < len(s) {
					word.WriteByte(s[j])
					word.WriteByte(s[j+1])
					j += 2
					continue
				}
				if s[j] == '"' {
					word.WriteByte(s[j])
					j++
					break
				}
				word.WriteByte(s[j])
				j++
			}
			i = j - 1
		case '\\':
			inWord = true
			word.WriteByte(c)
			if i+1 < len(s) {
				i++
				word.WriteByte(s[i])
			}
		default:
			inWord = true
			word.WriteByte(c)
		}
	}
	flush()
	return words
}

// isDoubleQuoteEscapable reports whether a backslash gives the next byte
// special meaning inside double quotes.
func isDoubleQuoteEscapable(c byte) bool {
	switch c {
	case '$', '`', '"', '\\', '\n':
		return true
	}
	return false
}

// operandHidesExpansion reports whether a word holds a separator inside a
// double-quoted span before an expansion anywhere later in the same word. A
// single-quoted or escaped `$` expands nothing.
func operandHidesExpansion(operand string) bool {
	inSingle, inDouble, escaped := false, false, false
	separatorInDouble := false

	for i := 0; i < len(operand); i++ {
		c := operand[i]
		switch {
		case escaped:
			escaped = false
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case inDouble:
			switch c {
			case '\\':
				// Inside double quotes a backslash only escapes $, `, ", \
				// and newline; before anything else it stays literal and the
				// next byte keeps its meaning, so `a\;$T` still holds a
				// separator ahead of the expansion.
				if i+1 < len(operand) && isDoubleQuoteEscapable(operand[i+1]) {
					escaped = true
				}
			case '"':
				inDouble = false
			case '$':
				if separatorInDouble {
					return true
				}
			case ';', '&', '|', '#':
				separatorInDouble = true
			}
		default:
			switch c {
			case '\\':
				escaped = true
			case '\'':
				inSingle = true
			case '"':
				inDouble = true
			case '$':
				if separatorInDouble {
					return true
				}
			}
		}
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
