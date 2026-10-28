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
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"ragflow/internal/entity"
)

// htmlMediaSpan locates one inline image tag in an HTML source document.
//
// TagStart/TagEnd delimit the raw <img ...> start tag. Resolving re-tokenizes
// that fragment, so the attribute value is decoded exactly as the parser saw
// it instead of being re-interpreted by a second byte-level parser.
type htmlMediaSpan struct {
	Ref      entity.MediaRef
	TagStart int
	TagEnd   int
}

// decodeHTMLSource converts a source document to UTF-8 the way the HTML
// parser does before walking it. A resolver that tokenizes raw bytes must
// apply it first: a UTF-16 source contains no ASCII markup until this step.
func decodeHTMLSource(data []byte) []byte {
	decoded, _ := decodeHTMLToUTF8(data)
	return decoded
}

// scanHTMLMediaSpans returns inline images in document order. Both the
// scanner and the parser use x/net/html, so entity references, quoting and
// unquoted attribute values follow the same rules by construction.
//
// The truncated flag reports that the span cap was reached. A missing digest
// in that state means the index is incomplete, not that the image is absent.
func scanHTMLMediaSpans(ctx context.Context, data []byte) ([]htmlMediaSpan, bool) {
	const maxSpans = 4096
	spans := make([]htmlMediaSpan, 0, 8)
	truncated := false
	walkHTMLMediaSpans(ctx, data, func(span htmlMediaSpan) bool {
		if len(spans) >= maxSpans {
			truncated = true
			return false
		}
		spans = append(spans, span)
		return true
	})
	return spans, truncated
}

// walkHTMLMediaSpans tokenizes data and visits every <img src> whose value is
// an accepted data URI. Returning false from visit stops the walk.
func walkHTMLMediaSpans(ctx context.Context, data []byte, visit func(htmlMediaSpan) bool) {
	tokenizer := html.NewTokenizer(bytes.NewReader(data))
	offset := 0
	for tokens := 0; ; tokens++ {
		if tokens%256 == 0 && ctx.Err() != nil {
			return
		}
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			return
		}
		raw := tokenizer.Raw()
		if tokenType == html.StartTagToken || tokenType == html.SelfClosingTagToken {
			if name, _ := tokenizer.TagName(); strings.EqualFold(string(name), "img") {
				if src, ok := htmlTagAttr(tokenizer, "src"); ok {
					if ref, ok := htmlDataURIRef(src); ok {
						if !visit(htmlMediaSpan{Ref: ref, TagStart: offset, TagEnd: offset + len(raw)}) {
							return
						}
					}
				}
			}
		}
		offset += len(raw)
	}
}

// htmlTagAttr returns one attribute of the current start tag. The tokenizer
// has already decoded character references in the value.
func htmlTagAttr(tokenizer *html.Tokenizer, key string) ([]byte, bool) {
	for {
		name, value, more := tokenizer.TagAttr()
		if strings.EqualFold(string(name), key) {
			return value, true
		}
		if !more {
			return nil, false
		}
	}
}

// htmlDataURIRef validates one src value and builds its content-addressed
// reference. It reuses the parser's normalization so both sides accept the
// same payloads and compute the same digest.
func htmlDataURIRef(src []byte) (entity.MediaRef, bool) {
	normalized, size, sum, ok := normalizeHTMLImageSource(string(src))
	if !ok {
		return entity.MediaRef{}, false
	}
	return *newHTMLMediaRef(dataURIMediaType(normalized), size, sum), true
}

// decodeHTMLImagePayload re-tokenizes one <img ...> start tag and decodes its
// src payload. The returned digest lets callers verify the payload against the
// reference without hashing it twice.
func decodeHTMLImagePayload(tag []byte) ([]byte, [sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	tokenizer := html.NewTokenizer(bytes.NewReader(tag))
	if tokenType := tokenizer.Next(); tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken {
		return nil, sum, errors.New("html media: span is not an img start tag")
	}
	src, ok := htmlTagAttr(tokenizer, "src")
	if !ok {
		return nil, sum, errors.New("html media: span has no src attribute")
	}
	normalized, _, sum, ok := normalizeHTMLImageSource(string(src))
	if !ok {
		return nil, sum, errors.New("html media: invalid image payload")
	}
	data, err := decodeNormalizedMediaURI(normalized)
	if err != nil {
		return nil, sum, err
	}
	return data, sum, nil
}

// decodeNormalizedMediaURI decodes the payload of a normalized
// "data:<mime>;base64,..." URI.
func decodeNormalizedMediaURI(normalized string) ([]byte, error) {
	separator := strings.IndexByte(normalized, ',')
	if separator < 0 {
		return nil, errors.New("html media: malformed data URI")
	}
	data, err := base64.StdEncoding.DecodeString(normalized[separator+1:])
	if err != nil {
		return nil, fmt.Errorf("html media: decode payload: %w", err)
	}
	return data, nil
}
