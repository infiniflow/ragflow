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
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
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
}

// NewAgentService create agent service
func NewAgentService() *AgentService {
	return &AgentService{
		canvasDAO:            dao.NewUserCanvasDAO(),
		userTenantDAO:        dao.NewUserTenantDAO(),
		userCanvasVersionDAO: dao.NewUserCanvasVersionDAO(),
		api4ConversationDAO:  dao.NewAPI4ConversationDAO(),
		canvasTemplateDAO:    dao.NewCanvasTemplateDAO(),
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

type DeleteAgentSessionsResult struct {
	Data    *DeleteAgentSessionsResponse
	Message string
}

type DeleteAgentSessionsResponse struct {
	SuccessCount int      `json:"success_count"`
	Errors       []string `json:"errors,omitempty"`
}

type TestDBConnectionRequest struct {
	DBType   string      `json:"db_type"`
	Database string      `json:"database"`
	Username string      `json:"username"`
	Host     string      `json:"host"`
	Port     interface{} `json:"port"`
	Password string      `json:"password"`
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

// checkDuplicateSessionIDs check duplicated ID in IDS and add some messages for it
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

// DeleteAgentSessions Delete sessions by ids
func (s *AgentService) DeleteAgentSessions(userID, agentID string, ids []string, deleteAll bool) (*DeleteAgentSessionsResult, common.ErrorCode, error) {
	if agentID == "" {
		return nil, common.CodeArgumentError, errors.New("agent_id is required")
	}

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
			errorsList = append(errorsList, "The agent doesn't own the session ")
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
		if successCount > 0 {
			return &DeleteAgentSessionsResult{
				Message: fmt.Sprintf("Partially deleted %d sessions with %d errors", successCount, len(duplicateMessages)),
				Data: &DeleteAgentSessionsResponse{
					SuccessCount: successCount,
					Errors:       duplicateMessages,
				},
			}, common.CodeSuccess, nil
		}
		return nil, common.CodeDataError, errors.New(strings.Join(duplicateMessages, ";"))
	}

	return &DeleteAgentSessionsResult{}, common.CodeSuccess, nil
}

// AssertHostIsSafe checks whether host resolves only to public IP addresses.
func AssertHostIsSafe(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("Host must not be empty.")
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		zap.L().Warn("SSRF guard could not resolve host",
			zap.String("host", host),
			zap.Error(err),
		)
		return "", fmt.Errorf("Could not resolve host %q: %w", host, err)
	}

	if len(ips) == 0 {
		zap.L().Warn("SSRF guard blocked host: resolved to no addresses",
			zap.String("host", host),
		)
		return "", fmt.Errorf("Host %q resolved to no addresses.", host)
	}

	var resolvedIP string

	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return "", fmt.Errorf("invalid resolved IP %q for host %q", ip.String(), host)
		}

		// Normalize IPv4-mapped IPv6, equivalent to Python _effective_ip().
		addr = addr.Unmap()

		if !isPublicAddr(addr) {
			zap.L().Warn("SSRF guard blocked host",
				zap.String("host", host),
				zap.String("resolved_ip", addr.String()),
			)
			return "", fmt.Errorf("Host resolves to a non-public address (%s), which is not allowed.", addr.String())
		}

		if resolvedIP == "" {
			resolvedIP = addr.String()
		}
	}

	if resolvedIP == "" {
		return "", fmt.Errorf("Host %q resolved to no addresses.", host)
	}

	return resolvedIP, nil
}

func isPublicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()

	if !addr.IsValid() {
		return false
	}

	if !addr.IsGlobalUnicast() {
		return false
	}

	if addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return false
	}

	return !isSpecialUseAddr(addr)
}

func isSpecialUseAddr(addr netip.Addr) bool {
	addr = addr.Unmap()

	specialCIDRs := []string{
		// IPv4 special-use / documentation / reserved ranges.
		"0.0.0.0/8",
		"100.64.0.0/10",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"192.0.0.0/24",
		"192.0.2.0/24",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"240.0.0.0/4",

		// IPv6 special-use / documentation / local ranges.
		"::/128",
		"::1/128",
		"64:ff9b:1::/48",
		"100::/64",
		"2001::/23",
		"2001:2::/48",
		"fc00::/7",
		"fe80::/10",
		"ff00::/8",
		"2001:db8::/32",
		"2002::/16",
	}

	for _, cidr := range specialCIDRs {
		prefix := netip.MustParsePrefix(cidr)
		if prefix.Contains(addr) {
			return true
		}
	}

	return false
}

// missingDBConnectionFields Check if request is missing something
func missingDBConnectionFields(req *TestDBConnectionRequest) []string {
	missing := make([]string, 0, 6)
	if req == nil || strings.TrimSpace(req.DBType) == "" {
		missing = append(missing, "db_type")
	}
	if req == nil || strings.TrimSpace(req.Database) == "" {
		missing = append(missing, "database")
	}
	if req == nil || strings.TrimSpace(req.Username) == "" {
		missing = append(missing, "username")
	}
	if req == nil || strings.TrimSpace(req.Host) == "" {
		missing = append(missing, "host")
	}
	if req == nil || dbConnectionPort(req.Port) == "" {
		missing = append(missing, "port")
	}
	if req == nil || req.Password == "" {
		missing = append(missing, "password")
	}
	return missing
}

func dbConnectionPort(port interface{}) string {
	switch value := port.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case float64:
		return strconv.Itoa(int(value))
	case float32:
		return strconv.Itoa(int(value))
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case json.Number:
		return value.String()
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func (s *AgentService) TestDBConnection(userID string, req *TestDBConnectionRequest) (common.ErrorCode, error) {
	if missing := missingDBConnectionFields(req); len(missing) > 0 {
		return common.CodeArgumentError, fmt.Errorf("required argument are missing: %s; ", strings.Join(missing, ","))
	}

	safeHost, err := AssertHostIsSafe(req.Host)
	if err != nil {
		zap.L().Warn(
			"Rejected test_db_connection: unsafe host",
			zap.String("host", req.Host),
			zap.String("db_type", req.DBType),
			zap.String("user", userID),
			zap.Error(err),
		)
		return common.CodeDataError, err
	}

	switch req.DBType {
	case "mysql", "mariadb", "oceanbase":
		port := dbConnectionPort(req.Port)
		dbProbeTimeout := 5 * time.Second
		config := mysql.Config{
			User:                 req.Username,
			Passwd:               req.Password,
			Net:                  "tcp",
			Addr:                 net.JoinHostPort(safeHost, port),
			DBName:               req.Database,
			Timeout:              dbProbeTimeout,
			AllowNativePasswords: true,
		}
		db, err := sql.Open("mysql", config.FormatDSN())
		if err != nil {
			return common.CodeExceptionError, err
		}
		defer db.Close()

		ctx, cancel := context.WithTimeout(context.Background(), dbProbeTimeout)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			return common.CodeExceptionError, err
		}
		if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
			return common.CodeExceptionError, err
		}
	default:
		return common.CodeExceptionError, errors.New("Unsupported database type.")
	}

	return common.CodeSuccess, nil
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
