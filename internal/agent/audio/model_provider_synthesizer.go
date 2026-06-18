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

// model_provider_synthesizer.go — TTS driver backed by the model
// provider service.
//
// The Python side routes TTS through `rag.llm.tts_model` factories
// that dispatch to Fish / OpenAI / StepFun / Xinference / LiteLLM
// proxies — all HTTP-based. The Go side has 60+ model drivers in
// `internal/entity/models/`, each with an `AudioSpeech` impl
// (per `types.go:32-33` BaseModel interface), registered via the
// per-tenant model provider service (`internal/service/model_service.go`).
//
// This file wires the audio.Synthesizer interface to that
// provider service via a callback that `cmd/server_main.go` plugs
// in at boot. The audio package stays decoupled from internal/service
// to avoid the import cycle.
//
// Cache layer mirrors `rag/utils/tts_cache.py:synthesize_with_cache`:
// SHA-256 of (text + voice + lang) under `tts:cache:<tenant>:<digest>`,
// TTL 7 days (env override `RAGFLOW_TTS_CACHE_TTL_SECONDS`).

package audio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/engine/redis"
)

// ModelProviderFunc is the contract the audio package uses to
// dispatch a TTS request to the project's model provider service.
// The callback receives the tenant id (resolved from canvas state)
// and the model identifier (the request Engine field, treated as
// a model hint), and returns the synthesized audio bytes.
//
// The audio package keeps this as a function type so it does not
// import internal/service directly. cmd/server_main.go wires the
// real implementation at boot.
type ModelProviderFunc func(ctx context.Context, req ModelProviderRequest) (*SynthesizeResponse, error)

// ModelProviderRequest is what the audio package hands to the
// model-provider callback. TenantID is resolved from the canvas
// state when empty; ModelName is the model identifier (if empty,
// the tenant's default TTS model is used).
type ModelProviderRequest struct {
	TenantID  string
	ModelName string
	Text      string
	Voice     string
	Lang      string
}

// SetModelProviderSynthesizer installs a real synthesizer backed
// by the project's model provider service. Passing nil reverts to
// the default stub. The audio package keeps only the interface;
// the model-provider callback is plugged in at boot.
//
// Idempotent: safe to call from cmd/server_main.go once at boot.
func SetModelProviderSynthesizer(fn ModelProviderFunc) {
	var s Synthesizer = stubSynthesizer{}
	if fn != nil {
		s = &modelProviderSynthesizer{fn: fn, redis: redis.Get()}
	}
	SetSynthesizer(s)
}

type modelProviderSynthesizer struct {
	fn    ModelProviderFunc
	redis *redis.RedisClient
}

func (m *modelProviderSynthesizer) Synthesize(ctx context.Context, req SynthesizeRequest) (*SynthesizeResponse, error) {
	if m.fn == nil {
		return nil, ErrTTSEngineNotConfigured
	}

	// Resolve tenant from canvas state when not on the request.
	tenantID := ""
	if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
		if uid, ok := state.Sys["user_id"].(string); ok {
			tenantID = uid
		}
	}

	// Try cache first (mirrors synthesize_with_cache in
	// rag/utils/tts_cache.py). Cache failures are non-fatal —
	// log and fall through to the model provider.
	cacheKey := buildTTSCacheKey(tenantID, req)
	if cacheKey != "" && m.redis != nil {
		if cached, _ := m.redis.Get(cacheKey); cached != "" {
			if b, err := hex.DecodeString(cached); err == nil && len(b) > 0 {
				return &SynthesizeResponse{Audio: b, MediaType: "audio/mpeg"}, nil
			}
		}
	}

	// Call model provider. The Engine field is repurposed as a
	// model hint — when non-empty, the wiring layer (cmd/server_main.go)
	// resolves it to a specific TTS model; when empty, the tenant's
	// default TTS model is used.
	resp, err := m.fn(ctx, ModelProviderRequest{
		TenantID:  tenantID,
		ModelName: string(req.Engine),
		Text:      req.Text,
		Voice:     req.Voice,
		Lang:      req.Lang,
	})
	if err != nil {
		return nil, fmt.Errorf("audio: TTS model-provider: %w", err)
	}
	if resp == nil || len(resp.Audio) == 0 {
		return nil, ErrSynthesizeEmpty
	}

	// Store in cache. TTL defaults to 7 days; env override via
	// RAGFLOW_TTS_CACHE_TTL_SECONDS (positive integer seconds;
	// 0 or invalid → default).
	if cacheKey != "" && m.redis != nil {
		ttl := ttsCacheTTL()
		if ttl > 0 {
			m.redis.Set(cacheKey, hex.EncodeToString(resp.Audio), ttl)
		}
	}
	return resp, nil
}

// buildTTSCacheKey mirrors `_build_key` in rag/utils/tts_cache.py:
// hash tenant + voice + lang + text, prefix `tts:cache:`.
// Returns empty string when text or tenant is empty (no cache
// for anonymous calls).
func buildTTSCacheKey(tenantID string, req SynthesizeRequest) string {
	if tenantID == "" || req.Text == "" {
		return ""
	}
	h := sha256.New()
	h.Write([]byte(tenantID))
	h.Write([]byte{0})
	h.Write([]byte(req.Voice))
	h.Write([]byte{0})
	h.Write([]byte(req.Lang))
	h.Write([]byte{0})
	h.Write([]byte(req.Text))
	return "tts:cache:" + tenantID + ":" + hex.EncodeToString(h.Sum(nil))
}

// ttsCacheTTL reads RAGFLOW_TTS_CACHE_TTL_SECONDS; default 7 days.
// Matches the Python `_ttl_seconds` default of 7 * 24 * 60 * 60.
func ttsCacheTTL() time.Duration {
	raw := os.Getenv("RAGFLOW_TTS_CACHE_TTL_SECONDS")
	if raw == "" {
		return 7 * 24 * time.Hour
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 7 * 24 * time.Hour
	}
	return time.Duration(n) * time.Second
}
