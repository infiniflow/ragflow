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
	TrustLevel string   // builtin | trusted | community
	Verdict    string   // safe | caution | dangerous
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

	return findings
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
