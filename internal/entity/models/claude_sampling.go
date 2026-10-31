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

package models

import (
	"regexp"
	"strconv"
	"strings"
)

// claudeNoSamplingMarkers name the Claude generations that accept no sampling
// parameter at all.
var claudeNoSamplingMarkers = []string{
	"claude-opus-4-7",
	"claude-opus-4-8",
	"claude-opus-5",
	"claude-sonnet-5",
	"claude-haiku-5",
	"claude-fable",
	"claude-mythos",
}

// claudeVersionRe reads the generation from names such as claude-sonnet-4-5-20250929,
// eu.anthropic.claude-opus-4-1-20250805-v1:0 or claude-fable-5-1. The minor version has at
// most two digits, so a date suffix (claude-sonnet-4-20250514) is not read as one.
var claudeVersionRe = regexp.MustCompile(`claude-(?:opus|sonnet|haiku|fable|mythos)-(\d+)(?:-(\d{1,2}))?(?:\D|$)`)

// claudeLegacyVersionRe reads the older naming, e.g. claude-3-5-sonnet-20241022.
var claudeLegacyVersionRe = regexp.MustCompile(`claude-(\d)(?:-(\d))?-(?:opus|sonnet|haiku)`)

// claudeVersion returns the (major, minor) Claude generation in a model name.
func claudeVersion(modelName string) (major, minor int, ok bool) {
	match := claudeVersionRe.FindStringSubmatch(modelName)
	if match == nil {
		match = claudeLegacyVersionRe.FindStringSubmatch(modelName)
	}
	if match == nil {
		return 0, 0, false
	}
	major, _ = strconv.Atoi(match[1])
	if match[2] != "" {
		minor, _ = strconv.Atoi(match[2])
	}
	return major, minor, true
}

// withClaudeSamplingPolicy returns a copy of cfg without the sampling
// parameters the Claude model would reject, whether it is served by Anthropic
// or through Bedrock:
//   - Opus 4.7+, Sonnet 5, Haiku 5, Fable and Mythos accept no temperature or top_p.
//   - Claude 4.1 and later reject temperature and top_p together (HTTP 400,
//     "temperature and top_p cannot both be specified for this model"), so
//     top_p is dropped and temperature, the knob the chat presets expose, is kept.
//   - Claude 3.x and 4.0 accept the pair. A Claude name whose generation cannot
//     be read counts as recent, since a dropped top_p beats a failing chat.
//
// Other models and a nil cfg come back unchanged, and the caller's cfg is never modified.
func withClaudeSamplingPolicy(modelName string, cfg *ChatConfig) *ChatConfig {
	name := strings.ToLower(modelName)
	if cfg == nil || !strings.Contains(name, "claude") {
		return cfg
	}
	adjusted := *cfg
	for _, marker := range claudeNoSamplingMarkers {
		if strings.Contains(name, marker) {
			adjusted.Temperature = nil
			adjusted.TopP = nil
			return &adjusted
		}
	}
	if major, minor, ok := claudeVersion(name); ok && (major < 4 || (major == 4 && minor < 1)) {
		return cfg
	}
	if adjusted.Temperature != nil {
		adjusted.TopP = nil
	}
	return &adjusted
}
