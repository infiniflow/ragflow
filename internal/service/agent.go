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

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"ragflow/internal/service/file"
	"ragflow/internal/utility"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/compose"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
	agentsandbox "ragflow/internal/agent/sandbox"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/tokenizer"

	dslpkg "ragflow/internal/agent/dsl"
)

// webhookPayloadKey is the unexported context key RunAgent reads to
// inject root["webhook_payload"]. Only the AgentService.RunAgentWithWebhook
// public wrapper sets it; the chat / agent-run paths leave it absent so
// existing callers see no behavior change.
//
// We deliberately do NOT surface the payload as a new RunAgent parameter
// — keeping the public signature stable means existing tests
// (agent_run_e2e_test.go, agent_wait_for_user_test.go) keep compiling.
type webhookPayloadKey struct{}

// LoadCanvasByID is the read-side counterpart of loadCanvasForUser that
// the webhook handler uses. It deliberately returns the raw DAO/service
// error (no error-code mapping) because the webhook envelope is 102
// "Canvas not found." while the chat/run envelope is 103 "Make sure you
// have permission..." — the choice must stay at the HTTP layer where
// each handler knows its own spec.
//
// Mirrors python: api/apps/restful_apis/agent_api.py:1570
// (`UserCanvasService.get_by_id(agent_id)`), with the same IDOR guard
// the chat handler uses.
func (s *AgentService) LoadCanvasByID(
	ctx context.Context, userID, canvasID string,
) (*entity.UserCanvas, error) {
	return s.loadCanvasForUser(ctx, userID, canvasID)
}

// RunAgentWithWebhook is a thin wrapper over RunAgent that attaches the
// webhook payload to the runner root so the Begin component can surface
// it as state.Sys["webhook_payload"] for downstream components.
//
// The payload is intentionally passed via context value (rather than a new
// RunAgent parameter) to keep the public RunAgent signature stable for
// the existing chat tests.
//
// Mirrors python: api/apps/restful_apis/agent_api.py:2125
// (`canvas.run(..., webhook_payload=clean_request)`).
func (s *AgentService) RunAgentWithWebhook(
	ctx context.Context, userID, canvasID string, payload map[string]any,
) (<-chan canvas.RunEvent, error) {
	if payload != nil {
		ctx = context.WithValue(ctx, webhookPayloadKey{}, payload)
	}
	return s.RunAgent(ctx, userID, canvasID, "", "", "", nil)
}

func emitAgentMessageEvents(emit func(string, string), answer, thinking string, reference any) {
	for _, ev := range buildAgentMessageEvents(answer, thinking, reference) {
		data, _ := json.Marshal(ev)
		emit("message", string(data))
	}
}

type agentMessageDeltaEmitter struct {
	emit              func(string, string)
	thinkState        *ThinkStreamState
	inThinking        bool
	explicitReasoning bool
	emitted           bool
}

func newAgentMessageDeltaEmitter(emit func(string, string)) *agentMessageDeltaEmitter {
	return &agentMessageDeltaEmitter{
		emit:       emit,
		thinkState: &ThinkStreamState{},
	}
}

func (e *agentMessageDeltaEmitter) emitEvent(ev canvas.MessageEvent) {
	emitAgentMessageEvent(e.emit, ev)
	e.emitted = true
}

func (e *agentMessageDeltaEmitter) startThinking() {
	if e.inThinking {
		return
	}
	e.emitEvent(canvas.MessageEvent{StartToThink: true})
	e.inThinking = true
}

func (e *agentMessageDeltaEmitter) endThinking() {
	if !e.inThinking {
		return
	}
	e.emitEvent(canvas.MessageEvent{EndToThink: true})
	e.inThinking = false
}

func (e *agentMessageDeltaEmitter) emitThinkDeltas(deltas []ThinkDelta) {
	for _, d := range deltas {
		switch {
		case d.Kind == ThinkDeltaMarker && d.Value == thinkOpen:
			e.startThinking()
		case d.Kind == ThinkDeltaMarker && d.Value == thinkClose:
			e.endThinking()
		case d.Kind == ThinkDeltaText && d.Value != "":
			e.emitEvent(canvas.MessageEvent{Content: d.Value})
		}
	}
}

func (e *agentMessageDeltaEmitter) Emit(contentDelta, thinkingDelta string) {
	if thinkingDelta != "" {
		e.startThinking()
		e.explicitReasoning = true
		e.emitEvent(canvas.MessageEvent{Content: thinkingDelta})
	}
	if contentDelta == "" {
		return
	}
	if e.explicitReasoning {
		e.endThinking()
		e.explicitReasoning = false
	}
	e.emitThinkDeltas(NextThinkDelta(e.thinkState, contentDelta, 0))
}

func (e *agentMessageDeltaEmitter) Finalize() bool {
	before := e.emitted
	e.emitThinkDeltas(FlushRemaining(e.thinkState))
	if e.explicitReasoning || e.inThinking {
		e.endThinking()
		e.explicitReasoning = false
	}
	return e.emitted && !before
}

func (e *agentMessageDeltaEmitter) Reset() {
	e.thinkState = &ThinkStreamState{}
	e.inThinking = false
	e.explicitReasoning = false
	e.emitted = false
}

func makeAgentMessageDeltaEmitter(emit func(string, string)) func(string, string) {
	return newAgentMessageDeltaEmitter(emit).Emit
}

func makeAgentMessageDeltaEmitterWithFinalizer(emit func(string, string)) (func(string, string), func() bool, func()) {
	emitter := newAgentMessageDeltaEmitter(emit)
	return emitter.Emit, emitter.Finalize, emitter.Reset
}

func emitAgentMessageEvent(emit func(string, string), ev canvas.MessageEvent) {
	data, _ := json.Marshal(ev)
	emit("message", string(data))
}

func buildAgentMessageEvents(answer, thinking string, reference any) []canvas.MessageEvent {
	answer, thinking = splitInlineThink(answer, thinking)
	if thinking == "" {
		return []canvas.MessageEvent{{
			Content:   answer,
			Reference: reference,
		}}
	}

	events := []canvas.MessageEvent{{StartToThink: true}}
	for _, chunk := range splitMessageContent(thinking) {
		events = append(events, canvas.MessageEvent{Content: chunk})
	}
	events = append(events, canvas.MessageEvent{EndToThink: true})
	for _, chunk := range splitMessageContent(answer) {
		events = append(events, canvas.MessageEvent{Content: chunk})
	}
	return events
}

func splitInlineThink(answer, thinking string) (string, string) {
	if thinking != "" {
		return answer, thinking
	}
	const startTag = "<think>"
	const endTag = "</think>"
	start := strings.Index(answer, startTag)
	if start < 0 {
		return answer, thinking
	}
	afterStart := start + len(startTag)
	endRel := strings.Index(answer[afterStart:], endTag)
	if endRel < 0 {
		return answer, thinking
	}
	end := afterStart + endRel
	thinking = answer[afterStart:end]
	answer = answer[:start] + answer[end+len(endTag):]
	answer = strings.TrimLeft(answer, "\r\n")
	return answer, thinking
}

func splitMessageContent(content string) []string {
	if content == "" {
		return nil
	}
	const maxRunes = 24
	runes := []rune(content)
	chunks := make([]string, 0, (len(runes)+maxRunes-1)/maxRunes)
	for len(runes) > 0 {
		n := maxRunes
		if len(runes) < n {
			n = len(runes)
		}
		chunks = append(chunks, string(runes[:n]))
		runes = runes[n:]
	}
	return chunks
}

// ErrAgentNotOwner is returned by DeleteAgent when the canvas exists and
// is accessible to the caller but is owned by a different user. It maps
// to the Python "Only the owner of the agent is authorized for this
// operation." message via handler.mapAgentError.
//
// The Python agent API keeps access-check and owner-check as two
// separate decorators (api/apps/restful_apis/agent_api.py:74-100);
// we mirror that distinction with ErrUserCanvasNotFound (access) and
// ErrAgentNotOwner (owner).
var ErrAgentNotOwner = errors.New("agent not owned by user")

// ErrAgentStorageError is returned by RunAgent when the underlying
// version / canvas / tenant DAO surfaces a non-sentinel error (DB
// connectivity, schema drift, deadlock, etc.). The handler's
// mapAgentError recognises this sentinel and maps it to
// common.CodeServerError (500) with a SANITIZED message — the raw
// DAO error string is never echoed to the client, so internal
// connection-string / table-name leaks are avoided.
//
// v3.5.2 follow-up: the prior af2ac2eda commit claimed "DB error ->
// 500" in the branch table, but the handler's mapAgentError did not
// actually classify those errors as CodeServerError — every DAO
// failure fell through to CodeDataError with the raw err.Error()
// string. This sentinel closes that gap.
var ErrAgentStorageError = errors.New("agent storage error")

// AgentService agent service
type AgentService struct {
	canvasDAO           *dao.UserCanvasDAO
	canvasTemplateDAO   *dao.CanvasTemplateDAO
	userDAO             *dao.UserDAO
	userTenantDAO       *dao.UserTenantDAO
	versionDAO          *dao.UserCanvasVersionDAO
	api4ConversationDAO *dao.API4ConversationDAO

	// driver is the per-process runner that drives canvas
	// invocations and produces SSE events. V1 persistence is
	// in-memory; a follow-up phase moves to Redis per plan §4.9.
	runner *canvas.Runner

	// Phase 4.4 V2 — Redis-backed run infrastructure. nil = in-memory
	// / no-tracking (test path, current production boot path until
	// cmd/server_main.go wires them in v3.6.0).
	//
	// checkpointStore + stateSerializer feed canvas.WithCheckPointStore
	// / canvas.WithStateSerializer so every Compile's check-point
	// payload and CanvasState snapshot round-trip to Redis.
	checkpointStore canvas.CheckPointStore
	stateSerializer canvas.StateSerializer

	// runTracker records per-run lifecycle (Start / MarkSucceeded /
	// MarkFailed / MarkCancelled) to Redis hash "agent:run:{id}".
	runTracker *canvas.RunTracker

	// runMu and runStreams coordinate active canvas run goroutines so that
	// CancelAgent can signal a specific canvas. The map is keyed by canvas
	// ID; values are channels that close to signal cancellation.
	runMu      sync.Mutex
	runStreams map[string]chan struct{}
}

// NewAgentService create agent service
func NewAgentService() *AgentService {
	return NewAgentServiceWithOptions(nil, nil, nil)
}

// NewAgentServiceWithOptions is the production constructor that
// injects the Redis-backed run infrastructure. The zero-arg
// NewAgentService() remains as a thin wrapper that calls this with
// all-nil options so existing call sites (cmd/server_main.go,
// handler tests, agent_test.go) keep compiling.
//
// Phase 4.4 V2: production boot wiring is deferred to v3.6.0; until
// then, tests can construct AgentService instances with mocked
// stores/tracker to exercise the real Compile/Invoke path without
// requiring Redis.
func NewAgentServiceWithOptions(
	cp canvas.CheckPointStore,
	ser canvas.StateSerializer,
	rt *canvas.RunTracker,
) *AgentService {
	if stub, ok := agenttool.GetSandboxClient().(interface{ IsStubSandboxClient() bool }); ok && stub.IsStubSandboxClient() {
		agenttool.SetSandboxClient(agentsandbox.NewManagerClient())
	}
	return &AgentService{
		canvasDAO:           dao.NewUserCanvasDAO(),
		canvasTemplateDAO:   dao.NewCanvasTemplateDAO(),
		userDAO:             dao.NewUserDAO(),
		userTenantDAO:       dao.NewUserTenantDAO(),
		versionDAO:          dao.NewUserCanvasVersionDAO(),
		api4ConversationDAO: dao.NewAPI4ConversationDAO(),
		runner:              canvas.NewRunner(),
		runStreams:          make(map[string]chan struct{}),
		checkpointStore:     cp,
		stateSerializer:     ser,
		runTracker:          rt,
	}
}

// ListTemplates returns every canvas template. Mirrors Python
// agent_api.list_agent_template, which iterates CanvasTemplateService.get_all()
// and serialises each row.
func (s *AgentService) ListTemplates() ([]*entity.CanvasTemplate, error) {
	return s.canvasTemplateDAO.GetAll()
}

// AgentItem is one entry in the list response.
type AgentItem struct {
	ID             string  `json:"id"`
	Avatar         *string `json:"avatar,omitempty"`
	Title          *string `json:"title,omitempty"`
	Description    *string `json:"description,omitempty"`
	Permission     string  `json:"permission"`
	UserID         string  `json:"user_id"`
	TenantID       string  `json:"tenant_id"`
	Nickname       string  `json:"nickname"`
	TenantAvatar   *string `json:"tenant_avatar,omitempty"`
	Tags           string  `json:"tags"`
	CanvasType     *string `json:"canvas_type,omitempty"`
	CanvasCategory string  `json:"canvas_category"`
	CreateTime     *int64  `json:"create_time,omitempty"`
	UpdateTime     *int64  `json:"update_time,omitempty"`
}

// ListAgentsResponse is the response body for GET /api/v1/agents.
type ListAgentsResponse struct {
	Canvas []*AgentItem `json:"canvas"`
	Total  int64        `json:"total"`
}

type AgentTagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

func toAgentItem(c *dao.UserCanvasListItem) *AgentItem {
	nickname := ""
	if c.Nickname != nil {
		nickname = *c.Nickname
	}
	if nickname == "" {
		nickname = c.TenantID
	}
	return &AgentItem{
		ID:             c.ID,
		Avatar:         c.Avatar,
		Title:          c.Title,
		Description:    c.Description,
		Permission:     c.Permission,
		UserID:         c.UserID,
		TenantID:       c.TenantID,
		Nickname:       nickname,
		TenantAvatar:   c.TenantAvatar,
		CanvasType:     c.CanvasType,
		CanvasCategory: c.CanvasCategory,
		Tags:           c.Tags,
		CreateTime:     c.CreateTime,
		UpdateTime:     c.UpdateTime,
	}
}

// ListAgents returns agent canvases visible to userID.
// Mirrors Python agent_api.list_agents — validates owner_ids against joined tenants,
// then delegates to the DAO.
func (s *AgentService) ListAgents(userID string, keywords string, page, pageSize int, orderBy string, desc bool, ownerIDs []string, canvasCategory, canvasType string, tags []string) (*ListAgentsResponse, common.ErrorCode, error) {
	// Build the set of tenant IDs the user is authorized to query.
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to get tenant IDs: %w", err)
	}
	authorised := make(map[string]struct{}, len(tenantIDs)+1)
	for _, id := range tenantIDs {
		authorised[id] = struct{}{}
	}
	authorised[userID] = struct{}{}

	var effectiveOwnerIDs []string
	if len(ownerIDs) > 0 {
		for _, id := range ownerIDs {
			if _, ok := authorised[id]; !ok {
				return nil, common.CodeOperatingError, fmt.Errorf("only authorized owner_ids can be queried")
			}
		}
		effectiveOwnerIDs = ownerIDs
	} else {
		effectiveOwnerIDs = make([]string, 0, len(authorised))
		for id := range authorised {
			effectiveOwnerIDs = append(effectiveOwnerIDs, id)
		}
	}

	canvases, total, err := s.canvasDAO.ListByTenantIDs(
		effectiveOwnerIDs,
		userID,
		page,
		pageSize,
		orderBy,
		desc,
		keywords,
		canvasCategory,
		canvasType,
		tags,
	)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to list agents: %w", err)
	}

	items := make([]*AgentItem, len(canvases))
	for i, c := range canvases {
		items[i] = toAgentItem(c)
	}
	return &ListAgentsResponse{Canvas: items, Total: total}, common.CodeSuccess, nil
}

// CreateAgentRequest is the input shape for CreateAgent.
type CreateAgentRequest struct {
	UserID         string         `json:"user_id"`
	Title          *string        `json:"title,omitempty"`
	Description    *string        `json:"description,omitempty"`
	Permission     string         `json:"permission"`
	CanvasType     *string        `json:"canvas_type,omitempty"`
	CanvasCategory string         `json:"canvas_category"`
	DSL            entity.JSONMap `json:"dsl,omitempty"`
}

// CreateAgent inserts a new user_canvas row. ID is assigned here.
//
// Returns the standard (T, common.ErrorCode, error) triple so the handler
// can map validation/duplicate errors to codes 101/102 without
// introducing a separate error type. Missing DSL or title and a
// duplicate title under the same owner all surface as specific code
// values that the Python agent API contract expects.
func (s *AgentService) CreateAgent(ctx context.Context, req *CreateAgentRequest) (*entity.UserCanvas, common.ErrorCode, error) {
	if req == nil {
		return nil, common.CodeArgumentError, errors.New("create agent: nil request")
	}
	if req.DSL == nil {
		return nil, common.CodeArgumentError, errors.New("no DSL data in request")
	}
	if req.Title == nil || strings.TrimSpace(*req.Title) == "" {
		return nil, common.CodeArgumentError, errors.New("no title in request")
	}
	title := strings.TrimSpace(*req.Title)
	req.Title = &title

	if existing, err := s.canvasDAO.GetByUserAndTitle(req.UserID, title, req.CanvasCategory); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("check duplicate title: %w", err)
	} else if existing != nil {
		return nil, common.CodeDataError, errors.New(title + " already exists.")
	}

	if req.Permission == "" {
		req.Permission = "me"
	}
	if req.CanvasCategory == "" {
		req.CanvasCategory = "agent_canvas"
	}
	// Normalize legacy v1 / Go-v2 payloads to a React-Flow-shaped graph so
	// the front-end can render the canvas without a migration. Idempotent;
	// no-op when graph.nodes is already non-empty.
	req.DSL = dslpkg.NormalizeForCanvas(req.DSL)
	row := &entity.UserCanvas{
		ID:             utility.GenerateUUID(),
		UserID:         req.UserID,
		Title:          req.Title,
		Description:    req.Description,
		Permission:     req.Permission,
		CanvasType:     req.CanvasType,
		CanvasCategory: req.CanvasCategory,
		DSL:            req.DSL,
	}
	if err := s.canvasDAO.Create(row); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("create agent: %w", err)
	}
	return row, common.CodeSuccess, nil
}

// loadCanvasForUser is the shared IDOR guard used by every non-List
// canvas method. It resolves the caller's tenant set, then asks the DAO
// to load the canvas subject to the (owner OR team-in-tenant) predicate.
// On miss or access-deny it returns dao.ErrUserCanvasNotFound so the
// handler layer can map every "not yours" case to the same 404 envelope
// — see plan §4.8 IDOR mitigation.
//
// DAO-error sanitization (v3.5.2 follow-up): the raw userTenantDAO and
// canvasDAO errors are wrapped with ErrAgentStorageError so mapAgentError
// classifies them as CodeServerError (500) with a sanitized message —
// the original DAO error string (DSN, table name, gorm stack frame)
// MUST NOT reach the client. Sentinels (ErrUserCanvasNotFound) pass
// through unchanged so they keep mapping to the 404 envelope.
//
// This is the FIRST storage access path RunAgent hits, so leaving the
// raw errors here would have left a DAO-string leak in the very first
// hop — the earlier af2ac2eda + 804854a5e commits only sanitized the
// version-read path, missing the canvas-access path.
func (s *AgentService) loadCanvasForUser(ctx context.Context, userID, canvasID string) (*entity.UserCanvas, error) {
	if canvasID == "" {
		return nil, dao.ErrUserCanvasNotFound
	}
	if userID == "" {
		return nil, dao.ErrUserCanvasNotFound
	}
	tenants, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		if errors.Is(err, dao.ErrUserCanvasNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("tenants for user %s: %w: %w", userID, err, ErrAgentStorageError)
	}
	row, err := s.canvasDAO.GetByIDForUser(canvasID, userID, tenants)
	if err != nil {
		if errors.Is(err, dao.ErrUserCanvasNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("load canvas %q for user %s: %w: %w", canvasID, userID, err, ErrAgentStorageError)
	}
	return row, nil
}

// GetAgent returns a single canvas visible to the requesting user.
// Returns dao.ErrUserCanvasNotFound (not 403) when the canvas is missing
// or belongs to another user.
func (s *AgentService) GetAgent(ctx context.Context, userID, canvasID string) (*entity.UserCanvas, error) {
	return s.loadCanvasForUser(ctx, userID, canvasID)
}

// UpdateAgent applies a draft patch to user_canvas. Settings updates may omit
// dsl; in that case the existing draft DSL must be preserved.
func (s *AgentService) UpdateAgent(ctx context.Context, userID, canvasID string, patch map[string]interface{}) error {
	canvasInstance, err := s.loadCanvasForUser(ctx, userID, canvasID)
	if err != nil {
		return err
	}

	updates := map[string]interface{}{}
	for _, key := range []string{"title", "avatar", "description", "permission", "canvas_type", "canvas_category"} {
		if value, ok := patch[key]; ok && value != nil {
			if key == "title" {
				if title, ok := value.(string); ok {
					value = strings.TrimSpace(title)
				}
			}
			updates[key] = value
		}
	}
	if dsl, ok := patch["dsl"]; ok && dsl != nil {
		dslMap, ok := dsl.(map[string]interface{})
		if !ok {
			if typed, ok := dsl.(entity.JSONMap); ok {
				dslMap = typed
			} else {
				return fmt.Errorf("update agent %s: dsl must be an object", canvasID)
			}
		}
		updates["dsl"] = entity.JSONMap(dslpkg.NormalizeForCanvas(dslMap))
	}

	_, err = s.canvasDAO.UpdateFields(canvasID, updates)
	if err != nil {
		return fmt.Errorf("update agent %s: %w", canvasID, err)
	}
	if dslValue, ok := updates["dsl"]; ok {
		dsl, ok := dslValue.(entity.JSONMap)
		if !ok {
			return fmt.Errorf("update agent %s: normalized dsl must be an object", canvasID)
		}
		title := ""
		if value, ok := updates["title"]; ok {
			title, _ = value.(string)
		} else if canvasInstance.Title != nil {
			title = *canvasInstance.Title
		}
		if _, err := s.saveOrReplaceVersion(ctx, userID, canvasID, dsl, title, nil, false); err != nil {
			return fmt.Errorf("update agent %s: save version: %w", canvasID, err)
		}
	}
	return nil
}

// ResetAgent clears the per-run state of a canvas (history, retrieval,
// memory, path) and zeroes every "sys.*" / "env.*" global, mirroring
// the Python handler at api/apps/restful_apis/agent_api.py:992. The
// reset transform is a pure DSL mutation; the persisted row in
// user_canvas.dsl is rewritten in place and the freshly reset DSL is
// returned so the caller can render it back to the client without an
// extra GET.
//
// Reset does NOT create a new user_canvas_version row. It also does NOT touch
// the in-flight run state of any currently executing canvas session; that is
// owned by the Python task executor and is out of scope for the Go port.
//
// Errors propagate the same way as GetAgent: a missing canvas, or a
// canvas that the user has no access to, surfaces as
// dao.ErrUserCanvasNotFound so mapAgentError emits the same 404 the
// Python handler does for "canvas not found."
func (s *AgentService) ResetAgent(ctx context.Context, userID, canvasID string) (entity.JSONMap, error) {
	row, err := s.loadCanvasForUser(ctx, userID, canvasID)
	if err != nil {
		return nil, err
	}
	reset := dslpkg.ResetForCanvas(row.DSL)
	// Re-normalize through the same entry point UpdateAgent uses so
	// any front-end that reads `graph.nodes` / `components[*].obj`
	// right after the response sees a renderable shape, not a partial
	// reset that left the legacy short-form DSL intact.
	row.DSL = dslpkg.NormalizeForCanvas(reset)
	row.Release = false
	if err := s.canvasDAO.Update(row); err != nil {
		return nil, fmt.Errorf("reset agent %s: %w", canvasID, err)
	}
	return row.DSL, nil
}

// DeleteAgent removes the canvas and cascades to its user_canvas_version
// rows in a single transaction so a mid-flight failure cannot leave
// orphan version rows (Phase 5 §2.9; review follow-up M2).
//
// Owner-only by design (mirrors _require_canvas_owner_sync in the Python
// agent API). Both "canvas does not exist" and "canvas is owned by
// someone else" surface as ErrAgentNotOwner so the handler emits the
// single "Only the owner..." 103 message — same envelope as the Python
// decorator (api/apps/restful_apis/agent_api.py:94-100), which uses
// UserCanvasService.query(user_id=..., id=...) and conflates those two
// cases into one OPERATING_ERROR response.
func (s *AgentService) DeleteAgent(ctx context.Context, userID, canvasID string) error {
	row, err := s.canvasDAO.GetByID(canvasID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAgentNotOwner
		}
		return fmt.Errorf("load agent %s: %w", canvasID, err)
	}
	if row.UserID != userID {
		return ErrAgentNotOwner
	}
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		if _, err = s.versionDAO.DeleteByCanvasIDTx(tx, canvasID); err != nil {
			return fmt.Errorf("delete agent: cascade versions: %w", err)
		}
		if err = s.canvasDAO.DeleteTx(tx, canvasID); err != nil {
			return fmt.Errorf("delete agent %s: %w", canvasID, err)
		}
		return nil
	})
}

// PublishAgentRequest is the input shape for PublishAgent.
type PublishAgentRequest struct {
	Title       *string        `json:"title,omitempty"`
	Description *string        `json:"description,omitempty"`
	DSL         entity.JSONMap `json:"dsl,omitempty"`
}

func (s *AgentService) PublishAgent(ctx context.Context, userID, canvasID string, req *PublishAgentRequest) (*entity.UserCanvasVersion, error) {
	canvasInstance, err := s.loadCanvasForUser(ctx, userID, canvasID)
	if err != nil {
		return nil, err
	}
	dsl := canvasInstance.DSL
	title := canvasInstance.Title
	description := canvasInstance.Description
	if req != nil {
		if req.DSL != nil {
			dsl = dslpkg.NormalizeForCanvas(req.DSL)
		}
		if req.Title != nil {
			trimmed := strings.TrimSpace(*req.Title)
			title = &trimmed
		}
		if req.Description != nil {
			description = req.Description
		}
	}

	canvasInstance.DSL = dsl
	canvasInstance.Title = title
	canvasInstance.Description = description
	canvasInstance.Release = true
	titleStr := ""
	if title != nil {
		titleStr = *title
	}
	opts := s.saveOrReplaceVersionOptions(ctx, userID, canvasID, dsl, titleStr, description, true)
	var row *entity.UserCanvasVersion
	if err = dao.DB.Transaction(func(tx *gorm.DB) error {
		if err := s.canvasDAO.UpdateTx(tx, canvasInstance); err != nil {
			return fmt.Errorf("publish agent %s: update parent: %w", canvasID, err)
		}
		saved, err := s.versionDAO.SaveOrReplaceLatestTx(tx, opts)
		if err != nil {
			return fmt.Errorf("publish agent %s: save version: %w", canvasID, err)
		}
		row = saved
		return nil
	}); err != nil {
		return nil, err
	}
	return row, nil
}

func (s *AgentService) saveOrReplaceVersion(ctx context.Context, userID, canvasID string, dsl entity.JSONMap, title string, description *string, release bool) (*entity.UserCanvasVersion, error) {
	return s.versionDAO.SaveOrReplaceLatest(s.saveOrReplaceVersionOptions(ctx, userID, canvasID, dsl, title, description, release))
}

func (s *AgentService) saveOrReplaceVersionOptions(ctx context.Context, userID, canvasID string, dsl entity.JSONMap, title string, description *string, release bool) dao.SaveOrReplaceLatestVersionOptions {
	nickname, err := s.userDAO.GetNicknameByID(ctx, userID)
	if err != nil || strings.TrimSpace(nickname) == "" {
		nickname = userID
	}
	versionTitle := buildVersionTitle(nickname, title, time.Now())
	return dao.SaveOrReplaceLatestVersionOptions{
		NewID:           utility.GenerateUUID(),
		UserCanvasID:    canvasID,
		Title:           &versionTitle,
		Description:     description,
		DSL:             dsl,
		Release:         release,
		KeepUnpublished: 20,
		SameDSL: func(latestDSL entity.JSONMap) bool {
			return reflect.DeepEqual(
				entity.JSONMap(dslpkg.NormalizeForCanvas(latestDSL)),
				dsl,
			)
		},
	}
}

func buildVersionTitle(userNickname, agentTitle string, ts time.Time) string {
	tenant := strings.TrimSpace(userNickname)
	if tenant == "" {
		tenant = "tenant"
	}
	title := strings.TrimSpace(agentTitle)
	if title == "" {
		title = "agent"
	}
	return fmt.Sprintf("%s_%s_%s", tenant, title, ts.Format("2006-01-02 15:04:05"))
}

// ListVersions returns every version for a canvas the user can see,
// newest first. The parent-canvas access check is enforced before the
// version list is loaded so unauthorized users cannot enumerate version
// ids of canvases they cannot read.
func (s *AgentService) ListVersions(ctx context.Context, userID, canvasID string) ([]*entity.UserCanvasVersion, error) {
	if _, err := s.loadCanvasForUser(ctx, userID, canvasID); err != nil {
		return nil, err
	}
	return s.versionDAO.ListByCanvasID(canvasID)
}

// GetVersion returns a single version of a canvas the user can see.
// Returns dao.ErrUserCanvasVersionNotFound when the version does not
// exist or belongs to a different canvas, and
// dao.ErrUserCanvasNotFound when the parent canvas is not visible to
// the requesting user.
func (s *AgentService) GetVersion(ctx context.Context, userID, canvasID, versionID string) (*entity.UserCanvasVersion, error) {
	if versionID == "" {
		return nil, dao.ErrUserCanvasVersionNotFound
	}
	if _, err := s.loadCanvasForUser(ctx, userID, canvasID); err != nil {
		return nil, err
	}
	row, err := s.versionDAO.GetByID(versionID)
	if err != nil {
		return nil, err
	}
	if row.UserCanvasID != canvasID {
		return nil, dao.ErrUserCanvasVersionNotFound
	}
	return row, nil
}

// DeleteVersion removes a single version of a canvas the user can see.
// Returns dao.ErrUserCanvasVersionNotFound when the row does not exist
// (or belongs to a different canvas) and dao.ErrUserCanvasNotFound when
// the parent canvas is not visible to the requesting user.
func (s *AgentService) DeleteVersion(ctx context.Context, userID, canvasID, versionID string) error {
	if versionID == "" {
		return dao.ErrUserCanvasVersionNotFound
	}
	if _, err := s.loadCanvasForUser(ctx, userID, canvasID); err != nil {
		return err
	}
	row, err := s.versionDAO.GetByID(versionID)
	if err != nil {
		return err
	}
	if row.UserCanvasID != canvasID {
		return dao.ErrUserCanvasVersionNotFound
	}
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		return s.versionDAO.DeleteTx(tx, versionID)
	})
}

// RunAgent starts a run for the given canvas and returns a channel of
// orchestrator events the HTTP layer streams back as SSE. The driver owns
// the wait-for-user cycle (eino interrupt, gap-analysis §11.6.4): the
// RunFunc returns an interrupt error when a UserFillUp node pauses the
// graph, the driver persists the interrupt id keyed by (canvasID,
// sessionID), and resumes when the next call supplies a non-empty
// userInput — at which point it injects (__resume_interrupt_id__,
// __resume_data__) into root so the RunFunc can call
// compose.ResumeWithData(ctx, id, data) before invoking the workflow.
//
// sessionID is required for the multi-turn cycle: the handler generates
// one (UUID) on the first call and reuses it on follow-up posts. version
// selects the published UserCanvasVersion row; "" uses the latest version.
//
// The per-run RunFunc is built by buildRunFunc — see its doc comment
// for the full production chain (real Compile/Invoke, resume path,
// error-layering contract).
func (s *AgentService) RunAgent(ctx context.Context, userID, canvasID, sessionID, version string, userInput any, files []map[string]interface{}) (<-chan canvas.RunEvent, error) {
	canvasRow, err := s.loadCanvasForUser(ctx, userID, canvasID)
	if err != nil {
		return nil, err
	}
	newSession := sessionID == ""
	if sessionID == "" {
		sessionID = utility.GenerateToken()
	}

	// Load the version row up front so the run is bound to a real DSL.
	//
	// IDOR guard (v3.5.2 review): when the caller supplies an explicit
	// version id, the row we load must belong to canvasID — otherwise
	// any caller who knows a foreign version id could run that
	// canvas's DSL against their own canvas (a clear
	// integrity/authorization boundary breach). GetVersion (at the
	// read path) already enforces this check; the run path did not.
	// We mirror that check here and surface ErrUserCanvasVersionNotFound
	// so the handler maps to a clean 404 rather than silently using
	// the foreign DSL.
	//
	// DAO-error visibility (v3.5.2 review): the previous code did
	// `versionRow, _ = ...` for both GetByID and GetLatest, which
	// masked every database failure as "no version published" and
	// let real ops issues hide behind the V1 placeholder answer.
	// We now distinguish three cases:
	//   - explicit version, row not found       → 404
	//   - explicit version, row from other canvas → 404 (IDOR)
	//   - explicit version, DB error            → 500 (surface it)
	//   - latest path, no rows + no error       → fall back to canvasRow.DSL (matches Python `completion()`)
	//   - latest path, DB error                 → 500 (surface it)
	//
	// v3.6 follow-up: when no published version exists, fall back to
	// the canvas's current editable DSL (canvasRow.DSL) instead of
	// the "no published version" placeholder. The Python reference at
	// api/db/services/canvas_service.py:332 does the same via
	// UserCanvasService.get_agent_dsl_with_release(agent_id,
	// release_mode=False, tenant_id=...) when release_mode is unset
	// on the request — the front-end's auto-save-on-run path means
	// the editable DSL is what the user just clicked "Run" against.
	// The buildRunFunc placeholder branch is reserved for the rare
	// "canvas exists but has no DSL at all" edge case.
	var (
		versionRow *entity.UserCanvasVersion
		dsl        map[string]any
	)
	if version != "" {
		row, err := s.versionDAO.GetByID(version)
		if err != nil {
			if errors.Is(err, dao.ErrUserCanvasVersionNotFound) {
				return nil, fmt.Errorf("RunAgent: load version %q: %w", version, err)
			}
			// Wrap DB-side errors with ErrAgentStorageError so
			// the handler maps them to CodeServerError (500)
			// with a sanitized message — raw DAO strings
			// (DSNs, table names, gorm stack frames) MUST NOT
			// reach the client.
			return nil, fmt.Errorf("RunAgent: load version %q: %w: %w", version, err, ErrAgentStorageError)
		}
		if row.UserCanvasID != canvasID {
			// IDOR — caller asked to run version X against canvas
			// Y, but version X belongs to canvas Z. Surface the
			// same "not found" envelope the read path uses.
			return nil, fmt.Errorf(
				"RunAgent: version %q belongs to canvas %q, not %q: %w",
				version, row.UserCanvasID, canvasID,
				dao.ErrUserCanvasVersionNotFound,
			)
		}
		versionRow = row
	}
	if versionRow == nil {
		row, lerr := s.versionDAO.GetLatest(canvasID)
		switch {
		case lerr == nil:
			versionRow = row
		case errors.Is(lerr, dao.ErrUserCanvasVersionNotFound):
			// No published version — fall back to the canvas's
			// current editable DSL (see v3.6 follow-up comment
			// above). Mirrors Python's
			// `get_agent_dsl_with_release(...release_mode=False)`
			// fallback in completion().
			if len(canvasRow.DSL) > 0 {
				dsl = dslpkg.NormalizeForRun(canvasRow.DSL)
			}
		default:
			// Wrap DB-side errors with ErrAgentStorageError
			// for the same reason as above (no DAO-string
			// leak to the client).
			return nil, fmt.Errorf("RunAgent: load latest version for canvas %q: %w: %w", canvasID, lerr, ErrAgentStorageError)
		}
	}
	if dsl == nil {
		dsl = normalisedDSLForRun(versionRow)
	}
	if sessionID != "" && s.api4ConversationDAO != nil {
		session, sessionErr := s.api4ConversationDAO.GetBySessionID(sessionID, canvasID)
		if sessionErr != nil {
			return nil, fmt.Errorf("RunAgent: load session %q: %w: %w", sessionID, sessionErr, ErrAgentStorageError)
		}
		if session != nil && session.UserID != userID {
			return nil, fmt.Errorf("RunAgent: session %q not found: %w", sessionID, dao.ErrUserCanvasNotFound)
		}
		if session != nil && len(session.DSL) > 0 {
			dsl = dslpkg.NormalizeForRun(session.DSL)
		}
	}
	if newSession && len(dsl) > 0 {
		if err := s.createAgentRunSession(sessionID, userID, canvasID, dsl, versionRow); err != nil {
			return nil, fmt.Errorf("RunAgent: create session %q: %w: %w", sessionID, err, ErrAgentStorageError)
		}
	}

	run := s.buildRunFunc(canvasID, versionRow, dsl)

	root := map[string]any{
		"canvas_id":  canvasID,
		"version_id": version,
		"session_id": sessionID,
		"user_id":    userID,
	}
	if userInput != nil {
		root["user_input"] = userInput
	}
	if len(files) > 0 {
		root["files"] = files
	}
	if dsl != nil {
		root["__dsl_present__"] = true
	}
	// Webhook payload injection. Only RunAgentWithWebhook sets this
	// context value; the chat / agent-run paths leave it nil so the
	// existing surface is unchanged. The 'BEGIN' component reads
	// inputs["webhook_payload"] and writes it to state.Sys so
	// downstream components can read sys.webhook_payload.
	if payload, ok := ctx.Value(webhookPayloadKey{}).(map[string]any); ok && payload != nil {
		root["webhook_payload"] = payload
	}
	// Match Python's @add_tenant_id_to_kwargs behavior for runtime
	// components and model credential lookup: the canvas runs under
	// the current caller's tenant id. Team-agent access was already
	// authorized by loadCanvasForUser above; do not replace this with
	// an arbitrary joined team tenant or LLM credential lookup can miss
	// the caller's configured provider key.
	root["tenant_id"] = userID

	// Preserve the historical RunTracker tenant dimension separately.
	// Existing tests and log filters expect the joined tenant id in the
	// run hash, but runtime state must keep tenant_id=userID.
	if tenantIDs, terr := s.userTenantDAO.GetTenantIDsByUserID(userID); terr == nil && len(tenantIDs) > 0 {
		root["run_tenant_id"] = tenantIDs[0]
	} else if terr != nil {
		common.Warn("service: RunAgent userTenantDAO.GetTenantIDsByUserID (best-effort, run tracker tenant not populated)",
			zap.String("user_id", userID),
			zap.Error(terr))
	}

	// v3.6.1 diagnostic: log what RunAgent put into root so we can
	// confirm tenant_id / user_id / session_id / user_input all
	// reached the buildRunFunc closure (which runs in the runner's
	// goroutine, possibly after a context switch).
	common.Debug("RunAgent root",
		zap.String("canvasID", canvasID),
		zap.String("userID", userID),
		zap.String("sessionID", sessionID),
		zap.Any("tenantID", root["tenant_id"]),
		zap.Any("userInput", root["user_input"]))

	return s.runner.Run(ctx, run, canvasID, sessionID, userInput, root), nil
}

// buildRunFunc assembles the per-run RunFunc the orchestrator (canvas.Runner)
// drives.
//
// Phase 4.4 V2: this is the real Compile/Invoke path. The previous
// V1 echo placeholder returned a synthesized answer without ever
// calling canvas.Compile — every RunAgent invocation pretended to
// run the canvas. The V2 body actually compiles the DSL, attaches
// the CanvasState to ctx via runtime.WithState (so component bodies
// can read it via runtime.GetStateFromContext), invokes the
// workflow, and surfaces real output through the Runner's existing
// answer-extraction contract.
//
// Nil-versionRow guard: the RunAgent call site treats "no version
// published" as a legal state and passes nil. We extract taskID
// safely and, when both versionRow and dsl are empty, fall back to
// a graceful "no published version" placeholder so the SSE surface
// still flows (TestRunAgent_NoVersionPublishedPlaceholder pins this
// behavior). The placeholder is written into state.Outputs under
// (cpn="answer", bucket="answer") so the answer extraction in
// first-pass lookup picks it up; the same trick the V1 placeholder
// used (the v3.5.2 fix landed this, and we keep it).
//
// Resume path: Runner.Run injects (__resume_interrupt_id__,
// __resume_data__) into root when userInput arrives on a session
// that previously paused at a wait-for-user interrupt. We consume
// them here and decorate ctx with compose.ResumeWithData so the
// targeted UserFillUp node (compile.go:53-55 lists them via
// compose.WithInterruptBeforeNodes) resumes and reads the user's
// follow-up via compose.GetResumeContext.
func (s *AgentService) buildRunFunc(canvasID string, versionRow *entity.UserCanvasVersion, dsl map[string]any) canvas.RunFunc {
	return func(ctx context.Context, root map[string]any) (*canvas.CanvasState, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Install a per-run token usage sink so every LLM call inside
		// this turn records its token usage (the sink is read at the end
		// and emitted in workflow_finished). Mirrors Python's
		// Canvas.run() installing token_usage_sink + langfuse_run_attrs.
		ctx = tokenizer.WithRunUsage(ctx)

		// Extract the event channel + metadata injected by Runner.Run.
		events, _ := root["__events__"].(chan canvas.RunEvent)
		messageID, _ := root["__message_id__"].(string)
		taskID, _ := root["__task_id__"].(string)
		sessionID, _ := root["__session_id__"].(string)
		userID, _ := root["user_id"].(string)

		// Install per-run Langfuse correlation attrs so LLM calls inside
		// this turn are grouped by session/user. Mirrors Python's
		// Canvas.run() setting langfuse_run_attrs.
		ctx = tokenizer.WithRunAttrs(ctx, &tokenizer.RunAttrs{
			SessionID: sessionID,
			UserID:    userID,
		})

		// Helper to build an SSE event with metadata.
		emit := func(typ, data string) {
			if events == nil {
				return
			}
			canvas.PushEvent(events, canvas.RunEvent{
				Type: typ, Data: data,
				MessageID: messageID,
				CreatedAt: time.Now().Unix(),
				TaskID:    taskID,
				SessionID: sessionID,
			})
		}

		// usagePayload returns the aggregated per-run token usage as a
		// JSON-serializable map, or nil when no sink was installed.
		usagePayload := func() map[string]int {
			sink := tokenizer.GetRunUsage(ctx)
			if sink == nil {
				return nil
			}
			pt, ct, tt, calls := sink.Snapshot()
			return map[string]int{
				"prompt_tokens":     pt,
				"completion_tokens": ct,
				"total_tokens":      tt,
				"calls":             calls,
			}
		}

		startedAt := float64(time.Now().UnixNano()) / 1e9

		userInput := root["user_input"]
		userInputText := ""
		if v, ok := userInput.(string); ok {
			userInputText = v
		}

		resumeID, isResume := root["__resume_interrupt_id__"].(string)
		if !isResume || resumeID == "" {
			wsData, _ := json.Marshal(map[string]any{"inputs": userInput})
			emit("workflow_started", string(wsData))
		}

		runID := runIDFor(canvasID, root)
		state := canvas.NewCanvasState(runID, taskID)

		// Graceful placeholder: no version published AND no DSL.
		if versionRow == nil && len(dsl) == 0 {
			answer := fmt.Sprintf("No published version found for canvas %q — publish a version before running.", canvasID)
			state.RecordOutput("answer", "answer", answer)
			// Emit a message event so the SSE surface matches the
			// normal-completion shape (test asserts message +
			// workflow_finished + done for the placeholder path).
			msgData, _ := json.Marshal(canvas.MessageEvent{Content: answer})
			meData, _ := json.Marshal(canvas.MessageEndEvent{})
			emit("message", string(msgData))
			emit("message_end", string(meData))
			wfPayload := map[string]any{"outputs": answer}
			if u := usagePayload(); u != nil {
				wfPayload["usage"] = u
			}
			wfData, _ := json.Marshal(wfPayload)
			emit("workflow_finished", string(wfData))
			return state, nil
		}

		// DSL → *Canvas.
		c, err := decodeCanvasFromDSL(dsl)
		if err != nil {
			s.markRunFailed(ctx, runID, "decode: "+err.Error())
			return nil, err
		}
		// Close MCP tool adapters and any other closeable resources
		// held by the canvas after execution completes. Mirrors
		// Python's finally: canvas.close() in canvas_service.py.
		defer c.Close()

		// Store events channel + run metadata on the context so the
		// per-node statePre/statePost wrappers (in scheduler.go) can
		// emit node_started / node_finished events at the correct
		// per-node lifecycle points. Context is used (rather than
		// state.Sys) because eino's WithGenLocalState creates a fresh
		// CanvasState per run — only the context thread survives from
		// the service layer into the state handlers.
		ctx2 := canvas.WithRunMeta(ctx, &canvas.RunMeta{
			Events:    events,
			MessageID: messageID,
			TaskID:    taskID,
			SessionID: sessionID,
		})
		agentMessageEmit, agentMessageFinalize, agentMessageReset := makeAgentMessageDeltaEmitterWithFinalizer(emit)
		ctx2 = runtime.WithAgentMessageEmitterControl(ctx2, agentMessageEmit, agentMessageFinalize, agentMessageReset)
		ctx2 = runtime.WithCanvasMessageEmitter(ctx2, func(content string) {
			emitAgentMessageEvent(emit, canvas.MessageEvent{Content: content})
		})

		// Seed initial env/sys values from the Canvas DSL globals.
		// Python's self.globals dict stores "sys.*" and "env.*" under
		// their full dotted keys; the Go port splits these into Sys /
		// Env / Globals maps so GetVar("env.counter") can look up
		// Env["counter"] directly. Without seeding, Env starts empty
		// and every env.* reference resolves to nil (unresolved ref).
		if c.Globals != nil {
			for k, v := range c.Globals {
				if strings.HasPrefix(k, "sys.") {
					state.Sys[strings.TrimPrefix(k, "sys.")] = v
				} else if strings.HasPrefix(k, "env.") {
					state.Env[strings.TrimPrefix(k, "env.")] = v
				} else {
					state.Globals[k] = v
				}
			}
		}
		state.SetHistory(c.History)
		state.SetMemory(c.Memory)
		state.EnsureSysDate()
		state.Sys["query"] = userInput
		state.AppendCurrentUser(userInput)
		state.AppendSysHistory("user: " + renderUserHistoryValue(userInput))
		if uid, ok := root["user_id"].(string); ok && uid != "" {
			state.Sys["user_id"] = uid
		}
		if tid, ok := root["tenant_id"].(string); ok && tid != "" {
			state.Sys["tenant_id"] = tid
		}
		if rawFiles, ok := root["files"].([]map[string]interface{}); ok && len(rawFiles) > 0 {
			// Only used for ParseAgentUploads (read-only); nil DocRemover means
			// this FileService MUST NOT be used for DeleteFiles.
			fileSvc := file.NewFileService(CheckFileTeamPermission, nil)
			files, ferr := fileSvc.ParseAgentUploads(userID, rawFiles, beginLayoutRecognize(c))
			if ferr != nil {
				s.markRunFailed(ctx2, runID, "parse files: "+ferr.Error())
				return nil, fmt.Errorf("parse agent files: %w", ferr)
			}
			state.Sys["files"] = files
		}
		state.IncrementConversationTurns()
		ctx2 = runtime.WithState(ctx2, state)

		// Resume path. The user input is the resume payload for the
		// previously-paused UserFillUp node — it should NOT also be
		// presented to UserFillUp:Menu (the first interactive node)
		// as a fresh "menu selection". Without this distinction, on
		// the follow-up RunAgent call sys.query=resume_payload would
		// be consumed by initialUserFillUpData in the menu body, the
		// menu would pick up the resume text as a brand-new branch
		// choice, Switch:Route would route to that branch, and the
		// previously-paused branch would be silently dropped (the
		// "second input doesn't resume" symptom). Clear sys.query so
		// the menu's initial-input fast path returns false and the
		// body falls through to compose.Interrupt — the menu pauses
		// for fresh input next time the user actually wants a
		// different branch.
		if isResume && resumeID != "" {
			resumeData := root["__resume_data__"]
			delete(root, "__resume_interrupt_id__")
			delete(root, "__resume_data__")
			state.Sys["query"] = ""
			ctx2 = compose.ResumeWithData(ctx2, resumeID, resumeData)
		}

		if s.runTracker != nil {
			_ = s.runTracker.Start(ctx2, runID, canvasID,
				tenantIDFromRoot(root), userInputText)
		}

		// Compile.
		var cc *canvas.CompiledCanvas
		switch {
		case s.checkpointStore != nil && s.stateSerializer != nil:
			cc, err = canvas.Compile(ctx2, c,
				canvas.WithCheckPointStore(s.checkpointStore),
				canvas.WithStateSerializer(s.stateSerializer),
			)
		case s.checkpointStore != nil:
			cc, err = canvas.Compile(ctx2, c,
				canvas.WithCheckPointStore(s.checkpointStore),
			)
		default:
			cc, err = canvas.Compile(ctx2, c)
		}
		if err != nil {
			common.Debug("RunAgent compile err",
				zap.String("canvas", canvasID),
				zap.String("session", sessionID),
				zap.String("task", taskID),
				zap.String("run", runID),
				zap.String("type", fmt.Sprintf("%T", err)),
				zap.Error(err))
			s.markRunFailed(ctx2, runID, "compile: "+err.Error())
			return nil, fmt.Errorf("canvas compile: %w: %w", ErrAgentStorageError, err)
		}

		cpID := ""
		if s.checkpointStore != nil {
			cpID = runID
		}

		// Invoke.
		var invokeOpts []compose.Option
		if cpID != "" {
			invokeOpts = []compose.Option{compose.WithCheckPointID(cpID)}
		}
		// On a resume, the user input is the resume payload for the
		// previously-paused UserFillUp node — it does NOT represent
		// a fresh sys.query. The 'BEGIN' node writes inputs["query"]
		// straight into state.Sys["query"] (begin.go:76), and
		// UserFillUp:Menu's initialUserFillUpData reads sys.query
		// back to drive the menu's initial-input fast path. If we
		// pass userInput through here on a resume, the menu would
		// re-consume the resume text as a brand-new branch choice
		// and Switch:Route would route to a fresh branch — the
		// previously-paused branch would be silently dropped (the
		// "second input doesn't resume" symptom reported for
		// categorize / iteration / code / wait_input etc.).
		wfInput := userInput
		if isResume && resumeID != "" {
			wfInput = ""
		}
		workflowOutput, invokeErr := cc.Workflow.Invoke(ctx2, map[string]any{"query": wfInput}, invokeOpts...)
		err = invokeErr

		if cpID != "" && s.runTracker != nil {
			_ = s.runTracker.AttachCheckpoint(ctx2, runID, cpID)
		}

		// Collect answer and references from the state snapshot.
		// node_finished events are already emitted per-node by the
		// statePost wrappers in scheduler.go.
		var answer string
		var thinking string
		var legacyReference []interface{}
		var downloads any
		now := float64(time.Now().UnixNano()) / 1e9
		for _, bucket := range state.Snapshot() {
			if v, ok := bucket["answer"].(string); ok && v != "" {
				if answer == "" {
					answer = v
				}
			}
			if v, ok := bucket["content"].(string); ok && v != "" && answer == "" {
				answer = v
			}
			if v, ok := bucket["result"].(string); ok && v != "" && answer == "" {
				answer = v
			}
			if v, ok := bucket["thinking"].(string); ok && v != "" && thinking == "" {
				thinking = v
			}
			if v, ok := bucket["reference"].([]interface{}); ok {
				legacyReference = append(legacyReference, v...)
			}
			if v, ok := bucket["downloads"]; ok && !emptyDownloadValue(v) {
				downloads = v
			}
		}
		referencePayload := agentRunReferencePayload(state, legacyReference)
		assistantOutput := terminalCanvasOutput(c, state, workflowOutput, answer, downloads)
		runtime.FinalizeAgentMessage(ctx2)
		messageEventsEmitted := runtime.AgentMessageEventsEmitted(ctx2)

		if err != nil {
			common.Debug("RunAgent invoke err",
				zap.String("canvas", canvasID),
				zap.String("session", sessionID),
				zap.String("task", taskID),
				zap.String("run", runID),
				zap.String("type", fmt.Sprintf("%T", err)),
				zap.Error(err))
			if canvas.IsInterruptError(err) {
				s.markRunFailed(ctx2, runID, "interrupt: "+err.Error())
				if answer != "" {
					appendAssistantHistory(state, partialAssistantOutput(answer, downloads))
				}
				if persistErr := s.persistAgentRunSession(canvasID, userID, sessionID, messageID, userInput, answer, referencePayload, dsl, state, answer != ""); persistErr != nil {
					return nil, fmt.Errorf("persist interrupted agent session: %w: %w", persistErr, ErrAgentStorageError)
				}
				if answer != "" {
					if !messageEventsEmitted {
						emitAgentMessageEvents(emit, answer, thinking, referencePayload)
					}

					meData, _ := json.Marshal(canvas.MessageEndEvent{
						Reference: referencePayload,
					})
					emit("message_end", string(meData))
				}
				return state, err
			}
			if shouldTreatAsCompletedLoopRun(err, answer) {
				appendAssistantHistory(state, assistantOutput)
				if persistErr := s.persistAgentRunSession(canvasID, userID, sessionID, messageID, userInput, answer, referencePayload, dsl, state, true); persistErr != nil {
					s.markRunFailed(ctx2, runID, "persist session: "+persistErr.Error())
					return nil, fmt.Errorf("persist agent session: %w: %w", persistErr, ErrAgentStorageError)
				}
				if !messageEventsEmitted {
					emitAgentMessageEvents(emit, answer, thinking, referencePayload)
				}

				meData, _ := json.Marshal(canvas.MessageEndEvent{
					Reference: referencePayload,
				})
				emit("message_end", string(meData))

				wfPayload := map[string]interface{}{
					"inputs":       map[string]any{"query": userInput},
					"outputs":      workflowOutputs(answer, downloads),
					"elapsed_time": now - startedAt,
					"created_at":   now,
				}
				if u := usagePayload(); u != nil {
					wfPayload["usage"] = u
				}
				wfData, _ := json.Marshal(wfPayload)
				emit("workflow_finished", string(wfData))

				s.markRunSucceeded(ctx2, runID)
				return state, nil
			}
			s.markRunFailed(ctx2, runID, "invoke: "+err.Error())
			return nil, fmt.Errorf("canvas invoke: %w", err)
		}

		// Emit message + message_end (mirrors Python's ans dict).
		appendAssistantHistory(state, assistantOutput)
		if persistErr := s.persistAgentRunSession(canvasID, userID, sessionID, messageID, userInput, answer, referencePayload, dsl, state, true); persistErr != nil {
			s.markRunFailed(ctx2, runID, "persist session: "+persistErr.Error())
			return nil, fmt.Errorf("persist agent session: %w: %w", persistErr, ErrAgentStorageError)
		}
		if !messageEventsEmitted {
			emitAgentMessageEvents(emit, answer, thinking, referencePayload)
		}

		meData, _ := json.Marshal(canvas.MessageEndEvent{
			Reference: referencePayload,
		})
		emit("message_end", string(meData))

		// Emit workflow_finished with the final outputs and aggregated
		// per-run token usage across all LLM calls in this turn.
		wfPayload := map[string]interface{}{
			"inputs":       map[string]any{"query": userInput},
			"outputs":      workflowOutputs(answer, downloads),
			"elapsed_time": now - startedAt,
			"created_at":   now,
		}
		if u := usagePayload(); u != nil {
			wfPayload["usage"] = u
		}
		wfData, _ := json.Marshal(wfPayload)
		emit("workflow_finished", string(wfData))

		s.markRunSucceeded(ctx2, runID)
		return state, nil
	}
}

func beginLayoutRecognize(c *canvas.Canvas) string {
	if c == nil {
		return ""
	}
	for _, comp := range c.Components {
		if !strings.EqualFold(comp.Obj.ComponentName, "Begin") {
			continue
		}
		layout, _ := comp.Obj.Params["layout_recognize"].(string)
		return layout
	}
	return ""
}

func (s *AgentService) createAgentRunSession(
	sessionID, userID, agentID string,
	runDSL map[string]any,
	versionRow *entity.UserCanvasVersion,
) error {
	if s == nil || s.api4ConversationDAO == nil {
		return errors.New("agent session storage is not configured")
	}
	source := "agent"
	session := &entity.API4Conversation{
		ID:        sessionID,
		DialogID:  agentID,
		UserID:    userID,
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
		Source:    &source,
		DSL:       entity.JSONMap(runDSL),
	}
	if versionRow != nil {
		session.VersionTitle = versionRow.Title
	}
	return s.api4ConversationDAO.Create(session)
}

// runIDFor builds the per-run CanvasState identifier: canvasID
// alone for first-touch runs, canvasID + sessionID for resumed runs
// (so two concurrent sessions on the same canvas don't collide in
// the snapshot map).
func runIDFor(canvasID string, root map[string]any) string {
	if s, ok := root["session_id"].(string); ok && s != "" {
		return canvasID + "-" + s
	}
	return canvasID
}

func workflowOutputs(content string, downloads any) any {
	if emptyDownloadValue(downloads) {
		return content
	}
	return map[string]any{
		"content":   content,
		"downloads": downloads,
	}
}

func emptyDownloadValue(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return v.Len() == 0
	default:
		return false
	}
}

func (s *AgentService) persistAgentRunSession(
	agentID, userID, sessionID, messageID string,
	userInput any,
	answer string,
	reference map[string]interface{},
	runDSL map[string]any,
	state *canvas.CanvasState,
	appendAssistantMessage bool,
) error {
	if sessionID == "" || s == nil || s.api4ConversationDAO == nil || dao.DB == nil {
		return nil
	}
	session, err := s.api4ConversationDAO.GetBySessionID(sessionID, agentID)
	if err != nil {
		common.Warn("agent run: load session for update failed", zap.String("agent_id", agentID), zap.String("session_id", sessionID), zap.Error(err))
		return nil
	}
	if session == nil || session.UserID != userID {
		return nil
	}
	messages := parseAgentSessionMessages(session.Message)
	now := time.Now().Unix()
	if text := stringifyAgentUserInput(userInput); text != "" {
		messages = append(messages, map[string]interface{}{"role": "user", "content": text, "id": utility.GenerateToken(), "created_at": now})
	}
	if appendAssistantMessage {
		messages = append(messages, map[string]interface{}{"role": "assistant", "content": answer, "id": messageID, "created_at": now})
	}
	if raw, err := json.Marshal(messages); err == nil {
		session.Message = raw
	}
	references := parseAgentSessionReferences(session.Reference)
	references = append(references, normalizeAgentReferenceEntry(reference))
	if raw, err := json.Marshal(references); err == nil {
		session.Reference = raw
	}
	if state != nil {
		session.DSL = buildPersistedAgentDSL(runDSL, state)
	}
	return s.api4ConversationDAO.Update(session)
}

func buildPersistedAgentDSL(runDSL map[string]any, state *canvas.CanvasState) entity.JSONMap {
	dsl := make(entity.JSONMap, len(runDSL)+3)
	for key, value := range runDSL {
		dsl[key] = value
	}
	if state == nil {
		return dsl
	}

	globals := make(map[string]any)
	if existing, ok := dsl["globals"].(map[string]any); ok {
		for key, value := range existing {
			globals[key] = value
		}
	}
	sysValues, envValues, globalValues := state.SnapshotNamespaces()
	for key := range globals {
		switch {
		case strings.HasPrefix(key, "sys."):
			if value, exists := sysValues[strings.TrimPrefix(key, "sys.")]; exists {
				globals[key] = value
			}
		case strings.HasPrefix(key, "env."):
			if value, exists := envValues[strings.TrimPrefix(key, "env.")]; exists {
				globals[key] = value
			}
		default:
			if value, exists := globalValues[key]; exists {
				globals[key] = value
			}
		}
	}
	for _, key := range []string{"query", "user_id", "conversation_turns", "files", "history", "date"} {
		if value, exists := sysValues[key]; exists {
			globals["sys."+key] = value
		}
	}

	dsl["globals"] = globals
	dsl["history"] = canvas.EncodeHistory(state.SnapshotHistory())
	dsl["memory"] = canvas.EncodeMemory(state.SnapshotMemory())
	return dsl
}

func agentRunReferencePayload(state *canvas.CanvasState, legacyChunks []interface{}) map[string]interface{} {
	if state != nil {
		if reference := state.GetRetrievalReference(); len(reference) > 0 {
			return reference
		}
	}
	if len(legacyChunks) == 0 {
		return nil
	}
	return map[string]interface{}{
		"chunks":   legacyChunks,
		"doc_aggs": []interface{}{},
		"total":    len(legacyChunks),
	}
}

func stringifyAgentUserInput(userInput any) string {
	switch v := userInput.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return fmt.Sprint(v)
	}
}

func appendAssistantHistory(state *canvas.CanvasState, payload map[string]any) {
	if state == nil {
		return
	}
	state.AppendHistory("assistant", payload)
	state.AppendSysHistory("assistant: " + pythonHistoryRepr(payload))
}

func partialAssistantOutput(answer string, downloads any) map[string]any {
	output := map[string]any{"content": answer}
	if !emptyDownloadValue(downloads) {
		output["downloads"] = downloads
	}
	return output
}

func terminalCanvasOutput(
	c *canvas.Canvas,
	state *canvas.CanvasState,
	workflowOutput map[string]any,
	answer string,
	downloads any,
) map[string]any {
	terminalIDs := make([]string, 0)
	if c != nil {
		for cpnID, component := range c.Components {
			if len(component.Downstream) == 0 {
				terminalIDs = append(terminalIDs, cpnID)
			}
		}
	}
	sort.Strings(terminalIDs)
	for _, cpnID := range terminalIDs {
		if output, ok := workflowOutput[cpnID].(map[string]any); ok && len(output) > 0 {
			return cloneCanvasOutput(output)
		}
	}
	if state != nil {
		snapshot := state.Snapshot()
		for _, cpnID := range terminalIDs {
			if output := snapshot[cpnID]; len(output) > 0 {
				return cloneCanvasOutput(output)
			}
		}
	}
	if len(workflowOutput) > 0 {
		return cloneCanvasOutput(workflowOutput)
	}
	fallback := map[string]any{"content": answer}
	if !emptyDownloadValue(downloads) {
		fallback["downloads"] = downloads
	}
	return fallback
}

func cloneCanvasOutput(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for key, value := range input {
		switch key {
		case "__cpn_id__", "state", "__legacy_noop__":
			continue
		}
		output[key] = value
	}
	return output
}

func renderUserHistoryValue(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case map[string]any:
		var buf strings.Builder
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(value); err != nil {
			return fmt.Sprint(value)
		}
		return strings.TrimSuffix(buf.String(), "\n")
	default:
		return pythonHistoryRepr(value)
	}
}

func pythonHistoryRepr(value any) string {
	switch item := value.(type) {
	case nil:
		return "None"
	case string:
		replacer := strings.NewReplacer(
			"\\", "\\\\",
			"'", "\\'",
			"\n", "\\n",
			"\r", "\\r",
			"\t", "\\t",
		)
		return "'" + replacer.Replace(item) + "'"
	case bool:
		if item {
			return "True"
		}
		return "False"
	case map[string]any:
		keys := make([]string, 0, len(item))
		for key := range item {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			leftPriority := pythonOutputKeyPriority(keys[i])
			rightPriority := pythonOutputKeyPriority(keys[j])
			if leftPriority != rightPriority {
				return leftPriority < rightPriority
			}
			return keys[i] < keys[j]
		})
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, pythonHistoryRepr(key)+": "+pythonHistoryRepr(item[key]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		parts := make([]string, 0, len(item))
		for _, child := range item {
			parts = append(parts, pythonHistoryRepr(child))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []string:
		parts := make([]string, 0, len(item))
		for _, child := range item {
			parts = append(parts, pythonHistoryRepr(child))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprint(item)
	}
}

// pythonOutputKeyPriority reconstructs the order produced by Python's
// ComponentParamBase output dictionaries: declared business outputs first,
// followed by the timing fields added by ComponentBase.invoke(). Message
// declares content then downloads, which is the terminal payload most often
// persisted in conversation history.
func pythonOutputKeyPriority(key string) int {
	switch key {
	case "content":
		return 0
	case "downloads":
		return 1
	case "_created_time":
		return 3
	case "_elapsed_time":
		return 4
	default:
		return 2
	}
}

// tenantIDFromRoot returns the optional run-tracker tenant id that
// RunAgent populated on the root map. Runtime components use
// root["tenant_id"] / state.Sys["tenant_id"] for the caller tenant;
// RunTracker keeps the historical joined-tenant dimension separately.
// Empty when absent — the RunTracker stores "" as the tenant id, which
// the test suite already exercises.
func tenantIDFromRoot(root map[string]any) string {
	if s, ok := root["run_tenant_id"].(string); ok {
		return s
	}
	if s, ok := root["tenant_id"].(string); ok {
		return s
	}
	return ""
}

func shouldTreatAsCompletedLoopRun(err error, answer string) bool {
	if err == nil || answer == "" {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "[GraphRunError] no tasks to execute")
}

// markRunSucceeded records the run as completed successfully via
// the Redis-backed RunTracker. No-op when tracker is nil (test path)
// or when the underlying Redis call fails (degraded boot).
func (s *AgentService) markRunSucceeded(ctx context.Context, runID string) {
	if s.runTracker == nil {
		return
	}
	if err := s.runTracker.MarkSucceeded(ctx, runID); err != nil {
		common.Warn("service: RunAgent runTracker.MarkSucceeded (best-effort, run not blocked)",
			zap.String("run_id", runID),
			zap.Error(err))
	}
}

// markRunFailed records the run as failed (with reason) via the
// Redis-backed RunTracker. No-op when tracker is nil or the
// underlying Redis call fails.
func (s *AgentService) markRunFailed(ctx context.Context, runID, reason string) {
	if s.runTracker == nil {
		return
	}
	if err := s.runTracker.MarkFailed(ctx, runID, reason); err != nil {
		common.Warn("service: RunAgent runTracker.MarkFailed (best-effort, run not blocked)",
			zap.String("run_id", runID),
			zap.String("reason", reason),
			zap.Error(err))
	}
}

// normalisedDSLForRun returns the DSL map for the given version, or
// nil when the version has no DSL or is missing. The map is a deep
// copy because canvas.Compile mutates some fields in place; reusing
// the same DSL across concurrent runs would race.
func normalisedDSLForRun(v *entity.UserCanvasVersion) map[string]any {
	if v == nil || len(v.DSL) == 0 {
		return nil
	}
	return dslpkg.NormalizeForRun(v.DSL)
}

// CancelAgent signals the in-flight run (if any) for the given canvas to
// stop. It is a no-op when no run is currently registered, or when the
// requesting user has no read access to the canvas.
func (s *AgentService) CancelAgent(ctx context.Context, userID, canvasID string) error {
	if _, err := s.loadCanvasForUser(ctx, userID, canvasID); err != nil {
		return err
	}
	s.runMu.Lock()
	cancel, ok := s.runStreams[canvasID]
	s.runMu.Unlock()
	if !ok {
		return nil
	}
	select {
	case <-cancel:
		// already closed
	default:
		close(cancel)
	}
	return nil
}
