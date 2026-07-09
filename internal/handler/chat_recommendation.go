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
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

// ChatRecommendationRequest is the request body for POST /api/v1/chat/recommendation.
type ChatRecommendationRequest struct {
	Question string `json:"question" binding:"required"`
	SearchID string `json:"search_id,omitempty"`
}

// Recommendation generates related search questions for a chat query.
// @Summary Generate Chat Recommendations
// @Description Generates related questions using the chat model configured by search_config.chat_id or the tenant default.
// @Tags chat
// @Accept json
// @Produce json
// @Param request body ChatRecommendationRequest true "Recommendation parameters"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/chat/recommendation [post]
func (h *ChatHandler) Recommendation(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req ChatRecommendationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "question is required")
		return
	}
	if strings.TrimSpace(req.Question) == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "question is required")
		return
	}
	questions, err := service.GenerateRelatedQuestions(user.ID, req.Question, req.SearchID, h.searchSvc, h.tenantSvc, h.llm)
	if err != nil {
		jsonInternalError(c, err)
		return
	}

	common.SuccessWithData(c, questions, "success")
}
