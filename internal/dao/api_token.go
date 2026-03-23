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

package dao

import (
	"ragflow/internal/model"
)

// APITokenDAO API token data access object
type APITokenDAO struct{}

// NewAPITokenDAO create API token DAO
func NewAPITokenDAO() *APITokenDAO {
	return &APITokenDAO{}
}

// Create creates a new API token
func (dao *APITokenDAO) Create(apiToken *model.APIToken) error {
	return DB.Create(apiToken).Error
}

// GetByTenantID gets API tokens by tenant ID
func (dao *APITokenDAO) GetByTenantID(tenantID string) ([]*model.APIToken, error) {
	var tokens []*model.APIToken
	err := DB.Where("tenant_id = ?", tenantID).Find(&tokens).Error
	return tokens, err
}

// DeleteByTenantID deletes all API tokens by tenant ID (hard delete)
func (dao *APITokenDAO) DeleteByTenantID(tenantID string) (int64, error) {
	result := DB.Unscoped().Where("tenant_id = ?", tenantID).Delete(&model.APIToken{})
	return result.RowsAffected, result.Error
}

// GetByToken gets API token by access key
func (dao *APITokenDAO) GetUserByAPIToken(token string) (*model.APIToken, error) {
	var apiToken model.APIToken
	err := DB.Where("token = ?", token).First(&apiToken).Error
	if err != nil {
		return nil, err
	}
	return &apiToken, nil
}

// DeleteByDialogIDs deletes API tokens by dialog IDs (hard delete)
func (dao *APITokenDAO) DeleteByDialogIDs(dialogIDs []string) (int64, error) {
	if len(dialogIDs) == 0 {
		return 0, nil
	}
	result := DB.Unscoped().Where("dialog_id IN ?", dialogIDs).Delete(&model.APIToken{})
	return result.RowsAffected, result.Error
}

// DeleteByTenantIDAndToken deletes a specific API token by tenant ID and token value
func (dao *APITokenDAO) DeleteByTenantIDAndToken(tenantID, token string) (int64, error) {
	result := DB.Unscoped().Where("tenant_id = ? AND token = ?", tenantID, token).Delete(&model.APIToken{})
	return result.RowsAffected, result.Error
}

// API4ConversationDAO API for conversation data access object
type API4ConversationDAO struct{}

// NewAPI4ConversationDAO create API4Conversation DAO
func NewAPI4ConversationDAO() *API4ConversationDAO {
	return &API4ConversationDAO{}
}

// DeleteByDialogIDs deletes API4Conversations by dialog IDs (hard delete)
func (dao *API4ConversationDAO) DeleteByDialogIDs(dialogIDs []string) (int64, error) {
	if len(dialogIDs) == 0 {
		return 0, nil
	}
	result := DB.Unscoped().Where("dialog_id IN ?", dialogIDs).Delete(&model.API4Conversation{})
	return result.RowsAffected, result.Error
}
