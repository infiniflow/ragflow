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

// Package audio holds the TTS Synthesizer interface and its
// model-provider-backed implementation. The Python Message
// component's `auto_play` field selects between `gtts` and
// `edge-tts`; neither has a pure-Go high-quality option. The
// production Python TTS layer is HTTP-based (rag/llm/tts_model.py
// dispatches to Fish / Qwen / OpenAI / StepFun / Xinference / etc.).
//
// The interface (Synthesizer) is small: one method that takes text
// + voice hint and returns raw audio bytes (mp3 / pcm / wav
// depending on engine). The production wiring is in
// model_provider_synthesizer.go, which routes through the
// per-tenant model provider service. When no synthesizer has been
// installed the default stub returns ErrTTSEngineNotConfigured.
package audio

import (
	"context"
	"errors"
	"sync"
)

// Engine is the TTS engine identifier. Mirrors the Python
// `auto_play` values: "gtts" / "edge-tts" / empty (no TTS).
type Engine string

const (
	EngineEmpty  Engine = ""
	EngineGTTS   Engine = "gtts"
	EngineEdge   Engine = "edge-tts"
	EngineCustom Engine = "custom"
)

// ErrTTSEngineNotConfigured is returned by the default synthesizer
// when no engine has been registered. Callers detect the deferred
// state via errors.Is(err, ErrTTSEngineNotConfigured).
var ErrTTSEngineNotConfigured = errors.New(
	"audio: TTS engine not configured — install a Synthesizer via SetSynthesizer at boot",
)

// ErrTTSUnsupportedEngine is returned by Synthesize for engine
// identifiers the runtime does not know how to dispatch.
var ErrTTSUnsupportedEngine = errors.New("audio: unsupported TTS engine")

// ErrSynthesizeEmpty is returned when the model-provider dispatcher
// succeeds (no error) but produces an empty TTSResponse — the
// model driver ran but yielded no audio. Distinct from
// ErrTTSEngineNotConfigured (the dispatcher is not installed at
// all) and ErrTTSUnsupportedEngine (the engine id is not handled)
// so callers can surface a "model returned no audio" diagnostic
// separately.
var ErrSynthesizeEmpty = errors.New("audio: TTS model-provider returned empty audio")

// SynthesizeRequest is the input shape for TTS. The Voice field
// is engine-specific (gtts: ignored, edge-tts: voice short-name).
type SynthesizeRequest struct {
	Engine Engine
	Text   string
	Voice  string
	// Lang is the BCP-47 language tag (e.g. "en", "zh-CN"). gtts
	// uses it as the language argument; edge-tts uses it as the
	// default-voice hint when Voice is empty.
	Lang string
}

// SynthesizeResponse carries the synthesized audio bytes plus the
// MIME type so SSE consumers can set Content-Type correctly.
type SynthesizeResponse struct {
	Audio     []byte
	MediaType string // "audio/mpeg" (gtts / edge-tts / most HTTP providers)
}

// Synthesizer is the abstract TTS interface. The default
// implementation is a no-op stub that returns
// ErrTTSEngineNotConfigured. Production wiring replaces it via
// SetSynthesizer.
type Synthesizer interface {
	Synthesize(ctx context.Context, req SynthesizeRequest) (*SynthesizeResponse, error)
}

var (
	synthMu   sync.RWMutex
	synthImpl Synthesizer = stubSynthesizer{}
)

// SetSynthesizer installs a custom synthesizer. Passing nil
// reverts to the default stub.
func SetSynthesizer(s Synthesizer) {
	synthMu.Lock()
	defer synthMu.Unlock()
	if s == nil {
		synthImpl = stubSynthesizer{}
		return
	}
	synthImpl = s
}

// GetSynthesizer returns the registered synthesizer.
func GetSynthesizer() Synthesizer {
	synthMu.RLock()
	defer synthMu.RUnlock()
	return synthImpl
}

// stubSynthesizer is the default no-op implementation. It returns
// ErrTTSEngineNotConfigured so callers can detect the deferred
// state. Once SetSynthesizer is called with a real impl, the call
// routes through.
type stubSynthesizer struct{}

func (stubSynthesizer) Synthesize(_ context.Context, req SynthesizeRequest) (*SynthesizeResponse, error) {
	if req.Engine == EngineEmpty {
		return nil, ErrTTSEngineNotConfigured
	}
	return nil, ErrTTSUnsupportedEngine
}
