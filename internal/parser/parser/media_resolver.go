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

// Package parser: this file materializes MediaRefs emitted by parsers. A
// parser that emits MediaRef keeps image bytes out of its result. Whoever
// needs an actual image (the chunker's upload pass, the vision enhancement
// path) hands the reference and a MediaSource to a MediaResolver, which reads
// the source once per invocation and returns exactly one image.
package parser

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/net/html"

	"ragflow/internal/entity"
)

// maxMediaImageBytes bounds one materialized image. It reuses the inline
// payload ceiling so a reference can never resolve to something the inline
// path would have rejected.
const maxMediaImageBytes = int64(MaxImagePayloadBytes)

// MediaSource supplies the raw bytes of the document a MediaRef points into.
// It is called at most once per MediaResolver, so implementations may fetch
// from object storage without adding a per-image round trip.
type MediaSource interface {
	Bytes(ctx context.Context) ([]byte, error)
}

// MediaSourceFunc adapts a function to MediaSource.
type MediaSourceFunc func(context.Context) ([]byte, error)

// Bytes implements MediaSource.
func (f MediaSourceFunc) Bytes(ctx context.Context) ([]byte, error) { return f(ctx) }

// ErrMediaNotFound reports that a readable source document does not contain
// the referenced image. Callers may skip the image; every other resolver
// error (source read, digest or size mismatch, unsupported reference) is a
// failure that must not be turned into a silent image drop.
var ErrMediaNotFound = errors.New("media: reference not found")

// MediaResolver materializes image bytes for a MediaRef.
type MediaResolver interface {
	Resolve(ctx context.Context, ref entity.MediaRef, src MediaSource) ([]byte, error)
}

// NewMediaResolver returns a resolver that caches the extracted media index
// for the first source it sees. Create one per document/invocation.
func NewMediaResolver() MediaResolver { return &mediaResolver{} }

type mediaResolver struct {
	once          sync.Once
	htmlSpans     map[string]htmlMediaSpan
	htmlData      []byte
	htmlTruncated bool

	// domOnce caches the tree built for the rare fallback path. The tree is
	// read-only after parsing, so concurrent Resolve calls can walk it.
	domOnce sync.Once
	dom     *html.Node
	domErr  error

	err error
}

func (r *mediaResolver) Resolve(ctx context.Context, ref entity.MediaRef, src MediaSource) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !ref.Valid() {
		return nil, errors.New("media: invalid reference")
	}
	if src == nil {
		return nil, errors.New("media: no source document")
	}
	if ref.Size > maxMediaImageBytes {
		return nil, fmt.Errorf("media: image %s exceeds the %d-byte limit", shortMediaDigest(ref.SHA256), maxMediaImageBytes)
	}
	switch ref.Kind {
	case entity.MediaKindDataURI:
		return r.resolveDataURI(ctx, ref, src)
	default:
		return nil, fmt.Errorf("media: unsupported kind %q", ref.Kind)
	}
}

func (r *mediaResolver) resolveDataURI(ctx context.Context, ref entity.MediaRef, src MediaSource) ([]byte, error) {
	if ref.Format != "html" {
		return nil, fmt.Errorf("media: unsupported data-uri format %q", ref.Format)
	}
	if err := r.loadHTML(ctx, src); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if span, ok := r.htmlSpans[ref.SHA256]; ok {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, sum, err := decodeHTMLImagePayload(r.htmlData[span.TagStart:span.TagEnd])
		if err != nil {
			return nil, err
		}
		return validateMediaPayload(data, sum, ref)
	}
	// The streaming tokenizer does not model foreign-content contexts (for
	// example <svg><title>) and other tree-construction edge cases. Fall back
	// to the parser's actual DOM walk, which is the source of truth for what
	// the parser emitted.
	if data, sum, found := r.findMediaPayloadInDOM(ctx, ref.SHA256); found {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return validateMediaPayload(data, sum, ref)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.htmlTruncated {
		return nil, fmt.Errorf("media: html media index was truncated before image %s", shortMediaDigest(ref.SHA256))
	}
	return nil, fmt.Errorf("%w: image %s not found in html source", ErrMediaNotFound, shortMediaDigest(ref.SHA256))
}

// validateMediaPayload applies the integrity and size checks shared by the
// tokenizer path and the DOM fallback.
func validateMediaPayload(data []byte, sum [32]byte, ref entity.MediaRef) ([]byte, error) {
	if got := hex.EncodeToString(sum[:]); got != ref.SHA256 {
		return nil, fmt.Errorf("media: image digest mismatch: reference %s, source %s", shortMediaDigest(ref.SHA256), shortMediaDigest(got))
	}
	if int64(len(data)) > maxMediaImageBytes {
		return nil, fmt.Errorf("media: image %s exceeds the %d-byte limit", shortMediaDigest(ref.SHA256), maxMediaImageBytes)
	}
	if ref.Size > 0 && int64(len(data)) != ref.Size {
		return nil, fmt.Errorf("media: image size mismatch: reference %d bytes, source %d bytes", ref.Size, len(data))
	}
	return data, nil
}

// findMediaPayloadInDOM resolves a tokenizer miss by rebuilding a tree with
// the same HTML parser the parser uses. The tree is built once per resolver:
// a document with several foreign-content images pays for one parse, and
// later misses only walk it. The fallback only runs after the low-memory
// tokenizer index misses.
func (r *mediaResolver) findMediaPayloadInDOM(ctx context.Context, digest string) ([]byte, [32]byte, bool) {
	r.domOnce.Do(func() {
		if err := ctx.Err(); err != nil {
			r.domErr = err
			return
		}
		r.dom, r.domErr = html.Parse(bytes.NewReader(r.htmlData))
	})
	if r.domErr != nil {
		return nil, [32]byte{}, false
	}
	return findHTMLMediaPayloadInNode(ctx, r.dom, digest)
}

func findHTMLMediaPayloadInNode(ctx context.Context, doc *html.Node, digest string) ([]byte, [32]byte, bool) {
	var sum [32]byte
	var payload []byte
	found := false
	nodes := 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found {
			return
		}
		nodes++
		if nodes%256 == 0 && ctx.Err() != nil {
			return
		}
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "img") {
			for _, attr := range n.Attr {
				if !strings.EqualFold(attr.Key, "src") {
					continue
				}
				normalized, _, candidate, ok := normalizeHTMLImageSource(attr.Val)
				if ok && hex.EncodeToString(candidate[:]) == digest {
					if decoded, err := decodeNormalizedMediaURI(normalized); err == nil {
						payload, sum, found = decoded, candidate, true
					}
				}
				break
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return payload, sum, found
}

// loadHTML reads and indexes the source once. The source bytes stay resident
// because a payload is returned by offset; the index itself holds only the
// spans, never the image bytes. Concurrent Resolve calls share both after the
// first call completes.
func (r *mediaResolver) loadHTML(ctx context.Context, src MediaSource) error {
	r.once.Do(func() {
		if err := ctx.Err(); err != nil {
			r.err = err
			return
		}
		data, err := src.Bytes(ctx)
		if err != nil {
			r.err = err
			return
		}
		if err := ctx.Err(); err != nil {
			r.err = err
			return
		}
		// Scan the same UTF-8 form the parser walked; the raw bytes of a
		// UTF-16 document would not contain the ASCII data-URI marker.
		data = decodeHTMLSource(data)
		if err := ctx.Err(); err != nil {
			r.err = err
			return
		}
		r.htmlData = data
		spans, truncated := scanHTMLMediaSpans(ctx, data)
		if err := ctx.Err(); err != nil {
			r.err = err
			return
		}
		r.htmlTruncated = truncated
		r.htmlSpans = make(map[string]htmlMediaSpan, len(spans))
		for _, span := range spans {
			r.htmlSpans[span.Ref.SHA256] = span
		}
	})
	return r.err
}

func shortMediaDigest(digest string) string {
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12]
}
