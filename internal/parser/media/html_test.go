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

func TestDecodeHTMLImageSourceCaseAndPadding(t *testing.T) {
	mime, data, ok := DecodeHTMLImageSource("DATA:IMAGE/PNG;BASE64,aGVsbG8")
	if !ok {
		t.Fatal("DecodeHTMLImageSource rejected a valid case-insensitive data URI")
	}
	if mime != "image/png" {
		t.Errorf("mime = %q, want image/png", mime)
	}
	if string(data) != "hello" {
		t.Errorf("data = %q, want hello", data)
	}
}

func TestDecodeHTMLImageSourceRejectsInvalidAndOversizedHeaders(t *testing.T) {
	for _, src := range []string{
		"data:image/png;base64,",
		"data:image/png;base64,YQ=",
		"data:image/png;base64,YQ===",
		"data:image/png;base64,G0==JbjQ",
		"data:image/png;" + strings.Repeat("x", 200) + ",aGVsbG8=",
	} {
		if _, _, ok := DecodeHTMLImageSource(src); ok {
			t.Errorf("DecodeHTMLImageSource(%q) succeeded", src)
		}
	}
}

func TestNormalizeHTMLImageSourceMatchesDecode(t *testing.T) {
	src := "data:image/png;base64,aGVs\nbG8"
	normalized, size, normalizedSum, ok := NormalizeHTMLImageSource(src)
	if !ok {
		t.Fatal("NormalizeHTMLImageSource rejected a valid payload")
	}
	mime, data, ok := DecodeHTMLImageSource(src)
	if !ok {
		t.Fatal("DecodeHTMLImageSource rejected a valid payload")
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

func TestHTMLImageExceedsConfiguredLimit(t *testing.T) {
	t.Setenv("RAGFLOW_HTML_IMAGE_MAX_BYTES", "5")
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{"data:image/png;base64,aGVsbG8=", false},
		{"data:image/png;base64,aGVsbG8", false},
		{"data:image/png;base64,aGVsbG8h", true},
		{"data:image/png;base64,YQ==", false},
	} {
		if got := HTMLImageExceedsLimit(tc.src); got != tc.want {
			t.Errorf("%q: got %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestHTMLImageLimitDefaultsAndConfiguration(t *testing.T) {
	t.Setenv("RAGFLOW_HTML_IMAGE_MAX_BYTES", "")
	if got := HTMLImageMaxBytes(); got != 32<<20 {
		t.Fatalf("default = %d", got)
	}
	t.Setenv("RAGFLOW_HTML_IMAGE_MAX_BYTES", "67108864")
	if got := HTMLImageMaxBytes(); got != 64<<20 {
		t.Fatalf("configured = %d", got)
	}
}
