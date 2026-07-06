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

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"ragflow/internal/entity"
	"regexp"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"

	"github.com/gin-gonic/gin"

	"go.uber.org/zap"
)

type OpenAIRequest struct {
	ChatID string
	Model  string
	// Chat is the loaded chat entity, mutated in place by MergeGenerationConfig.
	Chat *entity.Chat
	// Messages are pre-normalized: system messages removed, leading assistant
	// removed, content coerced to string (vision parts dropped).
	Messages           []map[string]interface{}
	Stream             bool
	NeedReference      bool
	IncludeRefMetadata bool
	MetadataFields     []string
	MetadataCondition  map[string]interface{}
	// Internet not plumbed — matches Python's openai_api.py behavior.
	GenerationConfig map[string]interface{}
}

// FormattedChunk is a normalized chunk matching Python's chunks_format output.
type FormattedChunk struct {
	ID               string      `json:"id"`
	Content          string      `json:"content"`
	DocumentID       string      `json:"document_id"`
	DocumentName     string      `json:"document_name"`
	DatasetID        string      `json:"dataset_id"`
	ImageID          string      `json:"image_id"`
	Positions        interface{} `json:"positions"`
	URL              interface{} `json:"url"`
	Similarity       interface{} `json:"similarity"`
	VectorSimilarity interface{} `json:"vector_similarity"`
	TermSimilarity   interface{} `json:"term_similarity"`
	RowID            interface{} `json:"row_id"`
	DocType          interface{} `json:"doc_type"`
	DocumentMetadata interface{} `json:"document_metadata"`
}

// OpenAICompletionResponse is the non-streaming response payload.
// The reasoning_tokens quirk (openai_api.py:348-352) lives in the c.JSON call.
type OpenAICompletionResponse struct {
	Model            string
	Content          string
	Reference        []FormattedChunk
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Created          int64
}

// OpenAIStreamEventKind discriminates stream events.
type OpenAIStreamEventKind int

const (
	OpenAIEventContent   OpenAIStreamEventKind = iota // delta.content
	OpenAIEventReasoning                              // delta.reasoning_content
	OpenAIEventFinal                                  // trailing chunk
	OpenAIEventError                                  // in-band error
)

// OpenAIStreamEvent is yielded by the event-translator inside OpenAIChatCompletions.
type OpenAIStreamEvent struct {
	Kind             OpenAIStreamEventKind
	Delta            string // for Content / Reasoning
	FinalAnswer      string // for Final
	FinalReference   []FormattedChunk
	Error            string // for Error
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// OpenAIChatService implements the /api/v1/openai/<chat_id>/chat/completions route.
// It composes ChatPipelineService for the shared RAG pipeline (AsyncChat) while
// keeping handler-level concerns (message filtering, generation config merge,
// reference metadata enrichment) on the service itself.
type OpenAIChatService struct {
	chatSvc      *ChatService
	tenantLLMSvc *TenantLLMService
	pipeline     *ChatPipelineService
}

func NewOpenAIChatService() *OpenAIChatService {
	return &OpenAIChatService{
		chatSvc:      NewChatService(),
		tenantLLMSvc: NewTenantLLMService(),
		pipeline:     NewChatPipelineService(),
	}
}

// OpenAIChatRequest mirrors the OpenAI Chat Completions request body.
// `stop` and `user` are omitted intentionally — JSON unmarshal silently drops them.
type OpenAIChatRequest struct {
	Model     string                   `json:"model"`
	Messages  []map[string]interface{} `json:"messages"`
	Stream    *bool                    `json:"stream,omitempty"`
	ExtraBody interface{}              `json:"extra_body,omitempty"`

	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	MaxTokens        *int     `json:"max_tokens,omitempty"`
}

func (s *OpenAIChatService) OpenAIChatCompletions(c *gin.Context, userID, chatID string, bodyBytes []byte) {
	var req OpenAIChatRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		s.writeArgError(c, err.Error())
		return
	}
	common.Info("OpenAIChatCompletions started", zap.String("chat_id", chatID))

	normalizedMessages, err := normalizeOpenAIMessages(req.Messages)
	if err != nil {
		s.writeDataError(c, err.Error())
		return
	}
	if len(normalizedMessages) == 0 {
		s.writeDataError(c, "You have to provide messages.")
		return
	}

	lastRole, _ := normalizedMessages[len(normalizedMessages)-1]["role"].(string)
	if lastRole != "user" {
		s.writeDataError(c, "The last content of this conversation is not from user.")
		return
	}

	if req.ExtraBody != nil {
		if _, ok := req.ExtraBody.(map[string]interface{}); !ok {
			s.writeDataError(c, "extra_body must be an object.")
			return
		}
	}

	var needReference = false
	var includeRefMetadata = false
	var metadataFields []string
	var metadataCondition map[string]interface{}
	if eb, ok := req.ExtraBody.(map[string]interface{}); ok {
		if v, hasRef := eb["reference"].(bool); hasRef {
			needReference = v
		}
		rawRM, hasRM := eb["reference_metadata"]
		if hasRM && rawRM != nil {
			rm, ok := rawRM.(map[string]interface{})
			if !ok {
				s.writeDataError(c, "reference_metadata must be an object.")
				return
			}
			if inc, hasInc := rm["include"].(bool); hasInc {
				includeRefMetadata = inc
			}
			if rawFields, hasFields := rm["fields"]; hasFields && rawFields != nil {
				rawArr, rawOK := rawFields.([]interface{})
				if !rawOK {
					s.writeDataError(c, "reference_metadata.fields must be an array.")
					return
				}
				if len(rawArr) == 0 {
					metadataFields = []string{}
				} else {
					for _, f := range rawArr {
						str, ok := f.(string)
						if !ok {
							s.writeDataError(c, "reference_metadata.fields must be an array.")
							return
						}
						metadataFields = append(metadataFields, str)
					}
				}
			}
		}
		if mc, hasMC := eb["metadata_condition"]; hasMC && mc != nil {
			mcMap, isObj := mc.(map[string]interface{})
			if !isObj {
				s.writeDataError(c, "metadata_condition must be an object.")
				return
			}
			if len(mcMap) > 0 {
				metadataCondition = mcMap
			}
		}
	}

	dialogResp, err := s.chatSvc.GetChat(userID, chatID)
	if err != nil {
		s.writeDataError(c, err.Error())
		return
	}
	dialog := dialogResp.Chat
	resolvedModel := req.Model
	if req.Model == "model" {
		resolvedModel = dialog.LLMID
		if resolvedModel == "" {
			resolvedModel = "model"
		}
	}
	if req.Model != "model" {
		if _, _, _, _, mErr := s.pipeline.ModelProviderSvc.GetChatModelConfig(dialog.TenantID, resolvedModel); mErr != nil {
			s.writeArgError(c, fmt.Sprintf("`llm_id` %s doesn't exist", req.Model))
			return
		}
		apiKey, apiErr := s.tenantLLMSvc.GetAPIKeyFromInstance(dialog.TenantID, req.Model)
		if apiErr != nil || apiKey == "" {
			s.writeDataError(c, fmt.Sprintf("Cannot use specified model %s.", req.Model))
			return
		}
		dialog.LLMID = resolvedModel
	}

	genCfg := extractGenerationConfig(&req)

	s.MergeGenerationConfig(dialog, genCfg)

	stream := req.Stream != nil && *req.Stream
	openaiReq := &OpenAIRequest{
		ChatID:             chatID,
		Model:              resolvedModel,
		Chat:               dialog,
		Messages:           normalizedMessages,
		Stream:             stream,
		NeedReference:      needReference,
		IncludeRefMetadata: includeRefMetadata,
		MetadataFields:     metadataFields,
		MetadataCondition:  metadataCondition,
		GenerationConfig:   genCfg,
	}

	completionID := fmt.Sprintf("chatcmpl-%s", openaiReq.ChatID)

	ctx := c.Request.Context()
	lfClient := LangfuseClientFromTenant(ctx, dialog.TenantID, userID, openaiReq.ChatID, openaiReq.Model)
	if lfClient != nil {
		ctx = context.WithValue(ctx, langfuseCtxKey, lfClient)
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = lfClient.Shutdown(shutdownCtx)
		}()
	}

	filteredMessages := s.filterMessages(openaiReq.Messages)

	var docIDsStr string
	if openaiReq.MetadataCondition != nil {
		common.Debug("metadata_condition filter started",
			zap.Any("condition", openaiReq.MetadataCondition))
		kbIDs := make([]string, 0, len(dialog.KBIDs))
		for _, raw := range dialog.KBIDs {
			if id, ok := raw.(string); ok && id != "" {
				kbIDs = append(kbIDs, id)
			}
		}
		metas, mdErr := s.pipeline.MetadataSvc.GetFlattedMetaByKBs(kbIDs)
		if mdErr != nil {
			s.writeDataError(c, fmt.Errorf("metadata_condition: load metadata: %w", mdErr).Error())
			return
		}
		docIDsStr = MetadataConditionToDocIDs(metas, openaiReq.MetadataCondition)
		common.Debug("metadata_condition filter ended", zap.String("doc_ids", docIDsStr))
	}

	common.Debug("OpenAI chat config resolved",
		zap.String("tenant_id", dialog.TenantID),
		zap.String("dialog_id", dialog.ID),
		zap.String("llm_id", dialog.LLMID),
		zap.Any("llm_setting", dialog.LLMSetting),
		zap.Any("request_generation_config", openaiReq.GenerationConfig),
		zap.String("doc_ids", docIDsStr))

	promptTokens := 0
	if lastMsg := filteredMessages[len(filteredMessages)-1]; lastMsg != nil {
		if content, ok := lastMsg["content"].(string); ok {
			promptTokens = tokenizer.NumTokensFromString(content)
		}
	}

	chatKwargs := map[string]interface{}{
		"toolcall_session": nil, // no tool calls on OpenAI-compat path
		"tools":            nil,
		"quote":            needReference,
	}
	if docIDsStr != "" {
		chatKwargs["doc_ids"] = docIDsStr
	}

	asyncResults, asyncErr := s.pipeline.AsyncChat(ctx, userID, dialog, filteredMessages, openaiReq.Stream, chatKwargs)
	if asyncErr != nil {
		s.writeDataError(c, asyncErr.Error())
		return
	}

	if stream {
		events := make(chan OpenAIStreamEvent, 16)
		go func() {
			defer close(events)
			defer func() {
				if r := recover(); r != nil {
					common.Warn("OpenAI streaming goroutine panic", zap.Any("recover", r))
					events <- OpenAIStreamEvent{Kind: OpenAIEventError, Error: fmt.Sprintf("internal error: %v", r)}
				}
			}()

			var (
				fullContent    string
				completionTok  int
				deltaCount     int
				finalReference []FormattedChunk
				lastResult     AsyncChatResult
			)

			for result := range asyncResults {
				lastResult = result

				if result.StartToThink || result.EndToThink {
					// Think markers only toggle routing state; no SSE event
					// emitted. Matches Python's _stream_chat_completion_sse
					// which ignores start_to_think/end_to_think flags and
					// never emits "<think>" or "</think>" as content.
					continue
				}

				if result.Final {
					finalContent := strings.TrimSpace(result.Answer)
					fullContent = finalContent
					if ref, ok := result.Reference["chunks"]; ok {
						if chunks, ok := ref.([]map[string]interface{}); ok {
							finalReference = formatChunks(chunks)
						}
					}
					s.enrichChunksWithDocumentMetadata(finalReference, dialog.TenantID, openaiReq.IncludeRefMetadata, openaiReq.MetadataFields)
					completionTok = tokenizer.NumTokensFromString(result.Answer)
					events <- OpenAIStreamEvent{
						Kind:             OpenAIEventFinal,
						FinalAnswer:      finalContent,
						FinalReference:   finalReference,
						PromptTokens:     promptTokens,
						CompletionTokens: completionTok,
						TotalTokens:      promptTokens + completionTok,
					}
					return
				}

				if result.Reasoning != "" {
					completionTok += tokenizer.NumTokensFromString(result.Reasoning)
					events <- OpenAIStreamEvent{Kind: OpenAIEventReasoning, Delta: result.Reasoning}
				}

				if result.Answer != "" {
					delta := result.Answer
					fullContent += delta
					completionTok += tokenizer.NumTokensFromString(delta)
					events <- OpenAIStreamEvent{Kind: OpenAIEventContent, Delta: delta}
					if deltaCount < 3 {
						common.Debug("OpenAI first content delta",
							zap.Int("delta_index", deltaCount),
							zap.String("delta", result.Answer),
							zap.Int("delta_len", len(result.Answer)))
						deltaCount++
					}
				}
			}

			if finalReference == nil && openaiReq.NeedReference {
				if ref, ok := lastResult.Reference["chunks"]; ok {
					if chunks, ok := ref.([]map[string]interface{}); ok {
						finalReference = formatChunks(chunks)
					}
				}
			}
			s.enrichChunksWithDocumentMetadata(finalReference, dialog.TenantID, openaiReq.IncludeRefMetadata, openaiReq.MetadataFields)
			events <- OpenAIStreamEvent{
				Kind:             OpenAIEventFinal,
				FinalAnswer:      strings.TrimSpace(fullContent),
				FinalReference:   finalReference,
				PromptTokens:     promptTokens,
				CompletionTokens: completionTok,
				TotalTokens:      promptTokens + completionTok,
			}
		}()
		if err := streamChatCompletionSSE(c, events, completionID, resolvedModel, openaiReq.NeedReference); err != nil {
			s.writeDataError(c, err.Error())
		}
	} else {
		var finalResult AsyncChatResult
		found := false
		for result := range asyncResults {
			if result.Final {
				finalResult = result
				found = true
				break
			}
		}
		if !found {
			s.writeDataError(c, "AsyncChat returned no final result")
			return
		}

		content := strings.TrimSpace(finalResult.Answer)
		completionTokens := tokenizer.NumTokensFromString(content)
		resp := &OpenAICompletionResponse{
			Created:          time.Now().Unix(),
			Model:            openaiReq.Model,
			Content:          content,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		}
		if openaiReq.NeedReference {
			if ref, ok := finalResult.Reference["chunks"]; ok {
				if chunks, ok := ref.([]map[string]interface{}); ok {
					resp.Reference = formatChunks(chunks)
				}
			}
			s.enrichChunksWithDocumentMetadata(resp.Reference, dialog.TenantID, openaiReq.IncludeRefMetadata, openaiReq.MetadataFields)
		}

		contextUsed := 0
		for _, m := range openaiReq.Messages {
			if c, ok := m["content"].(string); ok {
				contextUsed += tokenizer.NumTokensFromString(c)
			}
		}

		choices := []gin.H{{
			"index":         0,
			"finish_reason": "stop",
			"logprobs":      nil,
			"message": gin.H{
				"role":    "assistant",
				"content": resp.Content,
			},
		}}
		if openaiReq.NeedReference {
			choices[0]["message"].(gin.H)["reference"] = resp.Reference
		}

		c.JSON(http.StatusOK, gin.H{
			"id":      completionID,
			"object":  "chat.completion",
			"created": resp.Created,
			"model":   resp.Model,
			"usage": gin.H{
				"prompt_tokens":     resp.PromptTokens,
				"completion_tokens": resp.CompletionTokens,
				"total_tokens":      resp.PromptTokens + resp.CompletionTokens,
				"completion_tokens_details": gin.H{
					"reasoning_tokens":           contextUsed,
					"accepted_prediction_tokens": resp.CompletionTokens,
					"rejected_prediction_tokens": 0,
				},
			},
			"choices": choices,
		})
	}
	common.Info("OpenAIChatCompletions completed", zap.String("chat_id", chatID))
}

// MergeGenerationConfig merges request config into dialog.LLMSetting (mutating).
func (s *OpenAIChatService) MergeGenerationConfig(dialog *entity.Chat, config map[string]interface{}) {
	if config == nil {
		return
	}
	if dialog.LLMSetting == nil {
		dialog.LLMSetting = map[string]interface{}{}
	}
	for k, v := range config {
		dialog.LLMSetting[k] = v
	}
}

// filterMessages drops system messages and leading assistant messages.
func (s *OpenAIChatService) filterMessages(messages []map[string]interface{}) []map[string]interface{} {
	var out []map[string]interface{}
	for _, m := range messages {
		role, _ := m["role"].(string)
		if role == "system" {
			continue
		}
		if role == "assistant" && len(out) == 0 {
			continue
		}
		out = append(out, m)
	}
	return out
}

// cleanCitationMarkers strips "##N$$" markers from the answer.
func cleanCitationMarkers(s string) string {
	var citationMarkerRegex = regexp.MustCompile(`##\d+\$\$`)
	return citationMarkerRegex.ReplaceAllString(s, "")
}

// isContentDelta filters out "[DONE]" leaked by some drivers.
func isContentDelta(answer *string) bool {
	if answer == nil {
		return false
	}
	if *answer == "" {
		return false
	}
	if *answer == "[DONE]" {
		return false
	}
	return true
}

// extractGenerationConfig mirrors Python's extract_generation_config.
func extractGenerationConfig(req *OpenAIChatRequest) map[string]interface{} {
	cfg := make(map[string]interface{})
	if req.Temperature != nil {
		cfg["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		cfg["top_p"] = *req.TopP
	}
	if req.MaxTokens != nil {
		cfg["max_tokens"] = float64(*req.MaxTokens)
	}
	if req.FrequencyPenalty != nil {
		cfg["frequency_penalty"] = *req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		cfg["presence_penalty"] = *req.PresencePenalty
	}
	return cfg
}

// normalizeMessageContent coerces content to string (drops non-text parts).
func normalizeMessageContent(content interface{}) (string, error) {
	if content == nil {
		return "", nil
	}
	if s, ok := content.(string); ok {
		return s, nil
	}
	if arr, ok := content.([]interface{}); ok {
		parts := make([]string, 0, len(arr))
		for _, p := range arr {
			pm, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			if pm["type"] != "text" {
				continue
			}
			t, _ := pm["text"].(string)
			parts = append(parts, t)
		}
		return joinNonEmpty(parts, "\n"), nil
	}
	return "", fmt.Errorf("messages[].content must be a string or an array of content parts.")
}

// normalizeOpenAIMessages normalizes message content for all messages.
func normalizeOpenAIMessages(messages []map[string]interface{}) ([]map[string]interface{}, error) {
	out := make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		normalized := make(map[string]interface{}, len(m))
		for k, v := range m {
			normalized[k] = v
		}
		c, err := normalizeMessageContent(m["content"])
		if err != nil {
			return nil, err
		}
		normalized["content"] = c
		out = append(out, normalized)
	}
	return out, nil
}

// joinNonEmpty joins strings with sep, skipping empties.
func joinNonEmpty(parts []string, sep string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	out := ""
	for i, p := range nonEmpty {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}

// getValue reads chunk[m1] falling back to chunk[m2].
func getValue(chunk map[string]interface{}, k1, k2 string) interface{} {
	if v, ok := chunk[k1]; ok {
		return v
	}
	return chunk[k2]
}

func strVal(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// formatChunks normalizes chunk fields to a canonical schema, matching Python's chunks_format.
func formatChunks(chunks []map[string]interface{}) []FormattedChunk {
	out := make([]FormattedChunk, 0, len(chunks))
	for _, chunk := range chunks {
		out = append(out, FormattedChunk{
			ID:               strVal(getValue(chunk, "chunk_id", "id")),
			Content:          strVal(getValue(chunk, "content_with_weight", "content")),
			DocumentID:       strVal(getValue(chunk, "doc_id", "document_id")),
			DocumentName:     strVal(getValue(chunk, "docnm_kwd", "document_name")),
			DatasetID:        strVal(getValue(chunk, "kb_id", "dataset_id")),
			ImageID:          strVal(getValue(chunk, "image_id", "img_id")),
			Positions:        getValue(chunk, "positions", "position_int"),
			URL:              chunk["url"],
			Similarity:       sanitizeJSONFloats(chunk["similarity"]),
			VectorSimilarity: sanitizeJSONFloats(chunk["vector_similarity"]),
			TermSimilarity:   sanitizeJSONFloats(chunk["term_similarity"]),
			RowID:            chunk["row_id"],
			DocType:          getValue(chunk, "doc_type_kwd", "doc_type"),
			DocumentMetadata: chunk["document_metadata"],
		})
	}
	return out
}

// enrichChunksWithDocumentMetadata enriches chunks with document metadata.
// Mirrors Python's enrich_chunks_with_document_metadata() in
// api/utils/reference_metadata_utils.py.
// When fields is a non-nil empty slice (explicitly provided as []), enrichment
// is skipped — matching Python's behavior for {"fields": []}.
func (s *OpenAIChatService) enrichChunksWithDocumentMetadata(chunks []FormattedChunk, tenantID string, include bool, fields []string) {
	if !include || len(chunks) == 0 || s == nil || s.pipeline.MetadataSvc == nil {
		return
	}
	if fields != nil && len(fields) == 0 {
		return
	}
	maps := make([]map[string]interface{}, len(chunks))
	for i, ch := range chunks {
		maps[i] = map[string]interface{}{
			"kb_id":             ch.DatasetID,
			"doc_id":            ch.DocumentID,
			"document_metadata": ch.DocumentMetadata,
		}
	}
	s.pipeline.MetadataSvc.EnrichChunksWithDocMetadata(maps, tenantID, fields)
	for i, m := range maps {
		if md, ok := m["document_metadata"]; ok {
			chunks[i].DocumentMetadata = md
		}
	}
}

// streamChatCompletionSSE drains events and writes SSE chunks.
func streamChatCompletionSSE(
	c *gin.Context,
	events <-chan OpenAIStreamEvent,
	completionID string,
	requestedModel string,
	needReference bool,
) error {
	c.Header("Cache-control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Content-Type", "text/event-stream; charset=utf-8")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported")
	}

	writeSSE := func(payload gin.H) {
		body, _ := json.Marshal(payload)
		_, _ = c.Writer.Write([]byte("data:"))
		_, _ = c.Writer.Write(body)
		_, _ = c.Writer.Write([]byte("\n\n"))
		flusher.Flush()
	}

	for ev := range events {
		switch ev.Kind {
		case OpenAIEventContent:
			chunk := gin.H{
				"id":                 completionID,
				"object":             "chat.completion.chunk",
				"created":            time.Now().Unix(),
				"model":              requestedModel,
				"system_fingerprint": "",
				"usage":              nil,
				"choices": []gin.H{{
					"index": 0,
					"delta": gin.H{
						"role":              "assistant",
						"content":           ev.Delta,
						"reasoning_content": nil,
						"function_call":     nil,
						"tool_calls":        nil,
					},
					"finish_reason": nil,
					"logprobs":      nil,
				}},
			}
			writeSSE(chunk)

		case OpenAIEventReasoning:
			chunk := gin.H{
				"id":                 completionID,
				"object":             "chat.completion.chunk",
				"created":            time.Now().Unix(),
				"model":              requestedModel,
				"system_fingerprint": "",
				"usage":              nil,
				"choices": []gin.H{{
					"index": 0,
					"delta": gin.H{
						"role":              "assistant",
						"content":           nil,
						"reasoning_content": ev.Delta,
						"function_call":     nil,
						"tool_calls":        nil,
					},
					"finish_reason": nil,
					"logprobs":      nil,
				}},
			}
			writeSSE(chunk)

		case OpenAIEventError:
			chunk := gin.H{
				"id":                 completionID,
				"object":             "chat.completion.chunk",
				"created":            time.Now().Unix(),
				"model":              requestedModel,
				"system_fingerprint": "",
				"usage":              nil,
				"choices": []gin.H{{
					"index": 0,
					"delta": gin.H{
						"role":              "assistant",
						"content":           "**ERROR**: " + ev.Error,
						"reasoning_content": nil,
						"function_call":     nil,
						"tool_calls":        nil,
					},
					"finish_reason": nil,
					"logprobs":      nil,
				}},
			}
			writeSSE(chunk)

		case OpenAIEventFinal:
			delta := gin.H{
				"role":              "assistant",
				"content":           nil,
				"reasoning_content": nil,
				"function_call":     nil,
				"tool_calls":        nil,
			}
			if needReference {
				delta["reference"] = ev.FinalReference
				delta["final_content"] = ev.FinalAnswer
			}
			chunk := gin.H{
				"id":                 completionID,
				"object":             "chat.completion.chunk",
				"created":            time.Now().Unix(),
				"model":              requestedModel,
				"system_fingerprint": "",
				"usage": gin.H{
					"prompt_tokens":     ev.PromptTokens,
					"completion_tokens": ev.CompletionTokens,
					"total_tokens":      ev.TotalTokens,
				},
				"choices": []gin.H{{
					"index":         0,
					"delta":         delta,
					"finish_reason": "stop",
					"logprobs":      nil,
				}},
			}
			writeSSE(chunk)
		}
	}

	// Always terminate with data: [DONE]\n\n.
	_, _ = c.Writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
	return nil
}

// writeArgError writes a 101 JSON error envelope (malformed request).
func (s *OpenAIChatService) writeArgError(c *gin.Context, msg string) {
	common.ResponseWithCodeData(c, common.CodeArgumentError, nil, msg)
}

// writeDataError writes a 102 JSON error envelope (service failure).
func (s *OpenAIChatService) writeDataError(c *gin.Context, msg string) {
	common.ResponseWithCodeData(c, common.CodeDataError, nil, msg)
}
