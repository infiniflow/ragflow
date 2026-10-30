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
)

// DecodeDataImageSource validates and decodes one inline image data URI. The
// parser calls this once, then writes the bytes to the run media cache; later
// ingestion components materialize only the emitted MediaRef.
func DecodeDataImageSource(src string) (string, []byte, bool) {
	prefix, payload, ok := splitDataImageSource(src)
	if !ok {
		return "", nil, false
	}
	data, ok := decodeBase64ImagePayload(payload, ImageMaxBytes())
	if !ok {
		return "", nil, false
	}
	return DataURIMediaType(prefix), data, true
}

// DataImageDecodedSize validates an image data URI and returns the decoded
// payload size without allocating the decoded bytes. Malformed payloads are
// rejected; a syntactically valid payload above ImageMaxBytes is still
// reported by its size so callers can apply their own limit.
func DataImageDecodedSize(src string) (int, bool) {
	_, payload, ok := splitDataImageSource(src)
	if !ok {
		return 0, false
	}
	encoded, padding, ok := scanBase64ImagePayload(payload, len(payload))
	if !ok {
		return 0, false
	}
	padded := (encoded + 3) / 4 * 4
	return padded/4*3 - (padded - encoded) - padding, true
}

// splitDataImageSource validates the data-URI header and returns the normalized
// prefix and base64 payload.
func splitDataImageSource(src string) (prefix, payload string, ok bool) {
	src = strings.TrimSpace(src)
	if !IsDataImageSource(src) {
		return "", "", false
	}
	separator := strings.IndexByte(src, ',')
	if separator <= len("data:image/") || separator > 128 || !strings.EqualFold(src[separator-len(";base64"):separator], ";base64") {
		return "", "", false
	}
	return strings.ToLower(src[:separator+1]), src[separator+1:], true
}

// NormalizeDataImageSource validates an inline image while avoiding a decoded
// byte buffer. It remains the path for layouts that intentionally keep the
// normalized data URI inline rather than storing it in the run cache.
func NormalizeDataImageSource(src string) (string, int64, [sha256.Size]byte, bool) {
	var sum [sha256.Size]byte
	prefix, payload, ok := splitDataImageSource(src)
	if !ok {
		return "", 0, sum, false
	}
	normalized, size, sum, ok := NormalizeBase64ImagePayload(payload, ImageMaxBytes())
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

// IsDataImageSource reports whether src is an image data URI.
func IsDataImageSource(src string) bool {
	const dataPrefix = "data:image/"
	src = strings.TrimSpace(src)
	return len(src) >= len(dataPrefix) && strings.EqualFold(src[:len(dataPrefix)], dataPrefix)
}
