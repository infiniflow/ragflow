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
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

// APIKeyResponse key response
type APIKeyResponse struct {
	TenantID   string  `json:"tenant_id"`
	Token      string  `json:"token"`
	DialogID   *string `json:"dialog_id,omitempty"`
	Source     *string `json:"source,omitempty"`
	Beta       *string `json:"beta,omitempty"`
	CreateTime *int64  `json:"create_time,omitempty"`
	UpdateTime *int64  `json:"update_time,omitempty"`
}

// ListAPIKeys list all API keys for a tenant
func (s *SystemService) ListAPIKeys(tenantID string) ([]*APIKeyResponse, error) {
	APITokenDAO := dao.NewAPITokenDAO()
	keys, err := APITokenDAO.GetByTenantID(tenantID)
	if err != nil {
		return nil, err
	}

	responses := make([]*APIKeyResponse, len(keys))
	for i, key := range keys {
		beta := key.Beta
		if beta == nil || *beta == "" {
			generatedBeta := utility.GenerateBetaAPIToken()
			if err = dao.DB.Model(&entity.APIToken{}).
				Where("tenant_id = ? AND token = ?", tenantID, key.Token).
				Updates(map[string]interface{}{
					"beta": generatedBeta,
				}).Error; err != nil {
				return nil, err
			}
			beta = &generatedBeta
			key.Beta = beta
		}

		responses[i] = &APIKeyResponse{
			TenantID:   key.TenantID,
			Token:      key.Token,
			DialogID:   key.DialogID,
			Source:     key.Source,
			Beta:       beta,
			CreateTime: key.CreateTime,
			UpdateTime: key.UpdateTime,
		}
	}

	return responses, nil
}

// CreateAPIKeyRequest create key request
type CreateAPIKeyRequest struct {
	Name string `json:"name" form:"name"`
}

// CreateAPIKey creates a new API key for a tenant
func (s *SystemService) CreateAPIKey(tenantID string, req *CreateAPIKeyRequest) (*APIKeyResponse, error) {
	APITokenDAO := dao.NewAPITokenDAO()

	// Generate key and beta values
	// key: "ragflow-" + secrets.token_urlsafe(32)
	APIToken := utility.GenerateAPIToken()
	// beta: generate_confirmation_token().replace("ragflow-", "")[:32]
	betaAPIKey := utility.GenerateBetaAPIToken()

	APIKeyData := &entity.APIToken{
		TenantID: tenantID,
		Token:    APIToken,
		Beta:     &betaAPIKey,
	}

	if err := APITokenDAO.Create(APIKeyData); err != nil {
		return nil, err
	}

	return &APIKeyResponse{
		TenantID:   APIKeyData.TenantID,
		Token:      APIKeyData.Token,
		DialogID:   APIKeyData.DialogID,
		Source:     APIKeyData.Source,
		Beta:       APIKeyData.Beta,
		CreateTime: APIKeyData.CreateTime,
		UpdateTime: APIKeyData.UpdateTime,
	}, nil
}

func (s *SystemService) DeleteAPIKey(tenantID, key string) error {
	APITokenDAO := dao.NewAPITokenDAO()
	_, err := APITokenDAO.DeleteByTenantIDAndToken(tenantID, key)
	return err
}
