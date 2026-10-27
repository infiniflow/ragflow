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
	// MediaInline keeps the historical representation: image bytes only.
	MediaInline MediaMode = iota
	// MediaBoth emits the bytes and a media_ref. Rollout default: consumers
	// keep working, and the reference can be validated against real traffic.
	MediaBoth
	// MediaRef emits only the reference. The chunker materializes each image
	// from the source document on demand.
	MediaRef
)

// EmitsBytes reports whether the parse result carries image payloads.
func (m MediaMode) EmitsBytes() bool { return m != MediaRef }

// EmitsRefs reports whether the parser attaches media_ref to image items.
func (m MediaMode) EmitsRefs() bool { return m != MediaInline }

// EnvMediaRefMode names the environment variable that selects the media mode.
const EnvMediaRefMode = "RAGFLOW_MEDIA_REF_MODE"

// MediaModeFromEnv resolves the configured media mode. Unknown and empty
// values fall back to MediaBoth so a typo never silently drops image bytes
// from persisted parse results.
func MediaModeFromEnv() MediaMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvMediaRefMode))) {
	case "inline":
		return MediaInline
	case "ref":
		return MediaRef
	default:
		return MediaBoth
	}
}
