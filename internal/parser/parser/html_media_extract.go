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

package parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"ragflow/internal/entity"
)

// DecodeHTMLSource converts a source document to UTF-8 the way the HTML
// parser does before walking it. A resolver that scans raw bytes must apply
// it first: a UTF-16 source contains no ASCII "data:image/" marker until the
// decode step.
func DecodeHTMLSource(data []byte) []byte {
	decoded, _ := decodeHTMLToUTF8(data)
	return decoded
}

// HTMLMediaSpan locates one inline image in an HTML source document.
//
// The span records byte offsets instead of the payload: a document with many
// images must not force every image into memory at once just to build the
// lookup index. Resolvers read the source once, keep the offsets, and decode
// only the image they return.
type HTMLMediaSpan struct {
	Ref          entity.MediaRef
	PayloadStart int
	PayloadEnd   int
}

// ScanHTMLMediaSpans finds the inline images whose payloads the HTML parser
// would accept. It applies the same size limit and base64 validation as the
// parser walk, so every media_ref the parser emits has a matching span.
//
// The scan is deliberately a byte scan rather than a second DOM walk: a DOM
// walk would materialize every payload to build the index. Images the parser
// skipped (inside <script>, outside the document budget, ...) may also appear
// here; the index only ever looks up a hash the parser emitted, so extras are
// harmless.
func ScanHTMLMediaSpans(data []byte) []HTMLMediaSpan {
	const prefix = "data:image/"
	const base64Suffix = ";base64"
	// The parser emits references for at most maxEmbeddedMediaItems images,
	// but a source can contain far more data URIs (including ones the walker
	// skips). Cap the index so a pathological document cannot allocate an
	// unbounded span map; the slack covers images the parser never visits.
	const maxSpans = 16 * maxEmbeddedMediaItems

	var spans []HTMLMediaSpan
	for offset := 0; offset < len(data); {
		rel := indexFoldASCII(data[offset:], prefix)
		if rel < 0 {
			break
		}
		start := offset + rel
		// normalizeHTMLImageSource rejects headers longer than 128 bytes.
		// Searching past that window would let a crafted document contain many
		// data:image/ markers and one far-away comma, making the scan
		// quadratic. A missing comma only skips this marker.
		window := data[start:min(len(data), start+129)]
		rel = bytes.IndexByte(window, ',')
		if rel < 0 {
			offset = start + len(prefix)
			continue
		}
		comma := start + rel
		header := data[start:comma]
		if len(header) < len(prefix)+len(base64Suffix) ||
			!strings.EqualFold(string(header[len(header)-len(base64Suffix):]), base64Suffix) {
			offset = start + len(prefix)
			continue
		}
		payloadStart := comma + 1
		payloadEnd := payloadStart
		for payloadEnd < len(data) {
			c := data[payloadEnd]
			// "/>" terminates an unquoted attribute even though '/' is a
			// base64 character.
			if c == '/' && payloadEnd+1 < len(data) && data[payloadEnd+1] == '>' {
				break
			}
			if isBase64PayloadByte(c) {
				payloadEnd++
				continue
			}
			// The HTML parser sees the decoded attribute value, so a
			// character reference such as &#43; is a valid '+' in the
			// payload. Keep it in the span and decode it with the rest of
			// the payload.
			if c == '&' {
				if n := htmlCharacterReferenceLength(data[payloadEnd:]); n > 0 {
					raw := string(data[payloadEnd : payloadEnd+n])
					if decoded := html.UnescapeString(raw); decoded != raw && isBase64PayloadText(decoded) {
						payloadEnd += n
						continue
					}
				}
			}
			break
		}
		offset = payloadEnd
		payload := data[payloadStart:payloadEnd]
		for len(payload) > 0 {
			last := payload[len(payload)-1]
			if last != '\r' && last != '\n' {
				break
			}
			payload = payload[:len(payload)-1]
		}
		if len(payload) == 0 {
			continue
		}
		spanEnd := payloadStart + len(payload)
		decodedPayload := html.UnescapeString(string(payload))
		_, size, sum, ok := normalizeBase64ImagePayload(decodedPayload, maxEmbeddedImageBytes)
		if !ok {
			continue
		}
		spans = append(spans, HTMLMediaSpan{
			Ref:          *newHTMLMediaRef(dataURIMediaType(string(header)), size, sum),
			PayloadStart: payloadStart,
			PayloadEnd:   spanEnd,
		})
		if len(spans) >= maxSpans {
			break
		}
	}
	return spans
}

// DecodeHTMLImagePayload validates and decodes one payload located by
// ScanHTMLMediaSpans. The returned digest lets the caller verify the bytes
// against the reference without hashing them twice.
func DecodeHTMLImagePayload(payload []byte) ([]byte, [sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	decoded := html.UnescapeString(string(payload))
	normalized, _, sum, ok := normalizeBase64ImagePayload(decoded, maxEmbeddedImageBytes)
	if !ok {
		return nil, sum, errors.New("html media: invalid image payload")
	}
	data, err := base64.StdEncoding.DecodeString(normalized)
	if err != nil {
		return nil, sum, fmt.Errorf("html media: decode payload: %w", err)
	}
	return data, sum, nil
}

// indexFoldASCII returns the first index of an ASCII prefix, ignoring case.
func indexFoldASCII(data []byte, prefix string) int {
	if len(prefix) == 0 {
		return 0
	}
	for i := 0; i+len(prefix) <= len(data); i++ {
		match := true
		for j := 0; j < len(prefix); j++ {
			c := data[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != prefix[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// isBase64PayloadByte reports whether c can appear in a data-URI payload.
// CR/LF are included because the parser normalizes them away.
func isBase64PayloadByte(c byte) bool {
	switch {
	case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9':
		return true
	case c == '+', c == '/', c == '=', c == '\r', c == '\n':
		return true
	default:
		return false
	}
}

// htmlCharacterReferenceLength returns the length of a complete character
// reference at the start of b, or 0 when b does not start with one. The scan
// is bounded so a stray '&' cannot walk the rest of the document.
func htmlCharacterReferenceLength(b []byte) int {
	if len(b) < 3 || b[0] != '&' {
		return 0
	}
	limit := min(len(b), 32)
	for i := 1; i < limit; i++ {
		switch c := b[i]; {
		case c == ';':
			return i + 1
		case c == '#', 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9':
		default:
			return 0
		}
	}
	return 0
}

// isBase64PayloadText reports whether the decoded payload consists only of
// bytes that can appear in a data-URI payload. Non-ASCII runes fail because
// their bytes are outside the base64 alphabet.
func isBase64PayloadText(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isBase64PayloadByte(s[i]) {
			return false
		}
	}
	return true
}
