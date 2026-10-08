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

package common

import (
	"errors"
	"strings"
	"testing"
)

func TestSplitNameCounter(t *testing.T) {
	cases := []struct {
		stem      string
		wantBase  string
		wantCount int
		wantOK    bool
	}{
		{"test(5)", "test", 5, true},
		{"test", "test", 0, false},
		{"test (5)", "test", 5, true},
		{"a(1)(2)", "a(1)", 2, true},
		{"报告(12)", "报告", 12, true},
		{"(3)", "", 3, true},
		{"test()", "test()", 0, false},
	}
	for _, tc := range cases {
		base, count, ok := splitNameCounter(tc.stem)
		if base != tc.wantBase || count != tc.wantCount || ok != tc.wantOK {
			t.Errorf("splitNameCounter(%q) = (%q, %d, %v), want (%q, %d, %v)",
				tc.stem, base, count, ok, tc.wantBase, tc.wantCount, tc.wantOK)
		}
	}
}

func TestUniqueName_NoCollision(t *testing.T) {
	got, err := UniqueName("topic", func(string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatalf("UniqueName returned error: %v", err)
	}
	if got != "topic" {
		t.Fatalf("UniqueName = %q, want %q", got, "topic")
	}
}

func TestUniqueName_CounterContinuation(t *testing.T) {
	cases := []struct {
		name  string
		taken map[string]bool
		want  string
	}{
		{"topic", map[string]bool{"topic": true}, "topic(1)"},
		{"topic", map[string]bool{"topic": true, "topic(1)": true}, "topic(2)"},
		{"topic(1)", map[string]bool{"topic(1)": true}, "topic(2)"},
		{"topic(2)", map[string]bool{"topic(2)": true, "topic(3)": true}, "topic(4)"},
	}
	for _, tc := range cases {
		taken := tc.taken
		got, err := UniqueName(tc.name, func(c string) (bool, error) { return taken[c], nil })
		if err != nil {
			t.Fatalf("UniqueName(%q) error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("UniqueName(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestUniqueName_PreservesExtension(t *testing.T) {
	cases := []struct {
		name  string
		taken map[string]bool
		want  string
	}{
		{"report.pdf", map[string]bool{"report.pdf": true}, "report(1).pdf"},
		{"report.pdf", map[string]bool{"report.pdf": true, "report(1).pdf": true}, "report(2).pdf"},
		{"report(1).pdf", map[string]bool{"report(1).pdf": true}, "report(2).pdf"},
		{"archive.tar.gz", map[string]bool{"archive.tar.gz": true}, "archive.tar(1).gz"},
	}
	for _, tc := range cases {
		taken := tc.taken
		got, err := UniqueName(tc.name, func(c string) (bool, error) { return taken[c], nil })
		if err != nil {
			t.Fatalf("UniqueName(%q) error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("UniqueName(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestUniqueName_PropagatesLookupError(t *testing.T) {
	wantErr := errors.New("lookup failed")
	_, err := UniqueName("topic", func(string) (bool, error) { return false, wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("UniqueName error = %v, want %v", err, wantErr)
	}
}

func TestUniqueName_RejectsNameOverLimit(t *testing.T) {
	long := strings.Repeat("a", AppNameLimit)
	_, err := UniqueName(long, func(c string) (bool, error) { return c == long, nil })
	if err == nil {
		t.Fatal("expected length validation error for generated name")
	}
	if !strings.Contains(err.Error(), "large than") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNameAvailable_CaseOnlyChangeSkipsLookup(t *testing.T) {
	called := false
	ok, err := NameAvailable("Foo", "foo", func(string) (bool, error) {
		called = true
		return true, nil
	})
	if err != nil {
		t.Fatalf("NameAvailable error: %v", err)
	}
	if !ok {
		t.Fatal("expected case-only change to be available")
	}
	if called {
		t.Fatal("expected case-only change to skip the existence lookup")
	}
}

func TestNameAvailable_Taken(t *testing.T) {
	ok, err := NameAvailable("Original", "Taken", func(string) (bool, error) { return true, nil })
	if err != nil {
		t.Fatalf("NameAvailable error: %v", err)
	}
	if ok {
		t.Fatal("expected taken name to be unavailable")
	}
}

func TestNameAvailable_Free(t *testing.T) {
	ok, err := NameAvailable("Original", "Free", func(string) (bool, error) { return false, nil })
	if err != nil {
		t.Fatalf("NameAvailable error: %v", err)
	}
	if !ok {
		t.Fatal("expected free name to be available")
	}
}

func TestNameAvailable_PropagatesLookupError(t *testing.T) {
	wantErr := errors.New("lookup failed")
	ok, err := NameAvailable("Original", "New", func(string) (bool, error) { return false, wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("NameAvailable error = %v, want %v", err, wantErr)
	}
	if ok {
		t.Fatal("expected availability to be false on lookup error")
	}
}

func TestValidateName(t *testing.T) {
	if err := ValidateName("  "); err == nil {
		t.Fatal("expected empty name error")
	}
	if err := ValidateName(strings.Repeat("a", AppNameLimit+1)); err == nil {
		t.Fatal("expected length error")
	}
	if err := ValidateName("ok"); err != nil {
		t.Fatalf("unexpected error for valid name: %v", err)
	}
}
