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
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func withTestImageTranscoder(t *testing.T, mime string, fn ImageTranscoder) {
	t.Helper()
	prev := RegisterImageTranscoder(mime, fn)
	t.Cleanup(func() {
		RegisterImageTranscoder(mime, prev)
	})
}

func TestImageTranscoderRegistry(t *testing.T) {
	const mime = "image/x-test-vector"
	withTestImageTranscoder(t, mime, nil)
	if _, _, ok, err := TranscodeImage(t.Context(), mime, []byte("raw")); ok || err != nil {
		t.Fatalf("unregistered TranscodeImage = ok:%v err:%v, want false/nil", ok, err)
	}

	withTestImageTranscoder(t, " IMAGE/X-TEST-VECTOR ; charset=utf-8 ", func(_ context.Context, data []byte) (string, []byte, error) {
		return "IMAGE/PNG", append([]byte("PNG:"), data...), nil
	})
	outMIME, outData, ok, err := TranscodeImage(t.Context(), "image/x-test-vector; charset=utf-8", []byte("abc"))
	if err != nil || !ok || outMIME != "image/png" || string(outData) != "PNG:abc" {
		t.Fatalf("TranscodeImage = (%q, %q, %v, %v), want (image/png, PNG:abc, true, nil)", outMIME, outData, ok, err)
	}
}

func TestSinkResolveTranscodesDeduplicatesAndChargesOutputSize(t *testing.T) {
	const mime = "image/x-test-vector"
	transcodeCalls := 0
	withTestImageTranscoder(t, mime, func(_ context.Context, data []byte) (string, []byte, error) {
		transcodeCalls++
		return "image/png", bytes.Repeat([]byte("P"), len(data)*2), nil
	})

	cache := &recordingSinkCache{}
	budget := NewBudgetWithLimits(Limits{
		ImageMaxBytes:   16,
		MaxImagesPerRun: 4,
		MaxBytesPerRun:  32,
	})
	sink := NewSink(WithCache(t.Context(), cache), "docx", budget)

	loadCalls := 0
	load := func() (string, []byte, bool) {
		loadCalls++
		return mime, []byte("12345"), true // 5 bytes -> 10 bytes PNG
	}
	first, err := sink.Resolve("diagram", 5, load)
	if err != nil || first == nil {
		t.Fatalf("first Resolve = (%v, %v)", first, err)
	}
	repeated, err := sink.Resolve("diagram", 5, load)
	if err != nil || repeated == nil || *repeated != *first {
		t.Fatalf("repeated Resolve = (%v, %v)", repeated, err)
	}
	if loadCalls != 1 || transcodeCalls != 1 {
		t.Fatalf("loadCalls=%d transcodeCalls=%d, want 1 each", loadCalls, transcodeCalls)
	}
	if cache.lastMIME != "image/png" || len(cache.lastData) != 10 {
		t.Fatalf("cached (%q, %d bytes), want (image/png, 10 bytes)", cache.lastMIME, len(cache.lastData))
	}

	// An output larger than ImageMaxBytes (9*2=18 > 16) must be rejected by budget.
	oversized, err := sink.Resolve("too-big-after-transcode", UnknownSize, func() (string, []byte, bool) {
		return mime, bytes.Repeat([]byte("a"), 9), true
	})
	if err != nil || oversized != nil {
		t.Fatalf("oversized transcoded output = (%v, %v), want (nil, nil)", oversized, err)
	}
	if len(sink.Warnings()) == 0 {
		t.Fatalf("expected budget warning for oversized transcoded output")
	}
}

func TestSinkResolveSkipsTranscodeForOversizedInput(t *testing.T) {
	const mime = "image/x-test-vector"
	called := false
	withTestImageTranscoder(t, mime, func(_ context.Context, _ []byte) (string, []byte, error) {
		called = true
		return "image/png", []byte("png"), nil
	})

	budget := NewBudgetWithLimits(Limits{
		ImageMaxBytes:   4,
		MaxImagesPerRun: 4,
		MaxBytesPerRun:  32,
	})
	sink := NewSink(WithCache(t.Context(), &recordingSinkCache{}), "html", budget)
	ref, err := sink.Resolve("huge", UnknownSize, func() (string, []byte, bool) {
		return mime, []byte("12345"), true
	})
	if err != nil || ref != nil || called {
		t.Fatalf("oversized input = (%v, %v) called=%v, want nil/nil/false", ref, err, called)
	}
}

func TestSinkResolveDegradesOnTranscodeErrorAndPropagatesCancel(t *testing.T) {
	const mime = "image/x-test-vector"
	withTestImageTranscoder(t, mime, func(_ context.Context, _ []byte) (string, []byte, error) {
		return "", nil, errors.New("broken vector")
	})

	cache := &recordingSinkCache{}
	sink := NewSink(WithCache(t.Context(), cache), "pptx", NewBudget())
	ref, err := sink.Resolve("bad", 3, func() (string, []byte, bool) {
		return mime, []byte("svg"), true
	})
	if err != nil || ref == nil {
		t.Fatalf("degraded Resolve = (%v, %v), want non-nil ref and nil err", ref, err)
	}
	if cache.lastMIME != mime || string(cache.lastData) != "svg" {
		t.Fatalf("cached (%q, %q), want original (%q, svg)", cache.lastMIME, cache.lastData, mime)
	}
	if joined := strings.Join(sink.Warnings(), "\n"); !strings.Contains(joined, "transcode failure") {
		t.Fatalf("warnings = %q, want transcode failure warning", joined)
	}

	ctx, cancel := context.WithCancel(t.Context())
	withTestImageTranscoder(t, mime, func(_ context.Context, _ []byte) (string, []byte, error) {
		cancel()
		return "", nil, context.Canceled
	})
	canceledSink := NewSink(WithCache(ctx, &recordingSinkCache{}), "pptx", NewBudget())
	if _, err := canceledSink.Resolve("cancel", 3, func() (string, []byte, bool) {
		return mime, []byte("svg"), true
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

type recordingSinkCache struct {
	writes   int
	lastMIME string
	lastData []byte
}

func (c *recordingSinkCache) Put(_ context.Context, format, mime string, data []byte) (entity.MediaRef, error) {
	c.writes++
	c.lastMIME = mime
	c.lastData = append([]byte(nil), data...)
	return entity.MediaRef{
		Format: format,
		MIME:   mime,
		Size:   int64(len(data)),
		SHA256: strings.Repeat("b", 64),
		Kind:   entity.MediaKindKV,
		Scope:  "test",
	}, nil
}

func (*recordingSinkCache) Get(context.Context, entity.MediaRef) ([]byte, error) { panic("unused") }
