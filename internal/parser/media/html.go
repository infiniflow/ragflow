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
	"encoding/base64"
	"os"
	"strconv"
	"strings"
)

// DecodeHTMLImageSource validates and decodes one inline HTML image. The HTML
// parser calls this once, then writes the bytes to the run media cache; later
// ingestion components materialize only the emitted MediaRef.
func DecodeHTMLImageSource(src string) (string, []byte, bool) {
	prefix, payload, ok := splitHTMLImageSource(src)
	if !ok {
		return "", nil, false
	}
	data, ok := decodeBase64ImagePayload(payload, HTMLImageMaxBytes())
	if !ok {
		return "", nil, false
	}
	return DataURIMediaType(prefix), data, true
}

// HTMLImageMaxBytes is the per-image limit for HTML media.
func HTMLImageMaxBytes() int {
	const defaultLimit = 32 << 20
	if n, err := strconv.Atoi(os.Getenv("RAGFLOW_HTML_IMAGE_MAX_BYTES")); err == nil && n > 0 && n <= int(^uint(0)>>1)/4*3 {
		return n
	}
	return defaultLimit
}

// HTMLImageExceedsLimit identifies oversized, syntactically valid data URIs.
func HTMLImageExceedsLimit(src string) bool {
	_, payload, ok := splitHTMLImageSource(src)
	if !ok {
		return false
	}
	payload = strings.ReplaceAll(strings.ReplaceAll(payload, "\r", ""), "\n", "")
	n := len(payload)
	if n == 0 {
		return false
	}
	padding := 0
	for padding < n && payload[n-1-padding] == '=' {
		padding++
	}
	if padding > 2 {
		return false
	}
	if n%4 == 1 {
		return false
	}
	decoded := base64.StdEncoding.DecodedLen((n+3)/4*4) - padding
	if n%4 != 0 {
		decoded -= 4 - n%4
	}
	return decoded > HTMLImageMaxBytes()
}

// splitHTMLImageSource validates the data-URI header and returns the normalized
// prefix and base64 payload.
func splitHTMLImageSource(src string) (prefix, payload string, ok bool) {
	src = strings.TrimSpace(src)
	if !IsHTMLDataImageSource(src) {
		return "", "", false
	}
	separator := strings.IndexByte(src, ',')
	if separator <= len("data:image/") || separator > 128 || !strings.EqualFold(src[separator-len(";base64"):separator], ";base64") {
		return "", "", false
	}
	return strings.ToLower(src[:separator+1]), src[separator+1:], true
}

// NormalizeHTMLImageSource validates an inline image while avoiding a decoded
// byte buffer. It remains the path for HTML layouts that intentionally keep
// the normalized data URI inline rather than storing it in the run cache.
func NormalizeHTMLImageSource(src string) (string, int64, [sha256.Size]byte, bool) {
	var sum [sha256.Size]byte
	prefix, payload, ok := splitHTMLImageSource(src)
	if !ok {
		return "", 0, sum, false
	}
	normalized, size, sum, ok := NormalizeBase64ImagePayload(payload, MaxImagePayloadBytes)
	if !ok {
		return "", 0, sum, false
	}
	return prefix + normalized, size, sum, true
}

// DataURIMediaType returns the media type of a normalized
// "data:<mime>;base64," URI.
func DataURIMediaType(uri string) string {
	colon := strings.IndexByte(uri, ':')
	semi := strings.IndexByte(uri, ';')
	if colon < 0 || semi <= colon+1 {
		return ""
	}
	return strings.ToLower(uri[colon+1 : semi])
}

// IsHTMLDataImageSource reports whether src is an image data URI.
func IsHTMLDataImageSource(src string) bool {
	const dataPrefix = "data:image/"
	src = strings.TrimSpace(src)
	return len(src) >= len(dataPrefix) && strings.EqualFold(src[:len(dataPrefix)], dataPrefix)
}
