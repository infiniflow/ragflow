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

// Package media re-materializes parser MediaRefs from their source documents.
//
// A parser that emits MediaRef keeps image bytes out of its result. Whoever
// needs an actual image (the chunker's upload pass, the vision enhancement
// path) hands the reference and a Source to a Resolver, which reads the
// source once per invocation and returns exactly one image.
package media

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
)

// MaxImageBytes bounds one materialized image. It is the same ceiling the
// parser applies to inline payloads, so a reference can never resolve to
// something the inline path would have rejected.
const MaxImageBytes = int64(parser.MaxImagePayloadBytes)

// Source supplies the raw bytes of the document a MediaRef points into. It
// is called at most once per Resolver, so implementations may fetch from
// object storage without adding a per-image round trip.
type Source interface {
	Bytes(ctx context.Context) ([]byte, error)
}

// SourceFunc adapts a function to Source.
type SourceFunc func(context.Context) ([]byte, error)

// Bytes implements Source.
func (f SourceFunc) Bytes(ctx context.Context) ([]byte, error) { return f(ctx) }

// Resolver materializes image bytes for a MediaRef.
type Resolver interface {
	Resolve(ctx context.Context, ref entity.MediaRef, src Source) ([]byte, error)
}

// NewResolver returns a resolver that caches the extracted media index for
// the first source it sees. Create one per document/invocation.
func NewResolver() Resolver { return &resolver{} }

type resolver struct {
	once      sync.Once
	htmlSpans map[string]parser.HTMLMediaSpan
	htmlData  []byte

	err error
}

func (r *resolver) Resolve(ctx context.Context, ref entity.MediaRef, src Source) ([]byte, error) {
	if !ref.Valid() {
		return nil, errors.New("media: invalid reference")
	}
	if src == nil {
		return nil, errors.New("media: no source document")
	}
	if ref.Size > MaxImageBytes {
		return nil, fmt.Errorf("media: image %s exceeds the %d-byte limit", shortDigest(ref.SHA256), MaxImageBytes)
	}
	switch ref.Kind {
	case entity.MediaKindDataURI:
		return r.resolveDataURI(ctx, ref, src)
	default:
		return nil, fmt.Errorf("media: unsupported kind %q", ref.Kind)
	}
}

func (r *resolver) resolveDataURI(ctx context.Context, ref entity.MediaRef, src Source) ([]byte, error) {
	if ref.Format != "html" {
		return nil, fmt.Errorf("media: unsupported data-uri format %q", ref.Format)
	}
	if err := r.loadHTML(ctx, src); err != nil {
		return nil, err
	}
	span, ok := r.htmlSpans[ref.SHA256]
	if !ok {
		return nil, fmt.Errorf("media: image %s not found in html source", shortDigest(ref.SHA256))
	}
	data, sum, err := parser.DecodeHTMLImagePayload(r.htmlData[span.PayloadStart:span.PayloadEnd])
	if err != nil {
		return nil, err
	}
	if got := hex.EncodeToString(sum[:]); got != ref.SHA256 {
		return nil, fmt.Errorf("media: image digest mismatch: reference %s, source %s", shortDigest(ref.SHA256), shortDigest(got))
	}
	if int64(len(data)) > MaxImageBytes {
		return nil, fmt.Errorf("media: image %s exceeds the %d-byte limit", shortDigest(ref.SHA256), MaxImageBytes)
	}
	if ref.Size > 0 && int64(len(data)) != ref.Size {
		return nil, fmt.Errorf("media: image size mismatch: reference %d bytes, source %d bytes", ref.Size, len(data))
	}
	return data, nil
}

// loadHTML reads and indexes the source once. The source bytes stay resident
// because a payload is returned by offset; the index itself holds only the
// spans, never the image bytes. Concurrent Resolve calls share both after the
// first call completes.
func (r *resolver) loadHTML(ctx context.Context, src Source) error {
	r.once.Do(func() {
		data, err := src.Bytes(ctx)
		if err != nil {
			r.err = err
			return
		}
		// Scan the same UTF-8 form the parser walked; the raw bytes of a
		// UTF-16 document would not contain the ASCII data-URI marker.
		data = parser.DecodeHTMLSource(data)
		r.htmlData = data
		r.htmlSpans = make(map[string]parser.HTMLMediaSpan)
		for _, span := range parser.ScanHTMLMediaSpans(data) {
			r.htmlSpans[span.Ref.SHA256] = span
		}
	})
	return r.err
}

// Materialize fills m["image"] from m["media_ref"] when the item carries a
// reference and no inline bytes. The item map is the parser/chunker wire
// shape, so both components use the same seam.
func Materialize(ctx context.Context, m map[string]any, resolver Resolver, src Source) error {
	if m == nil || resolver == nil || src == nil {
		return nil
	}
	if img, _ := m["image"].(string); img != "" {
		return nil
	}
	ref := entity.MediaRefFromAny(m["media_ref"])
	if ref == nil {
		return nil
	}
	data, err := resolver.Resolve(ctx, *ref, src)
	if err != nil {
		return err
	}
	m["image"] = EncodeDataURI(ref.MIME, data)
	return nil
}

// EncodeDataURI renders image bytes in the form the chunker expects for
// upload and vision materialization. An empty MIME falls back to
// application/octet-stream; image references always carry a concrete type.
func EncodeDataURI(mime string, data []byte) string {
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func shortDigest(digest string) string {
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12]
}
