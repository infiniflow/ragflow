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

import "testing"

func TestPtrString_Nil(t *testing.T) {
	if got := PtrString[int](nil); got != "<nil>" {
		t.Errorf("PtrString(nil) = %q, want <nil>", got)
	}
}

func TestPtrString_Value(t *testing.T) {
	val := 42
	if got := PtrString(&val); got != "42" {
		t.Errorf("PtrString(&42) = %q, want 42", got)
	}
}

func TestPtrString_Bool(t *testing.T) {
	val := true
	if got := PtrString(&val); got != "true" {
		t.Errorf("PtrString(&true) = %q, want true", got)
	}
}

func TestChunkID_NotPanic(t *testing.T) {
	_ = ChunkID("", "")
	_ = ChunkID("doc-1", "content")
}

func TestChunkID_PreservesLeadingZero(t *testing.T) {
	got := ChunkID("doc-1", "")
	want := "037fe13bd80c56aa"
	if got != want {
		t.Fatalf("ChunkID(%q, %q) = %q, want %q", "doc-1", "", got, want)
	}
	if len(got) != 16 {
		t.Fatalf("ChunkID length = %d, want 16", len(got))
	}
}

func TestFormatMinimumShouldMatchPercent(t *testing.T) {
	tests := []struct {
		name     string
		fraction float64
		want     string
	}{
		{"exact percent", 0.29, "29%"},
		{"half rounds up", 0.125, "13%"},
		{"just over half rounds up", 0.135, "14%"},
		{"floating-point .5 rounds up", 0.285, "29%"},
		{"zero", 0.0, "0%"},
		{"one hundred", 1.0, "100%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatMinimumShouldMatchPercent(tt.fraction); got != tt.want {
				t.Fatalf("FormatMinimumShouldMatchPercent(%g) = %q, want %q", tt.fraction, got, tt.want)
			}
		})
	}
}

func TestBaseModelName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"bare model name stays", "BAAI/bge-m3", "BAAI/bge-m3"},
		{"2-part composite strips provider", "BAAI/bge-m3@SILICONFLOW", "BAAI/bge-m3"},
		{"3-part composite strips instance and provider", "BAAI/bge-m3@renew@SILICONFLOW", "BAAI/bge-m3"},
		{"embedded @ in model name is preserved", "text-embedding-nomic-embed-text-v1.5@q8_0@lmstudio@LM-Studio", "text-embedding-nomic-embed-text-v1.5@q8_0"},
		{"opaque id without @ stays intact", "2d8ff0a97d75431c8c91526549939328", "2d8ff0a97d75431c8c91526549939328"},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BaseModelName(tt.input); got != tt.want {
				t.Fatalf("BaseModelName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"plain value unchanged", []string{"a", "b"}, "a"},
		{"returns first when later match", []string{"x", "y", "z"}, "x"},
		{"trims chosen value", []string{"  picked  ", "b"}, "picked"},
		{"chosen whitespace later clean", []string{"  picked  ", "clean"}, "picked"},
		{"skips whitespace-only candidates", []string{"   ", "  ", "real"}, "real"},
		{"all empty returns empty", []string{"", " ", "  "}, ""},
		{"nil returns empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FirstNonEmpty(tt.in...); got != tt.want {
				t.Fatalf("FirstNonEmpty(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestIsUUID_Characterization pins the current 32-char lowercase alnum gate.
func TestIsUUID_Characterization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"32 lowercase alnum", "0123456789abcdef0123456789abcdef", true},
		{"32 digits", "00000000000000000000000000000000", true},
		{"31 chars rejected", "0123456789abcdef0123456789abcde", false},
		{"33 chars rejected", "0123456789abcdef0123456789abcdef0", false},
		{"empty rejected", "", false},
		{"uppercase rejected", "0123456789ABCDEF0123456789ABCDEF", false},
		{"hyphen rejected", "01234567-89ab-cdef-0123-456789abcdef", false},
		{"space rejected", "0123456789abcdef0123456789abcde ", false},
		{"unicode rejected", "0123456789abcdef0123456789abcde汉", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUUID(tt.in); got != tt.want {
				t.Fatalf("IsUUID(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestIsValidEmail_Characterization pins the shared email shape gate after
// consolidating the duplicated regex into a single helper.
func TestIsValidEmail_Characterization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"plain", "alice@example.com", true},
		{"subdomain and hyphen", "a.b_c-d@example-domain.com", true},
		{"multi-level domain", "user@sub.example.co.uk", true},
		{"short tld", "a@b.co", true},
		{"single char tld rejected", "a@b.c", false},
		{"no at rejected", "plainaddress", false},
		{"double at rejected", "a@@b.com", false},
		{"empty local rejected", "@example.com", false},
		{"no dot in domain rejected", "alice@example", false},
		{"empty tld rejected", "alice@example.", false},
		{"uppercase allowed", "Alice@Example.COM", true},
		{"plus tag rejected", "a+b@example.com", false},
		{"empty rejected", "", false},
		{"space rejected", "a b@example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidEmail(tt.in); got != tt.want {
				t.Fatalf("IsValidEmail(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
