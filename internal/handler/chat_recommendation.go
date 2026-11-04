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

	"go.uber.org/zap"
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

	var req ChatRecommendationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeArgumentError, "question is required")
		return
	}
	if strings.TrimSpace(req.Question) == "" {
		common.RespondError(c, common.CodeArgumentError, "question is required")
		return
	}
	ctx := c.Request.Context()
	questions, err := service.GenerateRelatedQuestions(ctx, req.Question, req.SearchID, h.searchSvc, h.modelFactory)
	if err != nil {
		common.Warn("chat recommendation failed", zap.String("error", err.Error()))
		common.RespondError(c, common.CodeOperatingError, err.Error())
		return
	}

	common.RespondSuccessList(c, questions, "success")
}
