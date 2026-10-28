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
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/media"
)

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
			data, err := media.NewMediaResolver().Resolve(context.Background(), *ref, media.MediaSourceFunc(func(context.Context) ([]byte, error) {
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
	if _, err := media.NewMediaResolver().Resolve(context.Background(), *ref, media.MediaSourceFunc(func(context.Context) ([]byte, error) {
		return []byte(input), nil
	})); err != nil {
		t.Fatalf("resolve: %v", err)
	}
}

// TestMediaResolverFindsImageAfterSpanCap pins the DOM fallback used when
// the capped tokenizer index stops before a real image.
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
	data, err := media.NewMediaResolver().Resolve(context.Background(), ref, media.MediaSourceFunc(func(context.Context) ([]byte, error) {
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
	if data := resolveMediaInput(t, input, *ref); string(data) != "hello" {
		t.Fatalf("data = %q, want hello", data)
	}
}
