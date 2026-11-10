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

// TestPrimaryTier pins the community primary ingestion lane. The community
// build has a single lane (the bare TaskSubject queue), so the admin pull
// endpoint and the memory wake-up fallback must resolve PrimaryTier() to the
// low lane, whose subject equals TaskSubject.
func TestPrimaryTier(t *testing.T) {
	const want = TierLow
	if got := PrimaryTier(); got != want {
		t.Fatalf("PrimaryTier() = %q, want %q", got, want)
	}
	subject, err := SubjectForTier(PrimaryTier())
	if err != nil {
		t.Fatalf("SubjectForTier(PrimaryTier()) returned error: %v", err)
	}
	if subject == "" {
		t.Fatalf("SubjectForTier(PrimaryTier()) returned empty subject")
	}
	if subject != TaskSubject {
		t.Fatalf("PrimaryTier() subject = %q, want %q", subject, TaskSubject)
	}
}
