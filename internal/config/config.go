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

package config

// AudioConfig holds configuration for audio processing (TTS / ASR).
type AudioConfig struct {
	// MaxUploadBytes is the maximum allowed size (in bytes) for an uploaded
	// audio file on the transcription endpoint.
	MaxUploadBytes int64

	// SupportedAudioExtensions lists file extensions (with leading dot) that are
	// explicitly accepted by the transcription endpoint. Extensions not in this
	// list will be rejected before any content inspection occurs.
	SupportedAudioExtensions []string
}

// audioDefaults returns the built-in defaults when no external configuration is
// provided. These values mirror the Python backend's limits.
func audioDefaults() AudioConfig {
	return AudioConfig{
		MaxUploadBytes: 50 * 1024 * 1024, // 50 MB
		SupportedAudioExtensions: []string{
			".wav", ".mp3", ".m4a", ".aac",
			".flac", ".ogg", ".webm", ".opus", ".wma",
		},
	}
}

// current holds the active AudioConfig. It is initialized once at startup and
// never mutated afterward, so it is safe for concurrent reads without a mutex.
var current = audioDefaults()

// GetAudioConfig returns the active AudioConfig. Callers must not modify the
// returned value or its slices.
func GetAudioConfig() *AudioConfig {
	return &current
}
