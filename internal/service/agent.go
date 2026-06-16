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
	"strings"
	"sync"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
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

// AgentService agent service
type AgentService struct {
	canvasDAO           *dao.UserCanvasDAO
	canvasTemplateDAO   *dao.CanvasTemplateDAO
	userTenantDAO       *dao.UserTenantDAO
	versionDAO          *dao.UserCanvasVersionDAO
	api4ConversationDAO *dao.API4ConversationDAO

	// runMu and runStreams coordinate active canvas run goroutines so that
	// CancelAgent can signal a specific canvas. The map is keyed by canvas
	// ID; values are channels that close to signal cancellation.
	runMu      sync.Mutex
	runStreams map[string]chan struct{}
}

// NewAgentService create agent service
func NewAgentService() *AgentService {
	return &AgentService{
		canvasDAO:           dao.NewUserCanvasDAO(),
		canvasTemplateDAO:   dao.NewCanvasTemplateDAO(),
		userTenantDAO:       dao.NewUserTenantDAO(),
		versionDAO:          dao.NewUserCanvasVersionDAO(),
		api4ConversationDAO: dao.NewAPI4ConversationDAO(),
		runStreams:          make(map[string]chan struct{}),
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
func (s *AgentService) loadCanvasForUser(ctx context.Context, userID, canvasID string) (*entity.UserCanvas, error) {
	if canvasID == "" {
		return nil, dao.ErrUserCanvasNotFound
	}
	if userID == "" {
		return nil, dao.ErrUserCanvasNotFound
	}
	tenants, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("tenants for user %s: %w", userID, err)
	}
	row, err := s.canvasDAO.GetByIDForUser(canvasID, userID, tenants)
	if err != nil {
		return nil, err
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
	row.DSL = dsl
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
			dsl = req.DSL
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

// RunAgent starts a run for the given canvas and returns a channel that
// emits synthetic "Phase 5 wiring pending" events then closes. The full
// eino execution loop is owned by the canvas package (§2.6); this service
// method is the wiring point so the HTTP layer can switch from stub to
// real execution without changing handlers.
//
// The returned cancel channel lets CancelAgent stop the stub run; when the
// real implementation lands it can be replaced with a Redis cancel key
// per §4.9.
func (s *AgentService) RunAgent(ctx context.Context, userID, canvasID, version string) (<-chan string, error) {
	if _, err := s.loadCanvasForUser(ctx, userID, canvasID); err != nil {
		return nil, err
	}

	cancel := make(chan struct{})

	s.runMu.Lock()
	prev, hadPrev := s.runStreams[canvasID]
	s.runStreams[canvasID] = cancel
	s.runMu.Unlock()
	if hadPrev {
		// Best-effort cancel a previous in-flight run; ignore error as it's
		// typically already closed.
		select {
		case <-prev:
		default:
			close(prev)
		}
	}

	out := make(chan string, 2)
	go func() {
		defer close(out)
		defer func() {
			s.runMu.Lock()
			if s.runStreams[canvasID] == cancel {
				delete(s.runStreams, canvasID)
			}
			s.runMu.Unlock()
		}()
		select {
		case <-ctx.Done():
			return
		case <-cancel:
			return
		case out <- fmt.Sprintf("Phase 5 wiring pending: the eino run loop is owned by canvas package (canvasID=%s, version=%s)", canvasID, version):
		}
	}()
	return out, nil
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
