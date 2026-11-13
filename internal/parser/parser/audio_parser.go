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

// AudioParser validates and stores configuration for audio files.
// The actual speech-to-text transcription is performed by the
// component-layer maybeDispatchAudio, which mirrors Python's
// Parser._audio() in rag/flow/parser/parser.py.

package parser

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// audioExtensions mirrors Python's ParserParam.Defaults() audio suffix
// list: ["da","wave","wav","mp3","aac","flac","ogg","aiff","au","midi",
// "wma","realaudio","vqf","oggvorbis","ape"].
var audioExtensions = map[string]bool{
	"da": true, "wave": true, "wav": true, "mp3": true,
	"aac": true, "flac": true, "ogg": true, "aiff": true,
	"au": true, "midi": true, "wma": true, "realaudio": true,
	"vqf": true, "oggvorbis": true, "ape": true,
}

// AudioParser handles audio files for transcription.
type AudioParser struct{}

// NewAudioParser constructs an AudioParser.
func NewAudioParser() *AudioParser {
	return &AudioParser{}
}

// ParseWithResult implements ParseResultProducer. It validates the
// file extension against the audio extension whitelist. The actual
// speech-to-text transcription happens via maybeDispatchAudio at the
// component layer (mirrors Python's LLMBundle.transcription call).
func (p *AudioParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	ext := strings.ToLower(filepath.Ext(filename))
	if len(ext) > 1 && ext[0] == '.' {
		ext = ext[1:]
	}
	if ext == "" || !audioExtensions[ext] {
		return ParseResult{
			Err: fmt.Errorf("audio: unsupported extension %q (filename: %s); accepted: .da/.wav/.wave/.mp3/.aac/.flac/.ogg/.aiff/.au/.midi/.wma/.realaudio/.vqf/.oggvorbis/.ape", ext, filename),
		}
	}

	return ParseResult{
		File: map[string]any{
			"name": filename,
			"size": len(data),
		},
	}
}
