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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"ragflow/internal/common"
)

// AliyunModel implements ModelDriver for Aliyun
type AliyunModel struct {
	baseModel BaseModel
}

// NewAliyunModel creates a new Aliyun model instance
func NewAliyunModel(baseURL map[string]string, urlSuffix URLSuffix) *AliyunModel {
	return &AliyunModel{
		baseModel: BaseModel{
			BaseURL:    baseURL,
			URLSuffix:  urlSuffix,
			httpClient: NewDriverHTTPClient(false),
		},
	}
}

func (a *AliyunModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewAliyunModel(baseURL, a.baseModel.URLSuffix)
}

func (a *AliyunModel) Name() string {
	return "Tongyi-Qianwen"
}

func (a *AliyunModel) ChatWithMessages(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage) (*ChatResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, a.baseModel.URLSuffix.Chat)

	// Build request body
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, false)

	if chatModelConfig != nil {

		if chatModelConfig.Thinking != nil {
			if *chatModelConfig.Thinking {
				reqBody["enable_thinking"] = true
			} else {
				reqBody["enable_thinking"] = false
			}
		}

		if chatModelConfig.Tools != nil {
			reqBody["tool_choice"] = aliyunToolChoice(modelName, messages, chatModelConfig.ToolChoice)
		}
	}

	// For qwen3 models on DashScope, enable_thinking defaults to true when
	// omitted. RAGFlow's default is to disable thinking unless explicitly
	// enabled by the user, matching Python's chat_model.py behavior.
	applyQwen3ThinkingDefault(modelName, reqBody)

	body, err := a.baseModel.doRequest(ctx, url, apiConfig, reqBody, nonStreamCallTimeout)
	if err != nil {
		return nil, err
	}

	return HandleNonStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig)
}

// ChatStreamlyWithSender sends messages and streams response via sender function (best performance, no channel)
func (a *AliyunModel) ChatStreamlyWithSender(ctx context.Context, modelName string, messages []Message, apiConfig *APIConfig, chatModelConfig *ChatConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return err
	}

	if len(messages) == 0 {
		return fmt.Errorf("messages is empty")
	}

	resolvedBaseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/%s", resolvedBaseURL, a.baseModel.URLSuffix.Chat)

	// Build request body with streaming enabled
	reqBody := buildRequestBody(chatModelConfig, modelName, messages, true)
	reqBody["stream_options"] = map[string]interface{}{"include_usage": true}

	if chatModelConfig != nil {
		if chatModelConfig.Stream != nil && !*chatModelConfig.Stream {
			return fmt.Errorf("stream must be true in ChatStreamlyWithSender")
		}
		chatModelConfig.ToolCallsResult = nil

		if chatModelConfig.Thinking != nil {
			reqBody["enable_thinking"] = *chatModelConfig.Thinking
		}

		if chatModelConfig.Tools != nil {
			reqBody["tool_choice"] = aliyunToolChoice(modelName, messages, chatModelConfig.ToolChoice)
		}
	}

	// For qwen3 models on DashScope, enable_thinking defaults to true when
	// omitted. RAGFlow's default is to disable thinking unless explicitly
	// enabled by the user, matching Python's chat_model.py behavior.
	applyQwen3ThinkingDefault(modelName, reqBody)

	return a.baseModel.doStreamRequest(ctx, url, apiConfig, reqBody, streamCallTimeout, func(body io.ReadCloser) error {
		return HandleStreamingResponse(body, modelUsage, chatModelConfig, OpenAIParserConfig, sender)
	})
}

// applyQwen3ThinkingDefault ensures enable_thinking=false is sent for qwen3
// models when it hasn't been explicitly configured. DashScope defaults
// enable_thinking to true for qwen3 models, which produces reasoning output
// that RAGFlow doesn't expect in most pipelines. Mirrors Python's
// chat_model.py default of enable_thinking=False for qwen3.
func applyQwen3ThinkingDefault(modelName string, reqBody map[string]interface{}) {
	if !strings.Contains(strings.ToLower(modelName), "qwen3") {
		return
	}
	if _, alreadySet := reqBody["enable_thinking"]; alreadySet {
		return
	}
	reqBody["enable_thinking"] = false
}

// aliyunToolChoice prevents qwen-flash from repeatedly issuing another tool call
// after a tool result has already been supplied. With "auto", qwen-flash can
// keep emitting tool_calls even for a successful result until the ReAct graph
// exhausts its step limit. Other models, initial calls, and explicit choices
// retain their configured behavior.
func aliyunToolChoice(modelName string, messages []Message, configured *string) string {
	choice := "auto"
	if configured != nil && strings.TrimSpace(*configured) != "" {
		choice = *configured
	}
	if !strings.EqualFold(strings.TrimSpace(choice), "auto") {
		return choice
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(modelName)), "qwen-flash") {
		return choice
	}
	for _, message := range messages {
		if strings.EqualFold(message.Role, "tool") && message.ToolCallID != "" {
			return "none"
		}
	}
	return choice
}

// Embed embeds a list of texts into embeddings
func (a *AliyunModel) Embed(ctx context.Context, modelName *string, request EmbedRequest, apiConfig *APIConfig, embeddingConfig *EmbeddingConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	if len(request.Texts) == 0 {
		return []EmbeddingData{}, nil
	}

	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL

	// Tongyi-Qianwen text embeddings are asymmetric (query vs document), and only
	// the native API can express that distinction (text_type). Every call therefore
	// goes to the native API, exactly as Python's QWenEmbed does
	// (embedding_model.py): a recognized DashScope host is mapped onto its /api/v1
	// root, and an unrecognized base URL is IGNORED — the dashscope SDK keeps its own
	// endpoint (DASHSCOPE_HTTP_BASE_URL, else
	// https://dashscope.aliyuncs.com/api/<DASHSCOPE_API_VERSION|v1>) and QWenEmbed
	// only warns once at construction time (:121-127, :134-138). There is
	// deliberately no OpenAI-compatible fallback: that endpoint cannot send
	// text_type, so it would encode query-side embeddings (dense seed / retrieval
	// query) in document space. Use the OpenAI-API-Compatible / vLLM factory for a
	// private gateway — Python requires the same choice.
	if nativeRoot := aliyunNativeEmbeddingRoot(baseURL); nativeRoot != "" {
		return a.embedNative(ctx, nativeRoot, *modelName, request, apiConfig, modelUsage)
	}

	// Unrecognized host: warn once per host, then use the SDK's default native root.
	aliyunWarnIgnoredBaseURL(baseURL)
	return a.embedNative(ctx, aliyunSDKDefaultNativeRoot(), *modelName, request, apiConfig, modelUsage)
}

// aliyunIgnoredHostWarned dedups the unrecognized-host warning to once per host.
var aliyunIgnoredHostWarned sync.Map

// aliyunWarnSink writes the warning line. Tests replace it to capture the warning
// without going through the logger.
var aliyunWarnSink = func(format string, args ...any) { common.StdLogger().Printf(format, args...) }

// aliyunWarnIgnoredBaseURL reports, once per host and never per call, that the
// configured base URL is not a DashScope host and is therefore ignored in favour
// of the SDK default endpoint — Python QWenEmbed warns once at construction time
// instead (embedding_model.py:121-127).
func aliyunWarnIgnoredBaseURL(baseURL string) {
	host := aliyunBaseURLHost(baseURL)
	if host == "" {
		return
	}
	if _, seen := aliyunIgnoredHostWarned.LoadOrStore(host, struct{}{}); seen {
		return
	}
	aliyunWarnSink(
		"[Qwen embedding] base URL host %q is not a DashScope host, so the configured base URL is ignored: "+
			"using the native text-embedding API at %s instead (Python QWenEmbed behaviour). Point the base URL at "+
			"dashscope.aliyuncs.com / dashscope-intl.aliyuncs.com, or end it with /api/v1 to address a "+
			"native-compatible endpoint directly.", host, aliyunSDKDefaultNativeRoot())
}

// aliyunSDKDefaultNativeRoot mirrors the dashscope SDK's default native API root
// (dashscope/common/env.py:14-23): DASHSCOPE_HTTP_BASE_URL when set, else
// https://dashscope.aliyuncs.com/api/<DASHSCOPE_API_VERSION|v1>. It is where
// Python's QWenEmbed ends up when the configured base URL is not a DashScope host,
// and the only override that still carries text_type.
func aliyunSDKDefaultNativeRoot() string {
	if root := strings.TrimSpace(os.Getenv("DASHSCOPE_HTTP_BASE_URL")); root != "" {
		return strings.TrimRight(root, "/")
	}
	version := strings.TrimSpace(os.Getenv("DASHSCOPE_API_VERSION"))
	if version == "" {
		version = "v1"
	}
	return fmt.Sprintf("https://dashscope.aliyuncs.com/api/%s", version)
}

// aliyunBaseURLHost returns the lowercase host of a configured base URL for log
// lines. Only the host is used, so credentials, path and query string cannot leak
// (Python's _dashscope_base_url_for_log is strict the same way,
// embedding_model.py:73-75).
func aliyunBaseURLHost(baseURL string) string {
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Hostname() != "" {
		return strings.ToLower(parsed.Hostname())
	}
	// Schemeless config (e.g. "gateway.internal/v1"): the first path segment is the
	// host.
	return strings.ToLower(strings.SplitN(strings.TrimPrefix(raw, "//"), "/", 2)[0])
}

// aliyunNativeEmbeddingPath is the DashScope native text-embedding endpoint,
// appended to the native API root ("https://<host>/api/v1").
const aliyunNativeEmbeddingPath = "services/embeddings/text-embedding/text-embedding"

// aliyunNativeEmbedBatchSize mirrors Python QWenEmbed.encode's batch_size = 4.
const aliyunNativeEmbedBatchSize = 4

// aliyunNativeEmbeddingRoot maps a configured DashScope base URL onto the native
// API root, mirroring Python embedding_model._dashscope_native_http_api_url
// (rag/llm/embedding_model.py:80-128): an already-native base is kept as-is,
// known DashScope hosts (CN and international) are mapped to their /api/v1 root,
// and anything else returns "" so the caller falls back to the dashscope SDK's
// default native root (aliyunSDKDefaultNativeRoot), as Python QWenEmbed does.
// Matching on the parsed hostname (never a substring of the full URL) prevents a
// crafted query string from selecting the native API.
func aliyunNativeEmbeddingRoot(baseURL string) string {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if u == "" {
		return ""
	}
	if strings.HasSuffix(u, "/api/v1") {
		return u
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	switch {
	case host == "dashscope-intl.aliyuncs.com" || strings.HasSuffix(host, ".dashscope-intl.aliyuncs.com"):
		return "https://dashscope-intl.aliyuncs.com/api/v1"
	case host == "dashscope.aliyuncs.com" || strings.HasSuffix(host, ".dashscope.aliyuncs.com"):
		return "https://dashscope.aliyuncs.com/api/v1"
	default:
		return ""
	}
}

type aliyunNativeEmbedInput struct {
	Texts []string `json:"texts"`
}

type aliyunNativeEmbedRequest struct {
	Model      string                 `json:"model"`
	Input      aliyunNativeEmbedInput `json:"input"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

type aliyunNativeEmbedResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// RequestID is the DashScope request id every native response carries (the same
	// id Python surfaces in its ModelException messages).
	RequestID string `json:"request_id"`
	Output    struct {
		Embeddings []struct {
			// TextIndex is a pointer so an absent/null key is distinguishable
			// from index 0: Python does embds[e["text_index"]] and raises
			// KeyError when the key is missing (wrapped into EmbeddingError).
			TextIndex *int      `json:"text_index"`
			Embedding []float64 `json:"embedding"`
		} `json:"embeddings"`
	} `json:"output"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// embedNative calls the DashScope native text-embedding API, the only transport
// that carries text_type ("document" for encode, "query" for encode_queries —
// Python QWenEmbed). Inputs are sent in batches of 4 like Python; the response's
// text_index is relative to the batch, so it is offset back to the caller's
// slice before being returned.
//
// The response is reassembled exactly like Python QWenEmbed.encode: each batch's
// chunk is sized by the NUMBER OF RETURNED EMBEDDINGS and every vector is
// PLACED at its batch-relative text_index (never appended in response order),
// because consumers such as NavEmbedder read the slice positionally and discard
// EmbeddingData.Index. As in Python, a duplicate text_index overwrites the
// earlier vector, a gap leaves an empty vector in that slot, and a text_index
// outside the returned chunk raises — a NEGATIVE text_index counts from the end
// of the chunk (embds[-1] is the last slot), exactly like a Python list
// assignment.
func (a *AliyunModel) embedNative(ctx context.Context, root, modelName string, request EmbedRequest, apiConfig *APIConfig, modelUsage *common.ModelUsage) ([]EmbeddingData, error) {
	textType := "document"
	if request.Query {
		textType = "query"
	}
	endpoint := fmt.Sprintf("%s/%s", strings.TrimRight(root, "/"), aliyunNativeEmbeddingPath)

	embeddings := make([]EmbeddingData, 0, len(request.Texts))
	for start := 0; start < len(request.Texts); start += aliyunNativeEmbedBatchSize {
		end := min(start+aliyunNativeEmbedBatchSize, len(request.Texts))
		batch := request.Texts[start:end]

		jsonData, err := json.Marshal(aliyunNativeEmbedRequest{
			Model:      modelName,
			Input:      aliyunNativeEmbedInput{Texts: batch},
			Parameters: map[string]interface{}{"text_type": textType},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}

		callCtx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
		req, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewBuffer(jsonData))
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

		resp, err := a.baseModel.httpClient.Do(req)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to send request: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read response: %w", readErr)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("aliyun native embeddings API error: %s, body: %s", resp.Status, string(body))
		}

		var parsed aliyunNativeEmbedResponse
		if err = json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		if parsed.Code != "" {
			return nil, fmt.Errorf("aliyun native embeddings API error: %s: %s", parsed.Code, parsed.Message)
		}
		// Mirror Python QWenEmbed.encode:
		//   embds = [[] for _ in range(len(resp["output"]["embeddings"]))]
		//   for e in resp["output"]["embeddings"]:
		//       embds[e["text_index"]] = e["embedding"]
		//   res.extend(embds)
		// including Python list-assignment indexing: a negative text_index
		// counts from the END of the chunk (embds[-1] is the last slot), and out
		// of range in either direction raises.
		embds := make([]EmbeddingData, len(parsed.Output.Embeddings))
		for _, item := range parsed.Output.Embeddings {
			if item.TextIndex == nil {
				return nil, fmt.Errorf("aliyun native embeddings response item missing text_index")
			}
			idx := *item.TextIndex
			if idx < 0 {
				idx += len(embds)
			}
			if idx < 0 || idx >= len(embds) {
				return nil, fmt.Errorf("aliyun native embeddings response index %d out of range for %d embeddings", *item.TextIndex, len(embds))
			}
			embds[idx] = EmbeddingData{
				Embedding: item.Embedding,
				Index:     len(embeddings) + idx,
			}
		}
		embeddings = append(embeddings, embds...)
		recordResponseUsage(modelUsage, parsed.RequestID, &TokenUsage{TotalTokens: parsed.Usage.TotalTokens}, "embedding")
	}
	return embeddings, nil
}

type aliyunRerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n"`
	ReturnDocuments bool     `json:"return_documents"`
}

type aliyunRerankResponse struct {
	ID      string `json:"id"`
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func (a *AliyunModel) Rerank(ctx context.Context, modelName *string, request RerankRequest, apiConfig *APIConfig, rerankConfig *RerankConfig, modelUsage *common.ModelUsage) (*RerankResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	documents := request.Documents
	query := request.Query

	if len(documents) == 0 {
		return &RerankResponse{}, nil
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}

	resolvedBaseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), a.baseModel.URLSuffix.Rerank)

	topN := len(documents)
	if rerankConfig != nil && rerankConfig.TopN > 0 {
		topN = rerankConfig.TopN
	}
	if topN == 0 {
		topN = len(documents)
	}

	reqBody := aliyunRerankRequest{
		Model:           *modelName,
		Query:           query,
		Documents:       documents,
		TopN:            topN,
		ReturnDocuments: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := a.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aliyun rerank API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed aliyunRerankResponse
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	rerankResponse := &RerankResponse{Data: make([]RerankResult, 0, len(parsed.Results))}
	for _, item := range parsed.Results {
		rerankResponse.Data = append(rerankResponse.Data, RerankResult{Index: item.Index, RelevanceScore: item.RelevanceScore})
	}
	recordResponseUsage(modelUsage, parsed.ID, &TokenUsage{
		TotalTokens: parsed.Usage.TotalTokens,
	}, "rerank")

	return rerankResponse, nil
}

// TranscribeAudio transcribe audio
func (a *AliyunModel) TranscribeAudio(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage) (*ASRResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AliyunModel) TranscribeAudioWithSender(ctx context.Context, modelName *string, file *string, apiConfig *APIConfig, asrConfig *ASRConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	return fmt.Errorf("%s, no such method", a.Name())
}

// aliyunTTSDefaultVoice is used when the caller does not specify a voice;
// DashScope's Qwen TTS models require one.
const aliyunTTSDefaultVoice = "Cherry"

// aliyunTTSRequest is the DashScope multimodal-generation request for Qwen
// TTS models (qwen-tts / qwen3-tts-flash family).
type aliyunTTSRequest struct {
	Model string         `json:"model"`
	Input aliyunTTSInput `json:"input"`
}

type aliyunTTSInput struct {
	Text string `json:"text"`
	// Voice is required by Qwen TTS models (e.g. "Cherry").
	Voice string `json:"voice"`
	// LanguageType hints the text language (e.g. "Chinese", "English");
	// omitted to let the model auto-detect.
	LanguageType string `json:"language_type,omitempty"`
}

// aliyunTTSResponse is the non-streaming DashScope multimodal-generation
// response. The synthesized audio is not inlined; output.audio.url points
// to a downloadable file (valid for 24h).
type aliyunTTSResponse struct {
	Output struct {
		Audio struct {
			URL string `json:"url"`
		} `json:"audio"`
		FinishReason string `json:"finish_reason"`
	} `json:"output"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// AudioSpeech convert text to audio
func (a *AliyunModel) AudioSpeech(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage) (*TTSResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}
	if modelName == nil || *modelName == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if audioContent == nil || *audioContent == "" {
		return nil, fmt.Errorf("audio content is empty")
	}
	if strings.TrimSpace(a.baseModel.URLSuffix.TTS) == "" {
		return nil, fmt.Errorf("aliyun TTS URL suffix is required")
	}

	resolvedBaseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(resolvedBaseURL, "/"), strings.TrimPrefix(a.baseModel.URLSuffix.TTS, "/"))

	input := aliyunTTSInput{Text: *audioContent, Voice: aliyunTTSDefaultVoice}
	if ttsConfig != nil {
		if voice, ok := ttsConfig.Params["voice"].(string); ok && strings.TrimSpace(voice) != "" {
			input.Voice = voice
		}
		if lang, ok := ttsConfig.Params["language_type"].(string); ok && strings.TrimSpace(lang) != "" {
			input.LanguageType = lang
		}
	}

	jsonData, err := json.Marshal(aliyunTTSRequest{Model: *modelName, Input: input})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, longOpCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := a.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aliyun TTS API error: %s, body: %s", resp.Status, string(body))
	}

	var parsed aliyunTTSResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse aliyun TTS response: %w, body: %s", err, string(body))
	}
	if parsed.Code != "" {
		return nil, fmt.Errorf("aliyun TTS API error: %s: %s", parsed.Code, parsed.Message)
	}
	if parsed.Output.Audio.URL == "" {
		return nil, fmt.Errorf("aliyun TTS response has no audio url, body: %s", string(body))
	}

	audio, err := a.downloadAliyunTTSAudio(ctx, parsed.Output.Audio.URL)
	if err != nil {
		return nil, err
	}
	// Qwen TTS audio files are WAV.
	return &TTSResponse{Audio: audio, MediaType: "audio/wav"}, nil
}

// aliyunTTSAudioMaxBytes caps a synthesized audio download. DashScope TTS
// audio is far smaller; this only guards against runaway responses.
const aliyunTTSAudioMaxBytes int64 = 64 << 20 // 64 MiB

// downloadAliyunTTSAudio fetches the synthesized audio file referenced by a
// DashScope TTS response.
func (a *AliyunModel) downloadAliyunTTSAudio(ctx context.Context, audioURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", audioURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create audio download request: %w", err)
	}
	resp, err := a.baseModel.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download aliyun TTS audio: %w", err)
	}
	defer resp.Body.Close()

	audio, err := io.ReadAll(io.LimitReader(resp.Body, aliyunTTSAudioMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read aliyun TTS audio: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download aliyun TTS audio: %s", resp.Status)
	}
	if int64(len(audio)) > aliyunTTSAudioMaxBytes {
		return nil, fmt.Errorf("aliyun TTS audio download exceeds %d bytes", aliyunTTSAudioMaxBytes)
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("aliyun TTS audio download is empty")
	}
	return audio, nil
}

func (a *AliyunModel) AudioSpeechWithSender(ctx context.Context, modelName *string, audioContent *string, apiConfig *APIConfig, ttsConfig *TTSConfig, modelUsage *common.ModelUsage, sender func(*string, *string) error) error {
	if sender == nil {
		return fmt.Errorf("sender is required")
	}

	// The non-streaming DashScope TTS endpoint returns the whole audio via
	// a downloadable URL; forward it as a single chunk.
	resp, err := a.AudioSpeech(ctx, modelName, audioContent, apiConfig, ttsConfig, modelUsage)
	if err != nil {
		return err
	}
	chunk := string(resp.Audio)
	return sender(&chunk, nil)
}

// OCRFile OCR file
func (a *AliyunModel) OCRFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, ocrConfig *OCRConfig, modelUsage *common.ModelUsage) (*OCRFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

// ParseFile parse file
func (a *AliyunModel) ParseFile(ctx context.Context, modelName *string, content []byte, url *string, apiConfig *APIConfig, parseFileConfig *ParseFileConfig, modelUsage *common.ModelUsage) (*ParseFileResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

type AliyunModelItem struct {
	ModelName    string `json:"model_name"`
	BaseCapacity int    `json:"base_capacity"`
}

type AliyunModelOutput struct {
	Models   []AliyunModelItem `json:"models"`
	PageNo   int               `json:"page_no"`
	PageSize int               `json:"page_size"`
	Total    int               `json:"total"`
}

type AliyunModelList struct {
	RequestID string            `json:"request_id"`
	Output    AliyunModelOutput `json:"output"`
}

func (a *AliyunModel) ListModels(ctx context.Context, apiConfig *APIConfig) ([]ListModelResponse, error) {
	if err := a.baseModel.APIConfigCheck(apiConfig); err != nil {
		return nil, err
	}

	resolvedBaseURL, err := a.baseModel.GetBaseURL(apiConfig)
	if err != nil {
		return nil, err
	}
	baseURL := resolvedBaseURL

	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(baseURL, "/"), a.baseModel.URLSuffix.Models)

	// Build request body
	reqBody := map[string]interface{}{}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, nonStreamCallTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *apiConfig.ApiKey))

	resp, err := a.baseModel.httpClient.Do(req)
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

	var modelList ModelList
	if err = json.Unmarshal(body, &modelList); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return ParseListModel(modelList), nil
}

func (a *AliyunModel) Balance(ctx context.Context, apiConfig *APIConfig) (map[string]interface{}, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AliyunModel) CheckConnection(ctx context.Context, apiConfig *APIConfig) error {
	_, err := a.ListModels(ctx, apiConfig)
	if err != nil {
		return err
	}
	return nil
}

func (a *AliyunModel) ListTasks(ctx context.Context, apiConfig *APIConfig) ([]ListTaskStatus, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}

func (a *AliyunModel) ShowTask(ctx context.Context, taskID string, apiConfig *APIConfig) (*TaskResponse, error) {
	return nil, fmt.Errorf("%s, no such method", a.Name())
}
