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
	"context"
	"errors"
	"testing"
)

// TestStubSynth_EmptyEngine: an empty engine returns
// ErrTTSEngineNotConfigured (the deferred-state sentinel).
func TestStubSynth_EmptyEngine(t *testing.T) {
	// Ensure the stub is installed (in case a previous test
	// registered a different one).
	SetSynthesizer(nil)
	synth := GetSynthesizer()
	_, err := synth.Synthesize(context.Background(), SynthesizeRequest{
		Engine: EngineEmpty,
		Text:   "hi",
	})
	if !errors.Is(err, ErrTTSEngineNotConfigured) {
		t.Errorf("got %v, want ErrTTSEngineNotConfigured", err)
	}
}

// TestStubSynth_UnknownEngine: a non-empty unknown engine
// returns ErrTTSUnsupportedEngine.
func TestStubSynth_UnknownEngine(t *testing.T) {
	SetSynthesizer(nil)
	synth := GetSynthesizer()
	_, err := synth.Synthesize(context.Background(), SynthesizeRequest{
		Engine: Engine("unknown-engine"),
		Text:   "hi",
	})
	if !errors.Is(err, ErrTTSUnsupportedEngine) {
		t.Errorf("got %v, want ErrTTSUnsupportedEngine", err)
	}
}

// TestSetSynthesizer_Roundtrip: a custom synthesizer set via
// SetSynthesizer is returned by GetSynthesizer.
func TestSetSynthesizer_Roundtrip(t *testing.T) {
	var called bool
	custom := &fakeSynth{called: &called}
	SetSynthesizer(custom)
	defer SetSynthesizer(nil)
	got := GetSynthesizer()
	if got != custom {
		t.Fatalf("synthesizer not registered")
	}
	resp, err := got.Synthesize(context.Background(), SynthesizeRequest{
		Engine: EngineGTTS,
		Text:   "hi",
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if !called {
		t.Errorf("custom Synthesize not called")
	}
	if string(resp.Audio) != "fake" {
		t.Errorf("audio bytes: got %q, want %q", string(resp.Audio), "fake")
	}
}

// TestSetSynthesizer_NilRevertsToStub: passing a nil
// synthesizer reverts to the default stub. (The legacy
// InstallShellSynthesizer was removed; its invented
// --engine --text --voice --lang protocol didn't match any real
// TTS binary. See tts_design.md.)
func TestSetSynthesizer_NilRevertsToStub(t *testing.T) {
	var called bool
	SetSynthesizer(&fakeSynth{called: &called})
	SetSynthesizer(nil)
	if _, ok := GetSynthesizer().(stubSynthesizer); !ok {
		t.Errorf("expected stub to remain after nil SetSynthesizer")
	}
}

// TestEngineConstants: the Engine constants match the Python
// DSL values.
func TestEngineConstants(t *testing.T) {
	if EngineGTTS != "gtts" {
		t.Errorf("EngineGTTS=%q, want 'gtts'", EngineGTTS)
	}
	if EngineEdge != "edge-tts" {
		t.Errorf("EngineEdge=%q, want 'edge-tts'", EngineEdge)
	}
	if EngineEmpty != "" {
		t.Errorf("EngineEmpty=%q, want ''", EngineEmpty)
	}
}

type fakeSynth struct {
	called *bool
}

func (f *fakeSynth) Synthesize(_ context.Context, _ SynthesizeRequest) (*SynthesizeResponse, error) {
	*f.called = true
	return &SynthesizeResponse{
		Audio:     []byte("fake"),
		MediaType: "audio/mpeg",
	}, nil
}
