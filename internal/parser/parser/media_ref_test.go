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
	"encoding/base64"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/entity"
)

func mediaRefFixture(t *testing.T) (string, entity.MediaRef) {
	t.Helper()
	html := `<p>before</p><p><img alt="figure" src="data:image/png;base64,aGVsbG8="></p>`
	spans := ScanHTMLMediaSpans([]byte(html))
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

func TestEncodeMediaDataURIRoundTrip(t *testing.T) {
	data := []byte("hello world")
	uri := encodeMediaDataURI("image/png", data)
	if !strings.HasPrefix(uri, "data:image/png;base64,") {
		t.Fatalf("uri = %q", uri)
	}
	const prefix = "data:image/png;base64,"
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, prefix))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(decoded) != string(data) {
		t.Fatalf("decoded = %q, want %q", decoded, data)
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

func TestMaterializeMediaRejectsInvalidMediaRef(t *testing.T) {
	m := map[string]any{"media_ref": map[string]any{"format": "html"}}
	src := MediaSourceFunc(func(context.Context) ([]byte, error) { return nil, nil })
	if err := MaterializeMedia(context.Background(), m, NewMediaResolver(), src); err == nil {
		t.Fatal("a present but unusable media_ref was treated as absent")
	}
}

func TestMaterializeMediaTreatsAbsentMediaRefAsNoop(t *testing.T) {
	var nilRef *entity.MediaRef
	for _, value := range []any{nil, "", map[string]any{}, nilRef} {
		m := map[string]any{"media_ref": value}
		src := MediaSourceFunc(func(context.Context) ([]byte, error) { return nil, nil })
		if err := MaterializeMedia(context.Background(), m, NewMediaResolver(), src); err != nil {
			t.Errorf("media_ref %#v: %v", value, err)
		}
	}
}
