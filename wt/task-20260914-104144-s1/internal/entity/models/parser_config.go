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

// ParserConfig maps a protocol to its usage and reasoning parsers. Drivers
// select a ParserConfig instead of implementing extraction individually.
type ParserConfig struct {
	// Protocol is the protocol identifier (e.g. "openai", "claude").
	Protocol string
	// ResponseParser extracts usage from a non-streaming response body.
	ResponseParser func(map[string]any) (*TokenUsage, bool)
	// StreamParser extracts usage from one streaming event.
	StreamParser func(map[string]any) (*TokenUsage, bool)
	// ExtractStreamReasoning extracts the reasoning text from a parsed
	// delta map (the delta field of one streaming event), if any.
	// Defaults to reading delta.reasoning_content.
	ExtractStreamReasoning func(delta map[string]any) string
}

// extractDefaultStreamReasoning reads the reasoning text from a parsed
// delta (delta.reasoning_content).
func extractDefaultStreamReasoning(delta map[string]any) string {
	if r, ok := delta["reasoning_content"].(string); ok {
		return r
	}
	return ""
}

// OpenAIParserConfig is the ParserConfig for OpenAI-compatible APIs
// (NVIDIA NIM, DeepSeek, Aliyun, Moonshot, xAI, ...).
var OpenAIParserConfig = &ParserConfig{
	Protocol:               "openai",
	ResponseParser:         extractOpenAIUsage,
	StreamParser:           extractOpenAIStreamUsage,
	ExtractStreamReasoning: extractDefaultStreamReasoning,
}

// extractOpenRouterStreamReasoning reads the reasoning text from an
// OpenRouter streaming event. OpenRouter uses delta.reasoning (not
// delta.reasoning_content) for its reasoning content.
func extractOpenRouterStreamReasoning(delta map[string]any) string {
	if r, ok := delta["reasoning"].(string); ok {
		return r
	}
	return ""
}

// OpenRouterParserConfig is the ParserConfig for OpenRouter, which emits
// reasoning under delta.reasoning instead of delta.reasoning_content.
var OpenRouterParserConfig = &ParserConfig{
	Protocol:               "openai",
	ResponseParser:         extractOpenAIUsage,
	StreamParser:           extractOpenAIStreamUsage,
	ExtractStreamReasoning: extractOpenRouterStreamReasoning,
}
