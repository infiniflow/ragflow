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
	got, err := UniqueName("topic", 0, func(string) (bool, error) { return false, nil })
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
		got, err := UniqueName(tc.name, 0, func(c string) (bool, error) { return taken[c], nil })
		if err != nil {
			t.Fatalf("UniqueName(%q) error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("UniqueName(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Non-file resource names keep their dots: the counter is appended to the end
// of the whole name instead of being inserted before the last dot.
func TestUniqueName_AppendsSuffixToDottedResourceName(t *testing.T) {
	taken := map[string]bool{"release v1.2": true}
	got, err := UniqueName("release v1.2", 0, func(c string) (bool, error) { return taken[c], nil })
	if err != nil {
		t.Fatalf("UniqueName error: %v", err)
	}
	if got != "release v1.2(1)" {
		t.Fatalf("UniqueName = %q, want %q", got, "release v1.2(1)")
	}
}

func TestUniqueFileName_PreservesExtension(t *testing.T) {
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
		got, err := UniqueFileName(tc.name, 0, func(c string) (bool, error) { return taken[c], nil })
		if err != nil {
			t.Fatalf("UniqueFileName(%q) error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("UniqueFileName(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestUniqueName_PropagatesLookupError(t *testing.T) {
	wantErr := errors.New("lookup failed")
	_, err := UniqueName("topic", 0, func(string) (bool, error) { return false, wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("UniqueName error = %v, want %v", err, wantErr)
	}
}

// The generated candidate must respect the caller's resource-specific limit,
// even when it is below the generic AppNameLimit.
func TestUniqueName_EnforcesCallerLimit(t *testing.T) {
	base := strings.Repeat("a", 128)
	onlyBaseTaken := func(c string) (bool, error) { return c == base, nil }

	if _, err := UniqueName(base, 128, onlyBaseTaken); err == nil {
		t.Fatal("expected length validation error for generated name at caller limit")
	} else if !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := UniqueName(base, 0, onlyBaseTaken)
	if err != nil {
		t.Fatalf("UniqueName with default limit error: %v", err)
	}
	if want := base + "(1)"; got != want {
		t.Fatalf("UniqueName = %q, want %q", got, want)
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

func TestUniqueName_CountsRunesNotBytes(t *testing.T) {
	// 128 CJK characters are 384 bytes but still fit a VARCHAR(128) column.
	base := strings.Repeat("报", 128)
	if _, err := UniqueName(base, 128, func(string) (bool, error) { return false, nil }); err != nil {
		t.Fatalf("UniqueName rejected 128 CJK characters: %v", err)
	}
	if _, err := UniqueName(base+"告", 128, func(string) (bool, error) { return false, nil }); err == nil {
		t.Fatal("expected 129 CJK characters to be rejected")
	}
}

func TestUniqueFileName_CountsRunesNotBytes(t *testing.T) {
	base := strings.Repeat("报", 251) + ".pdf" // 255 characters
	if _, err := UniqueFileName(base, 255, func(string) (bool, error) { return false, nil }); err != nil {
		t.Fatalf("UniqueFileName rejected 255 CJK characters: %v", err)
	}
	if _, err := UniqueFileName(strings.Repeat("报", 252)+".pdf", 255, func(string) (bool, error) { return false, nil }); err == nil {
		t.Fatal("expected 256 characters to be rejected")
	}
}

func TestValidateNameRunes(t *testing.T) {
	if err := ValidateNameRunes("  ", 128); err == nil {
		t.Fatal("expected empty name error")
	}
	if err := ValidateNameRunes(strings.Repeat("a", 128), 128); err != nil {
		t.Fatalf("unexpected error at limit: %v", err)
	}
	if err := ValidateNameRunes(strings.Repeat("a", 129), 128); err == nil {
		t.Fatal("expected length error")
	}
	if err := ValidateNameRunes(" "+strings.Repeat("a", 128), 128); err == nil {
		t.Fatal("expected whitespace to count toward the stored length")
	}
	if err := ValidateNameRunes(strings.Repeat("报", 128), 128); err != nil {
		t.Fatalf("unexpected error for 128 CJK characters: %v", err)
	}
	if err := ValidateNameRunes("ok", 128); err != nil {
		t.Fatalf("unexpected error for valid name: %v", err)
	}
}

func TestValidateAliasRunes(t *testing.T) {
	if err := ValidateAliasRunes("", 255); err != nil {
		t.Fatalf("empty alias should be valid: %v", err)
	}
	if err := ValidateAliasRunes("   ", 255); err != nil {
		t.Fatalf("whitespace alias should be valid: %v", err)
	}
	if err := ValidateAliasRunes(strings.Repeat("a", 255), 255); err != nil {
		t.Fatalf("alias at limit should be valid: %v", err)
	}
	if err := ValidateAliasRunes(strings.Repeat("a", 256), 255); err == nil {
		t.Fatal("expected alias length error")
	}
}
