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
	"errors"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

// These tests pin admission before loading, memoized omissions, and the
// distinction between resource limits and fatal cache failures.
func TestSinkResolvesSourceOnceAndKeepsReferencesAtLimit(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_MAX_IMAGES_PER_RUN", "1")
	cache := &sinkTestCache{}
	sink := NewSink(WithCache(t.Context(), cache), "docx", NewBudget())
	loads := 0
	load := func() (string, []byte, bool) { loads++; return "image/png", []byte("image"), true }
	first, err := sink.Resolve("first", 5, load)
	if err != nil || first == nil {
		t.Fatalf("first: %v, %v", first, err)
	}
	if ref, err := sink.Resolve("second", 5, load); ref != nil || err != nil {
		t.Fatalf("limit: %v, %v", ref, err)
	}
	repeated, err := sink.Resolve("first", 5, load)
	if err != nil || repeated == nil || *first != *repeated {
		t.Fatalf("repeat: %v, %v", repeated, err)
	}
	if loads != 1 || cache.writes != 1 {
		t.Fatalf("loads=%d writes=%d; want 1 each", loads, cache.writes)
	}
	if len(sink.Warnings()) == 0 {
		t.Fatal("missing limit warning")
	}
}

func TestSinkSkipsOversizedSourceBeforeLoading(t *testing.T) {
	t.Setenv("RAGFLOW_MEDIA_IMAGE_MAX_BYTES", "4")
	sink := NewSink(WithCache(t.Context(), &sinkTestCache{}), "epub", NewBudget())
	loaded := false
	ref, err := sink.Resolve("large", 5, func() (string, []byte, bool) { loaded = true; return "image/png", []byte("large"), true })
	if ref != nil || err != nil || loaded {
		t.Fatalf("oversized: %v %v loaded=%v", ref, err, loaded)
	}
	if len(sink.Warnings()) != 1 {
		t.Fatalf("warnings=%v", sink.Warnings())
	}
}

func TestSinkMemoizesLoadAndCacheLimitOmissions(t *testing.T) {
	for _, cacheLimit := range []bool{false, true} {
		t.Run(map[bool]string{false: "source", true: "cache_limit"}[cacheLimit], func(t *testing.T) {
			cache := &sinkTestCache{}
			if cacheLimit {
				cache.err = ErrMediaLimitExceeded
			}
			sink := NewSink(WithCache(t.Context(), cache), "html", NewBudget())
			loads := 0
			for range 2 {
				ref, err := sink.Resolve("source", 5, func() (string, []byte, bool) { loads++; return "image/png", []byte("image"), cacheLimit })
				if ref != nil || err != nil {
					t.Fatalf("omission: %v %v", ref, err)
				}
			}
			if loads != 1 {
				t.Fatalf("loads=%d; want 1", loads)
			}
			if cacheLimit && len(sink.Warnings()) != 1 {
				t.Fatalf("warnings=%v", sink.Warnings())
			}
		})
	}
}

func TestSinkPropagatesCacheFailureAndMissingCache(t *testing.T) {
	failure := errors.New("cache unavailable")
	load := func() (string, []byte, bool) { return "image/png", []byte("image"), true }
	sink := NewSink(WithCache(t.Context(), &sinkTestCache{err: failure}), "pptx", NewBudget())
	if _, err := sink.Resolve("source", 5, load); !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
	if _, err := sink.Resolve("source", 5, load); !errors.Is(err, failure) {
		t.Fatalf("repeated resolution swallowed the fatal cache failure: %v", err)
	}
	sink = NewSink(t.Context(), "pptx", NewBudget())
	if _, err := sink.Resolve("source", 5, load); err == nil || !strings.Contains(err.Error(), "cache is not configured") {
		t.Fatalf("error=%v", err)
	}
}

type sinkTestCache struct {
	writes int
	err    error
}

func (c *sinkTestCache) Put(_ context.Context, format, mime string, data []byte) (entity.MediaRef, error) {
	c.writes++
	return entity.MediaRef{Format: format, MIME: mime, Size: int64(len(data)), SHA256: strings.Repeat("a", 64), Kind: entity.MediaKindKV, Scope: "test"}, c.err
}
func (*sinkTestCache) Get(context.Context, entity.MediaRef) ([]byte, error) { panic("unused") }
