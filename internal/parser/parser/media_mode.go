// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"os"
	"strings"
)

// MediaMode selects how a parser represents embedded images.
type MediaMode int

const (
	// MediaModeInline keeps the historical representation: image bytes only.
	MediaModeInline MediaMode = iota
	// MediaModeBoth emits the bytes and a media_ref. Rollout default: consumers
	// keep working, and the reference can be validated against real traffic.
	MediaModeBoth
	// MediaModeRef emits only the reference. The chunker materializes each image
	// from the source document on demand.
	MediaModeRef
)

// EmitsBytes reports whether the parse result carries image payloads.
func (m MediaMode) EmitsBytes() bool { return m != MediaModeRef }

// EmitsRefs reports whether the parser attaches media_ref to image items.
func (m MediaMode) EmitsRefs() bool { return m != MediaModeInline }

// EnvMediaRefMode names the environment variable that selects the media mode.
const EnvMediaRefMode = "RAGFLOW_MEDIA_REF_MODE"

// mediaModeFromEnv resolves the configured media mode. Unknown and empty
// values fall back to MediaModeBoth so a typo never silently drops image bytes
// from persisted parse results.
func mediaModeFromEnv() MediaMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvMediaRefMode))) {
	case "inline":
		return MediaModeInline
	case "ref":
		return MediaModeRef
	default:
		return MediaModeBoth
	}
}
