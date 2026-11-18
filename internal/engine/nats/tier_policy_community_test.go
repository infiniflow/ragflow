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

package nats

import (
	"testing"

	"ragflow/internal/common"
)

// TestCommunityTierPolicyIsSingleQueue pins the open-source / community build to
// exactly one ingestion lane: the bare community subject "tasks.RAGFLOW". The
// community build must NOT create any additional streams (that would waste probe
// time on every pull and orphan unused streams). This is the guarantee that lets
// the OSS split reuse this engine unchanged: a single queue, no enterprise tier
// names.
func TestCommunityTierPolicyIsSingleQueue(t *testing.T) {
	if len(tierStreams) != 1 {
		t.Fatalf("community tierStreams has %d entries, want exactly 1 (single queue); got %v", len(tierStreams), tierStreams)
	}
	spec, ok := tierStreams[common.TierDefault]
	if !ok {
		t.Fatalf("community tierStreams missing TierDefault entry: %v", tierStreams)
	}
	if spec.streamName != "RAGFLOW_TASKS" {
		t.Fatalf("community default stream = %q, want RAGFLOW_TASKS (community queue name)", spec.streamName)
	}
	if len(spec.subjects) != 1 || spec.subjects[0] != "tasks.RAGFLOW" {
		t.Fatalf("community default subjects = %v, want [tasks.RAGFLOW]", spec.subjects)
	}
	if spec.consumerName != "RAGFLOW_CONSUMER" {
		t.Fatalf("community default consumer = %q, want RAGFLOW_CONSUMER", spec.consumerName)
	}
}
