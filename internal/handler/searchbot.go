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
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service"

	"go.uber.org/zap"
)

// searchbotLLM is the interface for LLM calls used by SearchbotHandler.
type searchbotLLM interface {
	Chat(tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (*modelModule.ChatResponse, error)
}

// ChunkServiceIface abstracts chunk retrieval for the searchbots handler.
type ChunkServiceIface interface {
	RetrievalTest(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error)
}

// SearchbotRealLLM wraps ModelProviderService to implement searchbotLLM.
type SearchbotRealLLM struct {
	Svc *service.ModelProviderService
}

func (r *SearchbotRealLLM) Chat(tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (*modelModule.ChatResponse, error) {
	chatModel, err := r.Svc.GetChatModel(tenantID, modelID)
	if err != nil {
		return nil, err
	}
	return chatModel.ModelDriver.ChatWithMessages(*chatModel.ModelName, messages, chatModel.APIConfig, config)
}

// SearchbotRetrievalTestRequest is the request body for POST /api/v1/searchbots/retrieval_test.
type SearchbotRetrievalTestRequest struct {
	KbIDs                  []string                `json:"kb_id" binding:"required"`
	Question               string                  `json:"question" binding:"required"`
	Page                   *int                    `json:"page,omitempty"`
	Size                   *int                    `json:"size,omitempty"`
	DocIDs                 []string                `json:"doc_ids,omitempty"`
	UseKG                  *bool                   `json:"use_kg,omitempty"`
	TopK                   *int                    `json:"top_k,omitempty"`
	CrossLanguages         []string                `json:"cross_languages,omitempty"`
	SearchID               *string                 `json:"search_id,omitempty"`
	MetaDataFilter         *map[string]interface{} `json:"meta_data_filter,omitempty"`
	TenantRerankID         *string                 `json:"tenant_rerank_id,omitempty"`
	RerankID               *string                 `json:"rerank_id,omitempty"`
	Keyword                *bool                   `json:"keyword,omitempty"`
	SimilarityThreshold    *float64                `json:"similarity_threshold,omitempty"`
	VectorSimilarityWeight *float64                `json:"vector_similarity_weight,omitempty"`
	Highlight              *bool                   `json:"highlight,omitempty"`
}

// SearchbotRequest is the request body for POST /api/v1/searchbots/related_questions.
type SearchbotRequest struct {
	Question string `json:"question" binding:"required"`
	SearchID string `json:"search_id,omitempty"`
}

// SearchbotHandler handles POST /api/v1/searchbots/related_questions.
type SearchbotHandler struct {
	searchSvc *service.SearchService
	tenantSvc *service.TenantService
	llm       searchbotLLM
	chunkSvc  ChunkServiceIface
}

// NewSearchbotHandler creates a new SearchbotHandler.
func NewSearchbotHandler(searchSvc *service.SearchService, tenantSvc *service.TenantService, llm searchbotLLM, chunkSvc ChunkServiceIface) *SearchbotHandler {
	return &SearchbotHandler{searchSvc: searchSvc, tenantSvc: tenantSvc, llm: llm, chunkSvc: chunkSvc}
}

// Handle generates related search questions based on a user query.
// @Summary Generate Related Questions
// @Description Generates 5-10 related search questions to expand the search scope.
// @Tags searchbots
// @Accept json
// @Produce json
// @Param request body SearchbotRequest true "Request body"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/searchbots/related_questions [post]
func (h *SearchbotHandler) Handle(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req SearchbotRequest
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
	if modelID == "" && h.tenantSvc != nil {
		defaultModel, err := h.tenantSvc.GetDefaultModelName(user.ID, entity.ModelTypeChat)
		if err == nil && defaultModel != "" {
			modelID = defaultModel
		}
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
		common.Warn("searchbot LLM call failed", zap.String("error", err.Error()))
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeOperatingError,
			"data":    nil,
			"message": "LLM call failed",
		})
		return
	}

	var questions []string
	if response != nil && response.Answer != nil {
		questions = parseRelatedQuestions(*response.Answer)
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    questions,
		"message": "",
	})
}

// RetrievalTest handles POST /api/v1/searchbots/retrieval_test.
func (h *SearchbotHandler) RetrievalTest(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		c.JSON(http.StatusUnauthorized, gin.H{"code": errorCode, "message": errorMessage})
		return
	}

	var req SearchbotRetrievalTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeArgumentError, "message": "invalid request body"})
		return
	}

	if len(req.KbIDs) == 0 || req.Question == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeArgumentError, "message": "kb_id and question are required"})
		return
	}

	svcReq := &service.RetrievalTestRequest{
		Datasets:               req.KbIDs,
		Question:               req.Question,
		Page:                   req.Page,
		Size:                   req.Size,
		DocIDs:                 req.DocIDs,
		UseKG:                  req.UseKG,
		TopK:                   req.TopK,
		CrossLanguages:         req.CrossLanguages,
		SearchID:               req.SearchID,
		Filter:                 h.resolveMetaDataFilter(req.MetaDataFilter),
		TenantRerankID:         req.TenantRerankID,
		RerankID:               req.RerankID,
		Keyword:                req.Keyword,
		SimilarityThreshold:    req.SimilarityThreshold,
		VectorSimilarityWeight: req.VectorSimilarityWeight,
	}

	result, err := h.chunkSvc.RetrievalTest(svcReq, user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "not_found") {
			c.JSON(http.StatusNotFound, gin.H{"code": common.CodeNotFound, "message": "No chunk found! Check the chunk status please!"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result, "message": "success"})
}

// resolveMetaDataFilter extracts the metadata filter map from the request.
func (h *SearchbotHandler) resolveMetaDataFilter(f *map[string]interface{}) map[string]interface{} {
	if f != nil {
		return *f
	}
	return nil
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
