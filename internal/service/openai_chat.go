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
	"fmt"
	"ragflow/internal/entity"
	"regexp"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"

	"go.uber.org/zap"
)

type openAIChatGetter interface {
	GetChat(ctx context.Context, userID, chatID string) (*GetChatResponse, error)
}

type openAITenantLLMKeyGetter interface {
	GetAPIKeyFromInstance(ctx context.Context, tenantID, compositeModelName string) (string, error)
}

type openAIModelResolver interface {
	ResolveModelConfig(ctx context.Context, tenantID string, modelType entity.ModelType, modelRef string) (*ModelTarget, error)
}

type openAIMetadataProvider interface {
	GetFlattedMetaByKBs(ctx context.Context, kbIDs []string) (common.MetaData, error)
	EnrichChunksWithDocMetadata(ctx context.Context, chunks []map[string]interface{}, tenantID string, metadataFields []string)
}

type openAIChatPipeline interface {
	AsyncChat(ctx context.Context, userID string, chat *entity.Chat, messages []map[string]interface{}, stream bool, kwargs map[string]interface{}) (<-chan AsyncChatResult, error)
}

type openAILangfuseFactory func(ctx context.Context, tenantID, userID, chatID, modelName string) *LangfuseClient

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
// The handler maps ContextTokens to the compatibility reasoning_tokens field.
type OpenAICompletionResponse struct {
	Model            string
	Content          string
	Reference        []FormattedChunk
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ContextTokens    int
	Created          *int64
}

// OpenAIStreamEventKind discriminates stream events.
type OpenAIStreamEventKind int

const (
	OpenAIEventContent   OpenAIStreamEventKind = iota // delta.content
	OpenAIEventReasoning                              // delta.reasoning_content
	OpenAIEventFinal                                  // trailing chunk
	OpenAIEventError                                  // in-band error
)

// OpenAIStreamEvent is yielded by the service-side stream producer.
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

// OpenAIChatStream is the prepared stream returned to the HTTP handler.
// Preparation and pipeline startup have completed before this value is returned.
type OpenAIChatStream struct {
	Events        <-chan OpenAIStreamEvent
	Model         string
	NeedReference bool
}

// OpenAIChatService prepares and executes OpenAI-compatible chat completions.
// HTTP request parsing and response rendering belong to the handler.
type OpenAIChatService struct {
	chatSvc               openAIChatGetter
	tenantLLMSvc          openAITenantLLMKeyGetter
	modelResolver         openAIModelResolver
	metadataSvc           openAIMetadataProvider
	pipeline              openAIChatPipeline
	langfuseClientFactory openAILangfuseFactory
}

func NewOpenAIChatService() *OpenAIChatService {
	pipeline := NewChatPipelineService()
	return &OpenAIChatService{
		chatSvc:               NewChatService(),
		tenantLLMSvc:          NewTenantLLMService(),
		modelResolver:         pipeline.ModelProviderSvc.modelSolver(),
		metadataSvc:           pipeline.MetadataSvc,
		pipeline:              pipeline,
		langfuseClientFactory: LangfuseClientFromTenant,
	}
}

// OpenAIChatRequest mirrors the OpenAI Chat Completions request body.
// `stop` and `user` are omitted intentionally — JSON unmarshal silently drops them.
type OpenAIChatRequest struct {
	Question  string                   `json:"question,omitempty"`
	Query     string                   `json:"query,omitempty"`
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

type openAIRequestOptions struct {
	needReference      bool
	includeRefMetadata bool
	metadataFields     []string
	metadataCondition  map[string]interface{}
}

type preparedOpenAIChat struct {
	ctx           context.Context
	request       *OpenAIRequest
	promptTokens  int
	contextTokens int
	results       <-chan AsyncChatResult
	finish        func()
}

// Complete runs a non-streaming OpenAI-compatible completion. It has no HTTP
// responsibilities; callers render the returned payload or coded error.
func (s *OpenAIChatService) Complete(
	ctx context.Context,
	userID, chatID string,
	req OpenAIChatRequest,
) (*OpenAICompletionResponse, error) {
	common.Info("OpenAIChatCompletions started", zap.String("chat_id", chatID))
	prepared, err := s.prepare(ctx, userID, chatID, req, false)
	if err != nil {
		return nil, err
	}
	defer prepared.finish()

	var finalResult AsyncChatResult
	found := false
	for result := range prepared.results {
		if result.Final {
			finalResult = result
			found = true
			break
		}
	}
	if !found {
		return nil, common.NewCodedError(common.CodeDataError, "AsyncChat returned no final result")
	}

	content := strings.TrimSpace(finalResult.Answer)
	completionTokens := tokenizer.NumTokensFromString(content)
	resp := &OpenAICompletionResponse{
		Model:            prepared.request.Model,
		Content:          content,
		PromptTokens:     prepared.promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      prepared.promptTokens + completionTokens,
		ContextTokens:    prepared.contextTokens,
	}
	if prepared.request.NeedReference {
		resp.Reference = []FormattedChunk{}
		if ref, ok := finalResult.Reference["chunks"]; ok {
			if chunks, ok := ref.([]map[string]interface{}); ok {
				resp.Reference = formatChunks(chunks)
			}
		}
		s.enrichChunksWithDocumentMetadata(
			prepared.ctx,
			resp.Reference,
			prepared.request.Chat.TenantID,
			prepared.request.IncludeRefMetadata,
			prepared.request.MetadataFields,
		)
	}
	common.Info("OpenAIChatCompletions completed", zap.String("chat_id", chatID))
	return resp, nil
}

// Stream prepares and starts an OpenAI-compatible streaming completion. The
// returned channel is owned and closed by the service producer.
func (s *OpenAIChatService) Stream(
	ctx context.Context,
	userID, chatID string,
	req OpenAIChatRequest,
) (*OpenAIChatStream, error) {
	common.Info("OpenAIChatCompletions started", zap.String("chat_id", chatID))
	prepared, err := s.prepare(ctx, userID, chatID, req, true)
	if err != nil {
		return nil, err
	}

	events := make(chan OpenAIStreamEvent, 16)
	go s.produceStream(prepared, events, chatID)
	return &OpenAIChatStream{
		Events:        events,
		Model:         prepared.request.Model,
		NeedReference: prepared.request.NeedReference,
	}, nil
}

func normalizeAndValidateOpenAIRequest(req OpenAIChatRequest) (OpenAIChatRequest, openAIRequestOptions, error) {
	var options openAIRequestOptions

	question, err := ResolveCompletionQuestion(req.Question, req.Query, req.Messages)
	if err != nil {
		return req, options, common.NewCodedError(common.CodeDataError, err.Error())
	}
	if req.Question != "" || req.Query != "" {
		req.Messages = []map[string]interface{}{{"role": "user", "content": question}}
	}
	if len(req.Messages) == 0 {
		return req, options, common.NewCodedError(common.CodeDataError, "You have to provide messages.")
	}

	extraBody, extraBodyOK := req.ExtraBody.(map[string]interface{})
	if req.ExtraBody != nil && !extraBodyOK {
		return req, options, common.NewCodedError(common.CodeArgumentError, "extra_body must be an object.")
	}
	if extraBody != nil {
		if rawRM, ok := extraBody["reference_metadata"].(map[string]interface{}); ok {
			if rawFields, hasFields := rawRM["fields"]; hasFields {
				rawArr, ok := rawFields.([]interface{})
				if !ok {
					return req, options, common.NewCodedError(common.CodeArgumentError, "reference_metadata.fields must be an array.")
				}
				for _, field := range rawArr {
					if _, ok := field.(string); !ok {
						return req, options, common.NewCodedError(common.CodeArgumentError, "reference_metadata.fields must be an array.")
					}
				}
			}
		}
		if rawCondition, ok := extraBody["metadata_condition"]; ok && rawCondition != nil {
			if _, ok := rawCondition.(map[string]interface{}); !ok {
				return req, options, common.NewCodedError(common.CodeArgumentError, "metadata_condition must be an object.")
			}
		}
	}
	if req.MaxTokens != nil && *req.MaxTokens <= 0 {
		return req, options, common.NewCodedError(common.CodeArgumentError, "`max_tokens` must be greater than 0.")
	}

	// Preserve the existing OpenAI endpoint behavior: only the final message is
	// passed into the shared chat pipeline.
	req.Messages = req.Messages[len(req.Messages)-1:]
	normalizedMessages, err := normalizeOpenAIMessages(req.Messages)
	if err != nil {
		return req, options, common.NewCodedError(common.CodeDataError, err.Error())
	}
	lastRole, _ := normalizedMessages[len(normalizedMessages)-1]["role"].(string)
	if lastRole != "user" {
		return req, options, common.NewCodedError(common.CodeDataError, "The last content of this conversation is not from user.")
	}
	req.Messages = normalizedMessages

	if extraBody != nil {
		if value, ok := extraBody["reference"].(bool); ok {
			options.needReference = value
		}
		rawRM, hasRM := extraBody["reference_metadata"]
		if hasRM && rawRM != nil {
			rm, ok := rawRM.(map[string]interface{})
			if !ok {
				return req, options, common.NewCodedError(common.CodeDataError, "reference_metadata must be an object.")
			}
			if inc, hasInc := rm["include"].(bool); hasInc {
				options.includeRefMetadata = inc
			}
			if rawFields, hasFields := rm["fields"]; hasFields && rawFields != nil {
				rawArr := rawFields.([]interface{})
				if len(rawArr) == 0 {
					options.metadataFields = []string{}
				} else {
					for _, f := range rawArr {
						options.metadataFields = append(options.metadataFields, f.(string))
					}
				}
			}
		}
		if rawCondition, hasCondition := extraBody["metadata_condition"]; hasCondition && rawCondition != nil {
			condition := rawCondition.(map[string]interface{})
			if len(condition) > 0 {
				options.metadataCondition = condition
			}
		}
	}
	return req, options, nil
}

func (s *OpenAIChatService) prepare(
	ctx context.Context,
	userID, chatID string,
	req OpenAIChatRequest,
	stream bool,
) (*preparedOpenAIChat, error) {
	req, options, err := normalizeAndValidateOpenAIRequest(req)
	if err != nil {
		return nil, err
	}

	dialogResp, err := s.chatSvc.GetChat(ctx, userID, chatID)
	if err != nil {
		return nil, common.NewCodedError(common.CodeDataError, err.Error())
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
		if _, mErr := s.modelResolver.ResolveModelConfig(ctx, dialog.TenantID, entity.ModelTypeChat, resolvedModel); mErr != nil {
			return nil, common.NewCodedError(common.CodeArgumentError, fmt.Sprintf("`llm_id` %s doesn't exist", req.Model))
		}
		apiKey, apiErr := s.tenantLLMSvc.GetAPIKeyFromInstance(ctx, dialog.TenantID, req.Model)
		if apiErr != nil || apiKey == "" {
			return nil, common.NewCodedError(common.CodeDataError, fmt.Sprintf("Cannot use specified model %s.", req.Model))
		}
		dialog.LLMID = resolvedModel
	}

	genCfg := extractGenerationConfig(&req)

	s.MergeGenerationConfig(dialog, genCfg)

	openaiReq := &OpenAIRequest{
		ChatID:             chatID,
		Model:              resolvedModel,
		Chat:               dialog,
		Messages:           req.Messages,
		Stream:             stream,
		NeedReference:      options.needReference,
		IncludeRefMetadata: options.includeRefMetadata,
		MetadataFields:     options.metadataFields,
		MetadataCondition:  options.metadataCondition,
		GenerationConfig:   genCfg,
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
		metas, mdErr := s.metadataSvc.GetFlattedMetaByKBs(ctx, kbIDs)
		if mdErr != nil {
			return nil, common.NewCodedError(common.CodeDataError,
				fmt.Errorf("metadata_condition: load metadata: %w", mdErr).Error())
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
		"quote":            openaiReq.NeedReference,
	}
	if docIDsStr != "" {
		chatKwargs["doc_ids"] = docIDsStr
	}

	contextTokens := 0
	for _, message := range openaiReq.Messages {
		if content, ok := message["content"].(string); ok {
			contextTokens += tokenizer.NumTokensFromString(content)
		}
	}

	runCtx, finish := s.withLangfuse(ctx, dialog.TenantID, userID, openaiReq.ChatID, openaiReq.Model)
	asyncResults, asyncErr := s.pipeline.AsyncChat(runCtx, userID, dialog, filteredMessages, openaiReq.Stream, chatKwargs)
	if asyncErr != nil {
		finish()
		return nil, common.NewCodedError(common.CodeDataError, asyncErr.Error())
	}
	if asyncResults == nil {
		finish()
		return nil, common.NewCodedError(common.CodeDataError, "AsyncChat returned a nil result channel")
	}
	return &preparedOpenAIChat{
		ctx:           runCtx,
		request:       openaiReq,
		promptTokens:  promptTokens,
		contextTokens: contextTokens,
		results:       asyncResults,
		finish:        finish,
	}, nil
}

func (s *OpenAIChatService) produceStream(prepared *preparedOpenAIChat, events chan<- OpenAIStreamEvent, chatID string) {
	defer close(events)
	defer prepared.finish()
	defer common.Info("OpenAIChatCompletions completed", zap.String("chat_id", chatID))

	var (
		fullContent    string
		completionTok  int
		deltaCount     int
		finalReference []FormattedChunk
		lastResult     AsyncChatResult
	)

	for result := range prepared.results {
		lastResult = result

		if result.Final {
			finalReference = []FormattedChunk{}
			if ref, ok := result.Reference["chunks"]; ok {
				if chunks, ok := ref.([]map[string]interface{}); ok {
					finalReference = formatChunks(chunks)
				}
			}
			s.enrichChunksWithDocumentMetadata(
				prepared.ctx,
				finalReference,
				prepared.request.Chat.TenantID,
				prepared.request.IncludeRefMetadata,
				prepared.request.MetadataFields,
			)
			completionTok = tokenizer.NumTokensFromString(result.Answer)
			events <- OpenAIStreamEvent{
				Kind:             OpenAIEventFinal,
				FinalAnswer:      strings.TrimSpace(result.Answer),
				FinalReference:   finalReference,
				PromptTokens:     prepared.promptTokens,
				CompletionTokens: completionTok,
				TotalTokens:      prepared.promptTokens + completionTok,
			}
			return
		}

		if result.Reasoning != "" {
			completionTok += tokenizer.NumTokensFromString(result.Reasoning)
			events <- OpenAIStreamEvent{Kind: OpenAIEventReasoning, Delta: result.Reasoning}
		}

		if result.Answer != "" {
			fullContent += result.Answer
			completionTok += tokenizer.NumTokensFromString(result.Answer)
			events <- OpenAIStreamEvent{Kind: OpenAIEventContent, Delta: result.Answer}
			if deltaCount < 3 {
				common.Debug("OpenAI first content delta",
					zap.Int("delta_index", deltaCount),
					zap.String("delta", result.Answer),
					zap.Int("delta_len", len(result.Answer)))
				deltaCount++
			}
		}
	}

	if finalReference == nil && prepared.request.NeedReference {
		finalReference = []FormattedChunk{}
		if ref, ok := lastResult.Reference["chunks"]; ok {
			if chunks, ok := ref.([]map[string]interface{}); ok {
				finalReference = formatChunks(chunks)
			}
		}
	}
	s.enrichChunksWithDocumentMetadata(
		prepared.ctx,
		finalReference,
		prepared.request.Chat.TenantID,
		prepared.request.IncludeRefMetadata,
		prepared.request.MetadataFields,
	)
	events <- OpenAIStreamEvent{
		Kind:             OpenAIEventFinal,
		FinalAnswer:      strings.TrimSpace(fullContent),
		FinalReference:   finalReference,
		PromptTokens:     prepared.promptTokens,
		CompletionTokens: completionTok,
		TotalTokens:      prepared.promptTokens + completionTok,
	}
}

func (s *OpenAIChatService) withLangfuse(
	ctx context.Context,
	tenantID, userID, chatID, modelName string,
) (context.Context, func()) {
	if s.langfuseClientFactory == nil {
		return ctx, func() {}
	}
	client := s.langfuseClientFactory(ctx, tenantID, userID, chatID, modelName)
	if client == nil {
		return ctx, func() {}
	}
	runCtx := context.WithValue(ctx, langfuseCtxKey, client)
	return runCtx, func() {
		shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = client.Shutdown(shutdownCtx)
	}
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

// NormalizeOpenAIMessageContent coerces OpenAI message content to text and
// drops unsupported non-text parts.
func NormalizeOpenAIMessageContent(content interface{}) (string, error) {
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
		c, err := NormalizeOpenAIMessageContent(m["content"])
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
			Content:          strVal(getValue(chunk, "content", "content_with_weight")),
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
func (s *OpenAIChatService) enrichChunksWithDocumentMetadata(ctx context.Context, chunks []FormattedChunk, tenantID string, include bool, fields []string) {
	if !include || len(chunks) == 0 || s == nil || s.metadataSvc == nil {
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
	s.metadataSvc.EnrichChunksWithDocMetadata(ctx, maps, tenantID, fields)
	for i, m := range maps {
		if md, ok := m["document_metadata"]; ok {
			chunks[i].DocumentMetadata = md
		}
	}
}
