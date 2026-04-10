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
	"fmt"
	"strings"
)

// Guard provides security policy enforcement
type Guard struct {
	trustedRepos map[string]bool
	policy       map[string][3]string
}

// NewGuard creates a new security guard
func NewGuard() *Guard {
	return &Guard{
		trustedRepos: TrustedRepos,
		policy:       InstallPolicy,
	}
}

// ResolveTrustLevel determines the trust level based on source and identifier
func (g *Guard) ResolveTrustLevel(source, identifier string) string {
	// Official/builtin source
	if source == "official" || source == "builtin" {
		return "builtin"
	}

	// Check against trusted repositories
	for repo := range g.trustedRepos {
		if strings.Contains(identifier, repo) {
			return "trusted"
		}
	}

	// Default to community
	return "community"
}

// ShouldAllowInstall determines if installation should be allowed based on scan results
// Returns (allowed bool, reason string)
func (g *Guard) ShouldAllowInstall(result *ScanResult, force bool) (bool, string) {
	policy, ok := g.policy[result.TrustLevel]
	if !ok {
		policy = g.policy["community"]
	}

	vi, ok := VerdictIndex[result.Verdict]
	if !ok {
		vi = 2 // dangerous
	}

	decision := policy[vi]

	switch decision {
	case "allow":
		return true, fmt.Sprintf("Allowed (%s source, %s verdict)", result.TrustLevel, result.Verdict)
	case "ask":
		return false, fmt.Sprintf("Requires confirmation (%s source + %s verdict, %d findings)",
			result.TrustLevel, result.Verdict, len(result.Findings))
	case "block":
		if force {
			return true, fmt.Sprintf("Force-installed despite %s verdict (%d findings)",
				result.Verdict, len(result.Findings))
		}
		return false, fmt.Sprintf("Blocked (%s source + %s verdict, %d findings). Use --force to override.",
			result.TrustLevel, result.Verdict, len(result.Findings))
	}

	return false, "Unknown policy decision"
}

// FormatScanReport formats a scan result for display
func (g *Guard) FormatScanReport(result *ScanResult) string {
	var sb strings.Builder

	sb.WriteString("╔════════════════════════════════════════════════════════════════╗\n")
	sb.WriteString(fmt.Sprintf("║ Security Scan Report: %-40s ║\n", result.SkillName))
	sb.WriteString("╚════════════════════════════════════════════════════════════════╝\n")
	sb.WriteString(fmt.Sprintf("Source:      %s\n", result.Source))
	sb.WriteString(fmt.Sprintf("Trust Level: %s\n", result.TrustLevel))
	sb.WriteString(fmt.Sprintf("Verdict:     %s\n", result.Verdict))
	sb.WriteString(fmt.Sprintf("Findings:    %d\n", len(result.Findings)))

	if len(result.Findings) > 0 {
		sb.WriteString("\n─── Findings ───\n")

		// Group by severity
		severityOrder := []string{"critical", "high", "medium", "low"}
		for _, sev := range severityOrder {
			for _, f := range result.Findings {
				if f.Severity == sev {
					sb.WriteString(fmt.Sprintf("\n[%s] %s\n", strings.ToUpper(sev), f.PatternID))
					sb.WriteString(fmt.Sprintf("  Category: %s\n", f.Category))
					sb.WriteString(fmt.Sprintf("  File: %s:%d\n", f.File, f.Line))
					sb.WriteString(fmt.Sprintf("  Match: %s\n", f.Match))
					sb.WriteString(fmt.Sprintf("  Description: %s\n", f.Description))
				}
			}
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// AddTrustedRepo adds a repository to the trusted list
func (g *Guard) AddTrustedRepo(repo string) {
	g.trustedRepos[repo] = true
}

// IsTrustedRepo checks if a repository is trusted
func (g *Guard) IsTrustedRepo(repo string) bool {
	return g.trustedRepos[repo]
}
