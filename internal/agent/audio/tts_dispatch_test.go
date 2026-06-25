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

// tts_dispatch_test.go — verifies that NewTTSDispatchFunc translates
// an audio.Synthesize request into the correct
// ModelProviderService.AudioSpeech call shape and surfaces the
// model's audio bytes back to the audio package as a
// SynthesizeResponse.

package audio

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/common"
	modelModule "ragflow/internal/entity/models"
)

// fakeTTSDispatcher records every AudioSpeech invocation and
// returns canned responses. Lives only in the test file.
type fakeTTSDispatcher struct {
	// Canned return values.
	resp *modelModule.TTSResponse
	code common.ErrorCode
	err  error

	// Recorded inputs.
	gotProviderName *string
	gotInstanceName *string
	gotModelName    *string
	gotModelID      *string
	gotUserID       string
	gotAudioContent *string
	gotAPIConfig    *modelModule.APIConfig
	gotTTSConfig    *modelModule.TTSConfig
}

func (f *fakeTTSDispatcher) AudioSpeech(
	providerName, instanceName, modelName, modelID *string,
	userID string,
	audioContent *string,
	apiConfig *modelModule.APIConfig,
	modelConfig *modelModule.TTSConfig,
) (*modelModule.TTSResponse, common.ErrorCode, error) {
	f.gotProviderName = providerName
	f.gotInstanceName = instanceName
	f.gotModelName = modelName
	f.gotModelID = modelID
	f.gotUserID = userID
	f.gotAudioContent = audioContent
	f.gotAPIConfig = apiConfig
	f.gotTTSConfig = modelConfig
	return f.resp, f.code, f.err
}

func TestNewTTSDispatchFunc_HappyPath(t *testing.T) {
	fake := &fakeTTSDispatcher{
		resp: &modelModule.TTSResponse{Audio: []byte("mp3bytes")},
		code: common.CodeSuccess,
	}
	fn := NewTTSDispatchFunc(fake)
	if fn == nil {
		t.Fatal("NewTTSDispatchFunc returned nil for non-nil dispatcher")
	}
	resp, err := fn(context.Background(), ModelProviderRequest{
		TenantID:  "tenant-1",
		ModelName: "tts-fish",
		Text:      "hello world",
		Voice:     "en-US-Aria",
		Lang:      "en-US",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if string(resp.Audio) != "mp3bytes" {
		t.Errorf("Audio = %q, want %q", resp.Audio, "mp3bytes")
	}
	if resp.MediaType != "audio/mpeg" {
		t.Errorf("MediaType = %q, want %q", resp.MediaType, "audio/mpeg")
	}

	// Field-mapping assertions.
	if fake.gotUserID != "tenant-1" {
		t.Errorf("userID = %q, want %q", fake.gotUserID, "tenant-1")
	}
	if fake.gotAudioContent == nil || *fake.gotAudioContent != "hello world" {
		t.Errorf("audioContent = %v, want pointer to %q", fake.gotAudioContent, "hello world")
	}
	if fake.gotModelName == nil || *fake.gotModelName != "tts-fish" {
		t.Errorf("modelName = %v, want pointer to %q", fake.gotModelName, "tts-fish")
	}
	if fake.gotProviderName != nil {
		t.Errorf("providerName = %v, want nil (resolved by name)", fake.gotProviderName)
	}
	if fake.gotInstanceName != nil {
		t.Errorf("instanceName = %v, want nil (resolved by name)", fake.gotInstanceName)
	}
	if fake.gotModelID != nil {
		t.Errorf("modelID = %v, want nil (looked up by name)", fake.gotModelID)
	}
	if fake.gotAPIConfig != nil {
		t.Errorf("apiConfig = %v, want nil (no per-request config)", fake.gotAPIConfig)
	}
	if fake.gotTTSConfig == nil || fake.gotTTSConfig.Params["voice"] != "en-US-Aria" {
		t.Errorf("ttsConfig.Params[voice] = %v, want %q", fake.gotTTSConfig, "en-US-Aria")
	}
	if fake.gotTTSConfig == nil || fake.gotTTSConfig.Params["lang"] != "en-US" {
		t.Errorf("ttsConfig.Params[lang] = %v, want %q", fake.gotTTSConfig, "en-US")
	}
}

func TestNewTTSDispatchFunc_EmptyModelName(t *testing.T) {
	fake := &fakeTTSDispatcher{
		resp: &modelModule.TTSResponse{Audio: []byte("x")},
		code: common.CodeSuccess,
	}
	fn := NewTTSDispatchFunc(fake)
	_, err := fn(context.Background(), ModelProviderRequest{
		TenantID: "t1",
		Text:     "no model hint",
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if fake.gotModelName != nil {
		t.Errorf("modelName = %v, want nil for empty ModelName (let the dispatcher default)", fake.gotModelName)
	}
}

func TestNewTTSDispatchFunc_EmptyVoiceAndLang(t *testing.T) {
	// Voice and Lang empty → TTSConfig.Params should be nil (not
	// an empty map) so the model's default voice/lang take effect.
	fake := &fakeTTSDispatcher{
		resp: &modelModule.TTSResponse{Audio: []byte("x")},
		code: common.CodeSuccess,
	}
	fn := NewTTSDispatchFunc(fake)
	_, err := fn(context.Background(), ModelProviderRequest{TenantID: "t1", Text: "hi"})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if fake.gotTTSConfig == nil {
		t.Fatal("TTSConfig is nil")
	}
	if len(fake.gotTTSConfig.Params) != 0 {
		t.Errorf("TTSConfig.Params = %v, want empty/nil (no voice, no lang)", fake.gotTTSConfig.Params)
	}
}

func TestNewTTSDispatchFunc_DispatcherError(t *testing.T) {
	sentinel := errors.New("dispatch boom")
	fake := &fakeTTSDispatcher{err: sentinel}
	fn := NewTTSDispatchFunc(fake)
	_, err := fn(context.Background(), ModelProviderRequest{TenantID: "t1", Text: "hi"})
	if err == nil {
		t.Fatal("expected error from dispatcher, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want wraps %v", err, sentinel)
	}
}

func TestNewTTSDispatchFunc_NonSuccessCode(t *testing.T) {
	fake := &fakeTTSDispatcher{
		resp: &modelModule.TTSResponse{Audio: []byte("ignored")},
		code: common.CodeNotFound,
	}
	fn := NewTTSDispatchFunc(fake)
	_, err := fn(context.Background(), ModelProviderRequest{TenantID: "t1", Text: "hi"})
	if err == nil {
		t.Fatal("expected error for non-CodeSuccess, got nil")
	}
}

func TestNewTTSDispatchFunc_EmptyAudioFromModel(t *testing.T) {
	// Some buggy model drivers return nil error + nil TTSResponse
	// (or empty audio). The dispatch must surface that as the
	// ErrSynthesizeEmpty sentinel so the audio package's caller
	// can distinguish "model produced no audio" from a transport
	// failure.
	cases := []struct {
		name string
		resp *modelModule.TTSResponse
	}{
		{"nil response", nil},
		{"empty audio", &modelModule.TTSResponse{Audio: nil}},
		{"zero-length audio", &modelModule.TTSResponse{Audio: []byte{}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeTTSDispatcher{
				resp: c.resp,
				code: common.CodeSuccess,
			}
			fn := NewTTSDispatchFunc(fake)
			_, err := fn(context.Background(), ModelProviderRequest{TenantID: "t1", Text: "hi"})
			if !errors.Is(err, ErrSynthesizeEmpty) {
				t.Errorf("err = %v, want ErrSynthesizeEmpty", err)
			}
		})
	}
}

func TestNewTTSDispatchFunc_NilDispatcher(t *testing.T) {
	if fn := NewTTSDispatchFunc(nil); fn != nil {
		t.Errorf("NewTTSDispatchFunc(nil) = %v, want nil", fn)
	}
}
