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
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	agentTagsFieldMax = 512
	agentTagMaxLen    = 64
)

// AgentService agent service
type AgentService struct {
	canvasDAO            *dao.UserCanvasDAO
	userTenantDAO        *dao.UserTenantDAO
	userCanvasVersionDAO *dao.UserCanvasVersionDAO
	canvasTemplateDAO    *dao.CanvasTemplateDAO
	replicaService       *CanvasReplicaService
}

// NewAgentService create agent service
func NewAgentService() *AgentService {
	return &AgentService{
		canvasDAO:            dao.NewUserCanvasDAO(),
		userTenantDAO:        dao.NewUserTenantDAO(),
		userCanvasVersionDAO: dao.NewUserCanvasVersionDAO(),
		canvasTemplateDAO:    dao.NewCanvasTemplateDAO(),
		replicaService:       NewCanvasReplicaService(),
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

// AgentDetailItem is the response body for GET /api/v1/agents/:agent_id.
type AgentDetailItem struct {
	*entity.UserCanvas
	LastPublishTime *int64 `json:"last_publish_time"`
}

// GetAgent returns the details of a specific agent canvas.
// Mirrors Python agent_api.get_agent with access check, replica bootstrap,
// latest publish time lookup, and DSL normalization.
// DataFlow datasets are currently omitted.
func (s *AgentService) GetAgent(userID, canvasID string) (*AgentDetailItem, error) {
	ok, err := s.CheckCanvasAccess(userID, canvasID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("canvas not found")
	}

	canvas, err := s.canvasDAO.GetByID(canvasID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("canvas not found")
		}
		return nil, err
	}

	// 3. Canvas Replica Bootstrap (Snapshot sync)
	title := ""
	if canvas.Title != nil {
		title = *canvas.Title
	}
	// Note: We use userID as the tenantID matching the Python @add_tenant_id_to_kwargs behaviour
	if _, err := s.replicaService.Bootstrap(canvasID, userID, userID, canvas.DSL, canvas.CanvasCategory, title); err != nil {
		// Log error but don't fail the request, allowing graceful degradation if Redis is down
		common.Warn("canvas replica bootstrap failed", zap.String("error", err.Error()))
	}

	// 4. Fetch the Last Publish Time (latest released version)
	var lastPublishTime *int64
	version, err := s.userCanvasVersionDAO.GetLatestReleasedVersion(canvasID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	} else if version != nil {
		lastPublishTime = version.UpdateTime
	}

	if canvas.DSL == nil {
		canvas.DSL = entity.JSONMap{}
	}
	canvas.DSL = NormalizeChunkerDSL(canvas.DSL)

	// Note: DataFlow datasets appending is omitted as it crosses into KnowledgebaseService boundaries

	return &AgentDetailItem{
		UserCanvas:      canvas,
		LastPublishTime: lastPublishTime,
	}, nil
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

// normalizeAgentTags returns an error for unsupported tag payload types
func normalizeAgentTags(rawTags interface{}) (string, error) {
	cleaned := make([]string, 0)
	switch tags := rawTags.(type) {
	case nil:
	case string:
		for _, tag := range strings.Split(tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				cleaned = append(cleaned, tag)
			}
		}
	case []string:
		for _, tag := range tags {
			tag = strings.TrimSpace(strings.ReplaceAll(tag, ",", " "))
			if tag != "" {
				cleaned = append(cleaned, tag)
			}
		}
	case []interface{}:
		for _, value := range tags {
			if value == nil {
				continue
			}
			tag := strings.TrimSpace(strings.ReplaceAll(fmt.Sprint(value), ",", " "))
			if tag != "" {
				cleaned = append(cleaned, tag)
			}
		}
	default:
		return "", fmt.Errorf("tags must be a string or array")
	}

	seen := make(map[string]struct{}, len(cleaned))
	normalized := make([]string, 0, len(cleaned))
	used := 0
	for _, tag := range cleaned {
		tag = truncateRunes(tag, agentTagMaxLen)
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}

		extra := len([]rune(tag))
		if len(normalized) > 0 {
			extra++
		}
		if used+extra > agentTagsFieldMax {
			break
		}

		seen[key] = struct{}{}
		normalized = append(normalized, tag)
		used += extra
	}

	return strings.Join(normalized, ","), nil
}

func truncateRunes(value string, maxLen int) string {
	runes := []rune(value)
	if len(runes) <= maxLen {
		return value
	}
	return string(runes[:maxLen])
}

func (s *AgentService) UpdateAgentTags(userID, canvasID string, tags interface{}) (bool, common.ErrorCode, error) {
	ok, err := s.CheckCanvasAccess(userID, canvasID)
	if err != nil {
		return false, common.CodeServerError, fmt.Errorf("failed to check agent permission: %w", err)
	}
	if !ok {
		return false, common.CodeOperatingError, fmt.Errorf("Agent not found or no permission.")
	}

	normalized, nErr := normalizeAgentTags(tags)
	if nErr != nil {
		return false, common.CodeBadRequest, nErr
	}
	rows, err := s.canvasDAO.UpdateTags(canvasID, normalized)
	if err != nil {
		return false, common.CodeServerError, fmt.Errorf("failed to update agent tags: %w", err)
	}
	if rows == 0 {
		if _, getErr := s.canvasDAO.GetByCanvasID(canvasID); getErr != nil {
			return false, common.CodeOperatingError, fmt.Errorf("Agent not found or no permission.")
		}
		return true, common.CodeSuccess, nil
	}

	return true, common.CodeSuccess, nil
}

// CheckCanvasAccess checks if a user has access to a canvas.
// Returns true if the user is the owner or has team-level permission.
func (s *AgentService) CheckCanvasAccess(userID, canvasID string) (bool, error) {
	canvas, err := s.canvasDAO.GetByID(canvasID)
	if err != nil {
		return false, err
	}
	// Owner always has access
	if canvas.UserID == userID {
		return true, nil
	}
	// Non-owner: only team-level permission grants tenant access
	if canvas.Permission != string(entity.TenantPermissionTeam) {
		return false, nil
	}
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return false, err
	}
	for _, tid := range tenantIDs {
		if canvas.UserID == tid {
			return true, nil
		}
	}
	return false, nil
}

// ListVersions returns all versions for an agent canvas, ordered by update_time DESC.
func (s *AgentService) ListVersions(canvasID string) ([]*entity.UserCanvasVersion, error) {
	return s.userCanvasVersionDAO.ListByCanvasID(canvasID)
}

// GetVersion returns a specific version by ID, verifying it belongs to the given canvas.
func (s *AgentService) GetVersion(canvasID, versionID string) (*entity.UserCanvasVersion, error) {
	version, err := s.userCanvasVersionDAO.GetByID(versionID)
	if err != nil {
		return nil, err
	}
	if version.UserCanvasID != canvasID {
		return nil, fmt.Errorf("version not found")
	}
	return version, nil
}
