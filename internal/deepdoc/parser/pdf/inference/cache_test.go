//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package inference

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	doctype "ragflow/internal/deepdoc/parser/type"
)

// ── fake store ───────────────────────────────────────────────────────

// fakeStore is an in-memory implementation of cacheStore used
// by tests. Safe for concurrent use.
type fakeStore struct {
	mu      sync.Mutex
	enabled bool
	data    map[string][]byte
	sets    int32
	gets    int32
}

func newFakeStore() *fakeStore {
	return &fakeStore{enabled: true, data: map[string][]byte{}}
}

func (s *fakeStore) Enabled() bool { return s.enabled }

func (s *fakeStore) GetObj(key string, dest any) bool {
	atomic.AddInt32(&s.gets, 1)
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.data[key]
	if !ok {
		return false
	}
	return json.Unmarshal(raw, dest) == nil
}

func (s *fakeStore) SetObj(key string, value any, _ time.Duration) bool {
	atomic.AddInt32(&s.sets, 1)
	raw, err := json.Marshal(value)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = raw
	return true
}

// ── fake analyser ──────────────────────────────────────────────────

// fakeAnalyzer is a deterministic, count-tracking DocAnalyzer.
// Each method increments a counter and returns a value shaped
// uniquely so a cache hit can be told apart from an inner call.
type fakeAnalyzer struct {
	dlaCount  int32
	tsrCount  int32
	ocrDetCnt int32
	ocrRecCnt int32

	healthy bool

	dlaOut   []doctype.DLARegion
	tsrOut   []doctype.TSRCell
	ocrDet   []doctype.OCRBox
	ocrRec   []doctype.OCRText
	dlaErr   error
	tsrErr   error
	ocrDetEr error
	ocrRecEr error
}

func (f *fakeAnalyzer) DLA(_ context.Context, _ image.Image) ([]doctype.DLARegion, error) {
	atomic.AddInt32(&f.dlaCount, 1)
	if f.dlaErr != nil {
		return nil, f.dlaErr
	}
	return f.dlaOut, nil
}

func (f *fakeAnalyzer) TSR(_ context.Context, _ image.Image) ([]doctype.TSRCell, error) {
	atomic.AddInt32(&f.tsrCount, 1)
	if f.tsrErr != nil {
		return nil, f.tsrErr
	}
	return f.tsrOut, nil
}

func (f *fakeAnalyzer) OCRDetect(_ context.Context, _ image.Image) ([]doctype.OCRBox, error) {
	atomic.AddInt32(&f.ocrDetCnt, 1)
	if f.ocrDetEr != nil {
		return nil, f.ocrDetEr
	}
	return f.ocrDet, nil
}

func (f *fakeAnalyzer) OCRRecognize(_ context.Context, _ image.Image) ([]doctype.OCRText, error) {
	atomic.AddInt32(&f.ocrRecCnt, 1)
	if f.ocrRecEr != nil {
		return nil, f.ocrRecEr
	}
	return f.ocrRec, nil
}

func (f *fakeAnalyzer) Health() bool { return f.healthy }

// ── helpers ────────────────────────────────────────────────────────

// newImage returns a tiny 4x4 RGBA image with stable pixel
// content. JPEG encoding of identical pixel data is
// deterministic, so the cache key for the same newImage() result
// is the same across calls.
func newImage(seed byte) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, color.RGBA{seed, seed * 2, seed * 3, 255})
		}
	}
	return img
}

// ── key tests ──────────────────────────────────────────────────────

func TestCacheKey_Deterministic(t *testing.T) {
	a, errA := cacheKey("dla", newImage(0x10))
	if errA != nil {
		t.Fatalf("cacheKey(dla,#1): %v", errA)
	}
	b, errB := cacheKey("dla", newImage(0x10))
	if errB != nil {
		t.Fatalf("cacheKey(dla,#2): %v", errB)
	}
	if a != b {
		t.Fatalf("same image → different key: %q vs %q", a, b)
	}
	if want := cacheKeyPrefix + "dla:"; len(a) < len(want) || a[:len(want)] != want {
		t.Fatalf("key missing prefix: %q", a)
	}

	c, errC := cacheKey("tsr", newImage(0x10))
	if errC != nil {
		t.Fatalf("cacheKey(tsr): %v", errC)
	}
	if c == a {
		t.Fatal("different methods should yield different keys")
	}

	d, errD := cacheKey("dla", newImage(0x11))
	if errD != nil {
		t.Fatalf("cacheKey(dla,#2): %v", errD)
	}
	if d == a {
		t.Fatal("different image bytes should yield different keys")
	}
}

func TestCacheKey_NilImageErrors(t *testing.T) {
	if _, err := cacheKey("dla", nil); err == nil {
		t.Fatal("want error for nil image, got nil")
	}
}

// ── cache hit / miss / disabled ────────────────────────────────────

func TestDocAnalyzerCache_MissThenHit(t *testing.T) {
	inner := &fakeAnalyzer{
		dlaOut: []doctype.DLARegion{{X0: 1, Y0: 2, X1: 3, Y1: 4, Label: "text"}},
	}
	s := newFakeStore()
	c := newDocAnalyzerCacheWithStore(inner, s, DefaultCacheTTL)

	img := newImage(0x20)

	out1, err := c.DLA(context.Background(), img)
	if err != nil {
		t.Fatalf("DLA #1: %v", err)
	}
	if got := atomic.LoadInt32(&inner.dlaCount); got != 1 {
		t.Fatalf("inner called %d times after first DLA, want 1", got)
	}
	if got := atomic.LoadInt32(&s.sets); got != 1 {
		t.Fatalf("store.SetObj called %d times after first DLA, want 1", got)
	}

	out2, err := c.DLA(context.Background(), img)
	if err != nil {
		t.Fatalf("DLA #2: %v", err)
	}
	if got := atomic.LoadInt32(&inner.dlaCount); got != 1 {
		t.Fatalf("inner called %d times after hit, want 1 (cached)", got)
	}
	if got := atomic.LoadInt32(&s.gets); got < 1 {
		t.Fatalf("store.GetObj not called, want ≥1")
	}

	if len(out1) != len(out2) || out1[0].Label != out2[0].Label {
		t.Fatalf("cached result diverged from inner: %v vs %v", out1, out2)
	}
}

func TestDocAnalyzerCache_StoreDisabledPassthrough(t *testing.T) {
	inner := &fakeAnalyzer{
		dlaOut: []doctype.DLARegion{{Label: "text"}},
	}
	s := newFakeStore()
	s.enabled = false
	c := newDocAnalyzerCacheWithStore(inner, s, DefaultCacheTTL)

	img := newImage(0x30)
	for i := 0; i < 3; i++ {
		if _, err := c.DLA(context.Background(), img); err != nil {
			t.Fatalf("DLA #%d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&inner.dlaCount); got != 3 {
		t.Fatalf("inner called %d times, want 3 (no caching when disabled)", got)
	}
	if got := atomic.LoadInt32(&s.sets); got != 0 {
		t.Fatalf("store.SetObj called %d times, want 0", got)
	}
}

func TestDocAnalyzerCache_NilImageBypassesCache(t *testing.T) {
	inner := &fakeAnalyzer{
		dlaOut: []doctype.DLARegion{{Label: "noop"}},
	}
	s := newFakeStore()
	c := newDocAnalyzerCacheWithStore(inner, s, DefaultCacheTTL)

	for i := 0; i < 2; i++ {
		if _, err := c.DLA(context.Background(), nil); err != nil {
			t.Fatalf("DLA nil #%d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&s.sets); got != 0 {
		t.Fatalf("nil image should bypass store.SetObj, got count %d", got)
	}
	if got := atomic.LoadInt32(&inner.dlaCount); got != 2 {
		t.Fatalf("inner called %d times, want 2 (nil → no cache)", got)
	}
}

// ── all 4 methods ───────────────────────────────────────────────────

func TestDocAnalyzerCache_AllMethods(t *testing.T) {
	inner := &fakeAnalyzer{
		dlaOut: []doctype.DLARegion{{Label: "title"}},
		tsrOut: []doctype.TSRCell{{X0: 0, Y0: 0, X1: 1, Y1: 1, Text: "A"}},
		ocrDet: []doctype.OCRBox{{X0: 0, Y0: 0, X1: 10, Y1: 10}},
		ocrRec: []doctype.OCRText{{Text: "x", Confidence: 0.9}},
	}
	s := newFakeStore()
	c := newDocAnalyzerCacheWithStore(inner, s, DefaultCacheTTL)

	img := newImage(0x40)

	if _, err := c.DLA(context.Background(), img); err != nil {
		t.Fatalf("DLA: %v", err)
	}
	if _, err := c.TSR(context.Background(), img); err != nil {
		t.Fatalf("TSR: %v", err)
	}
	if _, err := c.OCRDetect(context.Background(), img); err != nil {
		t.Fatalf("OCRDetect: %v", err)
	}
	if _, err := c.OCRRecognize(context.Background(), img); err != nil {
		t.Fatalf("OCRRecognize: %v", err)
	}

	if got := atomic.LoadInt32(&s.sets); got != 4 {
		t.Fatalf("store.SetObj called %d times, want 4", got)
	}

	if _, err := c.DLA(context.Background(), img); err != nil {
		t.Fatalf("DLA #2: %v", err)
	}
	if _, err := c.TSR(context.Background(), img); err != nil {
		t.Fatalf("TSR #2: %v", err)
	}
	if _, err := c.OCRDetect(context.Background(), img); err != nil {
		t.Fatalf("OCRDetect #2: %v", err)
	}
	if _, err := c.OCRRecognize(context.Background(), img); err != nil {
		t.Fatalf("OCRRecognize #2: %v", err)
	}

	if got := atomic.LoadInt32(&inner.dlaCount); got != 1 {
		t.Fatalf("DLA inner count = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&inner.tsrCount); got != 1 {
		t.Fatalf("TSR inner count = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&inner.ocrDetCnt); got != 1 {
		t.Fatalf("OCRDetect inner count = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&inner.ocrRecCnt); got != 1 {
		t.Fatalf("OCRRecognize inner count = %d, want 1", got)
	}
}

// ── inner error propagation ─────────────────────────────────────────

func TestDocAnalyzerCache_InnerErrorNotCached(t *testing.T) {
	inner := &fakeAnalyzer{
		dlaErr: errors.New("inference service down"),
	}
	s := newFakeStore()
	c := newDocAnalyzerCacheWithStore(inner, s, DefaultCacheTTL)

	img := newImage(0x50)
	if _, err := c.DLA(context.Background(), img); err == nil {
		t.Fatal("want error, got nil")
	}
	if got := atomic.LoadInt32(&s.sets); got != 0 {
		t.Fatalf("error result must not be cached, got sets=%d", got)
	}
}

// ── Health passthrough ─────────────────────────────────────────────

func TestDocAnalyzerCache_HealthPassthrough(t *testing.T) {
	for _, healthy := range []bool{true, false} {
		inner := &fakeAnalyzer{healthy: healthy}
		s := newFakeStore()
		c := newDocAnalyzerCacheWithStore(inner, s, DefaultCacheTTL)
		if got := c.Health(); got != healthy {
			t.Fatalf("Health() = %v, want %v", got, healthy)
		}
	}
}

// ── different images → different keys ───────────────────────────────

func TestDocAnalyzerCache_DifferentImagesBypassCache(t *testing.T) {
	inner := &fakeAnalyzer{
		dlaOut: []doctype.DLARegion{{Label: "a"}},
	}
	s := newFakeStore()
	c := newDocAnalyzerCacheWithStore(inner, s, DefaultCacheTTL)

	if _, err := c.DLA(context.Background(), newImage(0x80)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DLA(context.Background(), newImage(0x81)); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&inner.dlaCount); got != 2 {
		t.Fatalf("inner called %d times for 2 different images, want 2", got)
	}
}
