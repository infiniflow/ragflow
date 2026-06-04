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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/cache"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
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
	api4ConversationDAO  *dao.API4ConversationDAO
	fileDAO              *dao.FileDAO
}

// NewAgentService create agent service
func NewAgentService() *AgentService {
	return &AgentService{
		canvasDAO:            dao.NewUserCanvasDAO(),
		userTenantDAO:        dao.NewUserTenantDAO(),
		userCanvasVersionDAO: dao.NewUserCanvasVersionDAO(),
		api4ConversationDAO:  dao.NewAPI4ConversationDAO(),
		canvasTemplateDAO:    dao.NewCanvasTemplateDAO(),
		fileDAO:              dao.NewFileDAO(),
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

type ListAgentSessionsRequest struct {
	SessionID  string
	UserID     string
	Page       int
	PageSize   int
	Keywords   string
	FromDate   string
	ToDate     string
	OrderBy    string
	Desc       bool
	ExpUserID  string
	IncludeDSL bool
}

type ListAgentSessionsResponse struct {
	Data  []map[string]interface{} `json:"data"`
	Total int64                    `json:"total"`
}

func parseAgentSessionDate(value string, isEnd bool) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}

	if strings.Contains(value, "T") {
		normalized := strings.ReplaceAll(value, "Z", "+00:00")
		parsed, err := time.Parse(time.RFC3339, normalized)
		if err != nil {
			return nil, err
		}

		local := parsed.Local()
		return &local, nil
	}

	if len(value) == 10 {
		if isEnd {
			value += " 23:59:59"
		} else {
			value += " 00:00:00"
		}
	}

	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	if err != nil {
		return nil, err
	}

	return &parsed, nil
}

func normalizeAgentSession(session *entity.API4Conversation, includeDSL bool) map[string]interface{} {
	messages := parseAgentSessionMessages(session.Message)
	references := parseAgentSessionReferences(session.Reference)

	for _, message := range messages {
		delete(message, "prompt")
	}

	if len(references) > 0 {
		assistantMessages := make([]map[string]interface{}, 0)
		for i, message := range messages {
			role, _ := message["role"].(string)
			if i != 0 && role != "user" {
				assistantMessages = append(assistantMessages, message)
			}
		}

		for i := 0; i < len(assistantMessages) && i < len(references); i++ {
			rawChunks, _ := references[i]["chunks"].([]interface{})
			assistantMessages[i]["reference"] = normalizeAgentReferenceChunks(rawChunks)
		}
	}

	result := map[string]interface{}{
		"id":            session.ID,
		"name":          session.Name,
		"agent_id":      session.DialogID,
		"user_id":       session.UserID,
		"exp_user_id":   session.ExpUserID,
		"message":       messages,
		"tokens":        session.Tokens,
		"source":        session.Source,
		"duration":      session.Duration,
		"round":         session.Round,
		"thumb_up":      session.ThumbUp,
		"errors":        session.Errors,
		"version_title": session.VersionTitle,
		"create_time":   session.CreateTime,
		"create_date":   session.CreateDate,
		"update_time":   session.UpdateTime,
		"update_date":   session.UpdateDate,
	}

	if includeDSL {
		result["dsl"] = session.DSL
	}

	return result
}

func parseAgentSessionReferences(raw json.RawMessage) []map[string]interface{} {
	if len(raw) == 0 {
		return []map[string]interface{}{}
	}

	var references []map[string]interface{}
	if err := json.Unmarshal(raw, &references); err == nil {
		for i, reference := range references {
			references[i] = normalizeAgentReferenceEntry(reference)
		}
		return references
	}

	var referenceMap map[string]interface{}
	if err := json.Unmarshal(raw, &referenceMap); err != nil {
		return []map[string]interface{}{}
	}

	if _, ok := referenceMap["chunks"]; ok {
		return []map[string]interface{}{normalizeAgentReferenceEntry(referenceMap)}
	}

	keys := make([]string, 0, len(referenceMap))
	for key := range referenceMap {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		left, _ := strconv.Atoi(keys[i])
		right, _ := strconv.Atoi(keys[j])
		return left < right
	})

	result := make([]map[string]interface{}, 0, len(keys))
	for _, key := range keys {
		reference, ok := referenceMap[key].(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, normalizeAgentReferenceEntry(reference))
	}

	return result
}

func parseAgentSessionMessages(raw json.RawMessage) []map[string]interface{} {
	if len(raw) == 0 {
		return []map[string]interface{}{}
	}

	var messages []map[string]interface{}
	if err := json.Unmarshal(raw, &messages); err != nil {
		return []map[string]interface{}{}
	}

	return messages
}

func normalizeAgentReferenceEntry(reference map[string]interface{}) map[string]interface{} {
	if reference == nil {
		return map[string]interface{}{
			"chunks":   []interface{}{},
			"doc_aggs": []interface{}{},
		}
	}

	if _, ok := reference["chunks"]; ok {
		return map[string]interface{}{
			"chunks":   valueOrEmptySlice(reference["chunks"]),
			"doc_aggs": valueOrEmptySlice(reference["doc_aggs"]),
		}
	}

	if _, ok := reference["doc_aggs"]; ok {
		return map[string]interface{}{
			"chunks":   valueOrEmptySlice(reference["chunks"]),
			"doc_aggs": valueOrEmptySlice(reference["doc_aggs"]),
		}
	}

	return map[string]interface{}{
		"chunks":   valueOrEmptySlice(reference["reference"]),
		"doc_aggs": valueOrEmptySlice(reference["doc_aggs"]),
	}
}

func valueOrEmptySlice(value interface{}) interface{} {
	if value == nil {
		return []interface{}{}
	}
	return value
}

func normalizeAgentReferenceChunks(chunks []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(chunks))

	for _, rawChunk := range chunks {
		chunk, ok := rawChunk.(map[string]interface{})
		if !ok {
			continue
		}

		result = append(result, map[string]interface{}{
			"id":            firstNonNil(chunk["chunk_id"], chunk["id"]),
			"content":       firstNonNil(chunk["content_with_weight"], chunk["content"]),
			"document_id":   firstNonNil(chunk["doc_id"], chunk["document_id"]),
			"document_name": firstNonNil(chunk["docnm_kwd"], chunk["document_name"]),
			"dataset_id":    firstNonNil(chunk["kb_id"], chunk["dataset_id"]),
			"image_id":      firstNonNil(chunk["image_id"], chunk["img_id"]),
			"positions":     firstNonNil(chunk["positions"], chunk["position_int"]),
		})
	}

	return result
}

func firstNonNil(values ...interface{}) interface{} {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func (s *AgentService) ListAgentSessions(userID, tenantID, agentID string, req ListAgentSessionsRequest) (*ListAgentSessionsResponse, common.ErrorCode, error) {
	if agentID == "" {
		return nil, common.CodeArgumentError, errors.New("agent_id is required")
	}

	ok, err := s.CheckCanvasAccess(userID, agentID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to check agent permission: %w", err)
	}
	if !ok {
		return nil, common.CodeOperatingError, fmt.Errorf("Agent not found or no permission.")
	}

	sessionDAO := dao.NewChatSessionDAO()

	if req.ExpUserID != "" {
		rows, err := sessionDAO.ListAgentSessionNames(agentID, req.ExpUserID)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		return &ListAgentSessionsResponse{Data: rows, Total: int64(len(rows))}, common.CodeSuccess, nil
	}

	fromDate, err := parseAgentSessionDate(req.FromDate, false)
	if err != nil {
		return nil, common.CodeArgumentError, err
	}

	toDate, err := parseAgentSessionDate(req.ToDate, true)
	if err != nil {
		return nil, common.CodeArgumentError, err
	}

	total, sessions, err := sessionDAO.ListAgentSessions(dao.ListAgentSessionsParams{
		AgentID:    agentID,
		Page:       req.Page,
		PageSize:   req.PageSize,
		OrderBy:    req.OrderBy,
		Desc:       req.Desc,
		SessionID:  req.SessionID,
		UserID:     req.UserID,
		IncludeDSL: req.IncludeDSL,
		Keywords:   req.Keywords,
		FromDate:   fromDate,
		ToDate:     toDate,
		ExpUserID:  req.ExpUserID,
	})
	if err != nil {
		return nil, common.CodeServerError, err
	}

	data := make([]map[string]interface{}, 0, len(sessions))
	for _, session := range sessions {
		data = append(data, normalizeAgentSession(session, req.IncludeDSL))
	}

	return &ListAgentSessionsResponse{Data: data, Total: total}, common.CodeSuccess, nil
}

func (s *AgentService) GetAgentSession(userID, agentID, sessionID string) (*entity.API4Conversation, common.ErrorCode, error) {
	if sessionID == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("session_id is required")
	}
	ok, err := s.CheckCanvasAccess(userID, agentID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to check agent permission: %w", err)
	}
	if !ok {
		return nil, common.CodeOperatingError, fmt.Errorf("Agent not found or no permission.")
	}

	data, err := s.api4ConversationDAO.GetBySessionID(sessionID, agentID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("failed to fetch session: %w", err)
	}
	if data == nil {
		return nil, common.CodeNotFound, fmt.Errorf("agent session not found")
	}
	return data, common.CodeSuccess, nil
}

func (s *AgentService) DeleteAgentSessionItem(userID, agentID, sessionID string) (bool, common.ErrorCode, error) {
	if sessionID == "" {
		return false, common.CodeArgumentError, errors.New("session_id is required")
	}
	ok, err := s.CheckCanvasAccess(userID, agentID)
	if err != nil {
		return false, common.CodeServerError, fmt.Errorf("failed to check agent permission: %w", err)
	}
	if !ok {
		return false, common.CodeOperatingError, fmt.Errorf("Agent not found or no permission.")
	}

	row, err := s.api4ConversationDAO.DeleteBySessionIDAndAgentID(sessionID, agentID)
	if err != nil {
		return false, common.CodeServerError, err
	}
	if row == 0 {
		return false, common.CodeSuccess, nil
	}

	return true, common.CodeSuccess, nil
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

// GetAgent returns a single agent canvas after verifying the caller has access.
func (s *AgentService) GetAgent(userID, agentID string) (*entity.UserCanvas, error) {
	canvas, err := s.canvasDAO.GetByID(agentID)
	if err != nil {
		return nil, err
	}
	// Verify access: owner always OK; team permission requires joined-tenant check.
	if canvas.UserID != userID {
		if canvas.Permission != string(entity.TenantPermissionTeam) {
			return nil, fmt.Errorf("canvas not found")
		}
		tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
		if err != nil {
			return nil, err
		}
		found := false
		for _, tid := range tenantIDs {
			if canvas.UserID == tid {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("canvas not found")
		}
	}
	return canvas, nil
}

// GetAgentLogs retrieves execution logs for a given message from Redis.
// Key format mirrors Python: "{agent_id}-{message_id}-logs".
func (s *AgentService) GetAgentLogs(agentID, messageID string) (interface{}, error) {
	redisClient := cache.Get()
	if redisClient == nil {
		return map[string]interface{}{}, nil
	}
	key := fmt.Sprintf("%s-%s-logs", agentID, messageID)
	raw, err := redisClient.Get(key)
	if err != nil || raw == "" {
		return map[string]interface{}{}, nil
	}
	var payload interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return map[string]interface{}{}, nil
	}
	return payload, nil
}

// DownloadAgentFile retrieves the raw bytes for a file owned by the given tenant.
// Returns the blob and the file name (for content-type inference).
func (s *AgentService) DownloadAgentFile(tenantID, fileID string) ([]byte, string, error) {
	file, err := s.fileDAO.GetByID(fileID)
	if err != nil {
		return nil, "", fmt.Errorf("file not found")
	}
	if file.TenantID != tenantID {
		return nil, "", fmt.Errorf("file not found")
	}
	if file.Location == nil || *file.Location == "" {
		return nil, "", fmt.Errorf("file has no storage location")
	}
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, "", fmt.Errorf("storage not initialized")
	}
	blob, err := storageImpl.Get(file.ParentID, *file.Location)
	if err != nil {
		return nil, "", err
	}
	return blob, file.Name, nil
}

// DownloadAttachment retrieves raw bytes for an attachment stored under the tenant's bucket.
// attachmentID is the object name (storage key), tenantID is the bucket.
func (s *AgentService) DownloadAttachment(tenantID, attachmentID string) ([]byte, error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	blob, err := storageImpl.Get(tenantID, attachmentID)
	if err != nil {
		return nil, err
	}
	return blob, nil
}
