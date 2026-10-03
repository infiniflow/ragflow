// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

type stubOpenAIChatGetter struct {
	response *GetChatResponse
	err      error
}

func (s stubOpenAIChatGetter) GetChat(context.Context, string, string) (*GetChatResponse, error) {
	return s.response, s.err
}

type stubOpenAIPipeline struct {
	results     <-chan AsyncChatResult
	err         error
	panicOnCall interface{}
	called      bool
	ctx         context.Context
	stream      bool
}

type stubOpenAIMetadataProvider struct {
	err error
}

func (s stubOpenAIMetadataProvider) GetFlattedMetaByKBs(context.Context, []string) (common.MetaData, error) {
	return nil, s.err
}

func (stubOpenAIMetadataProvider) EnrichChunksWithDocMetadata(context.Context, []map[string]interface{}, string, []string) {
}

func (s *stubOpenAIPipeline) AsyncChat(
	ctx context.Context,
	_ string,
	_ *entity.Chat,
	_ []map[string]interface{},
	stream bool,
	_ map[string]interface{},
) (<-chan AsyncChatResult, error) {
	s.called = true
	s.ctx = ctx
	s.stream = stream
	if s.panicOnCall != nil {
		panic(s.panicOnCall)
	}
	return s.results, s.err
}

func newOpenAIContractService(pipeline *stubOpenAIPipeline) *OpenAIChatService {
	return &OpenAIChatService{
		chatSvc: stubOpenAIChatGetter{response: &GetChatResponse{Chat: &entity.Chat{
			ID:       "chat-1",
			TenantID: "tenant-1",
			LLMID:    "tenant-model",
		}}},
		pipeline: pipeline,
	}
}

func validOpenAIChatRequest() OpenAIChatRequest {
	return OpenAIChatRequest{
		Model:    "model",
		Messages: []map[string]interface{}{{"role": "user", "content": "hello"}},
	}
}

func assertOpenAICodedError(t *testing.T, err error, code common.ErrorCode, message string) {
	t.Helper()
	var coded *common.CodedError
	if !errors.As(err, &coded) {
		t.Fatalf("error = %T %v, want *common.CodedError", err, err)
	}
	if coded.Code != code || coded.Message != message {
		t.Fatalf("coded error = (%d, %q), want (%d, %q)", coded.Code, coded.Message, code, message)
	}
}

func TestNormalizeAndValidateOpenAIRequestContract(t *testing.T) {
	zero := 0
	tests := []struct {
		name    string
		req     OpenAIChatRequest
		code    common.ErrorCode
		message string
	}{
		{
			name:    "missing messages",
			req:     OpenAIChatRequest{Model: "model"},
			code:    common.CodeDataError,
			message: "You have to provide messages.",
		},
		{
			name: "last message is not user",
			req: OpenAIChatRequest{Model: "model", Messages: []map[string]interface{}{
				{"role": "assistant", "content": "answer"},
			}},
			code:    common.CodeDataError,
			message: "The last content of this conversation is not from user.",
		},
		{
			name: "invalid message content",
			req: OpenAIChatRequest{Model: "model", Messages: []map[string]interface{}{
				{"role": "user", "content": 42},
			}},
			code:    common.CodeDataError,
			message: "messages[].content must be a string or an array of content parts.",
		},
		{
			name:    "extra body must be object",
			req:     OpenAIChatRequest{Model: "model", Messages: validOpenAIChatRequest().Messages, ExtraBody: "bad"},
			code:    common.CodeArgumentError,
			message: "extra_body must be an object.",
		},
		{
			name: "reference metadata must be object",
			req: OpenAIChatRequest{Model: "model", Messages: validOpenAIChatRequest().Messages, ExtraBody: map[string]interface{}{
				"reference_metadata": "bad",
			}},
			code:    common.CodeDataError,
			message: "reference_metadata must be an object.",
		},
		{
			name: "reference metadata fields must be array",
			req: OpenAIChatRequest{Model: "model", Messages: validOpenAIChatRequest().Messages, ExtraBody: map[string]interface{}{
				"reference_metadata": map[string]interface{}{"fields": "author"},
			}},
			code:    common.CodeArgumentError,
			message: "reference_metadata.fields must be an array.",
		},
		{
			name: "reference metadata fields must contain strings",
			req: OpenAIChatRequest{Model: "model", Messages: validOpenAIChatRequest().Messages, ExtraBody: map[string]interface{}{
				"reference_metadata": map[string]interface{}{"fields": []interface{}{1}},
			}},
			code:    common.CodeArgumentError,
			message: "reference_metadata.fields must be an array.",
		},
		{
			name: "metadata condition must be object",
			req: OpenAIChatRequest{Model: "model", Messages: validOpenAIChatRequest().Messages, ExtraBody: map[string]interface{}{
				"metadata_condition": "bad",
			}},
			code:    common.CodeArgumentError,
			message: "metadata_condition must be an object.",
		},
		{
			name:    "max tokens must be positive",
			req:     OpenAIChatRequest{Model: "model", Messages: validOpenAIChatRequest().Messages, MaxTokens: &zero},
			code:    common.CodeArgumentError,
			message: "`max_tokens` must be greater than 0.",
		},
		{
			name: "last role wins validation precedence",
			req: OpenAIChatRequest{Model: "model", Messages: []map[string]interface{}{
				{"role": "assistant", "content": "answer"},
			}, ExtraBody: "bad"},
			code:    common.CodeDataError,
			message: "The last content of this conversation is not from user.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := normalizeAndValidateOpenAIRequest(test.req)
			assertOpenAICodedError(t, err, test.code, test.message)
		})
	}
}

func TestOpenAICompleteReturnsSuccessPayload(t *testing.T) {
	results := make(chan AsyncChatResult, 1)
	results <- AsyncChatResult{Answer: " final answer ", Final: true}
	close(results)
	pipeline := &stubOpenAIPipeline{results: results}

	resp, err := newOpenAIContractService(pipeline).Complete(t.Context(), "user-1", "chat-1", validOpenAIChatRequest())
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if !pipeline.called || pipeline.stream {
		t.Fatalf("pipeline called=%v stream=%v", pipeline.called, pipeline.stream)
	}
	if resp.Model != "tenant-model" || resp.Content != "final answer" {
		t.Fatalf("response = %+v", resp)
	}
	if resp.TotalTokens != resp.PromptTokens+resp.CompletionTokens {
		t.Fatalf("usage is inconsistent: %+v", resp)
	}
}

func TestOpenAICompleteSanitizesGenerationFailure(t *testing.T) {
	results := make(chan AsyncChatResult, 1)
	results <- AsyncChatResult{Answer: "**ERROR**: dial mysql.internal password=secret", Final: true}
	close(results)

	resp, err := newOpenAIContractService(&stubOpenAIPipeline{results: results}).Complete(
		t.Context(), "user-1", "chat-1", validOpenAIChatRequest())
	if resp != nil {
		t.Fatalf("response = %+v, want nil", resp)
	}
	assertOpenAICodedError(t, err, common.CodeDataError, openAIInternalErrorMessage)
	if strings.Contains(err.Error(), "mysql.internal") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaked internal details: %v", err)
	}
}

func TestOpenAIStreamSanitizesFailureAndStopsAtFirstTerminal(t *testing.T) {
	results := make(chan AsyncChatResult, 3)
	results <- AsyncChatResult{Answer: "partial"}
	results <- AsyncChatResult{Answer: "**ERROR**: provider secret", Final: true}
	results <- AsyncChatResult{Answer: "duplicate final", Final: true}
	close(results)
	pipeline := &stubOpenAIPipeline{results: results}

	stream, err := newOpenAIContractService(pipeline).Stream(t.Context(), "user-1", "chat-1", validOpenAIChatRequest())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if !pipeline.called || !pipeline.stream {
		t.Fatalf("pipeline called=%v stream=%v", pipeline.called, pipeline.stream)
	}

	var events []OpenAIStreamEvent
	for event := range stream.Events {
		events = append(events, event)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v, want content and error", events)
	}
	if events[0].Kind != OpenAIEventContent || events[0].Delta != "partial" {
		t.Fatalf("content event = %+v", events[0])
	}
	if events[1].Kind != OpenAIEventError || events[1].Error != openAIInternalErrorMessage {
		t.Fatalf("error event = %+v", events[1])
	}
	if strings.Contains(events[1].Error, "secret") {
		t.Fatalf("error event leaked provider detail: %+v", events[1])
	}
}

func TestOpenAIStreamPipelineStartFailureReturnsCodedError(t *testing.T) {
	pipeline := &stubOpenAIPipeline{err: errors.New("dial mysql.internal password=secret")}
	stream, err := newOpenAIContractService(pipeline).Stream(t.Context(), "user-1", "chat-1", validOpenAIChatRequest())
	if stream != nil {
		t.Fatalf("stream = %+v, want nil", stream)
	}
	assertOpenAICodedError(t, err, common.CodeDataError, openAIInternalErrorMessage)
}

func TestOpenAIStreamMetadataFailureIsSanitized(t *testing.T) {
	pipeline := &stubOpenAIPipeline{}
	svc := newOpenAIContractService(pipeline)
	svc.metadataSvc = stubOpenAIMetadataProvider{err: errors.New("query mysql.internal password=secret")}
	req := validOpenAIChatRequest()
	req.ExtraBody = map[string]interface{}{
		"metadata_condition": map[string]interface{}{"author": "Ada"},
	}

	stream, err := svc.Stream(t.Context(), "user-1", "chat-1", req)
	if stream != nil {
		t.Fatalf("stream = %+v, want nil", stream)
	}
	assertOpenAICodedError(t, err, common.CodeDataError, openAIInternalErrorMessage)
	if pipeline.called {
		t.Fatal("pipeline started after metadata preparation failed")
	}
	if strings.Contains(err.Error(), "mysql.internal") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("metadata error leaked internal detail: %v", err)
	}
}

func TestOpenAIStreamPreparePanicIsSanitized(t *testing.T) {
	pipeline := &stubOpenAIPipeline{panicOnCall: "database password=secret"}
	stream, err := newOpenAIContractService(pipeline).Stream(t.Context(), "user-1", "chat-1", validOpenAIChatRequest())
	if stream != nil {
		t.Fatalf("stream = %+v, want nil", stream)
	}
	assertOpenAICodedError(t, err, common.CodeDataError, openAIInternalErrorMessage)
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("panic detail leaked to caller: %v", err)
	}
}

func TestOpenAIStreamCancellationClosesProducer(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	results := make(chan AsyncChatResult)
	pipelineStopped := make(chan struct{})
	pipeline := &stubOpenAIPipeline{results: results}
	svc := newOpenAIContractService(pipeline)

	stream, err := svc.Stream(ctx, "user-1", "chat-1", validOpenAIChatRequest())
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	go func() {
		<-pipeline.ctx.Done()
		close(results)
		close(pipelineStopped)
	}()
	cancel()

	select {
	case <-pipelineStopped:
	case <-time.After(time.Second):
		t.Fatal("pipeline did not receive cancellation")
	}
	select {
	case _, ok := <-stream.Events:
		if ok {
			t.Fatal("unexpected event after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("stream producer did not close its channel")
	}
}
