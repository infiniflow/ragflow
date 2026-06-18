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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

const (
	agentTagsFieldMax = 512
	agentTagMaxLen    = 64
)

// ListAgentSessionsRequest are the parameters for ListAgentSessions.
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

// ListAgentSessionsResponse is the response body for ListAgentSessions.
type ListAgentSessionsResponse struct {
	Data  []map[string]interface{} `json:"data"`
	Total int64                    `json:"total"`
}

// DeleteAgentSessionsResult wraps DeleteAgentSessionsResponse with a message.
type DeleteAgentSessionsResult struct {
	Data    *DeleteAgentSessionsResponse
	Message string
}

// DeleteAgentSessionsResponse summarises a multi-id delete.
type DeleteAgentSessionsResponse struct {
	SuccessCount int      `json:"success_count"`
	Errors       []string `json:"errors,omitempty"`
}

// CheckCanvasAccess returns true when the user is the canvas owner or
// holds team-level permission for the owner's tenant.
func (s *AgentService) CheckCanvasAccess(userID, canvasID string) (bool, error) {
	canvas, err := s.canvasDAO.GetByID(canvasID)
	if err != nil {
		return false, err
	}
	if canvas.UserID == userID {
		return true, nil
	}
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

// checkDuplicateSessionIDs returns the de-duplicated id list and a slice of
// human-readable duplicate messages.
func checkDuplicateSessionIDs(ids []string) ([]string, []string) {
	seen := make(map[string]int, len(ids))
	uniqueIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		seen[id]++
		if seen[id] == 1 {
			uniqueIDs = append(uniqueIDs, id)
		}
	}

	duplicateMessages := make([]string, 0)
	for _, id := range uniqueIDs {
		if seen[id] > 1 {
			duplicateMessages = append(duplicateMessages, fmt.Sprintf("Duplicate session ids: %s", id))
		}
	}
	return uniqueIDs, duplicateMessages
}

// ListAgentSessions returns paginated agent sessions visible to the caller.
func (s *AgentService) ListAgentSessions(userID, tenantID, agentID string, req ListAgentSessionsRequest) (*ListAgentSessionsResponse, common.ErrorCode, error) {
	if agentID == "" {
		return nil, common.CodeArgumentError, errors.New("agent_id is required")
	}

	ok, err := s.CheckCanvasAccess(userID, agentID)
	if err != nil {
		// The Python agent API folds "canvas does not exist" into
		// the same 103 access envelope as a permission mismatch
		// (see @_require_canvas_access_async). Surface that
		// shape here instead of a 500 record not found so the
		// front-end does not log a server error for unknown ids.
		if errors.Is(err, dao.ErrUserCanvasNotFound) {
			return nil, common.CodeOperatingError, errors.New("Agent not found or no permission.")
		}
		return nil, common.CodeServerError, fmt.Errorf("failed to check agent permission: %w", err)
	}
	if !ok {
		return nil, common.CodeOperatingError, errors.New("Agent not found or no permission.")
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

// GetAgentSession fetches a single conversation belonging to agentID.
func (s *AgentService) GetAgentSession(userID, agentID, sessionID string) (*entity.API4Conversation, common.ErrorCode, error) {
	if sessionID == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("session_id is required")
	}
	ok, err := s.CheckCanvasAccess(userID, agentID)
	if err != nil {
		if errors.Is(err, dao.ErrUserCanvasNotFound) {
			return nil, common.CodeOperatingError, errors.New("Agent not found or no permission.")
		}
		return nil, common.CodeServerError, fmt.Errorf("failed to check agent permission: %w", err)
	}
	if !ok {
		return nil, common.CodeOperatingError, errors.New("Agent not found or no permission.")
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

// DeleteAgentSessionItem removes one conversation if it belongs to agentID.
func (s *AgentService) DeleteAgentSessionItem(userID, agentID, sessionID string) (bool, common.ErrorCode, error) {
	if sessionID == "" {
		return false, common.CodeArgumentError, errors.New("session_id is required")
	}
	ok, err := s.CheckCanvasAccess(userID, agentID)
	if err != nil {
		if errors.Is(err, dao.ErrUserCanvasNotFound) {
			return false, common.CodeOperatingError, errors.New("Agent not found or no permission.")
		}
		return false, common.CodeServerError, fmt.Errorf("failed to check agent permission: %w", err)
	}
	if !ok {
		return false, common.CodeOperatingError, errors.New("Agent not found or no permission.")
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

// DeleteAgentSessions removes multiple conversations owned by agentID.
// When ids is empty and deleteAll is true, every session under agentID is
// removed.
func (s *AgentService) DeleteAgentSessions(userID, agentID string, ids []string, deleteAll bool) (*DeleteAgentSessionsResult, common.ErrorCode, error) {
	if agentID == "" {
		return nil, common.CodeArgumentError, errors.New("agent_id is required")
	}

	// Owner-only by design: batch session deletion is destructive and must
	// not be available to team members even when the canvas has team
	// permission. CheckCanvasAccess (used elsewhere) would also allow team
	// access, which is too permissive for this operation.
	canvas, err := s.canvasDAO.GetByID(agentID)
	if err != nil || canvas == nil || canvas.UserID != userID {
		return nil, common.CodeDataError, fmt.Errorf("You don't own the agent %s", agentID)
	}

	if len(ids) == 0 {
		if !deleteAll {
			return &DeleteAgentSessionsResult{}, common.CodeSuccess, nil
		}

		ids, err = s.api4ConversationDAO.ListIDsByAgentID(agentID)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if len(ids) == 0 {
			return &DeleteAgentSessionsResult{}, common.CodeSuccess, nil
		}
	}

	sessionIDs, duplicateMessages := checkDuplicateSessionIDs(ids)
	errorsList := make([]string, 0)
	successCount := 0

	for _, sessionID := range sessionIDs {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			errorsList = append(errorsList, "Session ID is empty")
			continue
		}

		conv, err := s.api4ConversationDAO.GetBySessionID(sessionID, agentID)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if conv == nil {
			errorsList = append(errorsList, fmt.Sprintf("The agent doesn't own the session %s", sessionID))
			continue
		}

		if _, err := s.api4ConversationDAO.DeleteBySessionIDAndAgentID(sessionID, agentID); err != nil {
			return nil, common.CodeServerError, err
		}
		successCount++
	}

	if len(errorsList) > 0 {
		if successCount > 0 {
			return &DeleteAgentSessionsResult{
				Message: fmt.Sprintf("Partially deleted %d sessions with %d errors", successCount, len(errorsList)),
				Data: &DeleteAgentSessionsResponse{
					SuccessCount: successCount,
					Errors:       errorsList,
				},
			}, common.CodeSuccess, nil
		}
		return nil, common.CodeDataError, errors.New(strings.Join(errorsList, "; "))
	}

	if len(duplicateMessages) > 0 {
		return &DeleteAgentSessionsResult{
			Message: fmt.Sprintf("Partially deleted %d sessions with %d errors", successCount, len(duplicateMessages)),
			Data: &DeleteAgentSessionsResponse{
				SuccessCount: successCount,
				Errors:       duplicateMessages,
			},
		}, common.CodeSuccess, nil
	}

	return &DeleteAgentSessionsResult{}, common.CodeSuccess, nil
}

// normalizeAgentTags returns an error for unsupported tag payload types.
// The branch behaviour intentionally mirrors the Python implementation:
//   - string: treat the value as a CSV — split on "," and use each piece
//     as a separate tag ("alpha,beta" → ["alpha", "beta"]).
//   - []string / []interface{}: the caller already chose the boundary;
//     embedded commas are therefore replaced with spaces rather than
//     re-split (["alpha,beta"] → ["alpha beta"]).
//
// This asymmetry is required to keep tag handling byte-identical with
// agent_api.update_agent in the Python service.
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

// UpdateAgentTags normalises tags and persists them on a single canvas.
func (s *AgentService) UpdateAgentTags(userID, canvasID string, tags interface{}) (bool, common.ErrorCode, error) {
	ok, err := s.CheckCanvasAccess(userID, canvasID)
	if err != nil {
		if errors.Is(err, dao.ErrUserCanvasNotFound) {
			return false, common.CodeOperatingError, errors.New("Agent not found or no permission.")
		}
		return false, common.CodeServerError, fmt.Errorf("failed to check agent permission: %w", err)
	}
	if !ok {
		return false, common.CodeOperatingError, errors.New("Agent not found or no permission.")
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
			return false, common.CodeOperatingError, errors.New("Agent not found or no permission.")
		}
		return true, common.CodeSuccess, nil
	}
	return true, common.CodeSuccess, nil
}

// CreateAgentSessionRequest is the wire shape for POST
// /api/v1/agents/:agent_id/sessions.
type CreateAgentSessionRequest struct {
	UserID   string
	AgentID  string
	Name     string
	Source   string
	DSL      json.RawMessage
	Messages json.RawMessage
}

// CreateAgentSession inserts a fresh conversation row tied to the
// given agent canvas. The Phase 5 stub intentionally does NOT run
// Canvas(dsl).reset() (eino runtime is still unimplemented in the Go
// port); instead it stores a minimal but well-shaped row so that
// subsequent ListAgentSessions / GetAgentSession / chat-completion
// stubs can return a stable id and the integration suite can verify
// the create + read + delete cycle without depending on a real LLM
// run. When eino lands, the function will gain a pre-run prologue
// pass that calls Canvas.Reset() and stores the assistant message.
//
// Required columns (per the API4Conversation entity, see
// internal/entity/api_token.go:37-52):
//   - id          : 32-hex uuid, matches Python uuid.uuid4().hex
//   - dialog_id   : agent canvas id
//   - user_id     : caller's id
//   - message     : JSON array (default []); GET path normalises it
//   - reference   : JSON object (default {}) so GET-side parsing
//                   does not crash on .chunks
//   - dsl         : JSON map; copied from user_canvas.dsl if the
//                   caller did not pass one
//   - create_time : unix-millis
//   - update_time : unix-millis
//   - create_date : local-time.Truncate(time.Second)
//   - update_date : local-time.Truncate(time.Second)
func (s *AgentService) CreateAgentSession(req *CreateAgentSessionRequest) (*entity.API4Conversation, common.ErrorCode, error) {
	if req == nil {
		return nil, common.CodeArgumentError, errors.New("create agent session: nil request")
	}
	if req.AgentID == "" {
		return nil, common.CodeArgumentError, errors.New("create agent session: agent_id is required")
	}
	if req.UserID == "" {
		return nil, common.CodeArgumentError, errors.New("create agent session: user_id is required")
	}

	ok, err := s.CheckCanvasAccess(req.UserID, req.AgentID)
	if err != nil {
		return nil, common.CodeServerError, fmt.Errorf("check canvas access: %w", err)
	}
	if !ok {
		return nil, common.CodeOperatingError, errors.New("Agent not found or no permission.")
	}

	messages := req.Messages
	if len(messages) == 0 {
		messages = json.RawMessage(`[]`)
	}
	reference := json.RawMessage(`{}`)

	var dsl entity.JSONMap
	if len(req.DSL) > 0 {
		_ = json.Unmarshal(req.DSL, &dsl)
	}
	if len(dsl) == 0 {
		canvas, gErr := s.canvasDAO.GetByID(req.AgentID)
		if gErr != nil {
			if errors.Is(gErr, gorm.ErrRecordNotFound) {
				return nil, common.CodeOperatingError, errors.New("Agent not found or no permission.")
			}
			return nil, common.CodeServerError, fmt.Errorf("load canvas dsl: %w", gErr)
		}
		dsl = canvas.DSL
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "session"
	}
	var namePtr = &name
	var sourcePtr *string
	if req.Source != "" {
		sourcePtr = &req.Source
	}

	id := strings.ReplaceAll(uuid.New().String(), "-", "")[:32]

	// CreateTime / UpdateTime / CreateDate / UpdateDate are filled in
	// by entity.BaseModel.BeforeCreate when the DAO Create() call runs,
	// so we do not set them explicitly here.
	row := &entity.API4Conversation{
		ID:        id,
		Name:      namePtr,
		DialogID:  req.AgentID,
		UserID:    req.UserID,
		Message:   messages,
		Reference: reference,
		Source:    sourcePtr,
		DSL:       dsl,
	}
	if err := s.api4ConversationDAO.Create(row); err != nil {
		return nil, common.CodeServerError, fmt.Errorf("create agent session: %w", err)
	}
	return row, common.CodeSuccess, nil
}
