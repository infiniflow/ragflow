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
	"time"
)

// TokenResponse token response
type TokenResponse struct {
	TenantID   string  `json:"tenant_id"`
	Token      string  `json:"token"`
	DialogID   *string `json:"dialog_id,omitempty"`
	Source     *string `json:"source,omitempty"`
	Beta       *string `json:"beta,omitempty"`
	CreateTime *int64  `json:"create_time,omitempty"`
	UpdateTime *int64  `json:"update_time,omitempty"`
}

// ListAPITokens list all API tokens for a tenant
func (s *SystemService) ListAPITokens(tenantID string) ([]*TokenResponse, error) {
	APITokenDAO := dao.NewAPITokenDAO()
	tokens, err := APITokenDAO.GetByTenantID(tenantID)
	if err != nil {
		return nil, err
	}

	responses := make([]*TokenResponse, len(tokens))
	for i, token := range tokens {
		responses[i] = &TokenResponse{
			TenantID:   token.TenantID,
			Token:      token.Token,
			DialogID:   token.DialogID,
			Source:     token.Source,
			Beta:       token.Beta,
			CreateTime: token.CreateTime,
			UpdateTime: token.UpdateTime,
		}
	}

	return responses, nil
}

// CreateAPITokenRequest create token request
type CreateAPITokenRequest struct {
	Name string `json:"name" form:"name"`
}

// CreateAPIToken creates a new API token for a tenant
func (s *SystemService) CreateAPIToken(tenantID string, req *CreateAPITokenRequest) (*TokenResponse, error) {
	APITokenDAO := dao.NewAPITokenDAO()

	now := time.Now().Unix()
	nowDate := time.Now()

	// Generate token and beta values
	// token: "ragflow-" + secrets.token_urlsafe(32)
	APIToken := utility.GenerateAPIToken()
	// beta: generate_confirmation_token().replace("ragflow-", "")[:32]
	betaAPIKey := utility.GenerateBetaAPIToken(APIToken)

	APITokenData := &entity.APIToken{
		TenantID: tenantID,
		Token:    APIToken,
		Beta:     &betaAPIKey,
	}
	APITokenData.CreateDate = &nowDate
	APITokenData.CreateTime = &now

	if err := APITokenDAO.Create(APITokenData); err != nil {
		return nil, err
	}

	return &TokenResponse{
		TenantID:   APITokenData.TenantID,
		Token:      APITokenData.Token,
		DialogID:   APITokenData.DialogID,
		Source:     APITokenData.Source,
		Beta:       APITokenData.Beta,
		CreateTime: APITokenData.CreateTime,
		UpdateTime: APITokenData.UpdateTime,
	}, nil
}

// DeleteAPIToken deletes an API token by tenant ID and token value
func (s *SystemService) DeleteAPIToken(tenantID, token string) error {
	APITokenDAO := dao.NewAPITokenDAO()
	_, err := APITokenDAO.DeleteByTenantIDAndToken(tenantID, token)
	return err
}
