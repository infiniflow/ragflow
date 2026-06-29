//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.


package entity

import "context"

// ModelType represents the type of model
type ModelType string

const (
	// ModelTypeChat chat model
	ModelTypeChat ModelType = "chat"
	// ModelTypeEmbedding embedding model
	ModelTypeEmbedding ModelType = "embedding"
	// ModelTypeSpeech2Text speech to text model
	ModelTypeSpeech2Text ModelType = "speech2text"
	// ModelTypeImage2Text image to text model
	ModelTypeImage2Text ModelType = "image2text"
	// ModelTypeRerank rerank model
	ModelTypeRerank ModelType = "rerank"
	// ModelTypeTTS text to speech model
	ModelTypeTTS ModelType = "tts"
	// ModelTypeOCR optical character recognition model
	ModelTypeOCR ModelType = "ocr"
)

// TTSModel interface for text-to-speech models.
//
// The TTS method returns the number of tokens/units consumed by the call so
// callers can record usage (mirroring Python's LLMBundle.tts which treats an
// int sentinel yielded by the model as a usage charge). OpenAI-compatible
// providers don't report token counts and therefore return 0 — matching
// Python, which only charges when the underlying model yields an int sentinel.
type TTSModel interface {
	// TTS generates speech audio bytes from text, streaming via the sender
	// callback. ctx supports cancellation / deadline propagation to the
	// underlying HTTP client. It returns the number of chargeable units
	// consumed (0 if the provider doesn't report usage).
	TTS(ctx context.Context, text string, sender func(chunk []byte) error) (usedTokens int64, err error)
}

// TranscriptionEvent is one element of a streaming transcription, mirroring
// Python's stream_transcription generator which yields dicts like
// {"event": "delta"|"final"|"error", "text": "...", "streaming": bool}.
type TranscriptionEvent struct {
	Event     string `json:"event"`
	Text      string `json:"text"`
	Streaming bool   `json:"streaming"`
}

// Speech2TextModel interface for speech-to-text (ASR) models
type Speech2TextModel interface {
	// Transcription transcribes audio file at the given path to text
	Transcription(audioPath string) (string, error)
	// StreamTranscription streams transcription events. For OpenAI-compatible
	// providers (Whisper) the underlying API is non-streaming, so this yields a
	// single "final" event (matching Python's LLMBundle.stream_transcription
	// fallback). Providers with native streaming override this.
	StreamTranscription(audioPath string, sender func(event TranscriptionEvent) error) error
}
