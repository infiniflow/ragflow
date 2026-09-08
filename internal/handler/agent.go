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
	"github.com/google/uuid"
	"go.uber.org/zap"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/ingestion/task"
	"ragflow/internal/service"
	"ragflow/internal/service/document"
	"ragflow/internal/service/file"
	"ragflow/internal/utility"

	dslpkg "ragflow/internal/agent/dsl"
)

// AgentHandler agent handler
// fileUploader is the subset of FileService used by agent handlers.
//
// The full FileService also has UploadFile, but it is consumed by
// the FileHandler (handler/file.go), not by any agent handler, so
// the interface deliberately does NOT list it. (Code review CR1.)
type agentFileService interface {
	DownloadAgentFile(ctx context.Context, tenantID, location string) ([]byte, error)
	// UploadInfos stores raw bytes in the per-user downloads bucket and
	// returns lightweight descriptors.
	UploadInfos(ctx context.Context, userID string, files []*multipart.FileHeader) ([]map[string]interface{}, error)
	// UploadFromURL downloads a remote file (with SSRF protection) and
	// stores it as an info blob.
	UploadFromURL(ctx context.Context, tenantID, rawURL string) (map[string]interface{}, error)
}

// chatAgentService is the subset of AgentService used by the chat-completion
// endpoints (AgentChatCompletions, RunAgent). Kept as a separate interface so
// handler tests can inject a fake RunAgent without standing up the full
// AgentService (DB DAOs, eino runner, etc.). The production wiring in
// NewAgentHandler assigns the concrete *service.AgentService — which
// satisfies this interface because its RunAgent signature matches.
type chatAgentService interface {
	RunAgent(ctx context.Context, userID, canvasID, sessionID, version string, userInput any, files []map[string]interface{}) (<-chan canvas.RunEvent, error)
}

// documentRerunService is the surface RerunAgent needs from the document
// service: re-run the ingestion pipeline a pipeline operation log points
// at, with document accessibility enforced inside the service. Defined as
// an interface (instead of taking the concrete *document.DocumentService)
// so handler tests can inject a stub without spinning up the full service
// (DB DAOs, storage clients, …). The production *document.DocumentService
// satisfies this interface because its RerunDocument signature matches.
type documentRerunService interface {
	RerunDocument(ctx context.Context, userID, logID string, dsl map[string]interface{}, componentID string) error
}

// Compile-time proof that the production document service satisfies the
// rerun surface, keeping the interface and the service in lockstep.
var _ documentRerunService = (*document.DocumentService)(nil)

// AgentHandler agent handler
type AgentHandler struct {
	agentService *service.AgentService
	chatRunner   chatAgentService
	fileService  agentFileService
	loader       canvasLoader
	// documentService is required by RerunAgent. Wired in
	// cmd/ragflow_server.go after NewAgentHandler (which doesn't take it
	// to preserve the existing test-friendly signature). RerunAgent fails
	// closed (500) when it is nil: skipping the service would bypass the
	// document ownership gate.
	documentService documentRerunService
	// redisGet fetches a raw string from Redis. Defaults to the global
	// client (redis.Get) so production behaviour is unchanged; tests inject
	// a miniredis-backed getter to exercise Agent log endpoints without a
	// live Redis (mirrors the newExecutor injection pattern).
	redisGet func(key string) (string, error)
	// redisStore writes the debug-run log array. Defaults to the global
	// client (redis.Get, which satisfies task.DebugLogStore); tests inject a
	// miniredis-backed writer so runCanvasPipelineDebug can be exercised without a
	// live Redis.
	redisStore task.DebugLogStore
	// webhookTraceAppender records webhook test-run events. Production uses
	// appendWebhookTrace; tests inject a recorder without mutating global Redis.
	webhookTraceAppender func(context.Context, string, time.Time, canvas.RunEvent)
	// newExecutor builds the pipeline executor for a debug run. Defaults to
	// task.NewPipelineExecutor; tests inject a fake so the debug path can be
	// driven without a real canvas/DSL.
	newExecutor func(taskCtx *task.TaskContext, canvasID string, docBulkSize int) (debugExecutor, error)
}

// debugExecutor is the minimal executor surface runCanvasPipelineDebug needs. It is an
// interface (not *task.PipelineExecutor) so tests can substitute a fake that
// emits progress through the attached sink. *task.PipelineExecutor satisfies it.
type debugExecutor interface {
	WithProgressSink(sink pipelinepkg.ProgressSink) *task.PipelineExecutor
	Execute(ctx context.Context) (*task.PipelineResult, error)
}

// WithDocumentService injects the document service used by
// RerunAgent to resolve the pipeline operation log, enforce
// DocumentService accessibility, and enqueue the rerun. Returns the
// receiver for chaining in the server wiring.
func (h *AgentHandler) WithDocumentService(s documentRerunService) *AgentHandler {
	h.documentService = s
	return h
}

// NewAgentHandler create agent handler

func NewAgentHandler(ctx context.Context, agentService *service.AgentService, fileService *file.FileService) *AgentHandler {
	return &AgentHandler{
		agentService: agentService,
		chatRunner:   agentService,
		fileService:  fileService,
		loader:       agentService,
		redisGet:     func(key string) (string, error) { return redis.Get().Get(ctx, key) },
		redisStore:   redis.Get(),
		newExecutor: func(taskCtx *task.TaskContext, canvasID string, docBulkSize int) (debugExecutor, error) {
			return task.NewPipelineExecutor(taskCtx, canvasID, docBulkSize)
		},
	}
}

// WithRedisGetter overrides the Redis string getter used by Agent log endpoints.
// Tests pass a miniredis-backed getter so polling can be exercised end-to-end
// without a live Redis.
func (h *AgentHandler) WithRedisGetter(f func(key string) (string, error)) *AgentHandler {
	h.redisGet = f
	return h
}

// WithRedisStore overrides the Redis writer used by runCanvasPipelineDebug to persist
// the debug-run log. Tests pass a miniredis-backed writer.
func (h *AgentHandler) WithRedisStore(s task.DebugLogStore) *AgentHandler {
	h.redisStore = s
	return h
}

// WithNewExecutor overrides the pipeline executor factory used by
// runCanvasPipelineDebug. Tests inject a fake that emits progress through the
// attached sink without a real canvas/DSL.
func (h *AgentHandler) WithNewExecutor(f func(taskCtx *task.TaskContext, canvasID string, docBulkSize int) (debugExecutor, error)) *AgentHandler {
	h.newExecutor = f
	return h
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
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}

	// Filter-aggregation mode: the agents page filter bar fetches
	// GET /api/v1/agents?type=filter and expects
	// {filter: {owner, canvas_category}, total} instead of a canvas list.
	if c.Query("type") == "filter" {
		filters, code, err := h.agentService.ListAgentFilters(c.Request.Context(), user.ID)
		if err != nil {
			common.ResponseWithCodeData(c, code, false, err.Error())
			return
		}
		common.SuccessWithData(c, filters, "success")
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
	ctx := c.Request.Context()

	result, code, err := h.agentService.ListAgents(
		ctx,
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
// Four classes:
//   - service.ErrAgentNotOwner  -> "Only the owner..."        (DELETE only, 103)
//   - service.ErrAgentSessionBusy -> "session already running" (103)
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
	if errors.Is(err, service.ErrAgentSessionBusy) {
		return common.CodeOperatingError, "This agent session is already running."
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
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}
	var req service.CreateAgentRequest
	if err := decodeJSONWithNumber(c.Request.Body, &req); err != nil && !errors.Is(err, io.EOF) {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	req.UserID = user.ID
	row, code, err := h.agentService.CreateAgent(c.Request.Context(), &req)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
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
	// Attach last_publish_time so the front-end can render when the agent
	// was last published (Python get_agent parity).
	lastPublishTime, err := h.agentService.GetLastPublishTime(c.Request.Context(), canvasID)
	if err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	common.SuccessWithData(c, &agentDetailResponse{UserCanvas: row, LastPublishTime: lastPublishTime}, "success")
}

// agentDetailResponse wraps the canvas row with derived fields that Python's
// get_agent handler adds to the response.
type agentDetailResponse struct {
	*entity.UserCanvas
	LastPublishTime *int64 `json:"last_publish_time,omitempty"`
}

// updateAgentRequest is the wire shape for PUT /api/v1/agents/:canvas_id.
type updateAgentRequest map[string]interface{}

func decodeJSONWithNumber(body io.Reader, target any) error {
	decoder := json.NewDecoder(body)
	decoder.UseNumber()
	return decoder.Decode(target)
}

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
	if err := decodeJSONWithNumber(c.Request.Body, &req); err != nil && !errors.Is(err, io.EOF) {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "Invalid request: "+err.Error())
		return
	}
	if req == nil {
		req = updateAgentRequest{}
	}
	if err := h.agentService.UpdateAgent(c.Request.Context(), user.ID, canvasID, req); err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	canvasInstance, err := h.agentService.GetAgent(c.Request.Context(), user.ID, canvasID)
	if err != nil || canvasInstance == nil {
		common.SuccessWithData(c, map[string]interface{}{}, "success")
		return
	}
	common.SuccessWithData(c, map[string]interface{}{
		"update_time": canvasInstance.UpdateTime,
	}, "success")
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
		common.ErrorWithCode(c, errorCode, errorMessage)
		return
	}
	ctx := c.Request.Context()
	templates, err := h.agentService.ListTemplates(ctx)
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
	if sessionID == "" {
		// Allocate the ordinary-Agent session identity at the HTTP boundary.
		// Persistence of the session record remains owned by AgentService.RunAgent.
		sessionID = utility.GenerateToken()
	}
	userInput := readUserInput(c)

	events, err := h.chatRunner.RunAgent(c.Request.Context(), user.ID, canvasID, sessionID, version, userInput, nil)
	if err != nil {
		ec, em := mapAgentError(err)
		common.ResponseWithCodeData(c, ec, nil, em)
		return
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	doneSent := false
	for ev := range events {
		if ev.Type == "done" {
			doneSent = true
		}
		if err := service.WriteChatbotRunEvent(c.Writer, ev); err != nil {
			common.Debug("agent run: client disconnected",
				zap.String("canvas_id", canvasID),
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
			return
		}
	}
	if !doneSent {
		if err := service.WriteChatbotRunEvent(c.Writer, canvas.RunEvent{Type: "done"}); err != nil {
			common.Debug("agent run: failed to write [DONE]",
				zap.String("canvas_id", canvasID),
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
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

// runCanvasPipelineDebug runs a canvas in pipeline dry-run (debug) mode: it builds
// an in-memory debug TaskContext and executes the pipeline synchronously,
// returning the produced chunks. The run is fully synchronous: the complete
// debug log (including the terminal END marker) is flushed to Redis BEFORE
// this method returns, so the message_id it yields is for *replay* —
// re-fetching the log after a page refresh or from a shared link — not for
// incremental streaming; the front-end's poll therefore receives a finished
// log on the first request.
//
// It performs no persistence (no MinIO upload, index insert, or pipeline_log)
// because the debug context carries no KB (KB.ID == ""), which is the single
// debug signal used across the ingestion pipeline: the tokenizer skips
// embedding and the executor skips the persist stage. kb_id == "" occurs ONLY
// in debug mode; production ingestion always supplies a KB, so this path is
// never taken in normal operation. Debug takes no kb_id at all: it never
// resolves an embedder or writes to a KB, and parse/chunk work without it.
// Callers extract the (fileName, fileData) pair from their own request shape
// and pass them in.
func (h *AgentHandler) runCanvasPipelineDebug(ctx context.Context, user *entity.User,
	canvasID, fileName string, fileData []byte) (*task.PipelineResult, error) {
	// messageID is the polling key the front-end reads from the run response
	// (see web/src/pages/agent/hooks/use-run-dataflow.ts:40) to fetch the debug
	// log. It is generated up-front so the response always carries a stable key.
	messageID := uuid.New().String()

	// Force an empty KB: a debug context has no knowledgebase, so kb_id == ""
	// holds and the debug signal is unambiguous.
	taskCtx := task.NewDebugTaskContext(user.ID, canvasID, fileName, fileData)

	exec, err := h.newExecutor(taskCtx, canvasID, 0)
	if err != nil {
		return nil, err
	}

	// Attach a sink that records component progress into the
	// [{component_id, trace}] array the front-end polls for.
	sink := task.NewDebugLogSink(canvasID, messageID, h.redisStore)
	exec.WithProgressSink(sink)

	result, runErr := exec.Execute(ctx)
	// Flush even on error so the front-end can poll the failure log; the END
	// marker carries the error and keeps the run detectable as finished.
	sink.Flush(ctx, runErr)
	if result == nil {
		// The executor may return (nil, err) on failure; synthesize a result so
		// the polling key survives to the response and the front-end can still
		// reach the failure log written just above.
		result = &task.PipelineResult{}
	}
	result.MessageID = messageID
	return result, runErr
}

// respondWithDebugResult writes the outcome of a dataflow debug run. On a run
// failure it keeps a non-success code (errors must not be masked as success)
// but still carries message_id in the error envelope's data, because
// runCanvasPipelineDebug already flushed the failure log to Redis — the front-end
// needs that key to poll the failure timeline. On success it returns the
// message_id (the polling key) together with the produced chunks. Empty chunk
// slices become `[]` rather than `null` so the response shape stays stable.
// This is the single wire-shape helper for the chat/completions DataFlow debug
// entry point, and it matches the contract the front-end expects:
// `data.data.message_id` drives the log-poll loop and `data.data.chunks`
// renders the debug output.
func respondWithDebugResult(c *gin.Context, result *task.PipelineResult, err error) {
	if err != nil {
		messageID := ""
		if result != nil {
			messageID = result.MessageID
		}
		common.ResponseWithCodeData(c, common.CodeServerError, gin.H{"message_id": messageID}, err.Error())
		return
	}
	messageID := ""
	var chunks []map[string]any
	if result != nil {
		messageID = result.MessageID
		chunks = result.Chunks
	}
	if len(chunks) == 0 {
		chunks = []map[string]any{}
	}
	common.SuccessWithData(c, gin.H{"message_id": messageID, "chunks": chunks}, "success")
}

// extractChatDebugFile pulls the first uploaded file from a chat/completions
// request into (fileName, bytes) for a dataflow debug run. Unlike a debug
// multipart upload, chat/completions `files` are storage references
// (id/name/created_by); the raw bytes are fetched from the per-user downloads
// bucket via the file service. When no usable file is present it returns a
// default name and a nil slice so the pipeline can still run file-less.
func (h *AgentHandler) extractChatDebugFile(ctx context.Context, files []map[string]interface{}, user *entity.User) (string, []byte) {
	if len(files) == 0 {
		return "debug", nil
	}
	fd := files[0]
	name, _ := fd["name"].(string)
	id, _ := fd["id"].(string)
	if id == "" {
		if name == "" {
			return "debug", nil
		}
		return name, nil
	}
	data, err := h.fileService.DownloadAgentFile(ctx, user.ID, id)
	if err != nil || len(data) == 0 {
		if name == "" {
			return "debug", nil
		}
		return name, nil
	}
	if name == "" {
		name = "debug"
	}
	return name, data
}

// CancelSessionRun cancels one ordinary Agent run by session id.
// @Summary Cancel Agent Session Run
// @Tags agents
// @Produce json
// @Param session_id path string true "session id"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/tasks/{session_id}/cancel [post]
func (h *AgentHandler) CancelSessionRun(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	if h.agentService == nil {
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "agent service unavailable")
		return
	}
	if err := h.agentService.CancelSessionRun(c.Request.Context(), user.ID, c.Param("session_id")); err != nil {
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
	if err := decodeJSONWithNumber(c.Request.Body, &req); err != nil && !errors.Is(err, io.EOF) {
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
	ctx := c.Request.Context()
	rows, err := h.agentService.ListTemplates(ctx)
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
	ctx := c.Request.Context()
	rows, errCode, err := h.agentService.ListAgentTags(ctx, user.ID, strings.TrimSpace(c.Query("canvas_category")))
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
	ctx := c.Request.Context()
	ok, errCode, errMsg := h.agentService.UpdateAgentTags(ctx, user.ID, canvasID, body.Tags)
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
	orderby := c.DefaultQuery("orderby", "update_time")
	descParam := c.Query("desc")
	desc := descParam != "false" && descParam != "False"
	sessionID := c.Query("id")
	queryUserID := c.Query("user_id")
	expUserID := c.Query("exp_user_id")
	dslParam := c.Query("dsl")
	includeDSL := dslParam != "false" && dslParam != "False"
	ctx := c.Request.Context()
	resp, code, err := h.agentService.ListAgentSessions(ctx, user.ID, user.ID, canvasID, service.ListAgentSessionsRequest{
		Page:       page,
		PageSize:   pageSize,
		Keywords:   keywords,
		FromDate:   fromDate,
		ToDate:     toDate,
		OrderBy:    orderby,
		Desc:       desc,
		SessionID:  sessionID,
		UserID:     queryUserID,
		ExpUserID:  expUserID,
		IncludeDSL: includeDSL,
	})
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}
	common.SuccessWithDataAndTotal(c, resp.Data, resp.Total, "success")
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
	ctx := c.Request.Context()
	row, code, err := h.agentService.CreateAgentSession(ctx, &service.CreateAgentSessionRequest{
		UserID:  user.ID,
		AgentID: canvasID,
		Name:    body.Name,
		Source:  body.Source,
		DSL:     body.DSL,
	})
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
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
	ctx := c.Request.Context()
	row, code, err := h.agentService.GetAgentSession(ctx, user.ID, canvasID, sessionID)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
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
	ctx := c.Request.Context()
	canvasID := c.Param("canvas_id")
	sessionID := c.Param("session_id")
	if sessionID != "" {
		ok, code, err := h.agentService.DeleteAgentSessionItem(ctx, user.ID, canvasID, sessionID)
		if err != nil {
			common.ErrorWithCode(c, code, err.Error())
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
	result, code, err := h.agentService.DeleteAgentSessions(ctx, user.ID, canvasID, ids, deleteAll)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}
	common.SuccessWithData(c, result, "success")
}

// AgentChatCompletions POST /api/v1/agents/chat/completions
//
// Runs the canvas against `agent_id`. Mirrors the Python contract in
// api/db/services/canvas_service.py:313 (`completion()`) and
// api/apps/restful_apis/agent_api.py:1440-1676.
//
//   - When `stream` is true: streams SSE — one `data: {...}\n\n` frame per
//     canvas RunEvent, terminated by `data:[DONE]\n\n`.
//   - When `stream` is omitted or false (default, matches the Python
//     agent_chat_completion contract where `req.get("stream", False)`
//     defaults to non-streaming): collects all canvas events and returns a
//     plain JSON response with `data.content` set to the concatenated
//     message content (matching Python's final_ans["data"]["content"]).
//   - OpenAI-compatible path: requires `messages` and returns the direct
//     OpenAI chat-completion wire format, including streaming deltas,
//     references, token usage, and the `[DONE]` terminator. The protocol
//     adapter lives in agent_openai.go so the regular Agent event contract
//     remains unchanged.
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
	// Files carries the uploaded file references for a run, normalized to
	// the 1D file-dict list. See agentFiles for the two accepted wire
	// shapes.
	Files agentFiles `json:"files"`
}

// agentFiles carries the uploaded file references for a canvas run. The
// wire shape differs by caller: the agent chat front-end posts a 1D list
// of file dicts (`files: [{id, ...}]`, web use-send-agent-message.ts),
// while the dataflow debug front-end wraps it one extra level
// (`files: [[{id, ...}]]`, web use-run-dataflow.ts). Python accepts both
// because each consumer path only sees the shape its own front-end sends:
// the chat path iterates the 1D list directly (canvas.py get_files_async)
// and the dataflow debug branch takes `files[0]` (agent_api.py
// queue_dataflow call site). We normalize both to the 1D list here; for
// the 2D shape the first inner list wins, matching Python's `files[0]`.
type agentFiles []map[string]interface{}

// UnmarshalJSON accepts both the 1D (`[{...}]`) and 2D (`[[{...}]]`)
// wire shapes described on agentFiles and normalizes to the 1D list.
func (f *agentFiles) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*f = nil
		return nil
	}
	var oneD []map[string]interface{}
	if err := json.Unmarshal(data, &oneD); err == nil {
		*f = oneD
		return nil
	}
	var twoD [][]map[string]interface{}
	if err := json.Unmarshal(data, &twoD); err != nil {
		return err
	}
	if len(twoD) > 0 {
		*f = twoD[0]
	}
	return nil
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
		content, err := service.NormalizeOpenAIMessageContent(messages[i]["content"])
		if err == nil && content != "" {
			return content
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

	if req.OpenAICompat {
		h.handleOpenAICompat(c, user, &req)
		return
	}

	// DataFlow canvas: run a synchronous, side-effect-free debug (dry-run)
	// and return the parsed chunks inline — reusing this existing
	// chat/completions endpoint instead of a dedicated dataflow/debug route.
	// This mirrors the Python agent_api.py:1569 DataFlow branch, which is
	// reached only on the non-openai, non-session path (openai-compatible
	// requests return their choices shape above and never enter debug). The
	// The canvas is loaded only to read its category, and the debug path is
	// skipped when the loader is absent or the canvas is not a DataFlow. No
	// kb_id is involved: the debug context always runs KB-less (see
	// runCanvasPipelineDebug).
	if h.loader != nil {
		if cv, lerr := h.loader.LoadCanvasByID(c.Request.Context(), user.ID, req.AgentID); lerr == nil && cv.CanvasCategory == "dataflow_canvas" {
			fileName, fileData := h.extractChatDebugFile(c.Request.Context(), req.Files, user)
			if len(fileData) == 0 {
				common.ResponseWithCodeData(c, common.CodeDataError, nil, "dataflow debug requires an uploaded file")
				return
			}
			result, derr := h.runCanvasPipelineDebug(c.Request.Context(), user, req.AgentID, fileName, fileData)
			respondWithDebugResult(c, result, derr)
			return
		}
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
	if req.SessionID == "" {
		// Keep the effective session available to the non-stream response and
		// to the task_id=session_id wire alias even when the canvas emits no
		// events (for example an empty query).
		req.SessionID = utility.GenerateToken()
	}

	// req.Files is already normalized to the 1D file list by the
	// agentFiles unmarshaler. A nil/empty list reaches RunAgent as-is so
	// its `len(files) > 0` guard sees a file-less run.
	events, err := h.chatRunner.RunAgent(c.Request.Context(), user.ID, req.AgentID, req.SessionID, "", userInput, req.Files)
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

	if req.Stream {
		// SSE streaming: one `data: {...}\n\n` frame per canvas RunEvent,
		// terminated by `data:[DONE]\n\n`. We do NOT emit an SSE `event:`
		// line — the front-end's use-send-message.ts parser feeds each
		// `data:` line directly into JSON.parse and expects the event type
		// in the JSON object's top-level `event` field.
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		emitted := false
		doneSent := false
		for ev := range events {
			emitted = true
			if ev.Type == "done" {
				doneSent = true
			}
			common.Debug("agent chat completions: streaming event",
				zap.String("agent_id", req.AgentID),
				zap.String("session_id", req.SessionID),
				zap.String("event_type", ev.Type),
				zap.String("message_id", ev.MessageID),
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
			// after this branch because the canvas never sends a
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
		}
		if !doneSent {
			if err := service.WriteChatbotRunEvent(c.Writer, canvas.RunEvent{Type: "done"}); err != nil {
				common.Debug("agent chat completions: failed to write [DONE]",
					zap.Error(err),
				)
			}
		}
		common.Debug("agent chat completions: stream closed",
			zap.String("agent_id", req.AgentID),
			zap.String("session_id", req.SessionID),
		)
		return
	}

	// Non-streaming: collect all canvas events, accumulate message
	// content and reference, then return a plain JSON response matching
	// Python's get_result(data=final_ans) contract (agent_api.py:1616-1676).
	var fullContent string
	reference := map[string]any{}
	structuredOutput := map[string]any{}
	var runUsage any
	var finalAns *canvas.RunEvent
	hasEvents := false
	for ev := range events {
		hasEvents = true
		var evData map[string]any
		if err := json.Unmarshal([]byte(ev.Data), &evData); err == nil {
			if ev.Type == "message" {
				if c, ok := evData["content"].(string); ok {
					fullContent += c
				}
				// Mirror Python agent_api.py: the reasoning segment stays
				// wrapped in <think> tags in the aggregated answer so the
				// chat UI can render the "thought" section.
				if st, _ := evData["start_to_think"].(bool); st {
					fullContent += "<think>"
				} else if et, _ := evData["end_to_think"].(bool); et {
					fullContent += "</think>"
				}
			}
			if ref, _ := evData["reference"].(map[string]any); ref != nil {
				for k, v := range ref {
					reference[k] = v
				}
			}
			// #4: collect structured_output from node_finished events
			// (Python: agent_api.py:1628-1633).
			if ev.Type == "node_finished" {
				if outputs, ok := evData["outputs"].(map[string]any); ok {
					if structured, ok := outputs["structured"]; ok {
						if componentID, ok := evData["component_id"].(string); ok {
							structuredOutput[componentID] = structured
						}
					}
				}
			}
			// #5: capture run_usage from workflow_finished events
			// (Python: agent_api.py:1641-1644).
			if ev.Type == "workflow_finished" {
				if u := evData["usage"]; u != nil {
					runUsage = u
				}
			}
		}

		// Python final_ans selection: prefer "message_end" event,
		// skip "workflow_finished" (set by canvas as the last event
		// but should not be the final answer), use "user_inputs" as
		// fallback.
		if ev.Type == "message_end" {
			copy := ev
			finalAns = &copy
		}
		if ev.Type == "workflow_finished" {
			continue
		}
		if finalAns == nil {
			copy := ev
			finalAns = &copy
		}
	}

	if !hasEvents || finalAns == nil {
		// Canvas produced no events (e.g. empty query). Echo the
		// session_id so the client can resume the conversation
		// (fixes #15169). Python returns get_result(data={"session_id": session_id}).
		common.Info("empty agent output - returning session_id",
			zap.String("agent_id", req.AgentID),
			zap.String("session_id", req.SessionID),
		)
		common.SuccessWithData(c, gin.H{"session_id": req.SessionID}, "success")
		return
	}

	// Build the final answer payload:
	//   final_ans["data"]["content"] = full_content
	//   final_ans["data"]["reference"] = reference
	//   final_ans["data"]["usage"] = run_usage (if present)
	//   final_ans["data"]["structured"] = structured_output (if present)
	var ansData map[string]any
	if err := json.Unmarshal([]byte(finalAns.Data), &ansData); err != nil || ansData == nil {
		ansData = map[string]any{}
	}
	ansData["content"] = fullContent
	if len(reference) > 0 {
		ansData["reference"] = reference
	}
	if runUsage != nil {
		ansData["usage"] = runUsage
	}
	if len(structuredOutput) > 0 {
		ansData["structured"] = structuredOutput
	}

	result := gin.H{
		"event":      finalAns.Type,
		"data":       ansData,
		"message_id": finalAns.MessageID,
		"created_at": finalAns.CreatedAt,
		"task_id":    finalAns.SessionID,
		"session_id": finalAns.SessionID,
	}
	common.SuccessWithData(c, result, "success")
}

// RerunAgent POST /api/v1/agents/rerun — requires id, dsl, and
// component_id. The front-end dataflow "view result" page sends the
// pipeline operation LOG id plus the edited DSL (web
// useRerunDataflow); the service resolves log -> document, enforces
// ownership, persists the edited DSL, and re-enqueues the ingestion
// run (see DocumentService.RerunDocument).
//
// DocumentService.RerunDocument enforces accessibility BEFORE the
// rerun. The gate is REQUIRED: a nil documentService turns a wiring
// miss into an auth bypass (any caller could rerun an arbitrary doc
// id without an ownership check), so we fail closed with 500 instead
// of accepting the request. On denial the service returns
// "Document not found." so a caller cannot probe whether a document
// exists in another tenant.
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
	// means the handler was wired without the rerun service, which
	// would let any caller rerun an arbitrary doc id without proving
	// ownership. Surface as a 500 so a missing dependency is loud,
	// not silent.
	if h.documentService == nil {
		zap.L().Error("RerunAgent: documentService is nil; refusing request to prevent auth bypass")
		common.ResponseWithCodeData(c, common.CodeServerError, nil, "server misconfiguration: document service not wired")
		return
	}
	if err := h.documentService.RerunDocument(c.Request.Context(), user.ID, body.ID, body.DSL, body.ComponentID); err != nil {
		// Domain failures (unknown log/document, access denial,
		// document mid-run) map to the data-error envelope (code 102)
		// with the service's caller-safe message. Everything else is
		// an internal failure: log it server-side and answer 500 with
		// a generic message so internals (DB errors, queue failures)
		// are neither leaked to the tenant nor mislabeled as a data
		// error.
		var processingErr *document.RerunDocumentProcessingError
		switch {
		case errors.Is(err, document.ErrRerunDocumentNotFound):
			common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		case errors.As(err, &processingErr):
			common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		default:
			zap.L().Error("RerunAgent: rerun failed",
				zap.String("log_id", body.ID),
				zap.Error(err))
			common.ResponseWithCodeData(c, common.CodeServerError, nil, "rerun failed, please try again later")
		}
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
	ctx := c.Request.Context()
	code, err := h.agentService.TestDBConnection(ctx, user.ID, &req)
	if err != nil {
		common.ErrorWithCode(c, code, err.Error())
		return
	}
	common.SuccessWithData(c, true, "success")
}

// parseAgentLogs decodes the debug-log payload stored in Redis by the pipeline
// run. The Python pipeline (rag/flow/pipeline.py callback) writes a JSON
// *array* ([{component_id, trace:[...]}]) that the front-end's
// useFetchMessageTrace (web/src/hooks/use-agent-request.ts) requires as
// response.data. A missing, empty, or corrupt payload collapses to an empty
// object so the contract from get_agent_logs (api/apps/restful_apis/agent_api.py:1087)
// — `code == 0` and `data` is a (possibly empty) dict/array — stays satisfied
// instead of 500ing on bad bytes.
func parseAgentLogs(payload string) interface{} {
	if strings.TrimSpace(payload) == "" {
		return map[string]interface{}{}
	}
	var data interface{}
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return map[string]interface{}{}
	}
	return data
}

// GetAgentLogs GET /api/v1/agents/:canvas_id/logs/:message_id
//
// Reads "{agent_id}-{message_id}-logs" from Redis (same key format
// used by the Python agent API in api/apps/restful_apis/agent_api.py) and
// returns it as-is. Because runCanvasPipelineDebug flushes the ENTIRE log (including
// the END marker) before returning the run response, this endpoint returns a
// completed snapshot on the first poll — it is a replay/fetch of an already
// finished debug log, not a streaming subscription. Missing key returns an
// empty dict so the test contract `data is dict` and `code == 0` are both
// satisfied when nothing has been written yet.
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
	payload, rerr := h.redisGet(key)
	var data interface{} = map[string]interface{}{}
	if rerr == nil {
		data = parseAgentLogs(payload)
	}
	common.SuccessWithData(c, data, "success")
}

// GetAgentWebhookLogs GET /api/v1/agents/:canvas_id/webhook/logs
//
// The Python agent API returns 102 "Canvas not found." when the agent
// id does not resolve to a canvas owned by the caller (see
// api/apps/restful_apis/agent_api.py webhook_trace). We replicate
// that envelope here so the front-end poll does not surface a 500
// for unknown / foreign canvas ids. Polling follows the same cursor
// protocol: establish since_ts, discover a run, then fetch events by
// webhook_id until a finished event arrives.
func (h *AgentHandler) GetAgentWebhookLogs(c *gin.Context) {
	user, code, msg := GetUser(c)
	if code != common.CodeSuccess {
		common.ResponseWithCodeData(c, code, nil, msg)
		return
	}
	canvasID := c.Param("canvas_id")
	ctx := c.Request.Context()
	ok, err := h.agentService.CheckCanvasAccess(ctx, user.ID, canvasID)
	if err != nil || !ok {
		// CheckCanvasAccess now surfaces ErrUserCanvasNotFound when
		// the canvas row is missing; the Python envelope is
		// indistinguishable for missing vs foreign, so collapse
		// both into 102 "Canvas not found." here.
		if err != nil && !errors.Is(err, dao.ErrUserCanvasNotFound) {
			jsonInternalError(c, err)
			return
		}
		common.ResponseWithCodeData(c, common.CodeDataError, nil, "Canvas not found.")
		return
	}

	sinceTS, hasSinceTS := parseWebhookSinceTS(c.Query("since_ts"))
	if !hasSinceTS {
		common.SuccessWithData(c, newWebhookTracePoll(nil, float64(time.Now().UnixNano())/1e9, false), "success")
		return
	}
	if h.redisGet == nil {
		jsonInternalError(c, errors.New("agent webhook trace: redis getter not configured"))
		return
	}
	payload, err := h.redisGet(fmt.Sprintf("webhook-trace-%s-logs", canvasID))
	if err != nil {
		jsonInternalError(c, fmt.Errorf("read agent webhook trace: %w", err))
		return
	}
	result, err := pollWebhookTrace(payload, sinceTS, c.Query("webhook_id"))
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	common.SuccessWithData(c, result, "success")
}

// checkCanvasAccessForHandler is the shared 103 envelope helper for
// PR2 routes that need to call service.CheckCanvasAccess and surface
// the access-denied envelope with the same shape the existing
// loadCanvasForUser-based handlers use.
func (h *AgentHandler) checkCanvasAccessForHandler(c *gin.Context, userID, canvasID string) (bool, common.ErrorCode, string) {
	ctx := c.Request.Context()
	ok, err := h.agentService.CheckCanvasAccess(ctx, userID, canvasID)
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
