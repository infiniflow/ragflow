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

// tts_dispatch.go — TTS dispatcher interface for the audio package.
//
// The audio package's ModelProviderFunc contract (see
// model_provider_synthesizer.go) is a function-typed seam that the
// production boot (cmd/server_main.go) plugs in. This file extracts
// the dispatch logic into a small `TTSDispatcher` interface so the
// dispatch can be unit-tested without the audio package depending on
// internal/service. The interface is the minimum surface the audio
// package needs:
//
//   - Synthesize: a single method that the model's audio driver
//     actually exposes (see internal/entity/models/types.go:32-33
//     BaseModel.AudioSpeech); everything else (provider lookup,
//     tenant resolution, fallback model selection) is the model's
//     own internal responsibility.
//
// The audio package does not import internal/service directly; it
// takes a TTSDispatcher (typically the *service.ModelProviderService
// instance installed at boot). The function returns a non-nil
// SynthesizeResponse on success and a non-nil error on every
// failure path; the audio package's caller (modelProviderSynthesizer)
// maps nil-error-with-empty-audio to ErrSynthesizeEmpty and nil-
// error-with-non-empty-audio to a clean pass.

package audio

import (
	"context"
	"fmt"

	"ragflow/internal/common"
	modelModule "ragflow/internal/entity/models"
)

// TTSDispatcher is the minimum interface the audio package needs
// from the project's model provider service. It mirrors the
// *service.ModelProviderService.AudioSpeech method shape so the
// production wiring is a one-line cast. Tests can substitute a
// stub without spinning up a real model driver.
//
// The signature matches the real AudioSpeech exactly (including
// the common.ErrorCode return) so no adapter wrapper is needed
// at the call site. A non-CodeSuccess return is treated as an
// error; the audio package propagates the error to the SSE
// consumer.
type TTSDispatcher interface {
	AudioSpeech(
		providerName, instanceName, modelName, modelID *string,
		userID string,
		audioContent *string,
		apiConfig *modelModule.APIConfig,
		modelConfig *modelModule.TTSConfig,
	) (*modelModule.TTSResponse, common.ErrorCode, error)
}

// NewTTSDispatchFunc returns an audio.ModelProviderFunc that
// dispatches a SynthesizeRequest to the supplied TTSDispatcher.
//
// Field mapping (audio.SynthesizeRequest → model dispatch):
//
//   - ModelProviderRequest.ModelName (from req.Engine)  → modelName
//     The Engine field is repurposed as a model identifier hint
//     in the audio package's contract. Empty falls through to the
//     model's default TTS model.
//   - Text  → audioContent
//   - Voice → TTSConfig.Params["voice"]
//   - Lang  → TTSConfig.Params["lang"]
//
// Error contract: a non-nil error short-circuits the audio
// package's cache (no write) and surfaces to the caller as a
// failed Synthesize. A nil error with nil TTSResponse or empty
// audio is also an error (the audio package treats it as
// "model produced no audio"); we surface that as
// ErrSynthesizeEmpty so the failure is observable in logs.
func NewTTSDispatchFunc(d TTSDispatcher) ModelProviderFunc {
	if d == nil {
		return nil
	}
	return func(ctx context.Context, req ModelProviderRequest) (*SynthesizeResponse, error) {
		// ModelName / Engine may both be empty; both are legal —
		// the model dispatcher will fall back to the tenant's
		// default TTS model in that case.
		var modelName *string
		if req.ModelName != "" {
			mn := req.ModelName
			modelName = &mn
		}

		// We don't have a per-request APIConfig; leave nil so
		// the model's default credentials / base URL take effect.
		var apiConfig *modelModule.APIConfig

		// Build a TTSConfig from the request's voice + lang so
		// the model driver can select a voice variant when the
		// provider supports it (e.g. OpenAI's alloy/echo fable,
		// edge-tts' voice short-name).
		ttsConfig := &modelModule.TTSConfig{Params: map[string]any{}}
		if req.Voice != "" {
			ttsConfig.Params["voice"] = req.Voice
		}
		if req.Lang != "" {
			ttsConfig.Params["lang"] = req.Lang
		}
		if len(ttsConfig.Params) == 0 {
			ttsConfig.Params = nil
		}

		text := req.Text
		resp, code, err := d.AudioSpeech(
			nil, // providerName — let the dispatcher resolve by name
			nil, // instanceName — same
			modelName,
			nil, // modelID — look up by (provider, instance, model)
			req.TenantID,
			&text,
			apiConfig,
			ttsConfig,
		)
		if err != nil {
			return nil, fmt.Errorf("audio: TTS model-provider dispatch: %w", err)
		}
		if code != common.CodeSuccess {
			return nil, fmt.Errorf("audio: TTS model-provider dispatch: code=%d", code)
		}
		if resp == nil || len(resp.Audio) == 0 {
			return nil, ErrSynthesizeEmpty
		}
		return &SynthesizeResponse{
			Audio:     resp.Audio,
			MediaType: "audio/mpeg",
		}, nil
	}
}
