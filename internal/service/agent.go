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
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"ragflow/internal/harness"
	"ragflow/internal/harness/graph/interrupt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/agent/sandbox"
	"ragflow/internal/agent/tool"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	dslpkg "ragflow/internal/agent/dsl"
)

// genID32 returns a 32-char UUID-derived primary key suitable for the
// user_canvas and user_canvas_version tables. The format matches Python
// uuid.uuid4().hex used by the original DAO and keeps existing rows
// joinable across Python and Go writers.
func genID32() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")[:32]
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
	stateSerializer interface{}

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
	// Register the real sandbox client (overrides the package-level stub).
	tool.SetSandboxClient(sandbox.NewManagerClient())
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
	ser interface{},
	rt *canvas.RunTracker,
) *AgentService {
	return &AgentService{
		canvasDAO:           dao.NewUserCanvasDAO(),
		canvasTemplateDAO:   dao.NewCanvasTemplateDAO(),
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
	Permission     string  `json:"permission"`
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

func toAgentItem(c *entity.UserCanvas) *AgentItem {
	return &AgentItem{
		ID:             c.ID,
		Avatar:         c.Avatar,
		Title:          c.Title,
		Permission:     c.Permission,
		CanvasType:     c.CanvasType,
		CanvasCategory: c.CanvasCategory,
		CreateTime:     c.CreateTime,
		UpdateTime:     c.UpdateTime,
	}
}

// ListAgents returns agent canvases visible to userID.
// Mirrors Python agent_api.list_agents — validates owner_ids against joined tenants,
// then delegates to the DAO.
func (s *AgentService) ListAgents(userID string, keywords string, page, pageSize int, orderby string, desc bool, ownerIDs []string, canvasCategory string) (*ListAgentsResponse, common.ErrorCode, error) {
	// Build the set of tenant IDs the user is authorised to query.
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
		orderby,
		desc,
		keywords,
		canvasCategory,
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
		return nil, common.CodeArgumentError, errors.New("No DSL data in request.")
	}
	if req.Title == nil || strings.TrimSpace(*req.Title) == "" {
		return nil, common.CodeArgumentError, errors.New("No title in request.")
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
		ID:             genID32(),
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
// DAO-error sanitisation (v3.5.2 follow-up): the raw userTenantDAO and
// canvasDAO errors are wrapped with ErrAgentStorageError so mapAgentError
// classifies them as CodeServerError (500) with a sanitized message —
// the original DAO error string (DSN, table name, gorm stack frame)
// MUST NOT reach the client. Sentinels (ErrUserCanvasNotFound) pass
// through unchanged so they keep mapping to the 404 envelope.
//
// This is the FIRST storage access path RunAgent hits, so leaving the
// raw errors here would have left a DAO-string leak in the very first
// hop — the earlier af2ac2eda + 804854a5e commits only sanitised the
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

// UpdateAgent writes a new DSL to the draft (user_canvas.dsl) and toggles
// release=false. The call does NOT create a new user_canvas_version row —
// versions are produced only by PublishAgent.
func (s *AgentService) UpdateAgent(ctx context.Context, userID, canvasID string, dsl entity.JSONMap) error {
	row, err := s.loadCanvasForUser(ctx, userID, canvasID)
	if err != nil {
		return err
	}
	row.DSL = dslpkg.NormalizeForCanvas(dsl)
	row.Release = false
	if err := s.canvasDAO.Update(row); err != nil {
		return fmt.Errorf("update agent %s: %w", canvasID, err)
	}
	return nil
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
		if _, err := s.versionDAO.DeleteByCanvasIDTx(tx, canvasID); err != nil {
			return fmt.Errorf("delete agent: cascade versions: %w", err)
		}
		if err := s.canvasDAO.DeleteTx(tx, canvasID); err != nil {
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

// PublishAgent appends a new user_canvas_version row and marks the parent
// canvas as released in a single transaction. Existing versions are never
// overwritten (§2.9); the parent canvas DSL/title/description/release
// fields are updated atomically with the new version row.
func (s *AgentService) PublishAgent(ctx context.Context, userID, canvasID string, req *PublishAgentRequest) (*entity.UserCanvasVersion, error) {
	canvas, err := s.loadCanvasForUser(ctx, userID, canvasID)
	if err != nil {
		return nil, err
	}
	dsl := canvas.DSL
	title := canvas.Title
	description := canvas.Description
	if req != nil {
		if req.DSL != nil {
			dsl = dslpkg.NormalizeForCanvas(req.DSL)
		}
		if req.Title != nil {
			title = req.Title
		}
		if req.Description != nil {
			description = req.Description
		}
	}
	row := &entity.UserCanvasVersion{
		ID:           genID32(),
		UserCanvasID: canvasID,
		Title:        title,
		Description:  description,
		DSL:          dsl,
	}
	if err := dao.DB.Transaction(func(tx *gorm.DB) error {
		if err := s.versionDAO.CreateTx(tx, row); err != nil {
			return fmt.Errorf("publish agent %s: insert version: %w", canvasID, err)
		}
		canvas.DSL = dsl
		canvas.Title = title
		canvas.Description = description
		canvas.Release = true
		if err := s.canvasDAO.UpdateTx(tx, canvas); err != nil {
			return fmt.Errorf("publish agent %s: update parent: %w", canvasID, err)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return row, nil
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
// the wait-for-user cycle (harness interrupt): the
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
func (s *AgentService) RunAgent(ctx context.Context, userID, canvasID, sessionID, version, userInput string) (<-chan canvas.RunEvent, error) {
	canvasRow, err := s.loadCanvasForUser(ctx, userID, canvasID)
	if err != nil {
		return nil, err
	}
	if sessionID == "" {
		sessionID = strings.ReplaceAll(uuid.New().String(), "-", "")
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
	//   - latest path + canvas DSL available    → use canvas DSL
	//   - latest path, no rows + no canvas DSL  → placeholder
	//   - latest path, DB error                 → 500 (surface it)
	var versionRow *entity.UserCanvasVersion
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
		row, err := s.versionDAO.GetLatest(canvasID)
		if err != nil {
			if errors.Is(err, dao.ErrUserCanvasVersionNotFound) {
				// No published version — fall back to the
				// canvas's own DSL so unsaved canvases
				// (e.g. freshly created from a template)
				// still run.
			} else {
				// Wrap DB-side errors with ErrAgentStorageError
				// for the same reason as above (no DAO-string
				// leak to the client).
				return nil, fmt.Errorf("RunAgent: load latest version for canvas %q: %w: %w", canvasID, err, ErrAgentStorageError)
			}
		} else {
			versionRow = row
		}
	}
	dsl := normalisedDSLForRun(versionRow)
	// Fall back to canvas DSL when no version has been published.
	if dsl == nil && len(canvasRow.DSL) > 0 {
		dsl = dslpkg.NormalizeForCanvas(map[string]any(canvasRow.DSL))
	}

	// Pre-extract component metadata for the log-panel timeline
	// (runner.go reads from root["__comp_types__"] and root["__comp_names__"]
	// before safeInvoke so it can emit node_started before the canvas runs).
	compTypes, compNames, compIDs := extractComponentInfo(dsl)

	run := s.buildRunFunc(canvasID, versionRow, dsl)

	root := map[string]any{
		"canvas_id":  canvasID,
		"version_id": version,
		"session_id": sessionID,
		"user_id":    userID,
	}
	if userInput != "" {
		root["user_input"] = userInput
	}
	if dsl != nil {
		root["__dsl_present__"] = true
	}
	root["__comp_types__"] = compTypes
	root["__comp_names__"] = compNames
	root["__comp_ids__"] = compIDs
	// Phase 4.4 V2.1 (v3.6.1): populate root["tenant_id"] so the
	// RunTracker.Start call (in buildRunFunc) records the run
	// under the right tenant. The lookup is best-effort — a
	// failure here (DAO down, user has no tenants) logs and
	// continues with an empty tenant_id rather than failing
	// the run; the run still works, the only loss is the
	// per-tenant filterability of the run-history log.
	if tenantIDs, terr := s.userTenantDAO.GetTenantIDsByUserID(userID); terr == nil && len(tenantIDs) > 0 {
		root["tenant_id"] = tenantIDs[0]
	} else if terr != nil {
		log.Printf("service: RunAgent userTenantDAO.GetTenantIDsByUserID(%q): %v (best-effort, run not blocked)", userID, terr)
	}

	return s.runner.Run(ctx, run, canvasID, sessionID, userInput, root), nil
}

// buildRunFunc assembles the per-run RunFunc the orchestrator (canvas.Runner)
// drives.
//
// Phase 4.4 V2: this is the real Compile/Invoke path. The previous
// V1 echo placeholder returned a synthesised answer without ever
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
// behaviour). The placeholder is written into state.Outputs under
// (cpn="answer", bucket="answer") so extractAnswerFromState's
// first-pass lookup picks it up; the same trick the V1 placeholder
// used (the v3.5.2 fix landed this and we keep it).
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

		taskID := ""
		if versionRow != nil {
			taskID = versionRow.ID
		}

		userInput := ""
		if v, ok := root["user_input"].(string); ok {
			userInput = v
		}

		runID := runIDFor(canvasID, root)
		state := canvas.NewCanvasState(runID, taskID)

		// Graceful placeholder: no version published AND no DSL.
		// This is the legal "user clicked Run before publishing"
		// path. The orchestrator surfaces the placeholder answer
		// to the SSE consumer without an error event.
		if versionRow == nil && len(dsl) == 0 {
			answer := fmt.Sprintf("No published version found for canvas %q — publish a version before running.", canvasID)
			state.RecordOutput("answer", "answer", answer)
			return state, nil
		}

		// DSL → *Canvas. All non-sentinel errors are already
		// wrapped with ErrAgentStorageError so the handler's
		// mapAgentError classifies them as CodeServerError (500)
		// with a sanitized message.
		c, err := decodeCanvasFromDSL(dsl)
		if err != nil {
			s.markRunFailed(ctx, runID, "decode: "+err.Error())
			return nil, err
		}

		// Pre-populate Begin node outputs from DSL defaults so
		// template refs like {{begin@customer_review}} resolve
		// without needing customer_review as a graph channel.
		// The graph engine only registers "query" as a channel
		// (see scheduler.go BuildWorkflow) — passing unknown keys
		// would fail with "channel not found".
		for key, val := range extractBeginInputs(dsl) {
			state.SetVar("begin", key, val)
		}
		// Load DSL globals into state.Env (env.* defaults).
		// Without this, env.counter, env.zero etc. are missing
		// and VariableAssigner operators fail with
		// "ERROR:VARIABLE_NOT_NUMBER or PARAMETER_NOT_NUMBER".
		if globals, ok := dsl["globals"].(map[string]any); ok {
			for k, v := range globals {
				if strings.HasPrefix(k, "env.") {
					state.Env[strings.TrimPrefix(k, "env.")] = v
				}
			}
		}
		// Sys["query"] is the canonical Begin-node input key
		// (BeginComponent.Invoke reads inputs["query"] and writes
		// it into state.Sys["query"]). Pre-seeding it here lets
		// the first Begin run see the user's input even before
		// Begin writes back.
		//
		// On resume, DON'T overwrite Sys["query"] with the new user
		// input — the checkpoint channel restore already has the
		// original query, and overwriting would cause the
		// UserFillUp:Menu dispatch to use the new input for routing
		// instead of the original menu selection.  The new input is
		// passed through AppendResumeValue for the interrupted
		// UserFillUp component.
		if _, resume := root["__resume_interrupt_id__"]; !resume {
			state.Sys["query"] = userInput
		}
		if tid := tenantIDFromRoot(root); tid != "" {
			state.Sys["tenant_id"] = tid
		}
		// Attach the streaming progress channel (if present) to ctx
		// so the Agent/LLM component's streaming call can send chunks.
		ctx2 := runtime.WithState(ctx, state)
		if progCh, ok := root[canvas.ProgressCh].(chan runtime.ProgEvent); ok {
			ctx2 = runtime.WithProgressCh(ctx2, progCh)
		}

		// Resume path: if Runner.Run injected a saved interrupt id
		// + the user's follow-up, decorate ctx so the targeted
		// interrupt-emitting node resumes.
		if resumeID, ok := root["__resume_interrupt_id__"].(string); ok && resumeID != "" {
			resumeData := root["__resume_data__"]
			delete(root, "__resume_interrupt_id__")
			delete(root, "__resume_data__")
			ctx2 = interrupt.WithInterruptContext(ctx2)
			interrupt.AppendResumeValue(ctx2, resumeData)
		}

		// Run lifecycle: best-effort. Tracker may be nil (test
		// path) or Redis may be unreachable (degraded boot);
		// either way, the run itself must not be blocked.
		if s.runTracker != nil {
			_ = s.runTracker.Start(ctx2, runID, canvasID,
				tenantIDFromRoot(root), userInput)
		}

		// Compile. The CheckPointStore is wired independently of
		// the state serializer. The state serializer is
		// OPTIONAL: when the user does not set one, harness's
		// default InternalSerializer is used (which knows about
		// runtime.CanvasState via compose.RegisterSerializableType
		// in runtime/state.go:init). RAGFlow's plain-JSON
		// CanvasStateSerializer is incompatible with harness's
		// internal checkpoint format — see cmd/server_main.go
		// buildAgentRunOptions for the rationale.
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
			s.markRunFailed(ctx2, runID, "compile: "+err.Error())
			// Two-`%w` chain: ErrAgentStorageError first so
			// errors.Is(returnedErr, ErrAgentStorageError) is
			// true; the inner err is only rendered in
			// returnedErr.Error() for log diagnostics. Go 1.20+
			// supports multi-wrap via %w but the sentinel-match
			// contract requires sentinel-first ordering.
			return nil, fmt.Errorf("canvas compile: %w: %w", ErrAgentStorageError, err)
		}

		// Phase 4.4 V2 (Goal 7): generate a checkpoint id when
		// a store is configured and pair the run record with the
		// checkpoint payload. WithCheckPointID is a run-time
		// Option (not a GraphCompileOption), so the id has to be
		// generated per-Invoke. We use the existing runID as the
		// checkpoint id — it's already unique per (canvas, session)
		// and gives us a stable key for the Redis "agent:cp:{id}"
		// namespace.
		cpID := ""
		if s.checkpointStore != nil {
			cpID = runID
		}

		// Invoke. cc.Graph.Invoke runs the full harness graph.
		// A wait-for-user interrupt surfaces as a harness interrupt
		// that we pass through to Runner.Run unchanged.
		var runConfig *harness.RunnableConfig
		if cpID != "" {
			runConfig = harness.NewRunnableConfig()
			runConfig.ThreadID = cpID
			runConfig.Configurable = map[string]interface{}{
				"thread_id": cpID,
			}
		}
		_, err = cc.Graph.Invoke(ctx2, map[string]any{"query": userInput}, runConfig)

		// Attach the checkpoint payload to the run record. Best-
		// effort — tracker may be down; we don't fail the run.
		if cpID != "" && s.runTracker != nil {
			_ = s.runTracker.AttachCheckpoint(ctx2, runID, cpID)
		}

		if err != nil {
			if canvas.IsInterruptError(err) {
				// Interrupt: not a failure. Return state +
				// interrupt error so Runner.Run can extract the
				// InterruptCtx list and emit waiting_for_user.
				s.markRunFailed(ctx2, runID, "interrupt: "+err.Error())
				return state, err
			}
			s.markRunFailed(ctx2, runID, "invoke: "+err.Error())
			// Same sentinel-first two-%w wrap as the compile branch
			// above; preserves errors.Is(returnedErr, ErrAgentStorageError)
			// while keeping the inner error text in Error().
			return nil, fmt.Errorf("canvas invoke: %w: %w", ErrAgentStorageError, err)
		}

		s.markRunSucceeded(ctx2, runID)
		return state, nil
	}
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

// tenantIDFromRoot returns the optional tenant_id that the handler
// may have populated on the root map. Empty when absent — the
// RunTracker stores "" as the tenant id, which the test suite
// already exercises.
func tenantIDFromRoot(root map[string]any) string {
	if s, ok := root["tenant_id"].(string); ok {
		return s
	}
	return ""
}

// markRunSucceeded records the run as completed successfully via
// the Redis-backed RunTracker. No-op when tracker is nil (test path)
// or when the underlying Redis call fails (degraded boot).
func (s *AgentService) markRunSucceeded(ctx context.Context, runID string) {
	if s.runTracker == nil {
		return
	}
	if err := s.runTracker.MarkSucceeded(ctx, runID); err != nil {
		log.Printf("service: RunAgent runTracker.MarkSucceeded(%q): %v (best-effort, run not blocked)", runID, err)
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
		log.Printf("service: RunAgent runTracker.MarkFailed(%q, %q): %v (best-effort, run not blocked)", runID, reason, err)
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
	return dslpkg.NormalizeForRun(map[string]any(v.DSL))
}

// extractComponentInfo builds component-type and component-name maps from
// the raw DSL, for ONLY the top-level execution path (matching Python's
// Canvas.run loop which iterates self.path). Sub-graph components (nested
// sub-agents, tool definitions, etc.) are excluded so the log panel shows
// the same 3–5 entries as the Python version.
//
// Priority for display name:
//  1. dsl["graph"]["nodes"][i]["data"]["name"]
//  2. comp.Obj.Params["title"]
//  3. raw component id (last resort)
//
// Returns compTypes, compNames (both keyed by DSL component id) and
// compIDs (ordered slice, suitable for runner.go's emitNodeStarted).
// runner.go reads all three from root to emit node_started in the
// correct order (map iteration would be random).
func extractComponentInfo(dsl map[string]any) (compTypes, compNames map[string]string, compIDs []string) {
	compTypes = make(map[string]string)
	compNames = make(map[string]string)
	if dsl == nil {
		return
	}

	c, err := decodeCanvasFromDSL(dsl)
	if err != nil {
		return // best-effort; empty maps are safe for runner.go
	}

	// Determine which component IDs to show, PRESERVING ORDER.
	// Priority:
	//  1. c.Path — topological execution order (Python's self.path).
	//  2. Heuristic order using graph edges (topological sort).
	//  3. Fallback: sort with rules: begin first, message last.
	//
	// The graph.nodes array from the React-Flow layout is NOT in
	// topological order (Message can appear before Agent). We build
	// a proper topological sort from graph.edges instead.
	ids := c.Path
	if len(ids) == 0 {
		ids = topologicalSort(dsl, c.Components)
	}
	if len(ids) == 0 {
		// Last resort: sort with begin first, message components last.
		ids = make([]string, 0, len(c.Components))
		var messages []string
		for id := range c.Components {
			if id == "begin" {
				continue
			}
			if comp, ok := c.Components[id]; ok && strings.EqualFold(comp.Obj.ComponentName, "message") {
				messages = append(messages, id)
				continue
			}
			ids = append(ids, id)
		}
		sort.Strings(ids)
		sort.Strings(messages)
		ids = append([]string{"begin"}, ids...)
		ids = append(ids, messages...)
	}

	// Collect display names from the React-Flow graph layout
	// (user-defined names like "Deep research Agent").
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	if graphRaw, ok := dsl["graph"].(map[string]any); ok {
		if nodesRaw, ok := graphRaw["nodes"].([]any); ok {
			for _, raw := range nodesRaw {
				node, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				id, _ := node["id"].(string)
				if id == "" {
					continue
				}
				if _, in := idSet[id]; !in {
					continue
				}
				if dataRaw, ok := node["data"].(map[string]any); ok {
					if name, _ := dataRaw["name"].(string); name != "" {
						compNames[id] = name
					}
				}
			}
		}
	}

	// Component types for the selected IDs, preserving order.
	compIDs = ids
	for _, id := range ids {
		comp, ok := c.Components[id]
		if !ok {
			continue
		}
		compTypes[id] = comp.Obj.ComponentName
		if _, hasName := compNames[id]; !hasName {
			if title, ok := comp.Obj.Params["title"].(string); ok {
				compNames[id] = title
			} else {
				compNames[id] = id
			}
		}
	}

	// Insert sub-agent tool components into the ordered list.
	// For each Agent component, locate its "tools" param entries that
	// represent sub-agent tool definitions (entries with component_name,
	// id, and name).  Insert them immediately after the Agent so the log
	// panel shows them in the correct position (before Response/Message).
	expanded := make([]string, 0, len(compIDs)+4)
	for _, id := range compIDs {
		expanded = append(expanded, id)
		comp, ok := c.Components[id]
		if !ok {
			continue
		}
		if !strings.EqualFold(comp.Obj.ComponentName, "Agent") {
			continue
		}
		toolsRaw, ok := comp.Obj.Params["tools"]
		if !ok {
			continue
		}
		toolsArr, ok := toolsRaw.([]any)
		if !ok {
			continue
		}
		for _, raw := range toolsArr {
			cfg, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if _, ok := cfg["component_name"].(string); !ok {
				continue
			}
			if _, ok := cfg["id"].(string); !ok {
				continue
			}
			toolName, ok := cfg["name"].(string)
			if !ok || toolName == "" {
				continue
			}
			if _, ok := cfg["params"].(map[string]any); !ok {
				continue
			}
			// sanitizeFnName logic (inlined from component/agent.go)
			fn := sanitizeSubAgentName(toolName)
			if fn == "" {
				fn = toolName
			}
			compTypes[fn] = "Agent"
			compNames[fn] = toolName
			expanded = append(expanded, fn)
		}
	}
	compIDs = expanded
	return
}

// sanitizeSubAgentName mirrors component/agent.go's sanitizeFnName.
func sanitizeSubAgentName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else if r == ' ' {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "agent_tool"
	}
	return b.String()
}

// topologicalSort builds a deterministic component ID order from the DSL's
// graph edges (Kahn's algorithm). When no edges are available, falls back
// to begin-first, message-last heuristic.
func topologicalSort(dsl map[string]any, components map[string]canvas.CanvasComponent) []string {
	// Build in-degree map from graph edges.
	graphRaw, ok := dsl["graph"].(map[string]any)
	if !ok {
		return nil
	}
	edgesRaw, ok := graphRaw["edges"].([]any)
	if !ok || len(edgesRaw) == 0 {
		return nil
	}

	inDegree := make(map[string]int, len(components))
	succ := make(map[string][]string, len(components))
	for id := range components {
		inDegree[id] = 0
	}
	for _, raw := range edgesRaw {
		edge, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		src, _ := edge["source"].(string)
		tgt, _ := edge["target"].(string)
		if src == "" || tgt == "" {
			continue
		}
		if _, exists := components[src]; !exists {
			continue
		}
		if _, exists := components[tgt]; !exists {
			continue
		}
		inDegree[tgt]++
		succ[src] = append(succ[src], tgt)
	}

	// Queue nodes with 0 in-degree.
	queue := make([]string, 0, len(components))
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue) // deterministic order for same-degree nodes

	var order []string
	visited := make(map[string]bool, len(components))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if visited[id] {
			continue
		}
		visited[id] = true
		order = append(order, id)
		for _, s := range succ[id] {
			inDegree[s]--
			if inDegree[s] == 0 {
				queue = append(queue, s)
			}
		}
		sort.Strings(queue)
	}

	// Include any unvisited nodes (disconnected from the main graph).
	for id := range components {
		if !visited[id] {
			order = append(order, id)
		}
	}
	return order
}

// extractBeginInputs reads the Begin component's DSL-defined input fields
// and returns their default values as a map.  The caller merges additional
// fields (e.g. "query") into the returned map before passing it to the
// canvas invocation.
//
// DSL shape (components.begin.obj.params.inputs):
//
//	"begin": {
//	  "obj": { "params": { "inputs": {
//	    "customer_review": {
//	      "type": "line",
//	      "value": "什么手机口碑好"
//	    },
//	    ...
//	  }}}
//	}
//
// Returns an empty-but-non-nil map when the DSL has no begin component or
// no inputs — callers can always merge without a nil check.
func extractBeginInputs(dsl map[string]any) map[string]any {
	out := make(map[string]any)
	if dsl == nil {
		return out
	}
	comps, _ := dsl["components"].(map[string]any)
	if comps == nil {
		return out
	}
	beginRaw, ok := comps["begin"]
	if !ok {
		return out
	}
	begin, _ := beginRaw.(map[string]any)
	if begin == nil {
		return out
	}
	obj, _ := begin["obj"].(map[string]any)
	if obj == nil {
		return out
	}
	params, _ := obj["params"].(map[string]any)
	if params == nil {
		return out
	}
	inputs, _ := params["inputs"].(map[string]any)
	if inputs == nil {
		return out
	}
	for key, raw := range inputs {
		field, _ := raw.(map[string]any)
		if field == nil {
			continue
		}
		// Use the "value" field as the default value if present.
		if v, ok := field["value"]; ok {
			out[key] = v
		}
	}
	return out
}

// ResetAgent clears the per-run state of a canvas (history, retrieval,
// memory, path, dirty sys.* / env.* globals) and updates the stored DSL
// in place.  env.* globals are restored from variables.{name}.default
// when available; otherwise cleared.  The canvas Release flag is flipped
// to false.  Returns the reset DSL map.
func (s *AgentService) ResetAgent(ctx context.Context, userID, canvasID string) (entity.JSONMap, error) {
	canvas, err := s.loadCanvasForUser(ctx, userID, canvasID)
	if err != nil {
		return nil, err
	}
	// Deep-copy the DSL so we mutate a fresh map.
	dsl := deepCopyJSONMap(entity.JSONMap(canvas.DSL))
	// Reset per-run accumulators.
	dsl["history"] = []any{}
	dsl["retrieval"] = []any{}
	dsl["memory"] = []any{}
	dsl["path"] = []any{}
	// Load env.* default from variables.{name}.default.
	envDefaults := make(map[string]any)
	if variables, ok := dsl["variables"].(map[string]any); ok {
		for name, raw := range variables {
			cfg, _ := raw.(map[string]any)
			if cfg == nil {
				continue
			}
			if def, has := cfg["value"]; has {
				envDefaults["env."+name] = def
			}
		}
	}
	// Reset globals.
	if globals, ok := dsl["globals"].(map[string]any); ok {
		for k := range globals {
			if strings.HasPrefix(k, "sys.") {
				// Zero by inferred type.
				switch globals[k].(type) {
				case []any:
					globals[k] = []any{}
				case []string:
					globals[k] = []string{}
				default:
					globals[k] = ""
				}
			} else if strings.HasPrefix(k, "env.") {
				if def, has := envDefaults[k]; has {
					globals[k] = def
				} else {
					globals[k] = ""
				}
			}
		}
	}
	// Flip release to false.
	canvas.Release = false
	canvas.DSL = entity.JSONMap(dsl)
	if err := s.canvasDAO.Update(canvas); err != nil {
		return nil, fmt.Errorf("ResetAgent: update: %w", err)
	}
	return entity.JSONMap(dsl), nil
}

// deepCopyJSONMap returns a deep copy of m (JSON-safe values only).
func deepCopyJSONMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch x := v.(type) {
		case map[string]any:
			out[k] = deepCopyJSONMap(x)
		case []any:
			cp := make([]any, len(x))
			for i, item := range x {
				if m2, ok := item.(map[string]any); ok {
					cp[i] = deepCopyJSONMap(m2)
				} else {
					cp[i] = item
				}
			}
			out[k] = cp
		default:
			out[k] = v
		}
	}
	return out
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
