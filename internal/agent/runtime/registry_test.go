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

package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
)

// stubComponent is a minimal Component impl used as the factory's
// return value in tests. It echoes the params back into the output map
// so a test can assert what the factory actually received.
type stubComponent struct {
	name   string
	params map[string]any
}

func (s *stubComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	return map[string]any{"name": s.name, "params": s.params}, nil
}

func stubFactory(name string, params map[string]any) (Component, error) {
	return &stubComponent{name: name, params: params}, nil
}

// errFactory returns a fixed error so a test can assert that factory
// errors propagate via Lookup.
func errFactory(err error) ComponentFactory {
	return func(name string, params map[string]any) (Component, error) {
		return nil, err
	}
}

func TestRegistry_RegisterAndLookup_HappyPath(t *testing.T) {
	r := NewMemoryRegistry()
	meta := Metadata{
		Inputs:  map[string]string{"x": "input x"},
		Outputs: map[string]string{"y": "output y"},
	}
	if err := r.Register("Foo", CategoryAgent, stubFactory, meta); err != nil {
		t.Fatalf("Register(Foo) returned error: %v", err)
	}

	f, cat, gotMeta, ok := r.Lookup("Foo")
	if !ok {
		t.Fatalf("Lookup(Foo) missed")
	}
	if cat != CategoryAgent {
		t.Errorf("Lookup category = %q, want %q", cat, CategoryAgent)
	}
	if gotMeta.Inputs["x"] != "input x" || gotMeta.Outputs["y"] != "output y" {
		t.Errorf("Lookup metadata lost: got %+v", gotMeta)
	}

	c, err := f("Foo", map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("factory returned error: %v", err)
	}
	if _, ok := c.(*stubComponent); !ok {
		t.Errorf("factory returned wrong type %T", c)
	}
}

func TestRegistry_Lookup_CaseInsensitive(t *testing.T) {
	r := NewMemoryRegistry()
	if err := r.Register("ExampleComponent", CategoryShared, stubFactory, Metadata{Version: "legacy"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for _, variant := range []string{"ExampleComponent", "examplecomponent", "EXAMPLECOMPONENT", "  examplecomponent  "} {
		if _, _, _, ok := r.Lookup(variant); !ok {
			t.Errorf("Lookup(%q) missed; case-insensitive lookup must succeed for all variants", variant)
		}
	}
}

func TestRegistry_Register_DuplicateReturnsError(t *testing.T) {
	r := NewMemoryRegistry()
	if err := r.Register("Dup", CategoryAgent, stubFactory, Metadata{Version: "legacy"}); err != nil {
		t.Fatalf("first Register(Dup) returned error: %v", err)
	}
	err := r.Register("Dup", CategoryIngestion, stubFactory, Metadata{Version: "legacy"})
	if err == nil {
		t.Fatalf("second Register(Dup) succeeded; expected duplicate-key error")
	}
	// Duplicate detection is also case-insensitive.
	err2 := r.Register("DUP", CategoryIngestion, stubFactory, Metadata{Version: "legacy"})
	if err2 == nil {
		t.Fatalf("Register(DUP) succeeded; duplicate detection must be case-insensitive")
	}
}

func TestRegistry_Register_EmptyNameReturnsError(t *testing.T) {
	r := NewMemoryRegistry()
	if err := r.Register("", CategoryAgent, stubFactory, Metadata{Version: "legacy"}); err == nil {
		t.Fatalf("Register(\"\") succeeded; expected empty-name error")
	}
	if err := r.Register("   ", CategoryAgent, stubFactory, Metadata{Version: "legacy"}); err == nil {
		t.Fatalf("Register(\"   \") succeeded; expected empty-name error after trim")
	}
}

// TestRegistry_Register_EmptyMetadataReturnsError verifies the plan
// §4 Phase 0 task 1 contract: Register rejects empty metadata
// (Version, Inputs, Outputs all unset). Ingestion components MUST
// supply a Version string; the legacy adapter shim stamps
// {Version: "legacy"}; a single-field fill is also allowed (e.g.,
// only Inputs, or only Version).
func TestRegistry_Register_EmptyMetadataReturnsError(t *testing.T) {
	r := NewMemoryRegistry()
	// All three fields unset → empty-metadata error.
	err := r.Register("EmptyMeta", CategoryIngestion, stubFactory, Metadata{})
	if err == nil {
		t.Fatalf("Register with empty metadata succeeded; expected empty-metadata error")
	}
	// Verify it was not actually registered (re-register with valid
	// metadata should succeed).
	if err := r.Register("EmptyMeta", CategoryIngestion, stubFactory, Metadata{Version: "1.0.0"}); err != nil {
		t.Fatalf("Register with valid metadata after empty-metadata rejection failed: %v", err)
	}
}

// TestRegistry_Register_AcceptsPartialMetadata verifies the
// three-way empty check: filling ANY one of Version / Inputs /
// Outputs is enough to register successfully. This accommodates
// the migration path where a component has only inputs but no
// outputs yet, or where a shim stamps {Version: "legacy"}.
func TestRegistry_Register_AcceptsPartialMetadata(t *testing.T) {
	cases := []struct {
		name string
		meta Metadata
	}{
		{"OnlyVersion", Metadata{Version: "1.0.0"}},
		{"OnlyInputs", Metadata{Inputs: map[string]string{"x": "x"}}},
		{"OnlyOutputs", Metadata{Outputs: map[string]string{"y": "y"}}},
		{"LegacyAdapter", Metadata{Version: "legacy"}},
		{"Full", Metadata{Version: "1.0.0", Inputs: map[string]string{"x": "x"}, Outputs: map[string]string{"y": "y"}}},
	}
	for i, c := range cases {
		r := NewMemoryRegistry()
		name := c.name
		if err := r.Register(name, CategoryIngestion, stubFactory, c.meta); err != nil {
			t.Errorf("case %d (%s): Register returned error: %v", i, c.name, err)
		}
	}
}

func TestRegistry_MustRegister_PanicsOnDuplicate(t *testing.T) {
	// MustRegister operates on DefaultRegistry. Save and restore so this
	// test does not pollute the global.
	saved := DefaultRegistry
	defer func() { DefaultRegistry = saved }()
	DefaultRegistry = NewMemoryRegistry()

	MustRegister("Panic", CategoryAgent, stubFactory, Metadata{Version: "legacy"})

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("MustRegister on duplicate did not panic")
		}
	}()
	MustRegister("Panic", CategoryAgent, stubFactory, Metadata{Version: "legacy"})
}

func TestRegistry_Lookup_MissReturnsFalse(t *testing.T) {
	r := NewMemoryRegistry()
	f, cat, meta, ok := r.Lookup("NotThere")
	if ok {
		t.Errorf("Lookup on empty registry returned ok=true (f=%v cat=%q meta=%+v)", f, cat, meta)
	}
	if f != nil {
		t.Errorf("Lookup miss: factory should be nil, got %v", f)
	}
	if cat != "" {
		t.Errorf("Lookup miss: category should be empty, got %q", cat)
	}
	if meta.Inputs != nil || meta.Outputs != nil {
		t.Errorf("Lookup miss: metadata should be zero-value, got %+v", meta)
	}
}

func TestRegistry_NamesByCategory_FiltersCorrectly(t *testing.T) {
	r := NewMemoryRegistry()
	if err := r.Register("AgentComp", CategoryAgent, stubFactory, Metadata{Version: "legacy"}); err != nil {
		t.Fatalf("Register AgentComp: %v", err)
	}
	if err := r.Register("IngestComp", CategoryIngestion, stubFactory, Metadata{Version: "legacy"}); err != nil {
		t.Fatalf("Register IngestComp: %v", err)
	}
	if err := r.Register("SharedComp", CategoryShared, stubFactory, Metadata{Version: "legacy"}); err != nil {
		t.Fatalf("Register SharedComp: %v", err)
	}

	gotAgent := r.NamesByCategory(CategoryAgent)
	wantAgent := []string{"agentcomp"}
	if !equalSlices(gotAgent, wantAgent) {
		t.Errorf("NamesByCategory(CategoryAgent) = %v, want %v", gotAgent, wantAgent)
	}

	gotIngest := r.NamesByCategory(CategoryIngestion)
	wantIngest := []string{"ingestcomp"}
	if !equalSlices(gotIngest, wantIngest) {
		t.Errorf("NamesByCategory(CategoryIngestion) = %v, want %v", gotIngest, wantIngest)
	}

	gotShared := r.NamesByCategory(CategoryShared)
	wantShared := []string{"sharedcomp"}
	if !equalSlices(gotShared, wantShared) {
		t.Errorf("NamesByCategory(CategoryShared) = %v, want %v", gotShared, wantShared)
	}

	gotUnknown := r.NamesByCategory(Category("nonexistent"))
	if len(gotUnknown) != 0 {
		t.Errorf("NamesByCategory(unknown) = %v, want empty", gotUnknown)
	}
}

func TestRegistry_Names_ReturnsAllSorted(t *testing.T) {
	r := NewMemoryRegistry()
	for _, n := range []string{"Charlie", "alpha", "BRAVO"} {
		if err := r.Register(n, CategoryAgent, stubFactory, Metadata{Version: "legacy"}); err != nil {
			t.Fatalf("Register %s: %v", n, err)
		}
	}
	got := r.Names()
	// Keys are normalized to lowercase at registration time, so the
	// returned list is ["alpha", "bravo", "charlie"] (sorted).
	want := []string{"alpha", "bravo", "charlie"}
	if !equalSlices(got, want) {
		t.Errorf("Names() = %v, want %v", got, want)
	}
}

func TestRegistry_NamesByCategory_ReturnsSorted(t *testing.T) {
	r := NewMemoryRegistry()
	for _, n := range []string{"Zulu", "alpha", "mike", "BRAVO"} {
		if err := r.Register(n, CategoryIngestion, stubFactory, Metadata{Version: "legacy"}); err != nil {
			t.Fatalf("Register %s: %v", n, err)
		}
	}
	got := r.NamesByCategory(CategoryIngestion)
	want := []string{"alpha", "bravo", "mike", "zulu"}
	if !equalSlices(got, want) {
		t.Errorf("NamesByCategory(Ingestion) = %v, want %v", got, want)
	}
}

func TestRegistry_ThreadSafe(t *testing.T) {
	// Concurrency smoke test: N goroutines each register a distinct
	// name; after all join, Names() must contain every one. A
	// non-thread-safe map would lose entries or panic under -race.
	saved := DefaultRegistry
	defer func() { DefaultRegistry = saved }()
	r := NewMemoryRegistry()
	DefaultRegistry = r

	const N = 64
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("comp-%03d", i)
			if err := r.Register(name, CategoryIngestion, stubFactory, Metadata{Version: "legacy"}); err != nil {
				t.Errorf("Register(%s) returned error: %v", name, err)
			}
		}()
	}
	wg.Wait()

	got := r.NamesByCategory(CategoryIngestion)
	if len(got) != N {
		t.Errorf("NamesByCategory(Ingestion) returned %d names; expected %d", len(got), N)
	}
	// And a parallel Lookup burst against the same registry must
	// observe all N entries.
	var lookupWg sync.WaitGroup
	for i := 0; i < N; i++ {
		i := i
		lookupWg.Add(1)
		go func() {
			defer lookupWg.Done()
			name := fmt.Sprintf("comp-%03d", i)
			if _, _, _, ok := r.Lookup(name); !ok {
				t.Errorf("Lookup(%s) missed after concurrent registration", name)
			}
		}()
	}
	lookupWg.Wait()
}

func TestRegistry_FactoryErrorPropagates(t *testing.T) {
	// A factory that returns an error must propagate that error through
	// the Lookup → invoke path. This is not directly tested by the
	// plan checklist but it confirms the factory closure contract.
	r := NewMemoryRegistry()
	wantErr := errors.New("boom")
	if err := r.Register("Bad", CategoryAgent, errFactory(wantErr), Metadata{Version: "legacy"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	f, _, _, ok := r.Lookup("Bad")
	if !ok {
		t.Fatalf("Lookup missed")
	}
	_, err := f("Bad", nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("factory error not propagated: got %v, want %v", err, wantErr)
	}
}

func TestDefaultRegistry_Present(t *testing.T) {
	if DefaultRegistry == nil {
		t.Fatal("DefaultRegistry is nil")
	}
	// Names() must work without panicking even on the empty default.
	if got := DefaultRegistry.Names(); got == nil {
		t.Errorf("Names() returned nil; want non-nil slice")
	}
}

// equalSlices compares two []string for unordered (no — wait, we DO want
// order-sensitive comparison; Names() and NamesByCategory() guarantee
// sorted output).
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// extraSort helper kept here in case a future test wants sorted
// comparison without imposing a specific ordering.
var _ = sort.Strings
