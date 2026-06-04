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
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service"
)

// relatedQuestionLLM is the interface for LLM calls used by RelatedQuestionsHandler.
type relatedQuestionLLM interface {
	Chat(tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (*modelModule.ChatResponse, error)
}

// RelatedQuestionsRealLLM wraps ModelProviderService to implement relatedQuestionLLM.
type RelatedQuestionsRealLLM struct {
	Svc *service.ModelProviderService
}

func (r *RelatedQuestionsRealLLM) Chat(tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (*modelModule.ChatResponse, error) {
	chatModel, err := r.Svc.GetChatModel(tenantID, modelID)
	if err != nil {
		return nil, err
	}
	return chatModel.ModelDriver.ChatWithMessages(*chatModel.ModelName, messages, chatModel.APIConfig, config)
}

// RelatedQuestionsRequest is the request body for POST /api/v1/searchbots/related_questions.
type RelatedQuestionsRequest struct {
	Question string `json:"question" binding:"required"`
	SearchID string `json:"search_id,omitempty"`
}

// RelatedQuestionsHandler handles POST /api/v1/searchbots/related_questions.
type RelatedQuestionsHandler struct {
	searchSvc *service.SearchService
	llm       relatedQuestionLLM
}

// NewRelatedQuestionsHandler creates a new RelatedQuestionsHandler.
func NewRelatedQuestionsHandler(searchSvc *service.SearchService, llm relatedQuestionLLM) *RelatedQuestionsHandler {
	return &RelatedQuestionsHandler{searchSvc: searchSvc, llm: llm}
}

// Handle generates related search questions based on a user query.
// @Summary Generate Related Questions
// @Description Generates 5-10 related search questions to expand the search scope.
// @Tags searchbots
// @Accept json
// @Produce json
// @Param request body RelatedQuestionsRequest true "Request body"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/searchbots/related_questions [post]
func (h *RelatedQuestionsHandler) Handle(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req RelatedQuestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "question is required",
		})
		return
	}

	if req.Question == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "question is required",
		})
		return
	}

	// Resolve model ID from search config if provided
	modelID := ""
	if req.SearchID != "" && h.searchSvc != nil {
		if detail, err := h.searchSvc.GetDetail(req.SearchID); err == nil {
			if sc, ok := detail["search_config"].(map[string]interface{}); ok {
				if cid, ok := sc["chat_id"].(string); ok && cid != "" {
					modelID = cid
				}
			}
		}
	}
	if modelID == "" {
		modelID = user.ID
	}

	messages := []modelModule.Message{
		{Role: "system", Content: relatedQuestionPrompt},
		{Role: "user", Content: "Keywords: " + req.Question + "\nRelated search terms:\n"},
	}

	genConf := &modelModule.ChatConfig{
		Temperature: ptrFloat64(0.9),
	}

	response, err := h.llm.Chat(user.ID, modelID, messages, genConf)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeOperatingError,
			"data":    nil,
			"message": "LLM call failed: " + err.Error(),
		})
		return
	}

	questions := parseRelatedQuestions(*response.Answer)
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    questions,
		"message": "",
	})
}

// ptrFloat64 returns a pointer to a float64 value.
func ptrFloat64(v float64) *float64 { return &v }

// parseRelatedQuestions extracts numbered list items from an LLM response.
// Lines matching "^N. " are extracted and the number prefix is stripped.
func parseRelatedQuestions(text string) []string {
	lineRe := regexp.MustCompile(`^\d+\.\s`)
	var result []string
	for _, line := range strings.Split(text, "\n") {
		if lineRe.MatchString(line) {
			result = append(result, lineRe.ReplaceAllString(line, ""))
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}

// relatedQuestionPrompt is the system prompt for generating related search questions.
// Matches Python rag/prompts/related_question.md
const relatedQuestionPrompt = `# Role
You are an AI language model assistant tasked with generating **5-10 related questions** based on a user's original query.
These questions should help **expand the search query scope** and **improve search relevance**.

---

## Instructions

**Input:**
You are provided with a **user's question**.

**Output:**
Generate **5-10 alternative questions** that are **related** to the original user question.
These alternatives should help retrieve a **broader range of relevant documents** from a vector database.

**Context:**
Focus on **rephrasing** the original question in different ways, ensuring the alternative questions are **diverse but still connected** to the topic of the original query.
Do **not** create overly obscure, irrelevant, or unrelated questions.

**Fallback:**
If you cannot generate any relevant alternatives, do **not** return any questions.

---

## Guidance

1. Each alternative should be **unique** but still **relevant** to the original query.
2. Keep the phrasing **clear, concise, and easy to understand**.
3. Avoid overly technical jargon or specialized terms **unless directly relevant**.
4. Ensure that each question **broadens** the search angle, **not narrows** it.

---

## Example

**Original Question:**
> What are the benefits of electric vehicles?

**Alternative Questions:**
1. How do electric vehicles impact the environment?
2. What are the advantages of owning an electric car?
3. What is the cost-effectiveness of electric vehicles?
4. How do electric vehicles compare to traditional cars in terms of fuel efficiency?
5. What are the environmental benefits of switching to electric cars?
6. How do electric vehicles help reduce carbon emissions?
7. Why are electric vehicles becoming more popular?
8. What are the long-term savings of using electric vehicles?
9. How do electric vehicles contribute to sustainability?
10. What are the key benefits of electric vehicles for consumers?

---

## Reason
Rephrasing the original query into multiple alternative questions helps the user explore **different aspects** of their search topic, improving the **quality of search results**.
These questions guide the search engine to provide a **more comprehensive set** of relevant documents.`
