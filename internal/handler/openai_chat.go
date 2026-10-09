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
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/service"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type openAIChatService interface {
	Complete(ctx context.Context, userID, chatID string, req service.OpenAIChatRequest) (*service.OpenAICompletionResponse, error)
	Stream(ctx context.Context, userID, chatID string, req service.OpenAIChatRequest) (*service.OpenAIChatStream, error)
}

type OpenAIChatHandler struct {
	svc openAIChatService
}

func NewOpenAIChatHandler(svc openAIChatService) *OpenAIChatHandler {
	return &OpenAIChatHandler{svc: svc}
}

// OpenAIChatCompletions handles the OpenAI-compatible chat completions route.
// @Summary OpenAI Chat Completions
// @Description OpenAI-compatible chat completions endpoint
// @Tags openai
// @Accept json
// @Produce json
// @Param chat_id path string true "dialog id"
// @Param request body service.OpenAIChatRequest true "chat completion request"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/openai/{chat_id}/chat/completions [post]
func (h *OpenAIChatHandler) OpenAIChatCompletions(c *gin.Context) {
	chatID := c.Param("chat_id")
	if chatID == "" {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "You don't own the chat "+chatID)
		return
	}

	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}

	// Parse body into the typed request
	var req service.OpenAIChatRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}

	// Long agentic runs stream for minutes or compute before their single
	// write; clear http.Server.WriteTimeout so neither is cut off mid-response.
	clearResponseWriteDeadline(c)

	completionID := "chatcmpl-" + chatID
	if req.Stream != nil && *req.Stream {
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		stream, err := h.svc.Stream(ctx, user.ID, chatID, req)
		if err != nil {
			writeOpenAIChatError(c, err)
			return
		}
		if err := writeOpenAIChatSSE(ctx, c, stream, completionID); err != nil {
			cancel()
			if !errors.Is(err, context.Canceled) && c.Request.Context().Err() == nil {
				if !c.Writer.Written() {
					writeOpenAIChatError(c, common.NewCodedError(common.CodeDataError, err.Error()))
					return
				}
				common.Warn("OpenAI chat stream writer failed", zap.Error(err))
			}
		}
		return
	}

	resp, err := h.svc.Complete(c.Request.Context(), user.ID, chatID, req)
	if err != nil {
		writeOpenAIChatError(c, err)
		return
	}
	writeOpenAICompletion(c, completionID, resp)
}

func writeOpenAIChatError(c *gin.Context, err error) {
	if err == nil || c.Request.Context().Err() != nil {
		return
	}
	var codedErr *common.CodedError
	if errors.As(err, &codedErr) {
		common.ResponseWithCodeData(c, codedErr.Code, nil, codedErr.Message)
		return
	}
	jsonInternalError(c, err)
}

func writeOpenAICompletion(c *gin.Context, completionID string, resp *service.OpenAICompletionResponse) {
	choices := []gin.H{{
		"index":         0,
		"finish_reason": "stop",
		"logprobs":      nil,
		"message": gin.H{
			"role":    "assistant",
			"content": resp.Content,
		},
	}}
	if resp.Reference != nil {
		choices[0]["message"].(gin.H)["reference"] = resp.Reference
	}

	c.JSON(http.StatusOK, gin.H{
		"id":      completionID,
		"object":  "chat.completion",
		"created": getOpenAICreatedOrDefault(resp.Created),
		"model":   resp.Model,
		"usage": gin.H{
			"prompt_tokens":     resp.PromptTokens,
			"completion_tokens": resp.CompletionTokens,
			"total_tokens":      resp.TotalTokens,
			"completion_tokens_details": gin.H{
				"reasoning_tokens":           resp.ContextTokens,
				"accepted_prediction_tokens": resp.CompletionTokens,
				"rejected_prediction_tokens": 0,
			},
		},
		"choices": choices,
	})
}

func getOpenAICreatedOrDefault(created *int64) int64 {
	if created != nil {
		return *created
	}
	return time.Now().Unix()
}
