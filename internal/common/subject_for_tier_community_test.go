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

//go:build !ee

package common

import "testing"

// TestSubjectForTierCommunityDefault pins the community mapping in the
// build-tagged policy file: the single default lane resolves to the bare
// community subject. This guards the P1 fix — the mapping must live in the
// edition-specific file, not in the shared task.go switch.
func TestSubjectForTierCommunityDefault(t *testing.T) {
	got, err := subjectForTier(TierDefault)
	if err != nil {
		t.Fatalf("subjectForTier(TierDefault) returned error: %v", err)
	}
	if got != TaskSubject {
		t.Fatalf("subjectForTier(TierDefault) = %q, want %q", got, TaskSubject)
	}
}

// TestSubjectForTierCommunityUnknownErrors is the adversarial case: any tier
// other than the default must error rather than silently fall back to the
// default lane. A wrong implementation that returns TaskSubject for an unknown
// tier would fail this test, which is exactly the regression the build-tagged
// split prevents (the enterprise build supplies its own high/medium lanes).
func TestSubjectForTierCommunityUnknownErrors(t *testing.T) {
	const bogus Tier = "does-not-exist"
	got, err := subjectForTier(bogus)
	if err == nil {
		t.Fatalf("subjectForTier(%q) returned nil error; unknown tiers must error", bogus)
	}
	if got != "" {
		t.Fatalf("subjectForTier(%q) returned %q on error; want empty subject", bogus, got)
	}
}
