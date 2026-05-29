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
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// Sentinel errors so handlers can map bot failures to the Python response codes
// used by api/apps/restful_apis/bot_api.py.
var (
	// ErrBotInvalidAPIKey mirrors "Authentication error: API key is invalid!".
	ErrBotInvalidAPIKey = errors.New("Authentication error: API key is invalid!")
	// ErrBotNoChatbotAccess mirrors "Authentication error: no access to this chatbot!".
	ErrBotNoChatbotAccess = errors.New("Authentication error: no access to this chatbot!")
	// ErrBotNoTenant mirrors the "permission denined." denial (Python typo kept).
	ErrBotNoTenant = errors.New("permission denined.")
	// ErrBotNoSearchPermission mirrors "Has no permission for this operation.".
	ErrBotNoSearchPermission = errors.New("Has no permission for this operation.")
	// ErrBotSearchNotFound mirrors "Can't find this Search App!".
	ErrBotSearchNotFound = errors.New("Can't find this Search App!")
)

// statusValid is StatusEnum.VALID.value in Python.
const statusValid = "1"

// BotService backs the public chatbot/searchbot endpoints. These are
// authenticated with an SDK "beta" token rather than a session, so the service
// resolves the token to a tenant before serving bot metadata.
type BotService struct {
	apiTokenDAO   *dao.APITokenDAO
	chatDAO       *dao.ChatDAO
	searchDAO     *dao.SearchDAO
	userTenantDAO *dao.UserTenantDAO
}

// NewBotService create bot service
func NewBotService() *BotService {
	return &BotService{
		apiTokenDAO:   dao.NewAPITokenDAO(),
		chatDAO:       dao.NewChatDAO(),
		searchDAO:     dao.NewSearchDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
	}
}

// AuthByBetaToken resolves an SDK beta token to its tenant ID.
// Mirrors APIToken.query(beta=token) followed by objs[0].tenant_id.
func (s *BotService) AuthByBetaToken(token string) (string, error) {
	tokens, err := s.apiTokenDAO.GetByBeta(token)
	if err != nil {
		return "", err
	}
	if len(tokens) == 0 {
		return "", ErrBotInvalidAPIKey
	}
	return tokens[0].TenantID, nil
}

// GetChatbotInfo returns public chatbot metadata for a dialog the tenant owns.
// Equivalent to chatbots_inputs in api/apps/restful_apis/bot_api.py.
func (s *BotService) GetChatbotInfo(tenantID, dialogID string) (map[string]interface{}, error) {
	dialog, err := s.chatDAO.GetByID(dialogID)
	if err != nil {
		return nil, ErrBotNoChatbotAccess
	}
	if dialog.TenantID != tenantID || dialog.Status == nil || *dialog.Status != statusValid {
		return nil, ErrBotNoChatbotAccess
	}

	prologue := ""
	hasTavilyKey := false
	if dialog.PromptConfig != nil {
		if v, ok := dialog.PromptConfig["prologue"].(string); ok {
			prologue = v
		}
		if v, ok := dialog.PromptConfig["tavily_api_key"].(string); ok {
			hasTavilyKey = strings.TrimSpace(v) != ""
		}
	}

	return map[string]interface{}{
		"title":          dialog.Name,
		"avatar":         dialog.Icon,
		"prologue":       prologue,
		"has_tavily_key": hasTavilyKey,
	}, nil
}

// GetSearchbotDetail returns search-app detail when the tenant can access it.
// Equivalent to detail_share_embedded in api/apps/restful_apis/bot_api.py.
func (s *BotService) GetSearchbotDetail(tenantID, searchID string) (map[string]interface{}, error) {
	if tenantID == "" {
		return nil, ErrBotNoTenant
	}

	tenants, err := s.userTenantDAO.GetByUserID(tenantID)
	if err != nil {
		return nil, err
	}

	hasPermission := false
	for _, tenant := range tenants {
		searches, err := s.searchDAO.QueryByTenantIDAndID(tenant.TenantID, searchID)
		if err != nil {
			continue
		}
		if len(searches) > 0 {
			hasPermission = true
			break
		}
	}
	if !hasPermission {
		return nil, ErrBotNoSearchPermission
	}

	search, err := s.searchDAO.GetByID(searchID)
	if err != nil {
		return nil, ErrBotSearchNotFound
	}

	return searchAppDetail(search), nil
}

// searchAppDetail builds the search-app detail map, mirroring
// SearchService.GetDetail / Python SearchService.get_detail.
func searchAppDetail(search *entity.Search) map[string]interface{} {
	detail := map[string]interface{}{
		"id":            search.ID,
		"tenant_id":     search.TenantID,
		"name":          search.Name,
		"description":   search.Description,
		"created_by":    search.CreatedBy,
		"status":        search.Status,
		"create_time":   search.CreateTime,
		"update_time":   search.UpdateTime,
		"search_config": search.SearchConfig,
	}
	if search.Avatar != nil {
		detail["avatar"] = *search.Avatar
	}
	return detail
}
