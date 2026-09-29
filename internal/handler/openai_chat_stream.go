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

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

func writeOpenAIChatSSE(
	ctx context.Context,
	c *gin.Context,
	stream *service.OpenAIChatStream,
	completionID string,
) error {
	flush, err := openAISSEFlush(c)
	if err != nil {
		return err
	}

	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Content-Type", "text/event-stream; charset=utf-8")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-stream.Events:
			if !ok {
				return writeOpenAIDone(ctx, c, flush)
			}
			payload, terminal := openAIStreamPayload(event, completionID, stream.Model, stream.NeedReference)
			if payload == nil {
				continue
			}
			if err := writeOpenAISSEPayload(c, flush, payload); err != nil {
				return err
			}
			if terminal {
				return writeOpenAIDone(ctx, c, flush)
			}
		}
	}
}

func openAIStreamPayload(
	event service.OpenAIStreamEvent,
	completionID, model string,
	needReference bool,
) (gin.H, bool) {
	base := func(delta gin.H, usage interface{}, finishReason interface{}) gin.H {
		return gin.H{
			"id":                 completionID,
			"object":             "chat.completion.chunk",
			"created":            time.Now().Unix(),
			"model":              model,
			"system_fingerprint": "",
			"usage":              usage,
			"choices": []gin.H{{
				"index":         0,
				"delta":         delta,
				"finish_reason": finishReason,
				"logprobs":      nil,
			}},
		}
	}

	switch event.Kind {
	case service.OpenAIEventContent:
		return base(gin.H{
			"role":              "assistant",
			"content":           event.Delta,
			"reasoning_content": nil,
			"function_call":     nil,
			"tool_calls":        nil,
		}, nil, nil), false
	case service.OpenAIEventReasoning:
		return base(gin.H{
			"role":              "assistant",
			"content":           nil,
			"reasoning_content": event.Delta,
			"function_call":     nil,
			"tool_calls":        nil,
		}, nil, nil), false
	case service.OpenAIEventError:
		return base(gin.H{
			"role":              "assistant",
			"content":           "**ERROR**: " + event.Error,
			"reasoning_content": nil,
			"function_call":     nil,
			"tool_calls":        nil,
		}, nil, nil), false
	case service.OpenAIEventFinal:
		delta := gin.H{
			"role":              "assistant",
			"content":           nil,
			"reasoning_content": nil,
			"function_call":     nil,
			"tool_calls":        nil,
		}
		if needReference {
			delta["reference"] = event.FinalReference
			delta["final_content"] = event.FinalAnswer
		}
		return base(delta, gin.H{
			"prompt_tokens":     event.PromptTokens,
			"completion_tokens": event.CompletionTokens,
			"total_tokens":      event.TotalTokens,
		}, "stop"), true
	default:
		return nil, false
	}
}

func openAISSEFlush(c *gin.Context) (func() error, error) {
	writer := http.ResponseWriter(c.Writer)
	if unwrapper, ok := c.Writer.(interface{ Unwrap() http.ResponseWriter }); ok {
		writer = unwrapper.Unwrap()
	}
	if flusher, ok := writer.(interface{ FlushError() error }); ok {
		return flusher.FlushError, nil
	}
	if flusher, ok := writer.(http.Flusher); ok {
		return func() error {
			flusher.Flush()
			return nil
		}, nil
	}
	return nil, fmt.Errorf("streaming unsupported")
}

func writeOpenAISSEPayload(c *gin.Context, flush func() error, payload gin.H) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	frame := make([]byte, 0, len(body)+7)
	frame = append(frame, "data:"...)
	frame = append(frame, body...)
	frame = append(frame, '\n', '\n')
	if _, err := c.Writer.Write(frame); err != nil {
		return err
	}
	return flush()
}

func writeOpenAIDone(ctx context.Context, c *gin.Context, flush func() error) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if _, err := c.Writer.Write([]byte("data: [DONE]\n\n")); err != nil {
		return err
	}
	return flush()
}
