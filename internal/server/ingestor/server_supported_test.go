// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ingestor

import "testing"

// resetExtraSupportedTypes isolates the registry for a test, preserving any
// edition registrations installed by init() and restoring them afterward.
func resetExtraSupportedTypes() func() {
	saved := extraSupportedTypes
	extraSupportedTypes = map[string]struct{}{}
	return func() { extraSupportedTypes = saved }
}

// EE editions register extra supported types; they must be merged into the
// base list while the community types stay present.
func TestDefaultSupportedTypesIncludesRegistered(t *testing.T) {
	defer resetExtraSupportedTypes()()
	RegisterSupportedType("cad")
	ds := defaultSupportedTypes()
	has := map[string]bool{}
	for _, s := range ds {
		has[s] = true
	}
	for _, want := range []string{"pdf", "docx", "txt", "cad"} {
		if !has[want] {
			t.Fatalf("defaultSupportedTypes() missing %q; got %v", want, ds)
		}
	}
}

// With no registration the list is exactly the community base.
func TestDefaultSupportedTypesBaseOnlyWhenUnregistered(t *testing.T) {
	defer resetExtraSupportedTypes()()
	ds := defaultSupportedTypes()
	if len(ds) != 3 {
		t.Fatalf("defaultSupportedTypes() len = %d, want 3 (base only)", len(ds))
	}
}

// Duplicate registrations collapse to a single entry.
func TestDefaultSupportedTypesDedup(t *testing.T) {
	defer resetExtraSupportedTypes()()
	RegisterSupportedType("cad")
	RegisterSupportedType("cad")
	ds := defaultSupportedTypes()
	count := 0
	for _, s := range ds {
		if s == "cad" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("cad appears %d times, want 1", count)
	}
}

// Registration is case-insensitive and dedupes across cases.
func TestDefaultSupportedTypesCaseInsensitiveDedup(t *testing.T) {
	defer resetExtraSupportedTypes()()
	RegisterSupportedType("CAD")
	RegisterSupportedType("cad")
	ds := defaultSupportedTypes()
	count := 0
	for _, s := range ds {
		if s == "cad" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("cad/CAD should dedupe to 1, got %d", count)
	}
}

// An empty registration is a no-op.
func TestDefaultSupportedTypesEmptyIgnored(t *testing.T) {
	defer resetExtraSupportedTypes()()
	RegisterSupportedType("")
	ds := defaultSupportedTypes()
	if len(ds) != 3 {
		t.Fatalf("empty registration should be ignored; len = %d, want 3", len(ds))
	}
}
