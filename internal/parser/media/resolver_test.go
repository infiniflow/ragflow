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
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/entity"
)

func mediaRefFixture(t *testing.T) (string, entity.MediaRef) {
	t.Helper()
	html := `<p>before</p><p><img alt="figure" src="data:image/png;base64,aGVsbG8="></p>`
	spans, _ := scanHTMLMediaSpans(context.Background(), []byte(html))
	if len(spans) != 1 {
		t.Fatalf("fixture images = %d, want 1", len(spans))
	}
	return html, spans[0].Ref
}

func TestMediaResolverResolvesHTMLDataURI(t *testing.T) {
	html, ref := mediaRefFixture(t)
	var calls atomic.Int32
	src := MediaSourceFunc(func(context.Context) ([]byte, error) {
		calls.Add(1)
		return []byte(html), nil
	})

	resolver := NewMediaResolver()
	for i := 0; i < 2; i++ {
		data, err := resolver.Resolve(context.Background(), ref, src)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if string(data) != "hello" {
			t.Fatalf("data = %q, want hello", data)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("source reads = %d, want one read per resolver", calls.Load())
	}
}

func TestMediaResolverRejectsUnknownDigest(t *testing.T) {
	html, ref := mediaRefFixture(t)
	ref.SHA256 = strings.Repeat("0", 64)
	_, err := NewMediaResolver().Resolve(context.Background(), ref, MediaSourceFunc(func(context.Context) ([]byte, error) {
		return []byte(html), nil
	}))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestMediaResolverRejectsUnsupportedReference(t *testing.T) {
	html, ref := mediaRefFixture(t)
	src := MediaSourceFunc(func(context.Context) ([]byte, error) { return []byte(html), nil })
	ref.Kind = "zip-entry"
	if _, err := NewMediaResolver().Resolve(context.Background(), ref, src); err == nil {
		t.Fatal("unsupported kind resolved without an error")
	}
}

func TestMediaResolverReportsSourceFailure(t *testing.T) {
	_, ref := mediaRefFixture(t)
	src := MediaSourceFunc(func(context.Context) ([]byte, error) {
		return nil, context.DeadlineExceeded
	})
	if _, err := NewMediaResolver().Resolve(context.Background(), ref, src); err == nil {
		t.Fatal("source failure was ignored")
	}
}

func TestMediaRefDigestIsStable(t *testing.T) {
	_, ref := mediaRefFixture(t)
	sum := sha256.Sum256([]byte("hello"))
	if ref.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("digest = %s", ref.SHA256)
	}
}

func TestMediaResolverRejectsOversizedReference(t *testing.T) {
	_, ref := mediaRefFixture(t)
	ref.Size = maxMediaImageBytes + 1
	_, err := NewMediaResolver().Resolve(context.Background(), ref, MediaSourceFunc(func(context.Context) ([]byte, error) {
		return []byte("<p>no image needed before the size check</p>"), nil
	}))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v, want size rejection", err)
	}
}

func TestMediaResolverRejectsSizeMismatch(t *testing.T) {
	html, ref := mediaRefFixture(t)
	ref.Size = 4
	_, err := NewMediaResolver().Resolve(context.Background(), ref, MediaSourceFunc(func(context.Context) ([]byte, error) {
		return []byte(html), nil
	}))
	if err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("err = %v, want size mismatch", err)
	}
}

// TestMediaResolverFindsImageAfterSpanCap pins the DOM fallback used when
// the capped tokenizer index stops before a real image.
func TestMediaResolverFindsImageAfterSpanCap(t *testing.T) {
	img := `<img src="data:image/png;base64,aGVsbG8=">`
	last := `<img src="data:image/png;base64,aGVsbG8+">`
	input := []byte(strings.Repeat(img, 4096) + last)

	spans, truncated := scanHTMLMediaSpans(context.Background(), input)
	if !truncated {
		t.Fatal("scan did not report truncation")
	}
	if len(spans) != 4096 {
		t.Fatalf("spans = %d, want the cap", len(spans))
	}
	want, ok := htmlDataURIRef([]byte("data:image/png;base64,aGVsbG8+"))
	if !ok {
		t.Fatal("fixture ref was rejected")
	}
	data, err := NewMediaResolver().Resolve(context.Background(), want, MediaSourceFunc(func(context.Context) ([]byte, error) {
		return input, nil
	}))
	if err != nil || string(data) != "hello>" {
		t.Fatalf("resolved = %q, %v", data, err)
	}
}

// TestMediaResolverReportsTruncatedMissingImage pins the error split: a
// truncated scan that still has no match is not ErrMediaNotFound.
func TestMediaResolverReportsTruncatedMissingImage(t *testing.T) {
	img := `<img src="data:image/png;base64,aGVsbG8=">`
	input := strings.Repeat(img, 4097)
	ref, ok := htmlDataURIRef([]byte("data:image/png;base64,aGVsbG8+"))
	if !ok {
		t.Fatal("fixture ref was rejected")
	}
	_, err := NewMediaResolver().Resolve(context.Background(), ref, MediaSourceFunc(func(context.Context) ([]byte, error) {
		return []byte(input), nil
	}))
	if err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("err = %v, want a truncation error", err)
	}
	if errors.Is(err, ErrMediaNotFound) {
		t.Fatal("truncated scan was reported as a confirmed absent image")
	}
}

func TestMediaResolverHonorsCancelledContextOnCacheHit(t *testing.T) {
	html, ref := mediaRefFixture(t)
	src := MediaSourceFunc(func(context.Context) ([]byte, error) { return []byte(html), nil })
	resolver := NewMediaResolver()
	if _, err := resolver.Resolve(context.Background(), ref, src); err != nil {
		t.Fatalf("warm resolve: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.Resolve(ctx, ref, src); !errors.Is(err, context.Canceled) {
		t.Fatalf("cached resolve err = %v, want context.Canceled", err)
	}
}

// TestMediaResolverRetriesIndexAfterCancellation covers the vision path: the
// resolver is shared by every image in a document while each image resolves
// under its own deadline. A deadline that expires during index construction
// must not be cached and replayed to the next image.
func TestMediaResolverRetriesIndexAfterCancellation(t *testing.T) {
	html, ref := mediaRefFixture(t)
	resolver := NewMediaResolver()

	ctx, cancel := context.WithCancel(context.Background())
	src := MediaSourceFunc(func(context.Context) ([]byte, error) {
		cancel()
		return []byte(html), nil
	})
	if _, err := resolver.Resolve(ctx, ref, src); !errors.Is(err, context.Canceled) {
		t.Fatalf("resolve err = %v, want context.Canceled", err)
	}

	data, err := resolver.Resolve(context.Background(), ref, MediaSourceFunc(func(context.Context) ([]byte, error) {
		return []byte(html), nil
	}))
	if err != nil {
		t.Fatalf("retry after cancellation: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("data = %q, want hello", data)
	}
}

// TestMediaResolverCachesSourceFailure pins the other half of the contract:
// an error that does not come from the caller's deadline describes the
// source and stays cached instead of retrying storage for every image.
func TestMediaResolverCachesSourceFailure(t *testing.T) {
	_, ref := mediaRefFixture(t)
	storageDown := errors.New("storage down")
	var calls atomic.Int32
	src := MediaSourceFunc(func(context.Context) ([]byte, error) {
		calls.Add(1)
		return nil, storageDown
	})
	resolver := NewMediaResolver()
	for i := 0; i < 2; i++ {
		if _, err := resolver.Resolve(context.Background(), ref, src); !errors.Is(err, storageDown) {
			t.Fatalf("resolve %d err = %v, want source failure", i, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("source reads = %d, want one read for a terminal failure", calls.Load())
	}
}

// TestMediaResolverRetriesDOMFallbackAfterCancellation pins the same rule for
// the lazily built fallback tree.
func TestMediaResolverRetriesDOMFallbackAfterCancellation(t *testing.T) {
	input := `<svg><title><img src="data:image/png;base64,aGVsbG8="></title></svg>`
	ref, ok := htmlDataURIRef([]byte("data:image/png;base64,aGVsbG8="))
	if !ok {
		t.Fatal("fixture ref was rejected")
	}
	resolver := &mediaResolver{
		indexLoaded: true,
		htmlData:    []byte(input),
		htmlSpans:   map[string]htmlMediaSpan{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := resolver.findMediaPayloadInDOM(ctx, ref.SHA256); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled fallback err = %v, want context.Canceled", err)
	}

	data, sum, found, err := resolver.findMediaPayloadInDOM(context.Background(), ref.SHA256)
	if err != nil || !found {
		t.Fatalf("fallback retry: data=%q found=%v err=%v", data, found, err)
	}
	if string(data) != "hello" {
		t.Fatalf("data = %q, want hello", data)
	}
	if got := hex.EncodeToString(sum[:]); got != ref.SHA256 {
		t.Fatalf("digest = %s, want %s", got, ref.SHA256)
	}
}

// distinctImageDocument builds a document of count images with distinct
// payloads and returns their references in document order.
func distinctImageDocument(count int) ([]byte, []entity.MediaRef) {
	var b strings.Builder
	refs := make([]entity.MediaRef, 0, count)
	for i := 0; i < count; i++ {
		src := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("image-%d", i)))
		ref, ok := htmlDataURIRef([]byte(src))
		if !ok {
			panic("fixture image was rejected")
		}
		b.WriteString(`<img src="`)
		b.WriteString(src)
		b.WriteString(`">`)
		refs = append(refs, ref)
	}
	return []byte(b.String()), refs
}

// BenchmarkMediaResolverDOMFallback measures the fallback that runs when a
// reference is outside the 4096-span tokenizer index. Every lookup walks the
// cached tree from the root and hashes each <img> it passes, so the cost is
// proportional to misses × images.
func BenchmarkMediaResolverDOMFallback(b *testing.B) {
	const indexed = 4096
	const misses = 32
	html, refs := distinctImageDocument(indexed + misses)
	if spans, truncated := scanHTMLMediaSpans(context.Background(), html); !truncated || len(spans) != indexed {
		b.Fatalf("fixture spans = %d truncated=%v, want %d and truncated", len(spans), truncated, indexed)
	}
	missed := refs[indexed:]

	b.Run("cold-index", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			resolver := &mediaResolver{
				indexLoaded: true,
				htmlData:    html,
				htmlSpans:   map[string]htmlMediaSpan{},
			}
			if _, err := resolver.loadDOMImages(context.Background()); err != nil {
				b.Fatal(err)
			}
		}
	})

	for _, count := range []int{1, 8, misses} {
		b.Run(fmt.Sprintf("misses=%d", count), func(b *testing.B) {
			resolver := &mediaResolver{
				indexLoaded: true,
				htmlData:    html,
				htmlSpans:   map[string]htmlMediaSpan{},
			}
			if _, err := resolver.loadDOMImages(context.Background()); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, ref := range missed[:count] {
					if _, _, found, err := resolver.findMediaPayloadInDOM(context.Background(), ref.SHA256); err != nil || !found {
						b.Fatalf("fallback lookup: found=%v err=%v", found, err)
					}
				}
			}
		})
	}
}

func TestScanHTMLMediaSpansHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	spans, _ := scanHTMLMediaSpans(ctx, []byte(`<img src="data:image/png;base64,aGVsbG8=">`))
	if len(spans) != 0 {
		t.Fatalf("cancelled scan returned %d spans", len(spans))
	}
}

func TestScanHTMLMediaSpansHandlesCaseQuotesAndPadding(t *testing.T) {
	input := `<img src='DATA:IMAGE/PNG;BASE64,aGVsbG8'>`
	spans, _ := scanHTMLMediaSpans(context.Background(), []byte(input))
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Ref.MIME != "image/png" {
		t.Errorf("mime = %q, want image/png", spans[0].Ref.MIME)
	}
	data, sum, err := decodeHTMLImagePayload([]byte(input)[spans[0].TagStart:spans[0].TagEnd])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("data = %q, want hello", data)
	}
	if hex.EncodeToString(sum[:]) != spans[0].Ref.SHA256 {
		t.Errorf("scan digest and decode digest differ")
	}
}

// TestHTMLParser_RefModeKeepsResultIndependentOfImageBytes pins the core
// goal: the parse result no longer scales with image payloads.
func TestScanHTMLMediaSpansDecodesCharacterReferences(t *testing.T) {
	// aGVsbG8+ decodes to "hello>"; &#43; is the HTML-escaped '+'.
	input := `<img src="data:image/png;base64,aGVsbG8&#43;">`
	spans, _ := scanHTMLMediaSpans(context.Background(), []byte(input))
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	data, sum, err := decodeHTMLImagePayload([]byte(input)[spans[0].TagStart:spans[0].TagEnd])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(data) != "hello>" {
		t.Fatalf("data = %q, want hello>", data)
	}
	if hex.EncodeToString(sum[:]) != spans[0].Ref.SHA256 {
		t.Fatal("scan and decode digests differ")
	}
}

func TestScanHTMLMediaSpansSkipsHeadersBeyondTheParserLimit(t *testing.T) {
	// The parser rejects headers longer than 128 bytes; the scan must not
	// look past that window for a comma, and must still find the next image.
	input := `<img src="data:image/png;` + strings.Repeat("x", 200) + `">` +
		`<img src="data:image/png;base64,aGVsbG8=">`
	spans, _ := scanHTMLMediaSpans(context.Background(), []byte(input))
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want the valid image only", len(spans))
	}
}
