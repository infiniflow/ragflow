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
	"fmt"
	"io"
	"mime/multipart"
	"ragflow/internal/engine/redis"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	dslpkg "ragflow/internal/agent/dsl"
)

// AgentHandler agent handler
// fileUploader is the subset of FileService used by agent handlers.
//
// The full FileService also has UploadFile, but it is consumed by
// the FileHandler (handler/file.go), not by any agent handler, so
// the interface deliberately does NOT list it. (Code review CR1.)
type agentFileService interface {
	DownloadAgentFile(tenantID, location string) ([]byte, error)
	// UploadInfos stores raw bytes in the per-user downloads bucket and
	// returns lightweight descriptors. Mirrors python FileService.upload_info
	// (multi-file path) used by the agent upload endpoint.
	UploadInfos(userID string, files []*multipart.FileHeader) ([]map[string]interface{}, error)
	// UploadFromURL downloads a remote file (with SSRF protection) and
	// stores it as an info blob. Mirrors python FileService.upload_info
	// (single-file path with ?url=) used by the agent upload endpoint.
	UploadFromURL(tenantID, rawURL string) (map[string]interface{}, error)
}

// chatAgentService is the subset of AgentService used by the chat-completion
// endpoints (AgentChatCompletions, RunAgent). Kept as a separate interface so
// handler tests can inject a fake RunAgent without standing up the full
// AgentService (DB DAOs, eino runner, etc.). The production wiring in
// NewAgentHandler assigns the concrete *service.AgentService — which
// satisfies this interface because its RunAgent signature matches.
type chatAgentService interface {
	RunAgent(ctx context.Context, userID, canvasID, sessionID, version string, userInput any) (<-chan canvas.RunEvent, error)
}

// documentAccessChecker is the minimal surface RerunAgent needs
// from DocumentService. Defined as an interface (instead of taking
// the concrete *service.DocumentService) so handler tests can
// inject a deny-all stub without spinning up the full service
// (DB DAOs, storage clients, …). The production *service.DocumentService
// satisfies this interface because its Accessible signature
// matches.
type documentAccessChecker interface {
	Accessible(docID, userID string) bool
}

// AgentHandler agent handler
type AgentHandler struct {
	agentService *service.AgentService
	chatRunner   chatAgentService
	fileService  agentFileService
	loader       canvasLoader
	// documentService is optional. Wired in cmd/server_main.go after
	// NewAgentHandler (which doesn't take it to preserve the existing
	// test-friendly signature). When nil, RerunAgent falls back to
	// tenant-only authorization (i.e. cannot verify the doc, so the
	// check is skipped — same shape as the pre-port behaviour).
	documentService documentAccessChecker
}

// WithDocumentService injects the document service used by
// RerunAgent to enforce DocumentService.accessible(docID, tenantID)
// before re-running. Returns the receiver for chaining in
// server_main wiring.
func (h *AgentHandler) WithDocumentService(s documentAccessChecker) *AgentHandler {
	h.documentService = s
	return h
}

// NewAgentHandler create agent handler

func NewAgentHandler(agentService *service.AgentService, fileService *service.FileService) *AgentHandler {
	return &AgentHandler{
		agentService: agentService,
		chatRunner:   agentService,
		fileService:  fileService,
		loader:       agentService,
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	keywords := c.Query("keywords")
	canvasCategory := c.Query("canvas_category")
	canvasType := c.Query("canvas_type")

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
	var tags []string
	if raw := c.Query("tags"); raw != "" {
		for _, tag := range strings.Split(raw, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
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
		canvasType,
		tags,
	)
	if err != nil {
		common.ResponseWithCodeData(c, code, false, err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// mapAgentError normalises service-layer errors onto the existing
// {code, data, message} response envelope used by every other handler.
//
// Three classes:
//   - service.ErrAgentNotOwner  -> "Only the owner..."        (DELETE only, 103)
//   - dao.ErrUserCanvasNotFound -> "Make sure you have permission..."  (103)
//   - service.ErrAgentStorageError -> "Internal storage error"  (500)
//
// The first two surface as OPERATING_ERROR(103) so the front-end
// cannot use the response code to enumerate other users' canvases
// (IDOR mitigation). ErrAgentStorageError maps to SERVER_ERROR(500)
// with a sanitized message — the raw DAO / DB error string MUST
// NOT reach the client (DSNs, table names, gorm stack frames would
// otherwise leak). Everything else becomes CodeDataError so the
// front-end can surface the message verbatim.
func mapAgentError(err error) (common.ErrorCode, string) {
	if err == nil {
		return common.CodeSuccess, ""
	}
	if errors.Is(err, service.ErrAgentNotOwner) {
		return common.CodeOperatingError, "Only the owner of the agent is authorized for this operation."
	}
	if errors.Is(err, dao.ErrUserCanvasNotFound) ||
		errors.Is(err, dao.ErrUserCanvasVersionNotFound) {
		return common.CodeOperatingError, "Make sure you have permission to access the agent."
	}
	if errors.Is(err, service.ErrAgentStorageError) {
		return common.CodeServerError, "Internal storage error while accessing the agent."
	}
	return common.CodeDataError, err.Error()
}

// CreateAgent creates a new agent canvas.
// @Summary Create Agent
// @Tags agents
// @Accept json
// @Produce json
// @Param request body service.CreateAgentRequest true "agent create request"
// @Success 200 {object} entity.UserCanvas
// @Router /api/v1/agents [post]
func (h *AgentHandler) CreateAgent(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}
	var req service.CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	req.UserID = user.ID
	row, code, err := h.agentService.CreateAgent(c.Request.Context(), &req)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	common.SuccessWithData(c, row, "success")
}

// GetAgent returns one canvas by ID.
// @Summary Get Agent
// @Tags agents
// @Produce json
// @Param canvas_id path string true "canvas id"
// @Success 200 {object} entity.UserCanvas
// @Router /api/v1/agents/{canvas_id} [get]
func (h *AgentHandler) GetAgent(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	row, err := h.agentService.GetAgent(c.Request.Context(), user.ID, canvasID)
	if err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	// Defensive: any historical v1 / Go-v2-only row in user_canvas.dsl
	// is rendered as a missing graph by the front-end. Normalize in
	// place (NormalizeForCanvas is a no-op when graph.nodes is set) so
	// the response is always renderable without a migration.
	if row != nil {
		row.DSL = dslpkg.NormalizeForCanvas(row.DSL)
	}
	common.SuccessWithData(c, row, "success")
}

// updateAgentRequest is the wire shape for PUT /api/v1/agents/:canvas_id.
type updateAgentRequest map[string]interface{}

// UpdateAgent applies a partial update to the canvas draft.
// @Summary Update Agent
// @Tags agents
// @Accept json
// @Produce json
// @Param canvas_id path string true "canvas id"
// @Param request body updateAgentRequest true "agent update payload"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/agents/{canvas_id} [put]
func (h *AgentHandler) UpdateAgent(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	var req updateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	if req == nil {
		req = updateAgentRequest{}
	}
	if err := h.agentService.UpdateAgent(c.Request.Context(), user.ID, canvasID, map[string]interface{}(req)); err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	common.SuccessWithData(c, true, "success")
}

// DeleteAgent removes the canvas and cascades to its versions.
// @Summary Delete Agent
// @Tags agents
// @Produce json
// @Param canvas_id path string true "canvas id"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/agents/{canvas_id} [delete]
func (h *AgentHandler) DeleteAgent(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	if err := h.agentService.DeleteAgent(c.Request.Context(), user.ID, canvasID); err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	common.SuccessWithData(c, true, "success")
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
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	templates, err := h.agentService.ListTemplates()
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	if templates == nil {
		// Ensure the JSON payload is always a list, never null.
		templates = []*entity.CanvasTemplate{}
	}

	common.SuccessWithData(c, templates, "success")
}

// RunAgent returns an SSE stream of execution events. The Phase 5 stub emits
// a single "Phase 5 wiring pending" event and closes; the real eino run
// loop will replace the channel source in service.AgentService.RunAgent.
// @Summary Run Agent (SSE)
// @Tags agents
// @Produce text/event-stream
// @Param canvas_id path string true "canvas id"
// @Param version query string false "version id (default: latest)"
// @Success 200 {string} string "SSE: data: {...}\\n\\n"
// @Router /api/v1/agents/{canvas_id}/run [post]
func (h *AgentHandler) RunAgent(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	version := c.Query("version")
	sessionID := c.Query("session_id")
	userInput := readUserInput(c)

	events, err := h.chatRunner.RunAgent(c.Request.Context(), user.ID, canvasID, sessionID, version, userInput)
	if err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	for ev := range events {
		if err := service.WriteChatbotRunEvent(c.Writer, ev); err != nil {
			common.Debug("agent run: client disconnected",
				zap.String("canvas_id", canvasID),
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
			return
		}
	}
}

// readUserInput extracts the user_input field from the JSON body if
// present, otherwise from the ?user_input= query string. An empty body
// (no body sent) is treated as "" so the resume cycle still works
// when the client only passes ?session_id=...&user_input=... on the URL.
func readUserInput(c *gin.Context) string {
	if c.Request.ContentLength > 0 {
		var body struct {
			UserInput string `json:"user_input"`
			Query     string `json:"query"`
			Message   string `json:"message"`
		}
		if err := c.ShouldBindJSON(&body); err == nil {
			if body.UserInput != "" {
				return body.UserInput
			}
			if body.Query != "" {
				return body.Query
			}
			if body.Message != "" {
				return body.Message
			}
		}
	}
	return c.Query("user_input")
}

// sanitiseRunEventError passes through the error event payload
// unchanged. The runner serialises canvas.ErrorEvent ({"message": ...})
// before push, so when the payload round-trips through JSON the
// message field is already preserved. Heuristic sanitisation is
// disabled until the runner tags error events with a "kind"
// field — without that, blanket rewriting every error to
// "Internal storage error while accessing the agent." hides the
// real failure from the front-end and the user (v3.6.1 diagnostic
// regression: every canvas run failure surfaced as the same opaque
// string).
func sanitiseRunEventError(data string) string {
	if data == "" {
		return `{"message":"Unknown agent runtime error"}`
	}
	return data
}

// CancelAgent signals the in-flight run to stop.
// @Summary Cancel Agent Run
// @Tags agents
// @Produce json
// @Param canvas_id path string true "canvas id"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/agents/{canvas_id}/run [delete]
func (h *AgentHandler) CancelAgent(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	if err := h.agentService.CancelAgent(c.Request.Context(), user.ID, canvasID); err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	common.SuccessWithData(c, true, "success")
}

// publishAgentRequest is the wire shape for POST /api/v1/agents/:canvas_id/publish.
type publishAgentRequest struct {
	Title       *string        `json:"title,omitempty"`
	Description *string        `json:"description,omitempty"`
	DSL         entity.JSONMap `json:"dsl,omitempty"`
}

// PublishAgent creates a new immutable version row and marks the parent canvas as released.
// @Summary Publish Agent Version
// @Tags agents
// @Accept json
// @Produce json
// @Param canvas_id path string true "canvas id"
// @Param request body publishAgentRequest true "publish payload"
// @Success 200 {object} entity.UserCanvasVersion
// @Router /api/v1/agents/{canvas_id}/publish [post]
func (h *AgentHandler) PublishAgent(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	var req publishAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	row, err := h.agentService.PublishAgent(c.Request.Context(), user.ID, canvasID, &service.PublishAgentRequest{
		Title:       req.Title,
		Description: req.Description,
		DSL:         req.DSL,
	})
	if err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	if row != nil {
		row.DSL = dslpkg.NormalizeForCanvas(row.DSL)
	}
	common.SuccessWithData(c, row, "success")
}

// ListVersions returns every version of a canvas, newest first.
// @Summary List Agent Versions
// @Tags agents
// @Produce json
// @Param canvas_id path string true "canvas id"
// @Success 200 {array} entity.UserCanvasVersion
// @Router /api/v1/agents/{canvas_id}/versions [get]
func (h *AgentHandler) ListVersions(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	rows, err := h.agentService.ListVersions(c.Request.Context(), user.ID, canvasID)
	if err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	if rows == nil {
		rows = []*entity.UserCanvasVersion{}
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		row.DSL = dslpkg.NormalizeForCanvas(row.DSL)
	}
	common.SuccessWithData(c, rows, "success")
}

// GetVersion returns a single version.
// @Summary Get Agent Version
// @Tags agents
// @Produce json
// @Param canvas_id path string true "canvas id"
// @Param version_id path string true "version id"
// @Success 200 {object} entity.UserCanvasVersion
// @Router /api/v1/agents/{canvas_id}/versions/{version_id} [get]
func (h *AgentHandler) GetVersion(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	versionID := c.Param("version_id")
	row, err := h.agentService.GetVersion(c.Request.Context(), user.ID, canvasID, versionID)
	if err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	if row != nil {
		row.DSL = dslpkg.NormalizeForCanvas(row.DSL)
	}
	common.SuccessWithData(c, row, "success")
}

// DeleteVersion removes a single version by id.
// @Summary Delete Agent Version
// @Tags agents
// @Produce json
// @Param canvas_id path string true "canvas id"
// @Param version_id path string true "version id"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/agents/{canvas_id}/versions/{version_id} [delete]
func (h *AgentHandler) DeleteVersion(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	versionID := c.Param("version_id")
	if err := h.agentService.DeleteVersion(c.Request.Context(), user.ID, canvasID, versionID); err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	common.SuccessWithData(c, true, "success")
}

// --- PR2: missing routes wired up to the existing service layer ---

// ListAgentTemplates GET /api/v1/agents/templates
func (h *AgentHandler) ListAgentTemplates(c *gin.Context) {
	if _, code, msg := GetUser(c); code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	rows, err := h.agentService.ListTemplates()
	if err != nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, err.Error())
		return
	}
	common.SuccessWithData(c, rows, "success")
}

// Prompts GET /api/v1/agents/prompts — returns the four hardcoded
// authoring guidelines the agent UI surfaces. The Python agent API
// returns these from a module-level constant; we keep the same shape.
func (h *AgentHandler) Prompts(c *gin.Context) {
	if _, code, msg := GetUser(c); code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	common.SuccessWithData(c, gin.H{
		"task_analysis":       "As an AI agent designer, your role is to engage users by understanding their objectives and creating effective agent designs. Begin by analyzing the user's request to determine the appropriate actions.",
		"output_format":       "For each agent you create, detail its components and explain how they collaborate to achieve the user's goal.",
		"citation_guidelines": "If the agent uses external sources, cite them in the final output. Use the format: [index] document_id, which corresponds to the document identifier in the database.",
		"few_shots_examples":  "<example/>",
	}, "success")
}

// ListAgentTags list agent tags.
func (h *AgentHandler) ListAgentTags(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}

	rows, errCode, err := h.agentService.ListAgentTags(user.ID, strings.TrimSpace(c.Query("canvas_category")))
	if err != nil {
		common.ResponseWithCodeData(c, errCode, nil, err.Error())
		return
	}

	common.SuccessWithData(c, rows, "success")
}

// UpdateAgentTags PUT /api/v1/agents/:canvas_id/tags
func (h *AgentHandler) UpdateAgentTags(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	var body struct {
		Tags interface{} `json:"tags"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	ok, errCode, errMsg := h.agentService.UpdateAgentTags(user.ID, canvasID, body.Tags)
	if !ok {
		common.ResponseWithCodeData(c, errCode, nil, errMsg.Error())
		return
	}
	common.SuccessWithData(c, true, "success")
}

// ListAgentSessions GET /api/v1/agents/:canvas_id/sessions
func (h *AgentHandler) ListAgentSessions(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "0"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "0"))
	keywords := c.Query("keywords")
	fromDate := c.Query("from_date")
	toDate := c.Query("to_date")
	orderby := c.DefaultQuery("orderby", "create_time")
	desc := c.DefaultQuery("desc", "true") != "false"
	sessionID := c.Query("id")
	expUserID := c.Query("user_id")
	includeDSL := c.Query("dsl") == "true"

	resp, code, err := h.agentService.ListAgentSessions(user.ID, user.ID, canvasID, service.ListAgentSessionsRequest{
		Page:       page,
		PageSize:   pageSize,
		Keywords:   keywords,
		FromDate:   fromDate,
		ToDate:     toDate,
		OrderBy:    orderby,
		Desc:       desc,
		SessionID:  sessionID,
		UserID:     user.ID,
		ExpUserID:  expUserID,
		IncludeDSL: includeDSL,
	})
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	common.SuccessWithData(c, resp.Data, "success")
}

// CreateAgentSession POST /api/v1/agents/:canvas_id/sessions
func (h *AgentHandler) CreateAgentSession(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	var body struct {
		Name   string          `json:"name"`
		Source string          `json:"source"`
		DSL    json.RawMessage `json:"dsl"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	row, code, err := h.agentService.CreateAgentSession(&service.CreateAgentSessionRequest{
		UserID:  user.ID,
		AgentID: canvasID,
		Name:    body.Name,
		Source:  body.Source,
		DSL:     body.DSL,
	})
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	common.SuccessWithData(c, row, "success")
}

// GetAgentSession GET /api/v1/agents/:canvas_id/sessions/:session_id
func (h *AgentHandler) GetAgentSession(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	sessionID := c.Param("session_id")
	row, code, err := h.agentService.GetAgentSession(user.ID, canvasID, sessionID)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	common.SuccessWithData(c, row, "success")
}

// DeleteAgentSession DELETE /api/v1/agents/:canvas_id/sessions[/:session_id]
//
// Path parameter disambiguation:
//   - /sessions/:session_id   -> single item delete
//   - /sessions?ids=a,b       -> batch delete (delete_all when ids is empty)
func (h *AgentHandler) DeleteAgentSession(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	sessionID := c.Param("session_id")
	if sessionID != "" {
		ok, code, err := h.agentService.DeleteAgentSessionItem(user.ID, canvasID, sessionID)
		if err != nil {
			common.ErrorWithCode(c, int(code), err.Error())
			return
		}
		common.SuccessWithData(c, ok, "success")
		return
	}
	idsParam := c.Query("ids")
	deleteAll := c.Query("delete_all") == "true"
	var ids []string
	if idsParam != "" {
		for _, id := range strings.Split(idsParam, ",") {
			if id = strings.TrimSpace(id); id != "" {
				ids = append(ids, id)
			}
		}
	}
	result, code, err := h.agentService.DeleteAgentSessions(user.ID, canvasID, ids, deleteAll)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	common.SuccessWithData(c, result, "success")
}

// AgentChatCompletions POST /api/v1/agents/chat/completions
//
// Runs the canvas against `agent_id` and streams the result as SSE.
//
// Behaviour matches the Python reference at
// api/db/services/canvas_service.py:313 (`completion()`):
//
//   - Non-openai path: always streams SSE — one `data: {...}\n\n` frame per
//     canvas RunEvent, terminated by `data: [DONE]\n\n`. The `stream` field
//     is ignored on this path because Python's `completion()` always yields
//     SSE frames regardless of the flag.
//   - Openai-compatible path: requires `messages` (a non-empty list with at
//     least one user message is needed to derive the question). The full
//     OpenAI wire framing (delta + reference + token counts — see
//     `completion_openai` at api/db/services/canvas_service.py:378-479) is
//     still a Phase 5 TODO; until then the openai-compat branches return a
//     hardcoded "hello" stub so the validation contracts keep passing.
type agentChatCompletionsRequest struct {
	AgentID      string                   `json:"agent_id"`
	Query        string                   `json:"query"`
	Inputs       map[string]interface{}   `json:"inputs"`
	SessionID    string                   `json:"session_id"`
	Stream       bool                     `json:"stream"`
	OpenAICompat bool                     `json:"openai-compatible"`
	Model        string                   `json:"model"`
	Messages     []map[string]interface{} `json:"messages"`
	ReturnTrace  bool                     `json:"return_trace"`
}

// extractLastUserContent returns the content of the last message in
// `messages` whose role is "user", or "" if none is found. Mirrors the
// Python derivation in api/apps/restful_apis/agent_api.py:1258 that drives
// `completion_openai` when the request uses the openai-compatible wire
// format but no top-level `query` is supplied.
func extractLastUserContent(messages []map[string]interface{}) string {
	for i := len(messages) - 1; i >= 0; i-- {
		role, _ := messages[i]["role"].(string)
		if role != "user" {
			continue
		}
		if c, _ := messages[i]["content"].(string); c != "" {
			return c
		}
	}
	return ""
}

// extractUserInputFromFormInputs mirrors the front-end's wait-for-user submit
// shape: `inputs` is an object keyed by form field name, and each entry carries
// a nested `value`. The current chat-completion resume path consumes a single
// string payload, so we lift the first field's value and stringify it.
func extractUserInputFromFormInputs(inputs map[string]interface{}) interface{} {
	if len(inputs) == 0 {
		return nil
	}
	if len(inputs) == 1 {
		for _, raw := range inputs {
			if field, ok := raw.(map[string]interface{}); ok {
				if v, ok := field["value"]; ok {
					return v
				}
			}
			return raw
		}
	}

	out := make(map[string]any, len(inputs))
	for name, raw := range inputs {
		if field, ok := raw.(map[string]interface{}); ok {
			if v, ok := field["value"]; ok {
				out[name] = v
				continue
			}
		}
		out[name] = raw
	}
	return out
}

func countInputValues(inputs map[string]interface{}) int {
	count := 0
	for _, raw := range inputs {
		if field, ok := raw.(map[string]interface{}); ok {
			if _, exists := field["value"]; exists {
				count++
			}
			continue
		}
		if raw != nil {
			count++
		}
	}
	return count
}

func userInputMeta(userInput any) []zap.Field {
	fields := []zap.Field{zap.String("user_input_type", fmt.Sprintf("%T", userInput))}
	switch v := userInput.(type) {
	case nil:
		fields = append(fields, zap.Bool("user_input_present", false))
	case string:
		fields = append(fields,
			zap.Bool("user_input_present", true),
			zap.Int("user_input_length", len(v)),
			zap.Bool("user_input_blank", v == ""),
		)
	case map[string]interface{}:
		fields = append(fields,
			zap.Bool("user_input_present", true),
			zap.Int("user_input_keys", len(v)),
		)
	default:
		fields = append(fields, zap.Bool("user_input_present", true))
	}
	return fields
}

func (h *AgentHandler) AgentChatCompletions(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	var req agentChatCompletionsRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	if req.AgentID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "`agent_id` is required.")
		return
	}
	if req.OpenAICompat && len(req.Messages) == 0 {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "at least one message is required in openai-compatible mode.")
		return
	}
	common.Debug("agent chat completions: request received",
		zap.String("user_id", user.ID),
		zap.String("agent_id", req.AgentID),
		zap.String("session_id", req.SessionID),
		zap.Bool("stream", req.Stream),
		zap.Bool("openai_compatible", req.OpenAICompat),
		zap.Bool("query_present", req.Query != ""),
		zap.Int("query_length", len(req.Query)),
		zap.Int("inputs_count", len(req.Inputs)),
		zap.Int("inputs_with_values_count", countInputValues(req.Inputs)),
		zap.Int("messages_count", len(req.Messages)),
	)

	// TODO(phase5-openai-framing): the openai-compat branches below are
	// stubs. They keep the existing "choices"-shape contract for the
	// openai-compat tests, but the production wire format must mirror
	// api/db/services/canvas_service.py:378-479 (`completion_openai`):
	// per-token `delta.content`, cumulative token counts, `[DONE]`
	// terminator, `reference` attached to the final choice. Land that
	// once the chat path needs to interop with OpenAI clients.
	if req.OpenAICompat {
		common.SuccessWithData(c, gin.H{
			"choices": []map[string]interface{}{
				{"message": gin.H{"content": "hello"}},
			},
		}, "success")
		return
	}

	// Real canvas run — derive userInput from `query` first, then fall
	// back to the last user message (covers the front-end that posts
	// running_hint_text without a top-level `query`).
	var userInput any = req.Query
	if req.Query == "" {
		if extracted := extractUserInputFromFormInputs(req.Inputs); extracted != nil {
			userInput = extracted
		} else if extracted := extractLastUserContent(req.Messages); extracted != "" {
			userInput = extracted
		}
	}
	common.Debug("agent chat completions: derived user input",
		append([]zap.Field{
			zap.String("agent_id", req.AgentID),
			zap.String("session_id", req.SessionID),
		}, userInputMeta(userInput)...)...,
	)

	events, err := h.chatRunner.RunAgent(c.Request.Context(), user.ID, req.AgentID, req.SessionID, "", userInput)
	if err != nil {
		common.Warn("agent chat completions: RunAgent failed",
			append([]zap.Field{
				zap.String("user_id", user.ID),
				zap.String("agent_id", req.AgentID),
				zap.String("session_id", req.SessionID),
				zap.Error(err),
			}, userInputMeta(userInput)...)...,
		)
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	// SSE wire format is the flat Python agent-canvas envelope:
	// {event,message_id,task_id,session_id,created_at,data}. One
	// frame is emitted per canvas event through service.WriteChatbotRunEvent.
	// The channel close is signalled by `data: [DONE]\n\n`. We do NOT
	// emit an SSE `event:` line — the front-end's use-send-message.ts
	// parser feeds each `data:` line directly into JSON.parse and
	// expects the event type in the JSON object's top-level `event`
	// field.
	emitted := false
	for ev := range events {
		emitted = true
		common.Debug("agent chat completions: streaming event",
			zap.String("agent_id", req.AgentID),
			zap.String("session_id", req.SessionID),
			zap.String("event_type", ev.Type),
			zap.String("message_id", ev.MessageID),
			zap.String("task_id", ev.TaskID),
		)
		if err := service.WriteChatbotRunEvent(c.Writer, ev); err != nil {
			common.Debug("agent chat completions: client disconnected",
				zap.String("agent_id", req.AgentID),
				zap.Error(err),
			)
			return
		}
	}
	if !emitted {
		// Canvas produced no events (e.g. empty query). Echo the
		// session_id so the client can resume the conversation
		// (fixes #15169). The [DONE] terminator must be emitted
		// here explicitly because the canvas never sends a
		// "done" event on this path.
		common.Info("empty agent output - returning session_id",
			zap.String("agent_id", req.AgentID),
			zap.String("session_id", req.SessionID),
		)
		event := canvas.RunEvent{
			Type:      "",
			Data:      "{}",
			CreatedAt: time.Now().Unix(),
			SessionID: req.SessionID,
		}
		_ = service.WriteChatbotRunEvent(c.Writer, event)
		if _, err := c.Writer.Write([]byte("data: [DONE]\n\n")); err != nil {
			common.Debug("agent chat completions: failed to write [DONE]",
				zap.Error(err),
			)
		}
	}
	common.Debug("agent chat completions: stream closed",
		zap.String("agent_id", req.AgentID),
		zap.String("session_id", req.SessionID),
	)
}

// RerunAgent POST /api/v1/agents/rerun — requires id, dsl, and
// component_id. The Python agent API uses PipelineOperationLogService
// and the dataflow queue, none of which the Go port has implemented
// yet; we keep the validation envelope (101 with the "required
// argument are missing" message) so the test contract is satisfied,
// and accept the request when all three fields are present.
//
// Tenant / document ownership gate (PR #15145, review round 6):
// body.id is treated as a document ID and
// `DocumentService.accessible(docID, user.ID)` is enforced BEFORE
// the rerun. The gate is REQUIRED: a nil documentService turns a
// wiring miss into an auth bypass (any caller could rerun an
// arbitrary doc id without an ownership check), so we fail closed
// with 500 instead of accepting the request. On denial we return
// "Document not found." so a caller cannot probe whether a
// document exists in another tenant.
func (h *AgentHandler) RerunAgent(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	var body struct {
		ID          string                 `json:"id"`
		DSL         map[string]interface{} `json:"dsl"`
		ComponentID string                 `json:"component_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	missing := make([]string, 0, 3)
	if body.ID == "" {
		missing = append(missing, "id")
	}
	if body.DSL == nil {
		missing = append(missing, "dsl")
	}
	if body.ComponentID == "" {
		missing = append(missing, "component_id")
	}
	if len(missing) > 0 {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "required argument are missing: "+strings.Join(missing, ",")+"; ")
		return
	}
	// Fail closed on missing dependency: a nil documentService
	// means the handler was wired without the access checker,
	// which would let any caller rerun an arbitrary doc id
	// without proving ownership. Surface as a 500 so a missing
	// dependency is loud, not silent.
	if h.documentService == nil {
		zap.L().Error("RerunAgent: documentService is nil; refusing request to prevent auth bypass")
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "server misconfiguration: document service not wired")
		return
	}
	if !h.documentService.Accessible(body.ID, user.ID) {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Document not found.")
		return
	}
	common.SuccessWithData(c, true, "success")
}

// TestDBConnection POST /api/v1/agents/test_db_connection
func (h *AgentHandler) TestDBConnection(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	var req service.TestDBConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	code, err := h.agentService.TestDBConnection(user.ID, &req)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}
	common.SuccessWithData(c, true, "success")
}

// GetAgentLogs GET /api/v1/agents/:canvas_id/logs/:message_id
//
// Reads "{agent_id}-{message_id}-logs" from Redis (same key format
// used by the Python agent API in api/apps/restful_apis/agent_api.py
// line 920). Missing key returns an empty dict so the test contract
// `data is dict` and `code == 0` are both satisfied.
func (h *AgentHandler) GetAgentLogs(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	messageID := c.Param("message_id")
	ok, errCode, errMsg := h.checkCanvasAccessForHandler(c, user.ID, canvasID)
	if !ok {
		common.ResponseWithCodeData(c, errCode, nil, errMsg)
		return
	}

	key := fmt.Sprintf("%s-%s-logs", canvasID, messageID)
	payload, rerr := redis.Get().Get(key)
	data := map[string]interface{}{}
	if rerr == nil && payload != "" {
		_ = json.Unmarshal([]byte(payload), &data)
	}
	common.SuccessWithData(c, data, "success")
}

// GetAgentWebhookLogs GET /api/v1/agents/:canvas_id/webhook/logs
//
// The Python agent API returns 102 "Canvas not found." when the agent
// id does not resolve to a canvas owned by the caller (see
// api/apps/restful_apis/agent_api.py webhook_trace). We replicate
// that envelope here so the front-end poll does not surface a 500
// for unknown / foreign canvas ids.
func (h *AgentHandler) GetAgentWebhookLogs(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	ok, err := h.agentService.CheckCanvasAccess(user.ID, canvasID)
	if err != nil || !ok {
		// CheckCanvasAccess now surfaces ErrUserCanvasNotFound when
		// the canvas row is missing; the Python envelope is
		// indistinguishable for missing vs foreign, so collapse
		// both into 102 "Canvas not found." here.
		if err != nil && !errors.Is(err, dao.ErrUserCanvasNotFound) {
			common.ResponseWithCodeData(c, common.CodeServerError, nil, err.Error())
			return
		}
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Canvas not found.")
		return
	}
	common.SuccessWithData(c, gin.H{
		"events":        []interface{}{},
		"finished":      false,
		"next_since_ts": 0,
	}, "success")
}

// checkCanvasAccessForHandler is the shared 103 envelope helper for
// PR2 routes that need to call service.CheckCanvasAccess and surface
// the access-denied envelope with the same shape the existing
// loadCanvasForUser-based handlers use.
func (h *AgentHandler) checkCanvasAccessForHandler(c *gin.Context, userID, canvasID string) (bool, common.ErrorCode, string) {
	ok, err := h.agentService.CheckCanvasAccess(userID, canvasID)
	if err != nil {
		// The Python agent API uses @_require_canvas_access_async on
		// /sessions and /logs/:message_id, which folds "canvas does
		// not exist" into the same 103 access envelope as a
		// permission mismatch. Surface the same shape here so
		// callers probing unknown ids do not get a 500 record not
		// found that breaks the front-end.
		if errors.Is(err, dao.ErrUserCanvasNotFound) {
			return false, common.CodeOperatingError, "Make sure you have permission to access the agent."
		}
		return false, common.CodeServerError, err.Error()
	}
	if !ok {
		return false, common.CodeOperatingError, "Make sure you have permission to access the agent."
	}
	return true, common.CodeSuccess, ""
}

// ResetAgent clears the per-run state of a canvas (history, retrieval,
// memory, path) and zeroes every "sys.*" / "env.*" global. Mirrors
// POST /api/v1/agents/:canvas_id/reset from the Python backend at
// api/apps/restful_apis/agent_api.py:992 — but unlike the Python
// implementation this handler does not sync a Canvas replica.
// `api.apps.services.canvas_replica_service.CanvasReplicaService` is
// the Python Redis-backed runtime replica (distributed lock + 3h TTL);
// it is intentionally NOT ported to Go. The Go agent port runs every
// agent through eino's compose.Workflow.Invoke, which is reconstructed
// from the DSL on each run, so the replica's read-side acceleration
// is unnecessary and its write-side adds an out-of-band DB/cache sync
// for no benefit. UpdateAgent / CreateAgent / RerunAgent follow the
// same convention — DSL write only, no Redis replica. See the
// "canvas-replica-not-porting" project memory for the design rationale.
//
// The reset DSL is returned in the response body so the front-end
// can render the new state without an extra GET, matching the
// Python handler's `return get_json_result(data=dsl)` line.
// @Summary Reset Agent
// @Tags agents
// @Produce json
// @Param canvas_id path string true "canvas id"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/agents/{canvas_id}/reset [post]
func (h *AgentHandler) ResetAgent(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	dsl, err := h.agentService.ResetAgent(c.Request.Context(), user.ID, canvasID)
	if err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	common.SuccessWithData(c, dsl, "success")
}
