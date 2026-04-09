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
	"fmt"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/logger"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// SkillSearchHandler handles skill search HTTP requests
type SkillSearchHandler struct {
	searchService  *service.SkillSearchService
	indexerService *service.SkillIndexerService
	spaceService   *service.SkillSpaceService
	docEngine      engine.DocEngine
}

// NewSkillSearchHandler creates a new skill search handler
func NewSkillSearchHandler(docEngine engine.DocEngine) *SkillSearchHandler {
	return &SkillSearchHandler{
		searchService:  service.NewSkillSearchService(),
		indexerService: service.NewSkillIndexerService(),
		spaceService:   service.NewSkillSpaceService(),
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
// @Param space_id query string false "Skill Space ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/skills/config [get]
func (h *SkillSearchHandler) GetConfig(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	embdID := c.Query("embd_id")
	spaceID := c.Query("space_id")

	result, code, err := h.searchService.GetConfig(user.ID, spaceID, embdID)
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
	SpaceID string             `json:"space_id"`
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
		config, code, err := h.searchService.GetConfig(user.ID, req.SpaceID, "")
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

	// Ensure index exists before indexing (for both ES and Infinity)
	logger.Info("Ensuring skill index exists before indexing",
		zap.String("tenantID", user.ID),
		zap.String("spaceID", req.SpaceID),
		zap.String("engineType", h.docEngine.GetType()),
		zap.Int("skillCount", len(req.Skills)))

	if h.docEngine.GetType() == "elasticsearch" {
		if err := h.indexerService.EnsureIndex(c.Request.Context(), user.ID, req.SpaceID, h.docEngine, embdID); err != nil {
			jsonError(c, common.CodeOperatingError, err.Error())
			return
		}
	}

	if err := h.indexerService.BatchIndexSkills(c.Request.Context(), user.ID, req.SpaceID, req.Skills, h.docEngine, embdID); err != nil {
		logger.Error(fmt.Sprintf("Failed to batch index skills: tenantID=%s, spaceID=%s, error=%v", user.ID, req.SpaceID, err), err)
		jsonError(c, common.CodeOperatingError, err.Error())
		return
	}

	logger.Info("Successfully indexed skills",
		zap.String("tenantID", user.ID),
		zap.String("spaceID", req.SpaceID),
		zap.Int("indexedCount", len(req.Skills)))

	jsonResponse(c, common.CodeSuccess, gin.H{
		"indexed_count": len(req.Skills),
	}, "success")
}

// ReindexRequest represents the request to reindex skills
type ReindexRequest struct {
	SpaceID string `json:"space_id" binding:"required"`
	EmbdID  string `json:"embd_id"` // Optional, will use config's embd_id if empty
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
		config, code, err := h.searchService.GetConfig(user.ID, req.SpaceID, "")
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

	result, err := h.indexerService.ReindexAll(c.Request.Context(), user.ID, req.SpaceID, h.docEngine, embdID)
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
// @Param skill_id query string true "Skill ID (skill name)"
// @Param space_id query string true "Space ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/skills/index [delete]
func (h *SkillSearchHandler) DeleteSkillIndex(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	skillID := c.Query("skill_id")
	spaceID := c.Query("space_id")
	if skillID == "" {
		jsonError(c, common.CodeDataError, "skill_id is required")
		return
	}

	err := h.indexerService.DeleteSkillIndex(c.Request.Context(), user.ID, spaceID, skillID, h.docEngine)
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
// @Param space_id query string false "Skill Space ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/skill/search/init [post]
func (h *SkillSearchHandler) InitializeIndex(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	embdID := c.Query("embd_id")
	spaceID := c.Query("space_id")
	if embdID == "" {
		jsonError(c, common.CodeDataError, "embd_id is required")
		return
	}

	if err := h.indexerService.InitializeIndex(c.Request.Context(), user.ID, spaceID, h.docEngine, embdID); err != nil {
		jsonError(c, common.CodeOperatingError, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, gin.H{"initialized": true}, "success")
}

// ==================== Skill Space Management ====================

// ListSpaces handles the list skill spaces request
// @Summary List Skill Spaces
// @Description List all skill spaces for the current tenant
// @Tags skill-space
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/skills/spaces [get]
func (h *SkillSearchHandler) ListSpaces(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	result, code, err := h.spaceService.ListSpaces(user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// CreateSpaceRequest represents the request to create a skill space
type CreateSpaceRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	EmbdID      string `json:"embd_id"`
	RerankID    string `json:"rerank_id"`
}

// CreateSpace handles the create skill space request
// @Summary Create Skill Space
// @Description Create a new skill space with associated folder
// @Tags skill-space
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param request body CreateSpaceRequest true "space info"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/skills/spaces [post]
func (h *SkillSearchHandler) CreateSpace(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req CreateSpaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	result, code, err := h.spaceService.CreateSpace(&service.CreateSpaceRequest{
		TenantID:    user.ID,
		Name:        req.Name,
		Description: req.Description,
		EmbdID:      req.EmbdID,
		RerankID:    req.RerankID,
	})
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// GetSpace handles the get skill space request
// @Summary Get Skill Space
// @Description Get a skill space by ID
// @Tags skill-space
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param space_id path string true "Space ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/skills/spaces/{space_id} [get]
func (h *SkillSearchHandler) GetSpace(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	spaceID := c.Param("space_id")
	if spaceID == "" {
		jsonError(c, common.CodeDataError, "space_id is required")
		return
	}

	result, code, err := h.spaceService.GetSpace(spaceID, user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// UpdateSpaceRequest represents the request to update a skill space
type UpdateSpaceRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	EmbdID      string `json:"embd_id"`
	RerankID    string `json:"rerank_id"`
	TopK        int    `json:"top_k"`
}

// UpdateSpace handles the update skill space request
// @Summary Update Skill Space
// @Description Update a skill space
// @Tags skill-space
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param space_id path string true "Space ID"
// @Param request body UpdateSpaceRequest true "space updates"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/skills/spaces/{space_id} [put]
func (h *SkillSearchHandler) UpdateSpace(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	spaceID := c.Param("space_id")
	if spaceID == "" {
		jsonError(c, common.CodeDataError, "space_id is required")
		return
	}

	var req UpdateSpaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	result, code, err := h.spaceService.UpdateSpace(spaceID, user.ID, &service.UpdateSpaceRequest{
		Name:        req.Name,
		Description: req.Description,
		EmbdID:      req.EmbdID,
		RerankID:    req.RerankID,
		TopK:        req.TopK,
	})
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}

// DeleteSpace handles the delete skill space request
// @Summary Delete Skill Space
// @Description Delete a skill space and its associated folder
// @Tags skill-space
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param space_id path string true "Space ID"
// @Success 202 {object} map[string]interface{}
// @Router /api/v1/skills/spaces/{space_id} [delete]
func (h *SkillSearchHandler) DeleteSpace(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	spaceID := c.Param("space_id")
	if spaceID == "" {
		jsonError(c, common.CodeDataError, "space_id is required")
		return
	}

	// Get Authorization header for Python API calls
	authHeader := c.GetHeader("Authorization")

	code, err := h.spaceService.DeleteSpace(spaceID, user.ID, h.docEngine, authHeader)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	// Return 202 Accepted since deletion is async
	c.JSON(http.StatusAccepted, gin.H{
		"code":    0,
		"data":    gin.H{"deleting": true, "space_id": spaceID},
		"message": "success",
	})
}

// GetSpaceByFolder handles the get skill space by folder ID request
// @Summary Get Skill Space by Folder
// @Description Get a skill space by its folder ID
// @Tags skill-space
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param folder_id query string true "Folder ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/skills/space/by-folder [get]
func (h *SkillSearchHandler) GetSpaceByFolder(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	folderID := c.Query("folder_id")
	if folderID == "" {
		jsonError(c, common.CodeDataError, "folder_id is required")
		return
	}

	result, code, err := h.spaceService.GetSpaceByFolderID(folderID, user.ID)
	if err != nil {
		jsonError(c, code, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, result, "success")
}
