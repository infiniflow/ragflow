//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"strings"

	"golang.org/x/net/html"

	"ragflow/internal/parser/media"
)

// htmlImageSource picks a fetchable source for an <img>: an HTTP(S) src,
// then a known lazy-load attribute, then srcset/data-srcset, then a sibling
// <picture><source srcset>. It chooses the first candidate this parser can
// fetch; it does not reproduce the browser's media/type/sizes selection.
// When none is usable it returns the original src so callers keep their
// local-reference fallback.
func htmlImageSource(n *html.Node) string {
	src := htmlAttribute(n, "src")
	if remote, ok := normalizeRemoteImageSource(src); ok {
		return remote
	}
	for _, attr := range []string{"data-src", "data-original", "data-lazy-src", "data-url", "data-image"} {
		if candidate, ok := imageSourceCandidate(htmlAttribute(n, attr)); ok {
			return candidate
		}
	}
	for _, attr := range []string{"srcset", "data-srcset"} {
		if candidate, ok := firstSrcsetCandidate(htmlAttribute(n, attr)); ok {
			return candidate
		}
	}
	if picture := enclosingPicture(n); picture != nil {
		for child := picture.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && child.Data == "source" {
				if candidate, ok := firstSrcsetCandidate(htmlAttribute(child, "srcset")); ok {
					return candidate
				}
			}
		}
	}
	return src
}

// selectImageSource chooses the <img> source for one walk state. An EPUB
// keeps a non-remote src (archive path or data URI) ahead of remote lazy-load
// and srcset candidates because its resolver maps relative paths into the ZIP
// and cannot fetch remote URLs.
func (s *htmlWalkState) selectImageSource(n *html.Node) string {
	if s.preferLocalImageSource {
		if src := htmlAttribute(n, "src"); strings.TrimSpace(src) != "" && !isRemoteImageSource(src) {
			return src
		}
	}
	return htmlImageSource(n)
}

// imageSourceCandidate reports whether raw is a fetchable remote URL or an
// inline image data URI, returning its normalized form.
func imageSourceCandidate(raw string) (string, bool) {
	if remote, ok := normalizeRemoteImageSource(raw); ok {
		return remote, true
	}
	raw = strings.TrimSpace(raw)
	if media.IsDataImageSource(raw) {
		return raw, true
	}
	return "", false
}

// firstSrcsetCandidate returns the first fetchable candidate in a srcset
// value. Following the WHATWG URL collection step, a candidate URL runs to the
// first ASCII whitespace, so commas inside the URL (including a data URI's
// payload) stay part of it; commas only separate candidates when they trail
// the URL or end its descriptor.
func firstSrcsetCandidate(srcset string) (string, bool) {
	const asciiWhitespace = " \t\r\n\f"
	srcset = strings.TrimLeft(srcset, asciiWhitespace+",")
	for srcset != "" {
		urlEnd := strings.IndexAny(srcset, asciiWhitespace)
		token := srcset
		if urlEnd >= 0 {
			token = srcset[:urlEnd]
		}
		url := strings.TrimRight(token, ",")
		trailingComma := len(token) != len(url)
		if normalized, ok := imageSourceCandidate(url); ok {
			return normalized, true
		}
		if trailingComma {
			srcset = srcset[len(token):]
		} else {
			rest := srcset[len(token):]
			comma := strings.IndexByte(rest, ',')
			if comma < 0 {
				return "", false
			}
			srcset = rest[comma+1:]
		}
		srcset = strings.TrimLeft(srcset, asciiWhitespace+",")
	}
	return "", false
}

// enclosingPicture returns the nearest ancestor <picture> element, whose
// <source srcset> provides the responsive variants for its <img>.
func enclosingPicture(n *html.Node) *html.Node {
	for parent := n.Parent; parent != nil; parent = parent.Parent {
		if parent.Type == html.ElementNode && parent.Data == "picture" {
			return parent
		}
	}
	return nil
}
