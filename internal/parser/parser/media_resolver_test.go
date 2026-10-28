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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// TestMediaResolverMatchesParserAttributeSemantics pins the cases where a
// byte-level scanner used to disagree with the HTML parser: entities in the
// media type, an encoded comma, a semicolon-less numeric reference, and an
// unquoted value ending in "/>".
func TestMediaResolverMatchesParserAttributeSemantics(t *testing.T) {
	cases := map[string]string{
		"entity in header":        `<img src="data:image&#47;png;base64,aGVsbG8=">`,
		"entity comma":            `<img src="data:image/png;base64&#44;aGVsbG8=">`,
		"semicolonless entity":    `<img src="data:image/png;base64,aGVsbG8&#43">`,
		"unquoted trailing slash": `<img src=data:image/png;base64,aGVsbG8/>`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			p := NewHTMLParser()
			result := p.ParseWithResult(context.Background(), "case.html", []byte(input))
			if result.Err != nil {
				t.Fatal(result.Err)
			}
			var ref *entity.MediaRef
			for _, item := range result.JSON {
				m := item.ToMap()
				if m["doc_type_kwd"] != DocTypeImage {
					continue
				}
				ref = entity.MediaRefFromAny(m["media_ref"])
			}
			if ref == nil {
				t.Fatalf("parser emitted no media_ref: %+v", result.JSON)
			}
			data, err := NewMediaResolver().Resolve(context.Background(), *ref, MediaSourceFunc(func(context.Context) ([]byte, error) {
				return []byte(input), nil
			}))
			if err != nil {
				t.Fatalf("resolver lost the parser's image: %v", err)
			}
			sum := sha256.Sum256(data)
			if got := hex.EncodeToString(sum[:]); got != ref.SHA256 {
				t.Fatalf("digest = %s, want %s", got, ref.SHA256)
			}
		})
	}
}

// TestMediaResolverIgnoresScriptDataURIs pins that image-like text inside a
// script cannot exhaust the span index and hide a real image.
func TestMediaResolverIgnoresScriptDataURIs(t *testing.T) {
	img := `<img src="data:image/png;base64,aGVsbG8=">`
	input := "<script>" + strings.Repeat(img, 5000) + "</script>" + img

	spans, truncated := scanHTMLMediaSpans(context.Background(), []byte(input))
	if truncated {
		t.Fatal("script text consumed the span cap")
	}
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want the single real image", len(spans))
	}

	p := NewHTMLParser()
	result := p.ParseWithResult(context.Background(), "script.html", []byte(input))
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	var ref *entity.MediaRef
	for _, item := range result.JSON {
		m := item.ToMap()
		if m["doc_type_kwd"] == DocTypeImage {
			ref = entity.MediaRefFromAny(m["media_ref"])
		}
	}
	if ref == nil {
		t.Fatal("parser emitted no media_ref")
	}
	if _, err := NewMediaResolver().Resolve(context.Background(), *ref, MediaSourceFunc(func(context.Context) ([]byte, error) {
		return []byte(input), nil
	})); err != nil {
		t.Fatalf("resolve: %v", err)
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

func mediaRefFromHTML(t *testing.T, input string) *entity.MediaRef {
	t.Helper()
	p := NewHTMLParser()
	result := p.ParseWithResult(context.Background(), "case.html", []byte(input))
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	var ref *entity.MediaRef
	for _, item := range result.JSON {
		m := item.ToMap()
		if m["doc_type_kwd"] == DocTypeImage {
			ref = entity.MediaRefFromAny(m["media_ref"])
		}
	}
	if ref == nil {
		t.Fatalf("parser emitted no media_ref: %+v", result.JSON)
	}
	return ref
}

func resolveMediaInput(t *testing.T, input string, ref entity.MediaRef) []byte {
	t.Helper()
	data, err := NewMediaResolver().Resolve(context.Background(), ref, MediaSourceFunc(func(context.Context) ([]byte, error) {
		return []byte(input), nil
	}))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return data
}

func TestMediaResolverHandlesSelfClosingImg(t *testing.T) {
	input := `<img src="data:image/png;base64,aGVsbG8=" />`
	ref := mediaRefFromHTML(t, input)
	if data := resolveMediaInput(t, input, *ref); string(data) != "hello" {
		t.Fatalf("data = %q, want hello", data)
	}
}

func TestMediaResolverFallsBackToTheParserDOM(t *testing.T) {
	// In the HTML tree builder <svg><title> is foreign content, so the <img>
	// becomes an element. The streaming tokenizer treats <title> as RCDATA and
	// misses it; the DOM fallback must recover the image.
	input := `<svg><title><img src="data:image/png;base64,aGVsbG8="></title></svg>`
	ref := mediaRefFromHTML(t, input)
	if spans, _ := scanHTMLMediaSpans(context.Background(), []byte(input)); len(spans) != 0 {
		t.Fatalf("tokenizer unexpectedly indexed %d span(s)", len(spans))
	}
	if data := resolveMediaInput(t, input, *ref); string(data) != "hello" {
		t.Fatalf("data = %q, want hello", data)
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

func TestMediaResolverHonorsCancellationDuringSourceRead(t *testing.T) {
	html, ref := mediaRefFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	src := MediaSourceFunc(func(context.Context) ([]byte, error) {
		cancel()
		return []byte(html), nil
	})
	if _, err := NewMediaResolver().Resolve(ctx, ref, src); !errors.Is(err, context.Canceled) {
		t.Fatalf("resolve err = %v, want context.Canceled", err)
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
