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

package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/entity"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"
)

// openAICompatUsage mirrors the usage object emitted by the Python
// completion_openai implementation. Agent runs can contain multiple LLM
// calls, so completion tokens are accumulated from the message deltas rather
// than inferred from the number of RunEvent frames.
type openAICompatUsage struct {
	PromptTokens            int                                `json:"prompt_tokens"`
	CompletionTokens        int                                `json:"completion_tokens"`
	TotalTokens             int                                `json:"total_tokens"`
	CompletionTokensDetails openAICompatCompletionTokenDetails `json:"completion_tokens_details"`
}

type openAICompatCompletionTokenDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens"`
}

type openAICompatStreamDelta struct {
	Content      any `json:"content"`
	Role         string `json:"role"`
	FunctionCall any `json:"function_call"`
	ToolCalls    any `json:"tool_calls"`
	Reference    any `json:"reference,omitempty"`
}

type openAICompatStreamChoice struct {
	Delta        openAICompatStreamDelta `json:"delta"`
	FinishReason any                     `json:"finish_reason"`
	Index        int                     `json:"index"`
	Logprobs     any                     `json:"logprobs"`
}

type openAICompatStreamChunk struct {
	ID               string                     `json:"id"`
	Object           string                     `json:"object"`
	Created          int64                      `json:"created"`
	Model            string                     `json:"model"`
	SystemFingerprint string                     `json:"system_fingerprint"`
	Usage            *openAICompatUsage         `json:"usage"`
	Choices          []openAICompatStreamChoice `json:"choices"`
}

type openAICompatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Reference any    `json:"reference,omitempty"`
}

type openAICompatCompletionChoice struct {
	Message      openAICompatMessage `json:"message"`
	Logprobs     any                 `json:"logprobs"`
	FinishReason string              `json:"finish_reason"`
	Index        int                 `json:"index"`
}

type openAICompatCompletion struct {
	ID      string                         `json:"id"`
	Object  string                         `json:"object"`
	Created int64                          `json:"created"`
	Model   string                         `json:"model"`
	Param   any                            `json:"param"`
	Usage   openAICompatUsage              `json:"usage"`
	Choices []openAICompatCompletionChoice `json:"choices"`
}

// handleOpenAICompat adapts the Agent RunEvent stream to the direct OpenAI
// chat-completion wire format. It deliberately does not use the normal
// RAGFlow response helpers because those add the {code,data,message} REST
// envelope and are not accepted by OpenAI-compatible clients.
func (h *AgentHandler) handleOpenAICompat(c *gin.Context, user *entity.User, req *agentChatCompletionsRequest) {
	question := extractLastUserContent(req.Messages)
	if req.SessionID == "" {
		req.SessionID = utility.GenerateToken()
	}

	events, err := h.chatRunner.RunAgent(
		c.Request.Context(), user.ID, req.AgentID, req.SessionID, "", question, req.Files,
	)
	if err != nil {
		writeOpenAICompatError(c, err)
		return
	}

	completionID := req.SessionID
	promptTokens := tokenizer.NumTokensFromString(question)
	if req.Stream {
		h.streamOpenAICompat(c, events, completionID, req.AgentID, promptTokens)
		return
	}

	response := collectOpenAICompatCompletion(events, completionID, req.AgentID, promptTokens)
	c.JSON(http.StatusOK, response)
}

func (h *AgentHandler) streamOpenAICompat(
	c *gin.Context,
	events <-chan canvas.RunEvent,
	completionID, model string,
	promptTokens int,
) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	completionTokens := 0
	for ev := range events {
		if ev.Type != "message" && ev.Type != "message_end" {
			continue
		}

		content := ""
		var reference any
		if ev.Data != "" {
			var data map[string]any
			if err := json.Unmarshal([]byte(ev.Data), &data); err == nil {
				if value, ok := data["content"].(string); ok {
					content = value
				}
				reference = data["reference"]
			}
		}
		completionTokens += tokenizer.NumTokensFromString(content)

		chunk := newOpenAICompatStreamChunk(completionID, model, content, nil)
		if reference != nil {
			chunk.Choices[0].Delta.Reference = reference
		}
		if err := writeOpenAICompatSSE(c, chunk); err != nil {
			return
		}
	}

	finalChunk := newOpenAICompatStreamChunk(completionID, model, nil, "stop")
	finalChunk.Usage = &openAICompatUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      promptTokens + completionTokens,
	}
	if err := writeOpenAICompatSSE(c, finalChunk); err != nil {
		return
	}
	_, _ = c.Writer.Write([]byte("data: [DONE]\n\n"))
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func collectOpenAICompatCompletion(
	events <-chan canvas.RunEvent,
	completionID, model string,
	promptTokens int,
) openAICompatCompletion {
	content := ""
	completionTokens := 0
	var reference any
	for ev := range events {
		if ev.Type != "message" && ev.Type != "message_end" {
			continue
		}

		if ev.Data == "" {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(ev.Data), &data); err != nil {
			continue
		}
		if ev.Type == "message" {
			if value, ok := data["content"].(string); ok {
				content += value
			}
		}
		if value := data["reference"]; value != nil {
			reference = value
		}
	}
	completionTokens = tokenizer.NumTokensFromString(content)

	message := openAICompatMessage{Role: "assistant", Content: content}
	if reference != nil {
		message.Reference = reference
	}
	return openAICompatCompletion{
		ID:      completionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Param:   nil,
		Usage: openAICompatUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
		Choices: []openAICompatCompletionChoice{{
			Message:      message,
			Logprobs:     nil,
			FinishReason: "stop",
			Index:        0,
		}},
	}
}

func newOpenAICompatStreamChunk(
	completionID, model string, content any, finishReason any,
) openAICompatStreamChunk {
	return openAICompatStreamChunk{
		ID:                completionID,
		Object:            "chat.completion.chunk",
		Created:           time.Now().Unix(),
		Model:             model,
		SystemFingerprint: "",
		Usage:             nil,
		Choices: []openAICompatStreamChoice{{
			Delta: openAICompatStreamDelta{
				Content:      content,
				Role:         "assistant",
				FunctionCall: nil,
				ToolCalls:    nil,
			},
			FinishReason: finishReason,
			Index:        0,
			Logprobs:     nil,
		}},
	}
}

func writeOpenAICompatSSE(c *gin.Context, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := c.Writer.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := c.Writer.Write(data); err != nil {
		return err
	}
	if _, err := c.Writer.Write([]byte("\n\n")); err != nil {
		return err
	}
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

func writeOpenAICompatError(c *gin.Context, err error) {
	_, message := mapAgentError(err)
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "server_error",
		},
	})
}
