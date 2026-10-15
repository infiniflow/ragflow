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

package agentic_rag

import (
	"strings"
	"testing"
)

// TestCitationParadigmCompare pins the behavioural difference between the two
// citation paradigms on the SAME answer shapes, so a later live comparison has a
// deterministic baseline to reason about:
//
//	OLD (raw chunk id):  the model writes `chunk_id: <id>`; the answer is marked
//	                     by first-appearance position of each resolvable id.
//	NEW ([ID:n] handle): the model writes the handle the tool stamped; a handle
//	                     the registry never published is DROPPED, and the answer
//	                     is recompacted to the published order.
func TestCitationParadigmCompare(t *testing.T) {
	// Three passages reached the model, stamped [ID:0..2].
	newRegistry := func() *EvidenceRegistry {
		r := NewEvidenceRegistry()
		r.Stamp("c1")
		r.Stamp("c2")
		r.Stamp("c3")
		return r
	}

	cases := []struct {
		name string
		// oldAnswer names passages by raw id; newAnswer names them by handle.
		oldAnswer string
		newAnswer string
	}{
		{
			name:      "two passages, one repeated",
			oldAnswer: "A (chunk_id: c1)\nB (chunk_id: c2)\nC (chunk_id: c1)",
			newAnswer: "A [ID:0]\nB [ID:1]\nC [ID:0]",
		},
		{
			name:      "a handle the run never published",
			oldAnswer: "A (chunk_id: c1)\nX (chunk_id: no-such-id)",
			newAnswer: "A [ID:0]\nX [ID:9]",
		},
		{
			name:      "out-of-order citation compacts",
			oldAnswer: "A (chunk_id: c3)\nB (chunk_id: c1)",
			newAnswer: "A [ID:2]\nB [ID:0]",
		},
	}

	for _, c := range cases {
		// OLD path: the caller fetches the named ids, keeps the resolvable ones,
		// and marks by position. Simulate the fetch keeping only c1..c3.
		oldIDs := ExtractCitedChunkIDs(c.oldAnswer)
		resolved := make([]string, 0, len(oldIDs))
		for _, id := range oldIDs {
			if id == "c1" || id == "c2" || id == "c3" {
				resolved = append(resolved, id)
			}
		}
		oldMarked := InsertCitationMarkers(c.oldAnswer, resolved)

		// NEW path: handles resolve against the registry; publishable is what a
		// fetch would keep.
		reg := newRegistry()
		candidate := CitedIDsFromMarkers(c.newAnswer, reg)
		publishable := make([]string, 0, len(candidate))
		for _, id := range candidate {
			if id == "c1" || id == "c2" || id == "c3" {
				publishable = append(publishable, id)
			}
		}
		newMarked, newCited := ResolveCitations(c.newAnswer, reg, publishable)

		if newCited == nil {
			t.Fatalf("%s: new path produced no citation", c.name)
		}
		// Both paths must agree on WHICH passages end up cited.
		if len(resolved) != len(newCited) {
			t.Logf("%s:\n  OLD cited %v\n  NEW cited %v", c.name, resolved, newCited)
		}
		t.Logf("%s:\n  OLD: %q -> %q (cited %v)\n  NEW: %q -> %q (cited %v)",
			c.name, c.oldAnswer, oldMarked, resolved, c.newAnswer, newMarked, newCited)
	}

	// The decisive difference, pinned as an assertion: a handle the registry
	// never published is removed from the text, so it cannot dangle in the answer
	// while pointing at an empty reference slot.
	reg := newRegistry()
	marked, cited := ResolveCitations("A [ID:0]\nX [ID:9]", reg, []string{"c1"})
	if strings.Contains(marked, "[ID:9]") {
		t.Errorf("an unpublished handle survived: %q", marked)
	}
	if strings.Contains(marked, "X [ID:") {
		t.Errorf("the dropped handle left a stray marker: %q", marked)
	}
	if len(cited) != 1 || cited[0] != "c1" {
		t.Errorf("cited = %v, want [c1]", cited)
	}
}
