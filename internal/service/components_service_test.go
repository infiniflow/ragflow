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

// Phase 4 of plan port-rag-flow-pipeline-to-go.md — service-level
// tests for ComponentsService. The handler tests verify the HTTP
// envelope; these verify the projection layer in isolation. They
// share the same blank-import trick to register factories into
// runtime.DefaultRegistry.
package service

import (
	"sort"
	"testing"

	_ "ragflow/internal/agent/component" // registers agent components
	"ragflow/internal/agent/runtime"
	_ "ragflow/internal/ingestion/component"         // registers ingestion main components
	_ "ragflow/internal/ingestion/component/chunker" // registers 4 chunker variants
)

func TestComponentsService_List_NoFilter(t *testing.T) {
	svc := NewComponentsService()
	got, err := svc.List()
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	want := runtime.DefaultRegistry.Names()
	if len(got) != len(want) {
		t.Fatalf("List() returned %d components, want %d (== Names())", len(got), len(want))
	}
	// Sorted ascending by name.
	for i := 1; i < len(got); i++ {
		if got[i-1].Name > got[i].Name {
			t.Errorf("List() output not sorted at index %d: %q > %q", i, got[i-1].Name, got[i].Name)
		}
	}
	// And matches Names() in order.
	for i, d := range got {
		if d.Name != want[i] {
			t.Errorf("List()[%d].Name = %q, want %q", i, d.Name, want[i])
		}
	}
}

func TestComponentsService_List_FilterIngestion(t *testing.T) {
	svc := NewComponentsService()
	got, err := svc.List(runtime.CategoryIngestion)
	if err != nil {
		t.Fatalf("List(Ingestion) returned error: %v", err)
	}
	wantNames := []string{
		"extractor", "file", "grouptitlechunker", "hierarchytitlechunker",
		"parser", "titlechunker", "tokenchunker", "tokenizer",
	}
	assertComponentNameSet(t, "ingestion", namesOf(got), wantNames)
}

func TestComponentsService_List_FilterAgent(t *testing.T) {
	svc := NewComponentsService()
	got, err := svc.List(runtime.CategoryAgent)
	if err != nil {
		t.Fatalf("List(Agent) returned error: %v", err)
	}
	wantNames := runtime.DefaultRegistry.NamesByCategory(runtime.CategoryAgent)
	assertComponentNameSet(t, "agent", namesOf(got), wantNames)
}

func TestComponentsService_List_FilterIngestionAndShared(t *testing.T) {
	svc := NewComponentsService()
	got, err := svc.List(runtime.CategoryIngestion, runtime.CategoryShared)
	if err != nil {
		t.Fatalf("List(Ingestion,Shared) returned error: %v", err)
	}
	wantNames := []string{
		"extractor", "file", "grouptitlechunker", "hierarchytitlechunker",
		"parser", "titlechunker", "tokenchunker", "tokenizer",
	}
	assertComponentNameSet(t, "ingestion+shared", namesOf(got), wantNames)
}

// TestComponentsService_List_FilterDuplicates verifies that
// repeating a category in the input does not produce duplicate
// output rows. The handler parses the comma-separated ?category=
// query string and forwards each token; without dedupe, a
// ?category=ingestion,ingestion request would return every
// ingestion component twice.
func TestComponentsService_List_FilterDuplicates(t *testing.T) {
	svc := NewComponentsService()
	got, err := svc.List(runtime.CategoryIngestion, runtime.CategoryIngestion, runtime.CategoryIngestion)
	if err != nil {
		t.Fatalf("List(Ingestion x3) returned error: %v", err)
	}
	wantNames := []string{
		"extractor", "file", "grouptitlechunker", "hierarchytitlechunker",
		"parser", "titlechunker", "tokenchunker", "tokenizer",
	}
	if len(got) != len(wantNames) {
		t.Errorf("duplicate category produced %d rows, want %d", len(got), len(wantNames))
	}
	assertComponentNameSet(t, "ingestion deduped", namesOf(got), wantNames)
}

func TestComponentsService_List_DescriptorShape(t *testing.T) {
	svc := NewComponentsService()
	got, err := svc.List()
	if err != nil {
		t.Fatalf("List() returned error: %v", err)
	}
	allowed := map[string]bool{"agent": true, "ingestion": true, "shared": true}
	for _, d := range got {
		if d.Name == "" {
			t.Errorf("descriptor has empty name: %+v", d)
		}
		if !allowed[d.Category] {
			t.Errorf("descriptor %q has unknown category %q", d.Name, d.Category)
		}
		if d.Inputs == nil {
			t.Errorf("descriptor %q has nil inputs", d.Name)
		}
		if d.Outputs == nil {
			t.Errorf("descriptor %q has nil outputs", d.Name)
		}
	}
}

// namesOf extracts the .Name field from a slice of descriptors in
// input order; for sorted output the caller asserts order itself.
func namesOf(ds []ComponentDescriptor) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Name)
	}
	return out
}

// assertComponentNameSet compares two name slices as sets (order-insensitive).
func assertComponentNameSet(t *testing.T, label string, got, want []string) {
	t.Helper()
	gotSorted := append([]string(nil), got...)
	sort.Strings(gotSorted)
	wantSorted := append([]string(nil), want...)
	sort.Strings(wantSorted)
	if len(gotSorted) != len(wantSorted) {
		t.Errorf("%s: got %d (%v), want %d (%v)", label, len(gotSorted), gotSorted, len(wantSorted), wantSorted)
		return
	}
	for i := range wantSorted {
		if gotSorted[i] != wantSorted[i] {
			t.Errorf("%s: name[%d] = %q, want %q (got=%v want=%v)", label, i, gotSorted[i], wantSorted[i], gotSorted, wantSorted)
		}
	}
}
