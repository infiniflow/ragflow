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
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"ragflow/internal/utility"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// AgentHandler agent handler
// fileUploader is the subset of FileService used by agent handlers.
type agentFileService interface {
	UploadFile(tenantID, parentID string, files []*multipart.FileHeader) ([]map[string]interface{}, error)
	DownloadAgentFile(tenantID, location string) ([]byte, error)
}

// AgentHandler agent handler
type AgentHandler struct {
	agentService *service.AgentService
	fileService  agentFileService
}

// NewAgentHandler create agent handler

func NewAgentHandler(agentService *service.AgentService, fileService *service.FileService) *AgentHandler {
	return &AgentHandler{
		agentService: agentService,
		fileService:  fileService,
	}
}

// ListAgents lists agent canvases for the current user.
// @Summary List Agents
// @Description List agent canvases accessible to the current user (Home dashboard tile)
// @Tags agents
// @Produce json
// @Param keywords query string false "Filter by title keyword"
// @Param page query int false "Page number (0 = no pagination)"
// @Param page_size query int false "Items per page (0 = no pagination)"
// @Param orderby query string false "Order-by field (default: create_time)"
// @Param desc query bool false "Descending order (default: true)"
// @Param owner_ids query string false "Comma-separated owner IDs to filter (default: all authorised tenants)"
// @Param canvas_category query string false "Canvas category (default: agent_canvas)"
// @Success 200 {object} service.ListAgentsResponse
// @Router /api/v1/agents [get]
func (h *AgentHandler) ListAgents(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	keywords := c.Query("keywords")
	canvasCategory := c.Query("canvas_category")

	page := 0
	if v := c.Query("page"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 0
	if v := c.Query("page_size"); v != "" {
		if ps, err := strconv.Atoi(v); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	orderby := c.DefaultQuery("orderby", "create_time")

	desc := true
	if v := c.Query("desc"); v != "" {
		desc = strings.ToLower(v) != "false"
	}

	var ownerIDs []string
	if raw := c.Query("owner_ids"); raw != "" {
		for _, id := range strings.Split(raw, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				ownerIDs = append(ownerIDs, id)
			}
		}
	}

	result, code, err := h.agentService.ListAgents(
		user.ID,
		keywords,
		page,
		pageSize,
		orderby,
		desc,
		ownerIDs,
		canvasCategory,
	)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"data":    false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    result,
		"message": "success",
	})
}

// ListAgentSessions List all sessions
func (h *AgentHandler) ListAgentSessions(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "agent_id is required",
		})
		return
	}

	page := parsePositiveIntQuery(c, "page", 1)
	pageSize := parsePositiveIntQuery(c, "page_size", 30)
	if pageSize > 100 {
		pageSize = 100
	}

	req := service.ListAgentSessionsRequest{
		SessionID:  c.Query("id"),
		UserID:     c.Query("user_id"),
		Page:       page,
		PageSize:   pageSize,
		Keywords:   c.Query("keywords"),
		FromDate:   c.Query("from_date"),
		ToDate:     c.Query("to_date"),
		OrderBy:    defaultQueryString(c.Query("orderby"), "update_time"),
		ExpUserID:  c.Query("exp_user_id"),
		Desc:       c.Query("desc") != "False" && c.Query("desc") != "false",
		IncludeDSL: c.Query("dsl") != "False" && c.Query("dsl") != "false",
	}

	tenantID := user.ID
	result, code, err := h.agentService.ListAgentSessions(user.ID, tenantID, agentID, req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    result.Data,
		"message": "success",
		"total":   result.Total,
	})
}

func parsePositiveIntQuery(c *gin.Context, key string, defaultValue int) int {
	raw := c.Query(key)
	if raw == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}

	return value
}

func defaultQueryString(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func (h *AgentHandler) GetAgentSession(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeOperatingError,
			"data":    nil,
			"message": "agent_id is required",
		})
		return
	}

	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": "session_id is required",
		})
		return
	}

	userID := user.ID
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	agentID = strings.TrimSpace(agentID)

	data, code, err := h.agentService.GetAgentSession(userID, agentID, sessionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    data,
		"message": "success",
	})
}

func (h *AgentHandler) DeleteAgentSessionItem(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeOperatingError,
			"data":    nil,
			"message": "agent_id is required",
		})
		return
	}

	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": "session_id is required",
		})
		return
	}

	userID := user.ID
	userID = strings.TrimSpace(userID)
	sessionID = strings.TrimSpace(sessionID)
	agentID = strings.TrimSpace(agentID)

	ok, code, err := h.agentService.DeleteAgentSessionItem(userID, agentID, sessionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"data":    false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    code,
		"data":    ok,
		"message": "success",
	})
}

type deleteAgentSessionsRequest struct {
	IDs       []string `json:"ids"`
	DeleteAll bool     `json:"delete_all,omitempty"`
}

func (h *AgentHandler) DeleteAgentSessions(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	agentID := strings.TrimSpace(c.Param("agent_id"))
	if agentID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeOperatingError,
			"data":    nil,
			"message": "agent_id is required",
		})
		return
	}

	var req deleteAgentSessionsRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"data":    false,
			"message": err.Error(),
		})
		return
	}

	result, code, err := h.agentService.DeleteAgentSessions(strings.TrimSpace(user.ID), agentID, req.IDs, req.DeleteAll)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": err.Error(),
		})
		return
	}

	response := gin.H{"code": common.CodeSuccess}
	if result != nil && result.Data != nil {
		response["data"] = result.Data
	}
	if result != nil && result.Message != "" {
		response["message"] = result.Message
	}
	c.JSON(http.StatusOK, response)
}

// TestDBConnection Test DB connection
func (h *AgentHandler) TestDBConnection(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req service.TestDBConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"message": err.Error(),
		})
		return
	}

	code, err := h.agentService.TestDBConnection(user.ID, &req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"message": "success",
	})
}

// ListAgentVersions returns versions for a specific agent.
// @Summary List Agent Versions
// @Description Returns all versions for a specific agent, ordered by update_time DESC.
// @Tags agents
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/agents/{agent_id}/versions [get]
func (h *AgentHandler) ListAgentVersions(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "agent_id is required",
		})
		return
	}

	ok, err := h.agentService.CheckCanvasAccess(user.ID, agentID)
	if err != nil || !ok {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeOperatingError,
			"data":    nil,
			"message": "Agent not found or no permission.",
		})
		return
	}

	versions, err := h.agentService.ListVersions(agentID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeServerError,
			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    versions,
		"message": "",
	})
}

// ListTemplates lists every canvas template available to authenticated users.
// @Summary List Agent Templates
// @Description List the catalogue of canvas templates that authenticated users can clone.
// @Tags agents
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/agents/templates [get]
func (h *AgentHandler) ListTemplates(c *gin.Context) {
	if _, errorCode, errorMessage := GetUser(c); errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	templates, err := h.agentService.ListTemplates()
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}
	if templates == nil {
		// Ensure the JSON payload is always a list, never null.
		templates = []*entity.CanvasTemplate{}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    templates,
		"message": "success",
	})
}

// UploadAgentFile uploads one or more files associated with an agent.
// @Summary Upload Agent File
// @Description Upload one or more files for an agent canvas.

// GetAgentVersion returns a specific version for an agent.
// @Summary Get Agent Version
// @Description Returns a specific version by ID, verifying it belongs to the given agent.
// @Tags agents
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Param version_id path string true "Version ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/agents/{agent_id}/versions/{version_id} [get]
func (h *AgentHandler) GetAgentVersion(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	agentID := c.Param("agent_id")
	versionID := c.Param("version_id")
	if agentID == "" || versionID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "agent_id and version_id are required",
		})
		return
	}

	ok, err := h.agentService.CheckCanvasAccess(user.ID, agentID)
	if err != nil || !ok {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeOperatingError,
			"data":    nil,
			"message": "Agent not found or no permission.",
		})
		return
	}

	version, err := h.agentService.GetVersion(agentID, versionID)
	if err != nil {
		isNotFound := errors.Is(err, gorm.ErrRecordNotFound) || err.Error() == "version not found"
		if !isNotFound {
			common.Warn("get agent version failed", zap.String("error", err.Error()))
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeServerError,
				"data":    nil,
				"message": "Internal server error",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeNotFound,
			"data":    nil,
			"message": "Version not found.",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    version,
		"message": "",
	})
}

// @Tags agents
// @Accept multipart/form-data
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Param file formData file true "File(s) to upload (multiple files supported)"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/agents/{agent_id}/upload [post]
func (h *AgentHandler) UploadAgentFile(c *gin.Context) {

	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "agent_id is required",
		})
		return
	}

	ok, err := h.agentService.CheckCanvasAccess(user.ID, agentID)
	if err != nil || !ok {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeOperatingError,
			"data":    nil,
			"message": "Agent not found or no permission.",
		})
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": fmt.Sprintf("invalid form data: %v", err),
		})
		return
	}

	files := form.File["file"]
	if len(files) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "You have to upload at least one file.",
		})
		return
	}

	// Use the canvas owner's tenant ID for file ownership.
	uploaded, err := h.fileService.UploadFile(user.ID, "", files)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code": common.CodeOperatingError,

			"data":    nil,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": common.CodeSuccess,
		"data": uploaded,

		"message": "",
	})
}

type updateAgentTagsRequest struct {
	Tags interface{} `json:"tags"`
}

func (h *AgentHandler) UpdateAgentTags(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req updateAgentTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeBadRequest,
			"data":    false,
			"message": err.Error(),
		})
		return
	}

	data, code, err := h.agentService.UpdateAgentTags(user.ID, c.Param("agent_id"), req.Tags)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"data":    false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    data,
		"message": "success",
	})
}

func (h *AgentHandler) DownloadAgentFile(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	fileID := c.Query("id")
	if fileID == "" {
		jsonError(c, common.CodeArgumentError, "id parameter is required")
		return
	}

	blob, err := h.fileService.DownloadAgentFile(user.ID, fileID)
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}

	ext := utility.GetFileExtension(fileID)
	contentType := utility.GetContentType(ext, "")
	if contentType != "" {
		c.Header("Content-Type", contentType)
	} else {
		c.Header("Content-Type", "application/octet-stream")
	}

	if utility.ShouldForceAttachment(ext, contentType) {
		c.Header("X-Content-Type-Options", "nosniff")
		encodedName := url.QueryEscape(fileID)
		c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+encodedName)
	}

	c.Data(http.StatusOK, contentType, blob)
}

func (h *AgentHandler) GetPrompts(c *gin.Context) {
	if _, errorCode, errorMessage := GetUser(c); errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	taskAnalysisSys, err := service.LoadPrompt("analyze_task_system")
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}
	taskAnalysisUser, err := service.LoadPrompt("analyze_task_user")
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}
	planGeneration, err := service.LoadPrompt("next_step")
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}
	reflection, err := service.LoadPrompt("reflect")
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}
	citationGuidelines, err := service.LoadPrompt("citation_prompt")
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": common.CodeSuccess,
		"data": map[string]string{
			"task_analysis":       fmt.Sprintf("%s\n\n%s", taskAnalysisSys, taskAnalysisUser),
			"plan_generation":     planGeneration,
			"reflection":          reflection,
			"citation_guidelines": citationGuidelines,
		},
		"message": "success",
	})
}
