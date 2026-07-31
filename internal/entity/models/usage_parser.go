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

import "encoding/json"

// extractOpenAIUsage extracts token usage from an OpenAI-compatible
// non-streaming response body. Returns (nil, false) when the response
// carries no usable usage block.
func extractOpenAIUsage(body map[string]any) (*TokenUsage, bool) {
	rawUsage, ok := body["usage"].(map[string]any)
	if !ok {
		return nil, false
	}

	usage := &TokenUsage{}
	usage.PromptTokens = extractToken(rawUsage, "prompt_tokens", "input_tokens")
	usage.CompletionTokens = extractToken(rawUsage, "completion_tokens", "output_tokens")
	usage.TotalTokens = extractToken(rawUsage, "total_tokens")

	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	usage.CacheReadTokens = extractCacheReadTokens(rawUsage)
	usage.CacheWriteTokens = extractCacheWriteTokens(rawUsage)

	return usage, true
}

// extractOpenAIStreamUsage extracts token usage from one OpenAI-compatible
// streaming event. A missing or null usage field is not an error.
func extractOpenAIStreamUsage(event map[string]any) (*TokenUsage, bool) {
	rawUsage, ok := event["usage"].(map[string]any)
	if !ok || rawUsage == nil {
		return nil, false
	}

	usage := &TokenUsage{}
	usage.PromptTokens = extractToken(rawUsage, "prompt_tokens", "input_tokens")
	usage.CompletionTokens = extractToken(rawUsage, "completion_tokens", "output_tokens")
	usage.TotalTokens = extractToken(rawUsage, "total_tokens")

	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	usage.CacheReadTokens = extractCacheReadTokens(rawUsage)
	usage.CacheWriteTokens = extractCacheWriteTokens(rawUsage)

	return usage, true
}

// extractCacheReadTokens extracts cache-hit tokens from the usage block.
// Providers expose this field under different paths:
//   - DeepSeek: usage.prompt_cache_hit_tokens
//   - OpenAI 2024+: usage.prompt_tokens_details.cached_tokens
//   - Anthropic-compat: usage.cache_read_input_tokens
//   - input_tokens_details.cached_tokens
func extractCacheReadTokens(rawUsage map[string]any) int {
	if v := extractToken(rawUsage, "prompt_cache_hit_tokens", "cache_read_input_tokens"); v > 0 {
		return v
	}
	return extractNestedToken(rawUsage,
		[]string{"prompt_tokens_details", "cached_tokens"},
		[]string{"input_tokens_details", "cached_tokens"},
	)
}

// extractCacheWriteTokens extracts cache-write tokens from the usage block.
// Providers expose this field under different paths:
//   - DeepSeek: usage.prompt_cache_miss_tokens
//   - Anthropic-compat: usage.cache_creation_input_tokens
//   - input_tokens_details.cache_write_tokens
func extractCacheWriteTokens(rawUsage map[string]any) int {
	if v := extractToken(rawUsage, "prompt_cache_miss_tokens", "cache_creation_input_tokens"); v > 0 {
		return v
	}
	return extractNestedToken(rawUsage,
		[]string{"prompt_tokens_details", "cache_creation_tokens"},
		[]string{"input_tokens_details", "cache_write_tokens"},
	)
}

// extractNestedToken reads a numeric value at a nested path in the map.
func extractNestedToken(m map[string]any, paths ...[]string) int {
	for _, path := range paths {
		cur := m
		for i, key := range path {
			v, ok := cur[key]
			if !ok {
				break
			}
			if i == len(path)-1 {
				switch val := v.(type) {
				case float64:
					return int(val)
				case int:
					return val
				case int64:
					return int(val)
				case json.Number:
					if n, err := val.Int64(); err == nil {
						return int(n)
					}
				}
			} else {
				next, ok := v.(map[string]any)
				if !ok {
					break
				}
				cur = next
			}
		}
	}
	return 0
}

// extractToken reads a numeric field from a map, trying each key in order.
// Returns 0 when no key is present or the value is not a number.
func extractToken(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch val := v.(type) {
			case float64:
				return int(val)
			case int:
				return val
			case int64:
				return int(val)
			case json.Number:
				if n, err := val.Int64(); err == nil {
					return int(n)
				}
			}
		}
	}
	return 0
}
