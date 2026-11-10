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

package nats

import (
	"testing"

	"ragflow/internal/common"
)

// TestSubjectForTierLowIsCommunitySubject (Option A): the low tier subject must
// be the bare community/OSS subject "tasks.RAGFLOW", so the open-source
// edition's single queue maps onto the lowest-priority lane and one NATS server
// serves both editions. It must NOT be the ".low" suffixed subject.
func TestSubjectForTierLowIsCommunitySubject(t *testing.T) {
	got, err := common.SubjectForTier(common.TierLow)
	if err != nil {
		t.Fatalf("SubjectForTier(TierLow) returned error: %v", err)
	}
	if got != "tasks.RAGFLOW" {
		t.Fatalf("SubjectForTier(TierLow) = %q, want community subject tasks.RAGFLOW", got)
	}
}

// TestLowTierReusesCommunityQueueNames (Option A): the low lane must reuse the
// community/OSS stream, subject and consumer names (RAGFLOW_TASKS /
// tasks.RAGFLOW / RAGFLOW_CONSUMER) so an OSS split needs no rename and OSS and
// EE can share one NATS server. A wrong implementation that keeps the _LOW
// suffixed names would fail here.
func TestLowTierReusesCommunityQueueNames(t *testing.T) {
	spec := tierStreams[common.TierLow]
	if spec.streamName != "RAGFLOW_TASKS" {
		t.Fatalf("low tier stream = %q, want RAGFLOW_TASKS (community queue name)", spec.streamName)
	}
	if len(spec.subjects) != 1 || spec.subjects[0] != "tasks.RAGFLOW" {
		t.Fatalf("low tier subjects = %v, want [tasks.RAGFLOW]", spec.subjects)
	}
	if spec.consumerName != "RAGFLOW_CONSUMER" {
		t.Fatalf("low tier consumer = %q, want RAGFLOW_CONSUMER", spec.consumerName)
	}
}
