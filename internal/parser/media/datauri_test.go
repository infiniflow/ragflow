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

package media

import (
	"crypto/sha256"
	"strings"
	"testing"
)

func TestDecodeDataImageSourceCaseAndPadding(t *testing.T) {
	mime, data, ok := DecodeDataImageSource("DATA:IMAGE/PNG;BASE64,aGVsbG8")
	if !ok {
		t.Fatal("DecodeDataImageSource rejected a valid case-insensitive data URI")
	}
	if mime != "image/png" {
		t.Errorf("mime = %q, want image/png", mime)
	}
	if string(data) != "hello" {
		t.Errorf("data = %q, want hello", data)
	}
}

func TestDecodeDataImageSourceRejectsInvalidAndOversizedHeaders(t *testing.T) {
	for _, src := range []string{
		"data:image/png;base64,",
		"data:image/png;base64,YQ=",
		"data:image/png;base64,YQ===",
		"data:image/png;base64,G0==JbjQ",
		"data:image/png;" + strings.Repeat("x", 200) + ",aGVsbG8=",
	} {
		if _, _, ok := DecodeDataImageSource(src); ok {
			t.Errorf("DecodeDataImageSource(%q) succeeded", src)
		}
	}
}

func TestNormalizeDataImageSourceMatchesDecode(t *testing.T) {
	src := "data:image/png;base64,aGVs\nbG8"
	normalized, size, normalizedSum, ok := NormalizeDataImageSource(src)
	if !ok {
		t.Fatal("NormalizeDataImageSource rejected a valid payload")
	}
	mime, data, ok := DecodeDataImageSource(src)
	if !ok {
		t.Fatal("DecodeDataImageSource rejected a valid payload")
	}
	if normalized != "data:image/png;base64,aGVsbG8=" {
		t.Fatalf("normalized = %q", normalized)
	}
	decodeSum := sha256.Sum256(data)
	if size != int64(len(data)) || normalizedSum != decodeSum || mime != "image/png" {
		t.Fatalf("normalize/decode disagree: size=%d decodeSize=%d sum=%x decodeSum=%x mime=%q",
			size, len(data), normalizedSum, decodeSum, mime)
	}
}

// TestDataImageDecodedSizeMatchesDecode checks that the allocation-free
// preflight accepts exactly what DecodeDataImageSource accepts and reports the
// same decoded size, so budget admission never diverges from the actual write.
func TestDataImageDecodedSizeMatchesDecode(t *testing.T) {
	cases := []struct {
		src  string
		want int // -1 means the source is malformed
	}{
		{"data:image/png;base64,aGVsbG8=", 5},
		{"data:image/png;base64,aGVsbG8", 5},
		{"data:image/png;base64,aGVs\nbG8", 5},
		{"DATA:IMAGE/PNG;BASE64,aGVsbG8=", 5},
		{"data:image/png;base64,YQ==", 1},
		{"data:image/png;base64,YWI=", 2},
		{"data:image/png;base64,YQ", 1},
		{"data:image/png;base64,", -1},
		{"data:image/png;base64,YQ=", -1},
		{"data:image/png;base64,YQ===", -1},
		{"data:image/png;base64,G0==JbjQ", -1},
		{"data:image/png;base64,Y?==", -1},
		{"data:image/png;" + strings.Repeat("x", 200) + ",aGVsbG8=", -1},
	}
	for _, tc := range cases {
		size, ok := DataImageDecodedSize(tc.src)
		_, data, decodeOK := DecodeDataImageSource(tc.src)
		if ok != (tc.want >= 0) {
			t.Errorf("%q: DataImageDecodedSize ok = %v, want %v", tc.src, ok, tc.want >= 0)
			continue
		}
		if ok != decodeOK {
			t.Errorf("%q: preflight ok = %v, decode ok = %v", tc.src, ok, decodeOK)
			continue
		}
		if ok && (size != tc.want || size != len(data)) {
			t.Errorf("%q: size = %d, want %d (decoded %d)", tc.src, size, tc.want, len(data))
		}
	}
}

// TestDataImageDecodedSizeIgnoresPerImageLimit checks that the preflight
// reports the size of a syntactically valid payload even when it exceeds the
// configured per-image limit, leaving the limit decision to the caller.
func TestDataImageDecodedSizeIgnoresPerImageLimit(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_IMAGE_MAX_BYTES", "2")
	size, ok := DataImageDecodedSize("data:image/png;base64,aGVsbG8=")
	if !ok || size != 5 {
		t.Fatalf("size, ok = %d, %v, want 5, true", size, ok)
	}
	if size <= ImageMaxBytes() {
		t.Fatalf("size %d does not exceed the configured limit %d", size, ImageMaxBytes())
	}
}
