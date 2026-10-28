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

// Stateful HTML half of Resolver: the source index, the DOM fallback, and
// HTML data-uri resolution. Pure normalization and scanning live in html.go.

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"ragflow/internal/entity"
)

func (r *Resolver) resolveHTMLDataURI(ctx context.Context, ref entity.MediaRef) ([]byte, error) {
	if ref.Format != "html" {
		return nil, fmt.Errorf("media: unsupported data-uri format %q", ref.Format)
	}
	if err := r.loadHTML(ctx); err != nil {
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
	data, sum, found, err := r.findMediaPayloadInDOM(ctx, ref.SHA256)
	if err != nil {
		return nil, err
	}
	if found {
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

// loadDOMImages builds and caches the fallback tree and a digest-to-src map
// in one walk. The fallback only runs after the tokenizer index misses, but a
// document past the span cap can miss many times; indexing the tree once
// keeps a miss O(1) instead of rescanning every image.
//
// A call cancelled before the map is published caches nothing: this is
// document-level state, and a per-image deadline must not disable the
// fallback for later images.
func (r *Resolver) loadDOMImages(ctx context.Context) (map[string]string, error) {
	r.domMu.Lock()
	defer r.domMu.Unlock()
	if r.domImages != nil {
		return r.domImages, nil
	}
	if r.domErr != nil {
		return nil, r.domErr
	}
	if !r.domLoaded {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		dom, err := html.Parse(bytes.NewReader(r.htmlData))
		if err != nil {
			r.domErr = err
			return nil, err
		}
		r.dom = dom
		r.domLoaded = true
	}
	images := make(map[string]string)
	if err := collectHTMLMediaSources(ctx, r.dom, images); err != nil {
		return nil, err
	}
	r.domImages = images
	// The map's src strings share the tree's payload storage, so the node
	// tree itself is no longer needed. Drop it to keep steady-state fallback
	// memory at the index instead of the index plus the whole DOM.
	r.dom = nil
	r.domLoaded = false
	return images, nil
}

// collectHTMLMediaSources records the first data URI seen for each digest.
// The src strings share the tree's storage, so the index does not duplicate
// the base64 payloads.
func collectHTMLMediaSources(ctx context.Context, doc *html.Node, out map[string]string) error {
	nodes := 0
	var walk func(*html.Node) error
	walk = func(n *html.Node) error {
		nodes++
		if nodes%256 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "img") {
			for _, attr := range n.Attr {
				if !strings.EqualFold(attr.Key, "src") {
					continue
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if _, _, sum, ok := NormalizeHTMLImageSource(attr.Val); ok {
					digest := hex.EncodeToString(sum[:])
					if _, exists := out[digest]; !exists {
						out[digest] = attr.Val
					}
				}
				break
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(doc)
}

// findMediaPayloadInDOM resolves a tokenizer miss by rebuilding a tree with
// the same HTML parser the parser uses. The tree and its digest index are
// built once per resolver: a document with several foreign-content images
// pays for one parse and one walk, and later misses only decode their entry.
func (r *Resolver) findMediaPayloadInDOM(ctx context.Context, digest string) ([]byte, [32]byte, bool, error) {
	images, err := r.loadDOMImages(ctx)
	if err != nil {
		return nil, [32]byte{}, false, err
	}
	src, ok := images[digest]
	if !ok {
		if err := ctx.Err(); err != nil {
			return nil, [32]byte{}, false, err
		}
		return nil, [32]byte{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, [32]byte{}, false, err
	}
	_, payload, ok := splitHTMLImageSource(src)
	if !ok {
		return nil, [32]byte{}, false, errors.New("media: fallback image is not a data URI")
	}
	data, sum, ok := decodeBase64ImagePayload(payload, MaxImagePayloadBytes)
	if !ok {
		return nil, [32]byte{}, false, errors.New("media: fallback image is invalid")
	}
	return data, sum, true, nil
}

// loadHTML reads and indexes the source once. The source bytes stay resident
// because a payload is returned by offset; the index itself holds only the
// spans, never the image bytes. Concurrent Resolve calls share both after the
// first call completes.
//
// Cancellation is retryable: the vision path resolves images under a
// per-image deadline and shares one resolver per document, so a deadline
// that expires while the index is being built must not be returned to every
// later image. Other failures are cached because they describe the source,
// not the caller's deadline.
func (r *Resolver) loadHTML(ctx context.Context) error {
	r.indexMu.Lock()
	defer r.indexMu.Unlock()
	if r.indexLoaded {
		return nil
	}
	if r.indexErr != nil {
		return r.indexErr
	}
	if err := r.buildHTMLIndex(ctx); err != nil {
		if ctx.Err() == nil {
			r.indexErr = err
		}
		return err
	}
	r.indexLoaded = true
	return nil
}

func (r *Resolver) buildHTMLIndex(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := r.src.Bytes(ctx)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Scan the same UTF-8 form the parser walked; the raw bytes of a UTF-16
	// document would not contain the ASCII data-URI marker.
	data = decodeHTMLSource(data)
	if err := ctx.Err(); err != nil {
		return err
	}
	spans, truncated := scanHTMLMediaSpans(ctx, data)
	if err := ctx.Err(); err != nil {
		return err
	}
	byDigest := make(map[string]htmlMediaSpan, len(spans))
	for _, span := range spans {
		byDigest[span.Ref.SHA256] = span
	}
	r.htmlData = data
	r.htmlTruncated = truncated
	r.htmlSpans = byDigest
	return nil
}
