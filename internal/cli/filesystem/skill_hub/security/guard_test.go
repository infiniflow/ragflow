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
	"testing"
)

// resultFor builds a ScanResult at a given trust level and verdict with findings
// consistent with that verdict, for exercising the install policy.
func resultFor(trustLevel, verdict string) *ScanResult {
	res := &ScanResult{SkillName: "t", Source: "community", TrustLevel: trustLevel, Verdict: verdict}
	switch verdict {
	case "dangerous":
		res.Findings = []Finding{{PatternID: "p", Severity: "critical"}}
	case "caution":
		res.Findings = []Finding{{PatternID: "p", Severity: "high"}}
	default:
		res.Findings = nil
	}
	return res
}

// TestGuardInstallPolicyMatrix pins the 3 trust levels x 3 verdicts install
// policy. This is the gate that decides whether a scanned skill installs;
// the community column is the security-critical one (only safe installs).
func TestGuardInstallPolicyMatrix(t *testing.T) {
	g := NewGuard()
	cases := []struct {
		trust  string
		verdict string
		allow  bool
	}{
		{"builtin", "safe", true},
		{"builtin", "caution", true},
		{"builtin", "dangerous", true},
		{"trusted", "safe", true},
		{"trusted", "caution", true},
		{"trusted", "dangerous", false},
		{"community", "safe", true},
		{"community", "caution", false},
		{"community", "dangerous", false},
	}
	for _, c := range cases {
		t.Run(c.trust+"/"+c.verdict, func(t *testing.T) {
			allowed, reason := g.ShouldAllowInstall(resultFor(c.trust, c.verdict), false)
			if allowed != c.allow {
				t.Fatalf("trust=%s verdict=%s: allowed=%v want %v (reason: %s)", c.trust, c.verdict, allowed, c.allow, reason)
			}
		})
	}
}

// TestGuardForceOverridesBlock verifies --force overrides every blocked
// verdict, matching the install flow's force semantics.
func TestGuardForceOverridesBlock(t *testing.T) {
	g := NewGuard()
	blocked := []struct {
		trust   string
		verdict string
	}{
		{"trusted", "dangerous"},
		{"community", "caution"},
		{"community", "dangerous"},
	}
	for _, c := range blocked {
		t.Run(c.trust+"/"+c.verdict, func(t *testing.T) {
			// Without force the cell blocks.
			if allowed, _ := g.ShouldAllowInstall(resultFor(c.trust, c.verdict), false); allowed {
				t.Fatalf("trust=%s verdict=%s should block without --force", c.trust, c.verdict)
			}
			// With force it is allowed.
			allowed, reason := g.ShouldAllowInstall(resultFor(c.trust, c.verdict), true)
			if !allowed {
				t.Fatalf("trust=%s verdict=%s: --force should allow, got blocked (reason: %s)", c.trust, c.verdict, reason)
			}
		})
	}
}
