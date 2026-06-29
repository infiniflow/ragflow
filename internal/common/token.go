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

package common

// EstimateTokens provides a rough token estimate for a given text string.
// This uses a simple heuristic: approximately 4 characters per token for CJK
// text and ~4 characters per token for Latin text (matching tiktoken's cl100k_base
// average). For precise billing, providers that report usage should be preferred.
//
// This function is used by AudioService to record approximate token consumption
// for TTS/ASR operations when the underlying model does not report exact counts.
func EstimateTokens(text string) int64 {
	if text == "" {
		return 0
	}
	// Simple heuristic: count runes and divide by average chars-per-token
	runes := []rune(text)
	return int64((len(runes) + 3) / 4)
}
