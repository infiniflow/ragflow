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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NovitaModel implements ModelDriver for Novita.ai
// (https://novita.ai/docs/api-reference/).
//
// Novita exposes an OpenAI-compatible REST API at
// https://api.novita.ai/v3/openai (chat completions at
// /chat/completions, list models at /models). It serves a large
// catalog of third-party models (DeepSeek, Llama, Qwen3, Kimi,
// Gemma, Mistral, etc.) behind a single OpenAI-shaped surface.
//
// The wire shape matches OpenAI standard with ONE notable
// difference: reasoning models like qwen3-* embed their
// chain-of-thought INLINE inside content as <think>...</think>
// tags, rather than in a separate reasoning_content field. The
// driver detects those tags and routes the inner text to
// ChatResponse.ReasonContent (non-stream) or the sender's second
// arg (stream), keeping the answer clean of tag clutter.
type NovitaModel struct {
	BaseURL    map[string]string
	URLSuffix  URLSuffix
	httpClient *http.Client
}

// NewNovitaModel creates a new Novita model instance.
//
// Same transport convention as other Go drivers in this package:
// clone http.DefaultTransport, override the connection-pool fields,
// no client-level Timeout so SSE streams are not capped.
func NewNovitaModel(baseURL map[string]string, urlSuffix URLSuffix) *NovitaModel {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.DisableCompression = false
	transport.ResponseHeaderTimeout = 60 * time.Second

	return &NovitaModel{
		BaseURL:   baseURL,
		URLSuffix: urlSuffix,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (n *NovitaModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewNovitaModel(baseURL, n.URLSuffix)
}

func (n *NovitaModel) Name() string {
	return "novita"
}

func (n *NovitaModel) baseURLForRegion(region string) (string, error) {
	base, ok := n.BaseURL[region]
	if !ok || base == "" {
		return "", fmt.Errorf("novita: no base URL configured for region %q", region)
	}
	// Strip a trailing "/" so callers can safely do
	// fmt.Sprintf("%s/%s", base, suffix) without producing "//" in
	// the path. The shipped config has no trailing slash, but a
	// tenant can override the URL per-instance and may add one.
	return strings.TrimSuffix(base, "/"), nil
}

const (
	novitaThinkOpen  = "<think>"
	novitaThinkClose = "</think>"
)

// splitNovitaThink walks a complete content string and returns the
// visible portion + the concatenated chain-of-thought from inside
// any <think>...</think> blocks. Multiple think blocks are
// concatenated; tags themselves are stripped. Used by the
// non-streaming path where the whole content is available at once.
func splitNovitaThink(raw string) (visible, reasoning string) {
	var v, r strings.Builder
	inside := false
	for {
		var marker string
		if inside {
			marker = novitaThinkClose
		} else {
			marker = novitaThinkOpen
		}
		idx := strings.Index(raw, marker)
		if idx < 0 {
			if inside {
				r.WriteString(raw)
			} else {
				v.WriteString(raw)
			}
			break
		}
		if inside {
			r.WriteString(raw[:idx])
		} else {
			v.WriteString(raw[:idx])
		}
		raw = raw[idx+len(marker):]
		inside = !inside
	}
	return v.String(), r.String()
}

// novitaThinkExtractor maintains state across streaming chunks so
// that a <think>...</think> block spanning multiple SSE events still
// gets split correctly between content and reasoning. The buffer
// preserves up to (len(closingMarker)-1) trailing bytes of each
// chunk in case the next chunk completes a partial tag.
type novitaThinkExtractor struct {
	buf    strings.Builder
	inside bool
}

// novitaThinkSegment is one routing decision: emit `content` via the
// sender's first arg, or emit `reasoning` via the sender's second arg.
// Exactly one of the two fields is non-empty.
type novitaThinkSegment struct {
	content   string
	reasoning string
}

// Feed appends an incoming chunk and returns any segments that are
// now safe to emit. Trailing bytes that could be the start of a tag
// are held back in the buffer until the next call.
func (e *novitaThinkExtractor) Feed(chunk string) []novitaThinkSegment {
	e.buf.WriteString(chunk)
	s := e.buf.String()
	var out []novitaThinkSegment
	for {
		var marker, otherMarker string
		if e.inside {
			marker = novitaThinkClose
			otherMarker = novitaThinkOpen
		} else {
			marker = novitaThinkOpen
			otherMarker = novitaThinkClose
		}
		idx := strings.Index(s, marker)
		if idx < 0 {
			// No closing/opening marker yet. Emit everything except a
			// possible partial-tag suffix at the very end. Reserve
			// (max marker length - 1) trailing bytes so we don't
			// emit "<thin" as content when the next chunk completes
			// it to "<think>".
			reserve := len(marker) - 1
			if len(otherMarker)-1 > reserve {
				reserve = len(otherMarker) - 1
			}
			safe := len(s) - reserve
			if safe < 0 {
				safe = 0
			}
			// Don't reserve if the trailing bytes can't possibly be
			// the start of a tag (no '<' suffix).
			if safe < len(s) && !strings.Contains(s[safe:], "<") {
				safe = len(s)
			}
			if safe > 0 {
				if e.inside {
					out = append(out, novitaThinkSegment{reasoning: s[:safe]})
				} else {
					out = append(out, novitaThinkSegment{content: s[:safe]})
				}
				s = s[safe:]
			}
			break
		}
		if idx > 0 {
			if e.inside {
				out = append(out, novitaThinkSegment{reasoning: s[:idx]})
			} else {
				out = append(out, novitaThinkSegment{content: s[:idx]})
			}
		}
		s = s[idx+len(marker):]
		e.inside = !e.inside
	}
	e.buf.Reset()
	e.buf.WriteString(s)
	return out
}

// Flush returns the buffered tail when the stream ends. A stream that
// ends mid-tag would not normally happen with a well-behaved upstream,
// but if it does the partial bytes are emitted according to the
// current mode so nothing is silently lost.
func (e *novitaThinkExtractor) Flush() *novitaThinkSegment {
	s := e.buf.String()
	e.buf.Reset()
	if s == "" {
		return nil
	}
	if e.inside {
		return &novitaThinkSegment{reasoning: s}
	}
	return &novitaThinkSegment{content: s}
}

// ChatWithMessages sends multiple messages with roles and returns the response.
func (n *NovitaModel) ChatWithMessages(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig) (*ChatResponse, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := n.baseURLForRegion(region)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, n.URLSuffix.Chat)

	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
		"stream":   false,
	}

	if chatModelConfig != nil {
		if chatModelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *chatModelConfig.MaxTokens
		}
		if chatModelConfig.Temperature != nil {
			reqBody["temperature"] = *chatModelConfig.Temperature
		}
		if chatModelConfig.TopP != nil {
			reqBody["top_p"] = *chatModelConfig.TopP
		}
		if chatModelConfig.Stop != nil {
			reqBody["stop"] = *chatModelConfig.Stop
		}
		// Map ChatConfig.Thinking -> Novita's `enable_thinking`.
		// Per https://novita.ai/docs/api-reference/model-apis-llm-create-chat-completion,
		// enable_thinking (boolean | null, default true) "controls the
		// switches between thinking and non-thinking modes" for
		// zai-org/glm-4.5, deepseek/deepseek-v3.1[-terminus|-exp]. For
		// models outside that supported set Novita ignores the field,
		// so it's safe to forward whenever the caller opts in. Tenants
		// can now disable thinking mode at request time without having
		// to use prompt-level hacks like "/no_think".
		if chatModelConfig.Thinking != nil {
			reqBody["enable_thinking"] = *chatModelConfig.Thinking
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	firstChoice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid choice format")
	}

	messageMap, ok := firstChoice["message"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid message format")
	}

	rawContent, ok := messageMap["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid content format")
	}

	// Novita emits chain-of-thought in two different shapes depending
	// on the model and on enable_thinking:
	//   - qwen3-* and other inline-style models: chain-of-thought is
	//     embedded inside content as <think>...</think> tags.
	//   - deepseek-v3.1 / glm-4.5 (and any model with separate
	//     reasoning enabled): chain-of-thought arrives in a separate
	//     `reasoning_content` field, with `content` already cleaned.
	// Handle both so the visible Answer is always tag-free and any
	// reasoning the upstream supplied is preserved.
	visible, reasoning := splitNovitaThink(rawContent)
	if r, ok := messageMap["reasoning_content"].(string); ok && r != "" {
		if reasoning != "" {
			reasoning += "\n" + r
		} else {
			reasoning = r
		}
	}

	return &ChatResponse{
		Answer:        &visible,
		ReasonContent: &reasoning,
	}, nil
}

// ChatStreamlyWithSender sends messages and streams the response via
// the sender. Handles both reasoning shapes Novita can emit:
//   - delta.reasoning_content (deepseek-v3.1 / glm-4.5 / any model
//     with separate reasoning): forwarded as-is to the second arg.
//   - delta.content containing <think>...</think> (qwen3-* and other
//     inline-style models): a stateful extractor splits tag bytes
//     across SSE chunk boundaries, then routes content/reasoning to
//     the first/second sender arg respectively.
func (n *NovitaModel) ChatStreamlyWithSender(modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}
	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return fmt.Errorf("api key is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := n.baseURLForRegion(region)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", baseURL, n.URLSuffix.Chat)

	apiMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		apiMessages[i] = map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
	}

	reqBody := map[string]interface{}{
		"model":    modelName,
		"messages": apiMessages,
		"stream":   true,
	}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
			return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
		}
		if chatModelConfig.MaxTokens != nil {
			reqBody["max_tokens"] = *chatModelConfig.MaxTokens
		}
		if chatModelConfig.Temperature != nil {
			reqBody["temperature"] = *chatModelConfig.Temperature
		}
		if chatModelConfig.TopP != nil {
			reqBody["top_p"] = *chatModelConfig.TopP
		}
		if chatModelConfig.Stop != nil {
			reqBody["stop"] = *chatModelConfig.Stop
		}
		// See ChatWithMessages for why we forward this.
		if chatModelConfig.Thinking != nil {
			reqBody["enable_thinking"] = *chatModelConfig.Thinking
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	extractor := &novitaThinkExtractor{}
	sawTerminal := false
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[5:])
		if data == "[DONE]" {
			sawTerminal = true
			break
		}
		var event map[string]interface{}
		if err = json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		choices, ok := event["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			continue
		}
		firstChoice, ok := choices[0].(map[string]interface{})
		if !ok {
			continue
		}
		delta, ok := firstChoice["delta"].(map[string]interface{})
		if !ok {
			continue
		}
		// deepseek-v3.1 / glm-4.5 (and other models that emit reasoning
		// separately) put chain-of-thought in delta.reasoning_content
		// rather than inside content as <think>...</think>. Surface it
		// before any content from the same chunk so callers piping to
		// a UI render reasoning before the visible answer for that
		// token, matching the wire ordering Novita emits.
		if r, ok := delta["reasoning_content"].(string); ok && r != "" {
			rr := r
			if err := sender(nil, &rr); err != nil {
				return err
			}
		}
		if c, ok := delta["content"].(string); ok && c != "" {
			for _, seg := range extractor.Feed(c) {
				if seg.reasoning != "" {
					r := seg.reasoning
					if err := sender(nil, &r); err != nil {
						return err
					}
				}
				if seg.content != "" {
					cc := seg.content
					if err := sender(&cc, nil); err != nil {
						return err
					}
				}
			}
		}
		if finish, ok := firstChoice["finish_reason"].(string); ok && finish != "" {
			sawTerminal = true
			break
		}
	}

	// Flush any buffered tail (rare, but covers the case where the
	// stream ends right after the last chunk without us seeing the
	// closing tag).
	if seg := extractor.Flush(); seg != nil {
		if seg.reasoning != "" {
			r := seg.reasoning
			if err := sender(nil, &r); err != nil {
				return err
			}
		}
		if seg.content != "" {
			cc := seg.content
			if err := sender(&cc, nil); err != nil {
				return err
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan response body: %w", err)
	}
	if !sawTerminal {
		return fmt.Errorf("novita: stream ended before [DONE] or finish_reason")
	}

	endOfStream := "[DONE]"
	if err := sender(&endOfStream, nil); err != nil {
		return err
	}

	return nil
}

// ListModels returns the list of model ids visible to the API key.
func (n *NovitaModel) ListModels(apiConfig *APIConfig) ([]string, error) {
	if apiConfig == nil || apiConfig.ApiKey == nil || *apiConfig.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}

	region := "default"
	if apiConfig.Region != nil && *apiConfig.Region != "" {
		region = *apiConfig.Region
	}

	baseURL, err := n.baseURLForRegion(region)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", baseURL, n.URLSuffix.Models)

	ctx, cancel := context.WithTimeout(context.Background(), nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	data, ok := result["data"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid models list format")
	}

	models := make([]string, 0)
	for _, model := range data {
		modelMap, ok := model.(map[string]interface{})
		if !ok {
			continue
		}
		modelName, ok := modelMap["id"].(string)
		if !ok {
			continue
		}
		models = append(models, modelName)
	}
	return models, nil
}

// CheckConnection runs a lightweight ListModels call to verify the API key.
func (n *NovitaModel) CheckConnection(apiConfig *APIConfig) error {
	_, err := n.ListModels(apiConfig)
	return err
}

// Embed is not exposed on Novita's OpenAI-compatible surface yet.
func (n *NovitaModel) Embed(modelName *string, texts []string, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig) ([]EmbeddingData, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

// Rerank is not exposed by the Novita API.
func (n *NovitaModel) Rerank(modelName *string, query string, documents []string, apiConfig *APIConfig, rerankConfig *RerankConfig) (*RerankResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

// Balance is not exposed by the Novita API.
func (n *NovitaModel) Balance(apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

func (n *NovitaModel) TranscribeAudio(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

func (n *NovitaModel) TranscribeAudioWithSender(modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", n.Name())
}

func (n *NovitaModel) AudioSpeech(modelName *string, audioContent *string, apiConfig *APIConfig, asrConfig *TTSConfig) (*TTSResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}

func (n *NovitaModel) AudioSpeechWithSender(modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", n.Name())
}

// OCRFile OCR file
func (n *NovitaModel) OCRFile(modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig) (*OCRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", n.Name())
}
