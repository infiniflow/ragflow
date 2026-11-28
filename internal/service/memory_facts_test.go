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

// memory_facts_test.go — fact-level behaviour of the memory feature: the type
// filter that separates extracted facts from raw conversations, the temporal
// validity predicate over valid_at/invalid_at, and the ingest-time fold that
// keeps one statement from being stored twice.

package service

import (
	"strings"
	"testing"
	"time"
)

func TestMemoryMessageTypesValidatesNames(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		want    string
		wantErr bool
	}{
		{name: "absent selects nothing", value: nil, want: ""},
		{name: "single type", value: []string{"semantic"}, want: "semantic"},
		{name: "list keeps canonical order", value: []string{"procedural", "semantic"}, want: "procedural,semantic"},
		{name: "raw allowed", value: []string{"raw", "episodic"}, want: "raw,episodic"},
		{name: "case and spaces normalised", value: []string{" Semantic "}, want: "semantic"},
		{name: "duplicates collapse", value: []string{"semantic", "semantic"}, want: "semantic"},
		{name: "unknown rejected", value: []string{"semantic", "working"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := memoryMessageTypes(test.value)
			if test.wantErr {
				if err == nil {
					t.Fatalf("memoryMessageTypes(%v) error = nil, want error", test.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("memoryMessageTypes(%v) error = %v", test.value, err)
			}
			if strings.Join(got, ",") != test.want {
				t.Fatalf("memoryMessageTypes(%v) = %v, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestFactOnlyMessageTypesExcludesRaw(t *testing.T) {
	got := strings.Join(FactOnlyMessageTypes(), ",")
	if strings.Contains(got, "raw") {
		t.Fatalf("FactOnlyMessageTypes() = %q, must not include raw", got)
	}
	for _, want := range []string{"memory_notes", "memory_facts"} {
		if !strings.Contains(got, want) {
			t.Fatalf("FactOnlyMessageTypes() = %q, missing %q", got, want)
		}
	}
}

func TestDefaultMemoryTypesRunsEveryBuiltinPipeline(t *testing.T) {
	got := strings.Join(DefaultMemoryTypes(), ",")
	if want := "memory_notes,memory_facts"; got != want {
		t.Fatalf("DefaultMemoryTypes() = %q, want %q", got, want)
	}
}

// TestMemoryValidityFilterAsOf pins the "true at this instant" reading: a fact
// with no invalid_at stays true, an ended fact stops being true at its
// invalid_at, and a fact that has not become true yet is not visible.
func TestMemoryValidityFilterAsOf(t *testing.T) {
	asOf := time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)
	filter, err := parseMemoryValidityFilter(map[string]interface{}{"as_of": "2026-03-01 00:00:00"})
	if err != nil {
		t.Fatalf("parseMemoryValidityFilter() error = %v", err)
	}
	if !filter.active() {
		t.Fatal("as_of must activate the predicate")
	}
	tests := []struct {
		name string
		row  map[string]interface{}
		want bool
	}{
		{name: "open ended fact is visible", row: map[string]interface{}{"valid_at": "2026-01-01 00:00:00"}, want: true},
		{name: "fact ending after as_of is visible", row: map[string]interface{}{"valid_at": "2026-01-01 00:00:00", "invalid_at": "2026-06-01 00:00:00"}, want: true},
		{name: "fact ended before as_of is hidden", row: map[string]interface{}{"valid_at": "2026-01-01 00:00:00", "invalid_at": "2026-02-01 00:00:00"}, want: false},
		{name: "future fact is hidden", row: map[string]interface{}{"valid_at": "2026-09-01 00:00:00"}, want: false},
		{name: "timeless fact stays visible", row: map[string]interface{}{}, want: true},
		{name: "unparseable bound does not hide the row", row: map[string]interface{}{"valid_at": "not a date"}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := filter.matches(test.row); got != test.want {
				t.Fatalf("matches(%v) = %v, want %v", test.row, got, test.want)
			}
		})
	}
	if !filter.hasAsOf || !filter.asOf.Equal(asOf) {
		t.Fatalf("as_of parsed to %v, want %v", filter.asOf, asOf)
	}
}

// TestMemoryValidityFilterWindow pins valid_at_from/to as a window over when a
// fact became true, with an upper bound excluding timeless facts.
func TestMemoryValidityFilterWindow(t *testing.T) {
	filter, err := parseMemoryValidityFilter(map[string]interface{}{
		"valid_at_from": "2026-01-01 00:00:00",
		"valid_at_to":   "2026-12-31 23:59:59",
	})
	if err != nil {
		t.Fatalf("parseMemoryValidityFilter() error = %v", err)
	}
	tests := []struct {
		name string
		row  map[string]interface{}
		want bool
	}{
		{name: "inside the window", row: map[string]interface{}{"valid_at": "2026-05-05 00:00:00"}, want: true},
		{name: "before the window", row: map[string]interface{}{"valid_at": "2025-12-31 00:00:00"}, want: false},
		{name: "after the window", row: map[string]interface{}{"valid_at": "2027-01-01 00:00:00"}, want: false},
		{name: "timeless fact excluded by the upper bound", row: map[string]interface{}{}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := filter.matches(test.row); got != test.want {
				t.Fatalf("matches(%v) = %v, want %v", test.row, got, test.want)
			}
		})
	}
}

func TestMemoryValidityFilterRejectsBadTimestamp(t *testing.T) {
	if _, err := parseMemoryValidityFilter(map[string]interface{}{"as_of": "yesterday"}); err == nil {
		t.Fatal("parseMemoryValidityFilter() error = nil, want error")
	}
	// RFC3339 is accepted alongside the stored layout.
	if _, err := parseMemoryValidityFilter(map[string]interface{}{"as_of": "2026-03-01T10:00:00Z"}); err != nil {
		t.Fatalf("RFC3339 as_of rejected: %v", err)
	}
}

func TestDedupeExtractedFacts(t *testing.T) {
	items := []extractedMemory{
		{MessageType: "semantic", Content: "The capital of France is Paris."},
		{MessageType: "semantic", Content: "  the capital of france is paris.  "},
		{MessageType: "episodic", Content: "The capital of France is Paris."},
		{MessageType: "semantic", Content: "   "},
		{MessageType: "semantic", Content: "x"},
	}
	got := dedupeExtractedFacts(items)
	if len(got) != 2 {
		t.Fatalf("dedupeExtractedFacts() kept %d items, want 2: %+v", len(got), got)
	}
	if got[0].Content != "The capital of France is Paris." {
		t.Errorf("content not trimmed/preserved: %q", got[0].Content)
	}
	if got[1].MessageType != "episodic" {
		t.Errorf("type-specific repeat must survive: %+v", got[1])
	}
}

func TestDropKnownFacts(t *testing.T) {
	known := map[string]struct{}{
		factKey("semantic", "Water boils at 100C."): {},
	}
	items := []extractedMemory{
		{MessageType: "semantic", Content: " water   boils at 100c. "},
		{MessageType: "semantic", Content: "Water freezes at 0C."},
	}
	got := dropKnownFacts(items, known)
	if len(got) != 1 || got[0].Content != "Water freezes at 0C." {
		t.Fatalf("dropKnownFacts() = %+v, want only the new fact", got)
	}
	if out := dropKnownFacts(items, nil); len(out) != len(items) {
		t.Fatal("an empty known set must not drop anything")
	}
}

func TestFactTypesOf(t *testing.T) {
	got := factTypesOf([]extractedMemory{
		{MessageType: "semantic"},
		{MessageType: "SEMANTIC"},
		{MessageType: "episodic"},
		{MessageType: ""},
	})
	if strings.Join(got, ",") != "semantic,episodic" {
		t.Fatalf("factTypesOf() = %v, want [semantic episodic]", got)
	}
}
