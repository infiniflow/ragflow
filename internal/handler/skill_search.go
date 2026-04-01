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
	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// SkillSearchHandler handles skill search HTTP requests
type SkillSearchHandler struct {
	searchService  *service.SkillSearchService
	indexerService *service.SkillIndexerService
	docEngine      engine.DocEngine
}

// NewSkillSearchHandler creates a new skill search handler
func NewSkillSearchHandler(docEngine engine.DocEngine) *SkillSearchHandler {
	return &SkillSearchHandler{
		searchService:  service.NewSkillSearchService(),
		indexerService: service.NewSkillIndexerService(),
		docEngine:      docEngine,
	}
}

// GetConfig handles the get skill search config request
// @Summary Get Skill Search Config
// @Description Get the search configuration for skills
// @Tags skill-search
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param embd_id query string true "Embedding Model ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/skills/config [get]
func (h *SkillSearchHandler) GetConfig(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	embdID := c.Query("embd_id")

	result, code, err := h.searchService.GetConfig(user.ID, embdID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// UpdateConfig handles the update skill search config request
// @Summary Update Skill Search Config
// @Description Update the search configuration for skills
// @Tags skill-search
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body service.UpdateConfigRequest true "config info"
// @Success 200 {object} map[string]interface{}
// @Router /v1/skills/config [post]
func (h *SkillSearchHandler) UpdateConfig(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.UpdateConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	req.TenantID = user.ID

	result, code, err := h.searchService.UpdateConfig(&req)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// Search handles the skill search request
// @Summary Search Skills
// @Description Search skills using configured search strategy
// @Tags skill-search
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body service.SearchRequest true "search query"
// @Success 200 {object} map[string]interface{}
// @Router /v1/skills/search [post]
func (h *SkillSearchHandler) Search(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	req.TenantID = user.ID

	result, code, err := h.searchService.Search(c.Request.Context(), &req, h.docEngine)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// IndexSkillsRequest represents the request to index skills
type IndexSkillsRequest struct {
	Skills []service.SkillInfo `json:"skills" binding:"required"`
	EmbdID string              `json:"embd_id"` // Optional, will use config's embd_id if empty
}

// IndexSkills handles the index skills request
// @Summary Index Skills
// @Description Index skills for search. If embd_id is not provided, will use the one from skill search config.
// @Tags skill-search
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body IndexSkillsRequest true "skills to index"
// @Success 200 {object} map[string]interface{}
// @Router /v1/skills/index [post]
func (h *SkillSearchHandler) IndexSkills(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req IndexSkillsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	// If embd_id not provided, get from skill search config
	embdID := req.EmbdID
	if embdID == "" {
		config, code, err := h.searchService.GetConfig(user.ID, "")
		if err != nil {
			jsonError(c, code, "failed to get skill search config: "+err.Error())
			return
		}
		embdID = config["embd_id"].(string)
		if embdID == "" {
			jsonError(c, common.CodeDataError, "no embedding model configured in skill search config")
			return
		}
	}

	// Ensure index exists
	if err := h.indexerService.EnsureIndex(c.Request.Context(), user.ID, h.docEngine, embdID); err != nil {
		jsonError(c, common.CodeOperatingError, err.Error())
		return
	}

	if err := h.indexerService.BatchIndexSkills(c.Request.Context(), user.ID, req.Skills, h.docEngine, embdID); err != nil {
		jsonError(c, common.CodeOperatingError, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, gin.H{
		"indexed_count": len(req.Skills),
	}, "success")
}

// ReindexRequest represents the request to reindex skills
type ReindexRequest struct {
	Skills []service.SkillInfo `json:"skills" binding:"required"`
	EmbdID string              `json:"embd_id"` // Optional, will use config's embd_id if empty
}

// Reindex handles the reindex all skills request
// @Summary Reindex All Skills
// @Description Reindex all skills for a tenant. If embd_id is not provided, will use the one from skill search config.
// @Tags skill-search
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body ReindexRequest true "skills to reindex"
// @Success 200 {object} map[string]interface{}
// @Router /v1/skills/reindex [post]
func (h *SkillSearchHandler) Reindex(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req ReindexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	// If embd_id not provided, get from skill search config
	embdID := req.EmbdID
	if embdID == "" {
		config, code, err := h.searchService.GetConfig(user.ID, "")
		if err != nil {
			jsonError(c, code, "failed to get skill search config: "+err.Error())
			return
		}
		embdID = config["embd_id"].(string)
		if embdID == "" {
			jsonError(c, common.CodeDataError, "no embedding model configured in skill search config")
			return
		}
	}

	result, err := h.indexerService.ReindexAll(c.Request.Context(), user.ID, req.Skills, h.docEngine, embdID)
	if err != nil {
		jsonError(c, common.CodeOperatingError, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// DeleteSkillIndex handles the delete skill index request
// @Summary Delete Skill Index
// @Description Delete a skill's search index
// @Tags skill-search
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param skill_id path string true "Skill ID (skill name)"
// @Success 200 {object} map[string]interface{}
// @Router /v1/skills/index/{skill_id} [delete]
func (h *SkillSearchHandler) DeleteSkillIndex(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	skillID := c.Param("skill_id")
	if skillID == "" {
		jsonError(c, common.CodeDataError, "skill_id is required")
		return
	}

	err := h.indexerService.DeleteSkillIndex(c.Request.Context(), user.ID, skillID, h.docEngine)
	if err != nil {
		jsonError(c, common.CodeOperatingError, "failed to delete skill index")
		return
	}

	jsonResponse(c, common.CodeSuccess, true, "success")
}

// InitializeIndex handles the initialize skill search index request
// @Summary Initialize Skill Search Index
// @Description Initialize the skill search index for a tenant
// @Tags skill-search
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param embd_id query string true "Embedding Model ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/skill/search/init [post]
func (h *SkillSearchHandler) InitializeIndex(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	embdID := c.Query("embd_id")
	if embdID == "" {
		jsonError(c, common.CodeDataError, "embd_id is required")
		return
	}

	if err := h.indexerService.InitializeIndex(c.Request.Context(), user.ID, h.docEngine, embdID); err != nil {
		jsonError(c, common.CodeOperatingError, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, gin.H{"initialized": true}, "success")
}
