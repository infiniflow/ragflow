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

// Media-ref materialization: a parser that emits MediaRef keeps image bytes
// out of its result. Whoever needs an actual image (the chunker's upload pass,
// the vision enhancement path) hands the reference and a MediaSource to a
// MediaResolver, which reads the source once per invocation and returns
// exactly one image.
package parser

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

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
	once      sync.Once
	htmlSpans map[string]HTMLMediaSpan
	htmlData  []byte

	err error
}

func (r *mediaResolver) Resolve(ctx context.Context, ref entity.MediaRef, src MediaSource) ([]byte, error) {
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
	span, ok := r.htmlSpans[ref.SHA256]
	if !ok {
		return nil, fmt.Errorf("%w: image %s not found in html source", ErrMediaNotFound, shortMediaDigest(ref.SHA256))
	}
	data, sum, err := DecodeHTMLImagePayload(r.htmlData[span.PayloadStart:span.PayloadEnd])
	if err != nil {
		return nil, err
	}
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

// loadHTML reads and indexes the source once. The source bytes stay resident
// because a payload is returned by offset; the index itself holds only the
// spans, never the image bytes. Concurrent Resolve calls share both after the
// first call completes.
func (r *mediaResolver) loadHTML(ctx context.Context, src MediaSource) error {
	r.once.Do(func() {
		data, err := src.Bytes(ctx)
		if err != nil {
			r.err = err
			return
		}
		// Scan the same UTF-8 form the parser walked; the raw bytes of a
		// UTF-16 document would not contain the ASCII data-URI marker.
		data = DecodeHTMLSource(data)
		r.htmlData = data
		r.htmlSpans = make(map[string]HTMLMediaSpan)
		for _, span := range ScanHTMLMediaSpans(data) {
			r.htmlSpans[span.Ref.SHA256] = span
		}
	})
	return r.err
}

// MaterializeMedia fills m["image"] from m["media_ref"] when the item carries
// a reference and no inline bytes. The item map is the parser/chunker wire
// shape, so every consumer uses the same seam.
func MaterializeMedia(ctx context.Context, m map[string]any, resolver MediaResolver, src MediaSource) error {
	if m == nil || resolver == nil || src == nil {
		return nil
	}
	if img, _ := m["image"].(string); img != "" {
		return nil
	}
	ref := entity.MediaRefFromAny(m["media_ref"])
	if ref == nil {
		if mediaRefPresent(m["media_ref"]) {
			return errors.New("media: media_ref is present but has no usable format/kind/digest")
		}
		return nil
	}
	data, err := resolver.Resolve(ctx, *ref, src)
	if err != nil {
		return err
	}
	m["image"] = encodeMediaDataURI(ref.MIME, data)
	return nil
}

// mediaRefPresent reports whether a media_ref value claims to be a reference.
// A nil pointer, empty string, or empty map is treated as absent; anything
// else that did not decode into a usable MediaRef is a malformed reference.
func mediaRefPresent(v any) bool {
	switch ref := v.(type) {
	case nil:
		return false
	case *entity.MediaRef:
		return ref != nil
	case string:
		return ref != ""
	case map[string]any:
		return len(ref) > 0
	default:
		return true
	}
}

// encodeMediaDataURI renders image bytes in the form the chunker expects for
// upload and vision materialization. An empty MIME falls back to
// application/octet-stream; image references always carry a concrete type.
func encodeMediaDataURI(mime string, data []byte) string {
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func shortMediaDigest(digest string) string {
	if len(digest) <= 12 {
		return digest
	}
	return digest[:12]
}
