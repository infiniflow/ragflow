//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package audio

import (
	"strings"
	"testing"
	"time"
)

// TestBuildTTSCacheKey pins the cache key shape. The Python
// `synthesize_with_cache` uses `tts:cache:<model_id>:<sha256>` —
// the Go side uses `tts:cache:<tenant_id>:<sha256>` because
// CanvasState carries the tenant (== user_id) directly while the
// Python side carries the model. Different but equivalent: both
// namespaces avoid cross-tenant and cross-model cache collisions.
func TestBuildTTSCacheKey(t *testing.T) {
	t.Run("stable for same input", func(t *testing.T) {
		a := buildTTSCacheKey("tenant-1", SynthesizeRequest{
			Text: "hello world", Voice: "alloy", Lang: "en",
		})
		b := buildTTSCacheKey("tenant-1", SynthesizeRequest{
			Text: "hello world", Voice: "alloy", Lang: "en",
		})
		if a != b {
			t.Fatalf("expected stable key, got %q vs %q", a, b)
		}
		if !strings.HasPrefix(a, "tts:cache:tenant-1:") {
			t.Fatalf("unexpected prefix: %q", a)
		}
		if len(a) != len("tts:cache:tenant-1:")+64 {
			t.Fatalf("expected sha256 hex digest, got key length %d", len(a))
		}
	})
	t.Run("different voice gives different key", func(t *testing.T) {
		a := buildTTSCacheKey("t", SynthesizeRequest{Text: "hi", Voice: "alloy"})
		b := buildTTSCacheKey("t", SynthesizeRequest{Text: "hi", Voice: "echo"})
		if a == b {
			t.Fatal("expected different keys for different voices")
		}
	})
	t.Run("different tenant gives different key", func(t *testing.T) {
		a := buildTTSCacheKey("t1", SynthesizeRequest{Text: "hi"})
		b := buildTTSCacheKey("t2", SynthesizeRequest{Text: "hi"})
		if a == b {
			t.Fatal("expected different keys for different tenants")
		}
	})
	t.Run("empty tenant returns empty key", func(t *testing.T) {
		if k := buildTTSCacheKey("", SynthesizeRequest{Text: "hi"}); k != "" {
			t.Fatalf("expected empty key for empty tenant, got %q", k)
		}
	})
	t.Run("empty text returns empty key", func(t *testing.T) {
		if k := buildTTSCacheKey("t", SynthesizeRequest{}); k != "" {
			t.Fatalf("expected empty key for empty text, got %q", k)
		}
	})
}

// TestTTSCacheTTL pins the default + env override. Python default
// is 7 * 24 * 60 * 60 = 604800 seconds; Go side mirrors.
func TestTTSCacheTTL(t *testing.T) {
	// Default: 7 days.
	t.Setenv("RAGFLOW_TTS_CACHE_TTL_SECONDS", "")
	if got := ttsCacheTTL(); got != 7*24*time.Hour {
		t.Fatalf("default TTL: got %v, want 7d", got)
	}
	// Env override.
	t.Setenv("RAGFLOW_TTS_CACHE_TTL_SECONDS", "3600")
	if got := ttsCacheTTL(); got != time.Hour {
		t.Fatalf("override TTL: got %v, want 1h", got)
	}
	// Invalid env falls back to default.
	t.Setenv("RAGFLOW_TTS_CACHE_TTL_SECONDS", "not-a-number")
	if got := ttsCacheTTL(); got != 7*24*time.Hour {
		t.Fatalf("invalid env: got %v, want default 7d", got)
	}
	// Zero / negative falls back to default.
	t.Setenv("RAGFLOW_TTS_CACHE_TTL_SECONDS", "0")
	if got := ttsCacheTTL(); got != 7*24*time.Hour {
		t.Fatalf("zero env: got %v, want default 7d", got)
	}
}

// TestSetModelProviderSynthesizer_NilRevertsToStub ensures the
// nil-arg path reverts to the default stub (which returns
// ErrTTSEngineNotConfigured for empty engine).
func TestSetModelProviderSynthesizer_NilRevertsToStub(t *testing.T) {
	SetModelProviderSynthesizer(nil)
	defer SetSynthesizer(stubSynthesizer{})
	synth := GetSynthesizer()
	if _, ok := synth.(stubSynthesizer); !ok {
		t.Fatalf("expected stubSynthesizer, got %T", synth)
	}
}
