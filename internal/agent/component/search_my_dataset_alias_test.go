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

package component

import (
	"strings"
	"testing"
)

// TestSearchMyDataset_AliasRegistered covers OQ #14: the
// Python-typo name `SearchMyDataset` must be a registered
// Universe A alias of `Retrieval`. Operators who named their
// canvas node with the typo on Python need a matching Go-side
// registration; without it they get "unknown component" and the
// canvas buildNodeBody call errors out.
//
// Both names currently map to the same factory (the real
// delegation wrapper in universe_a_wrappers.go). This test only
// asserts the registration is present, not which factory.
func TestSearchMyDataset_AliasRegistered(t *testing.T) {
	t.Parallel()

	// Lookup by the alias should succeed.
	c, err := New("SearchMyDataset", nil)
	if err != nil {
		t.Fatalf("New(SearchMyDataset) errored; alias not registered: %v", err)
	}
	if c == nil {
		t.Fatalf("New(SearchMyDataset) returned nil component")
	}

	// And it should report the canonical name (the stub's Name()
	// returns the constant it was registered with, not the alias
	// we looked it up by — so we accept either canonical or alias).
	if name := c.Name(); name != "Retrieval" && name != "SearchMyDataset" {
		t.Errorf("alias c.Name() = %q, want %q or %q", name, "Retrieval", "SearchMyDataset")
	}

	// RegisteredNames list should contain both spellings.
	have := map[string]bool{}
	for _, n := range RegisteredNames() {
		have[strings.ToLower(n)] = true
	}
	if !have["retrieval"] {
		t.Errorf("registered names missing %q (alias without canonical?)", "Retrieval")
	}
	if !have["searchmydataset"] {
		t.Errorf("registered names missing %q", "SearchMyDataset")
	}
}
