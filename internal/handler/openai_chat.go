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
	"io"
	"ragflow/internal/common"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

type OpenAIChatHandler struct {
	svc *service.OpenAIChatService
}

func NewOpenAIChatHandler(svc *service.OpenAIChatService) *OpenAIChatHandler {
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

	// Messages presence
	if len(req.Messages) == 0 {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "You have to provide messages.")
		return
	}

	// extra_body shape validation
	extraBody, extraBodyOK := req.ExtraBody.(map[string]interface{})
	if req.ExtraBody != nil && !extraBodyOK {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "extra_body must be an object.")
		return
	}

	// reference_metadata shape validation
	if extraBody != nil {
		if rm, ok := extraBody["reference_metadata"].(map[string]interface{}); ok {
			if rawFields, has := rm["fields"]; has {
				if rawArr, ok := rawFields.([]interface{}); !ok {
					common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
						"reference_metadata.fields must be an array.")
					return
				} else {
					for _, item := range rawArr {
						if _, ok := item.(string); !ok {
							common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
								"reference_metadata.fields must be an array.")
							return
						}
					}
				}
			}
		}
	}

	// metadata_condition shape validation
	if extraBody != nil {
		if mc, ok := extraBody["metadata_condition"]; ok && mc != nil {
			if _, ok := mc.(map[string]interface{}); !ok {
				common.ResponseWithCodeData(c, common.CodeArgumentError, nil,
					"metadata_condition must be an object.")
				return
			}
		}
	}

	// Last message must be from the user
	if last := req.Messages[len(req.Messages)-1]; last != nil {
		if role, _ := last["role"].(string); role != "user" {
			common.ResponseWithCodeData(c, common.CodeDataError, nil, "The last content of this conversation is not from user.")
			return
		}
	}

	// All early-rejection checks passed. Delegate to the service for the
	// actual LLM call.
	h.svc.OpenAIChatCompletions(c, user.ID, chatID, bodyBytes)
}
