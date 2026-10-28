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

// Package media materializes the MediaRefs parsers emit. A parser that emits
// a MediaRef keeps image bytes out of its result. Whoever needs an actual
// image (the chunker's upload pass, the vision enhancement path) hands the
// reference and a MediaSource to a Resolver, which reads the source once
// and returns exactly one image per call. An index build cancelled by the
// caller's context is retried instead of cached.
package media

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/net/html"

	"ragflow/internal/entity"
)

// MediaSource supplies the raw bytes of the document a MediaRef points into.
// A completed read is reused for every image in that document, so
// implementations may fetch from object storage without adding a per-image
// round trip. A read cancelled by the caller's context may be retried.
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

// Resolver materializes image bytes for a MediaRef. One resolver serves
// exactly one source document, bound at construction, so a call can never
// resolve against a different document than the cached index.
type Resolver struct {
	src MediaSource

	// indexMu guards the source index. A cancellation is never cached as an
	// index failure: Resolve may be called once per image with a per-image
	// deadline, and one expired call must not hide the document from the
	// next one.
	indexMu       sync.Mutex
	indexLoaded   bool
	indexErr      error
	htmlSpans     map[string]htmlMediaSpan
	htmlData      []byte
	htmlTruncated bool

	// domMu caches the digest-to-src index built for the rare fallback path.
	// The tree is read-only after parsing and is released once the index is
	// built; the index's src strings keep the payload storage alive.
	domMu     sync.Mutex
	domLoaded bool
	dom       *html.Node
	domImages map[string]string
	domErr    error
}

// NewResolver returns a resolver bound to src. Create one per document.
func NewResolver(src MediaSource) *Resolver { return &Resolver{src: src} }

func (r *Resolver) Resolve(ctx context.Context, ref entity.MediaRef) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !ref.Valid() {
		return nil, errors.New("media: invalid reference")
	}
	if r == nil || r.src == nil {
		return nil, errors.New("media: no source document")
	}
	if ref.Size > MaxImagePayloadBytes {
		return nil, fmt.Errorf("media: image %s exceeds the %d-byte limit", shortMediaDigest(ref.SHA256), MaxImagePayloadBytes)
	}
	switch ref.Kind {
	case entity.MediaKindDataURI:
		return r.resolveHTMLDataURI(ctx, ref)
	default:
		return nil, fmt.Errorf("media: unsupported kind %q", ref.Kind)
	}
}

// validateMediaPayload applies the integrity and size checks shared by the
// tokenizer path and the DOM fallback.
func validateMediaPayload(data []byte, sum [32]byte, ref entity.MediaRef) ([]byte, error) {
	if got := hex.EncodeToString(sum[:]); got != ref.SHA256 {
		return nil, fmt.Errorf("media: image digest mismatch: reference %s, source %s", shortMediaDigest(ref.SHA256), shortMediaDigest(got))
	}
	if int64(len(data)) > MaxImagePayloadBytes {
		return nil, fmt.Errorf("media: image %s exceeds the %d-byte limit", shortMediaDigest(ref.SHA256), MaxImagePayloadBytes)
	}
	if ref.Size > 0 && int64(len(data)) != ref.Size {
		return nil, fmt.Errorf("media: image size mismatch: reference %d bytes, source %d bytes", ref.Size, len(data))
	}
	return data, nil
}

func shortMediaDigest(digest string) string {
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12]
}
