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
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/tokenizer"
)

type agentOpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type agentOpenAICompletionState struct {
	content   strings.Builder
	reference []interface{}
	usage     agentOpenAIUsage
	usageSeen bool
}

func (s *agentOpenAICompletionState) apply(event canvas.RunEvent) (string, []interface{}, error) {
	switch event.Type {
	case "message":
		var message canvas.MessageEvent
		if err := json.Unmarshal([]byte(event.Data), &message); err != nil {
			return "", nil, fmt.Errorf("decode agent message event: %w", err)
		}
		s.content.WriteString(message.Content)
		if len(message.Reference) > 0 {
			s.reference = append([]interface{}(nil), message.Reference...)
		}
		return message.Content, message.Reference, nil
	case "message_end":
		var messageEnd canvas.MessageEndEvent
		if err := json.Unmarshal([]byte(event.Data), &messageEnd); err != nil {
			return "", nil, fmt.Errorf("decode agent message_end event: %w", err)
		}
		if len(messageEnd.Reference) > 0 {
			s.reference = append([]interface{}(nil), messageEnd.Reference...)
		}
		return "", messageEnd.Reference, nil
	case "workflow_finished":
		var finished struct {
			Usage *agentOpenAIUsage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(event.Data), &finished); err != nil {
			return "", nil, fmt.Errorf("decode agent workflow_finished event: %w", err)
		}
		if finished.Usage != nil {
			s.usage = *finished.Usage
			s.usageSeen = true
		}
	case "error":
		var runError canvas.ErrorEvent
		if err := json.Unmarshal([]byte(event.Data), &runError); err != nil {
			return "", nil, fmt.Errorf("decode agent error event: %w", err)
		}
		content := "**ERROR**: " + runError.Message
		s.content.WriteString(content)
		return content, nil, nil
	}
	return "", nil, nil
}

func (s *agentOpenAICompletionState) finalUsage(question string) agentOpenAIUsage {
	if !s.usageSeen {
		s.usage.PromptTokens = tokenizer.NumTokensFromString(question)
		s.usage.CompletionTokens = tokenizer.NumTokensFromString(s.content.String())
		s.usage.TotalTokens = s.usage.PromptTokens + s.usage.CompletionTokens
	} else if s.usage.TotalTokens == 0 {
		s.usage.TotalTokens = s.usage.PromptTokens + s.usage.CompletionTokens
	}
	return s.usage
}

func writeOpenAIAgentCompletion(
	c *gin.Context,
	request agentChatCompletionsRequest,
	events <-chan canvas.RunEvent,
	question string,
) error {
	completionID := request.SessionID
	if completionID == "" {
		completionID = uuid.NewString()
	}
	if request.Stream {
		return streamOpenAIAgentCompletion(c, events, completionID, request.AgentID, question)
	}

	state := &agentOpenAICompletionState{}
	for event := range events {
		if _, _, err := state.apply(event); err != nil {
			state.content.WriteString("**ERROR**: " + err.Error())
		}
	}

	message := gin.H{
		"role":    "assistant",
		"content": state.content.String(),
	}
	if len(state.reference) > 0 {
		message["reference"] = state.reference
	}
	usage := state.finalUsage(question)
	c.JSON(http.StatusOK, gin.H{
		"id":      completionID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   request.AgentID,
		"param":   nil,
		"usage": gin.H{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
			"completion_tokens_details": gin.H{
				"reasoning_tokens":           0,
				"accepted_prediction_tokens": 0,
				"rejected_prediction_tokens": 0,
			},
		},
		"choices": []gin.H{{
			"message":       message,
			"logprobs":      nil,
			"finish_reason": "stop",
			"index":         0,
		}},
	})
	return nil
}

func streamOpenAIAgentCompletion(
	c *gin.Context,
	events <-chan canvas.RunEvent,
	completionID string,
	agentID string,
	question string,
) error {
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Content-Type", "text/event-stream; charset=utf-8")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported")
	}
	writeChunk := func(delta gin.H, finishReason interface{}, usage interface{}) error {
		body, err := json.Marshal(gin.H{
			"id":      completionID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   agentID,
			"usage":   usage,
			"choices": []gin.H{{
				"delta":         delta,
				"finish_reason": finishReason,
				"index":         0,
			}},
		})
		if err != nil {
			return err
		}
		if _, err := c.Writer.Write(append(append([]byte("data: "), body...), []byte("\n\n")...)); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	state := &agentOpenAICompletionState{}
	for event := range events {
		content, reference, err := state.apply(event)
		if err != nil {
			content = "**ERROR**: " + err.Error()
			state.content.WriteString(content)
		}
		if content == "" && len(reference) == 0 {
			continue
		}
		delta := gin.H{"content": content}
		if len(reference) > 0 {
			delta["reference"] = reference
		}
		if err := writeChunk(delta, nil, nil); err != nil {
			return err
		}
	}

	usage := state.finalUsage(question)
	finalDelta := gin.H{"content": nil}
	if len(state.reference) > 0 {
		finalDelta["reference"] = state.reference
	}
	if err := writeChunk(finalDelta, "stop", usage); err != nil {
		return err
	}
	if _, err := c.Writer.Write([]byte("data: [DONE]\n\n")); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
