// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package entity

import (
	"strings"
	"testing"
)

func validMediaRef() *MediaRef {
	return &MediaRef{
		Format: "html",
		Kind:   MediaKindDataURI,
		SHA256: strings.Repeat("a", 64),
		MIME:   "image/png",
		Size:   5,
	}
}

func TestMediaRefValid(t *testing.T) {
	valid := validMediaRef()
	for _, tc := range []struct {
		name string
		ref  *MediaRef
		want bool
	}{
		{"valid", valid, true},
		{"nil", nil, false},
		{"missing format", &MediaRef{Kind: MediaKindDataURI, SHA256: valid.SHA256}, false},
		{"missing kind", &MediaRef{Format: "html", SHA256: valid.SHA256}, false},
		{"missing digest", &MediaRef{Format: "html", Kind: MediaKindDataURI}, false},
		{"short digest", &MediaRef{Format: "html", Kind: MediaKindDataURI, SHA256: valid.SHA256[:63]}, false},
		{"long digest", &MediaRef{Format: "html", Kind: MediaKindDataURI, SHA256: valid.SHA256 + "a"}, false},
		{"uppercase digest", &MediaRef{Format: "html", Kind: MediaKindDataURI, SHA256: strings.ToUpper(valid.SHA256)}, false},
		{"non-hex digest", &MediaRef{Format: "html", Kind: MediaKindDataURI, SHA256: strings.Repeat("z", 64)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ref.Valid(); got != tc.want {
				t.Fatalf("Valid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMediaRefIsZero(t *testing.T) {
	valid := validMediaRef()
	for _, tc := range []struct {
		name string
		ref  *MediaRef
		want bool
	}{
		{"nil", nil, true},
		{"empty", &MediaRef{}, true},
		{"format only", &MediaRef{Format: "html"}, false},
		{"size only", &MediaRef{Size: 1}, false},
		{"valid", valid, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ref.IsZero(); got != tc.want {
				t.Fatalf("IsZero() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMediaRefFromAny(t *testing.T) {
	valid := validMediaRef()
	wire := map[string]any{
		"format": "html",
		"kind":   string(MediaKindDataURI),
		"sha256": valid.SHA256,
		"mime":   "image/png",
		"size":   float64(5),
	}
	for _, tc := range []struct {
		name string
		in   any
		want bool
	}{
		{"nil", nil, false},
		{"empty string", "", false},
		{"empty map", map[string]any{}, false},
		{"typed pointer", valid, true},
		{"typed value", *valid, true},
		{"wire map", wire, true},
		{"partial map", map[string]any{"format": "html"}, false},
		{"unknown fields", map[string]any{"unknown": "value"}, false},
		{"wrong type", "not-a-ref", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MediaRefFromAny(tc.in)
			if (got != nil) != tc.want {
				t.Fatalf("MediaRefFromAny() = %+v, want present=%v", got, tc.want)
			}
			if got != nil && got.SHA256 != valid.SHA256 {
				t.Fatalf("MediaRefFromAny() digest = %q, want %q", got.SHA256, valid.SHA256)
			}
		})
	}
}
