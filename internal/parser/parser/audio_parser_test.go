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

package parser

import (
	"strings"
	"testing"
)

func TestAudioParser_ValidExtension(t *testing.T) {
	ctx := t.Context()
	p := NewAudioParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "text",
		"vlm": map[string]any{
			"llm_id": "whisper-1",
		},
	})

	result := p.ParseWithResult(ctx, "test.mp3", []byte{1, 2, 3, 4})
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.OutputFormat != "text" {
		t.Errorf("output_format = %q, want text", result.OutputFormat)
	}
	if name, ok := result.File["name"].(string); !ok || name != "test.mp3" {
		t.Errorf("name = %q, want test.mp3", name)
	}
}

func TestAudioParser_AllExtensions(t *testing.T) {
	ctx := t.Context()
	for _, ext := range []string{"da", "wave", "wav", "mp3", "aac", "flac", "ogg", "aiff", "au", "midi", "wma", "realaudio", "vqf", "oggvorbis", "ape"} {
		p := NewAudioParser()
		result := p.ParseWithResult(ctx, "audio."+ext, []byte{})
		if result.Err != nil {
			t.Errorf("extension .%s rejected: %v", ext, result.Err)
		}
	}
}

func TestAudioParser_InvalidExtension(t *testing.T) {
	ctx := t.Context()
	p := NewAudioParser()
	result := p.ParseWithResult(ctx, "notaudio.txt", []byte{})
	if result.Err == nil {
		t.Fatal("expected error for .txt extension")
	}
	if !strings.Contains(result.Err.Error(), "unsupported extension") {
		t.Errorf("error message should mention unsupported extension: %v", result.Err)
	}
}

func TestAudioParser_ConfigureVLM(t *testing.T) {
	p := NewAudioParser()
	p.ConfigureFromSetup(map[string]any{
		"vlm": map[string]any{
			"llm_id": "openai-whisper",
		},
	})
	if p.VLMModelID != "openai-whisper" {
		t.Errorf("VLMModelID = %q, want openai-whisper", p.VLMModelID)
	}
}

func TestAudioParser_ConfigureNilSafe(t *testing.T) {
	ctx := t.Context()
	p := NewAudioParser()
	p.ConfigureFromSetup(nil)
	p.ConfigureFromSetup(map[string]any{})
	// Should not panic.
	result := p.ParseWithResult(ctx, "test.wav", []byte{})
	if result.Err != nil {
		t.Errorf("unexpected error: %v", result.Err)
	}
}

func TestAudioParser_DefaultOutputFormat(t *testing.T) {
	ctx := t.Context()
	p := NewAudioParser()
	result := p.ParseWithResult(ctx, "test.flac", []byte{})
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.OutputFormat != "text" {
		t.Errorf("output_format = %q, want text (default)", result.OutputFormat)
	}
}
