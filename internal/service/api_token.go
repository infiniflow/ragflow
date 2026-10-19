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
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"time"
)

// APIKeyResponse key response
type APIKeyResponse struct {
	TenantID   string     `json:"tenant_id"`
	Token      string     `json:"token"`
	DialogID   *string    `json:"dialog_id,omitempty"`
	Source     *string    `json:"source,omitempty"`
	Beta       *string    `json:"beta,omitempty"`
	CreateTime *int64     `json:"create_time,omitempty"`
	CreateDate *time.Time `json:"create_date,omitempty"`
	UpdateTime *int64     `json:"update_time,omitempty"`
	UpdateDate *time.Time `json:"update_date,omitempty"`
}

// maskAPIToken renders an API token safe for list responses: a visible
// prefix/suffix pair so an operator can still tell keys apart, never the
// middle. The shape follows maskAPIKey in model_service.go (PR #20549): ""
// stays "" (a no-record field must remain distinguishable from a masked
// one), anything too short to survive truncation collapses to a constant so
// a short secret is never partially echoed, and longer tokens keep 3 + 4
// visible characters — enough to recognize `rag`-prefixed ragflow tokens.
func maskAPIToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 7 {
		return "***"
	}
	return token[:3] + "***" + token[len(token)-4:]
}

// ListAPIKeys list all API keys for a tenant
func (s *SystemService) ListAPIKeys(ctx context.Context, tenantID string) ([]*APIKeyResponse, error) {
	APITokenDAO := dao.NewAPITokenDAO()
	keys, err := APITokenDAO.GetByTenantID(ctx, dao.DB, tenantID)
	if err != nil {
		return nil, err
	}

	responses := make([]*APIKeyResponse, len(keys))
	for i, key := range keys {
		beta := key.Beta
		if beta == nil || *beta == "" {
			generatedBeta := common.GenerateBetaAPIToken()
			if err = dao.DB.WithContext(ctx).Model(&entity.APIToken{}).
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
			TenantID: key.TenantID,
			// The full token is returned exactly once, by CreateAPIKey, so
			// the user can copy it. A GET response travels through proxies,
			// browser history, and dev-tool network panels, so the list path
			// must never echo the credential back (same class of leak the
			// user-profile fix #20277 closed).
			Token:      maskAPIToken(key.Token),
			DialogID:   key.DialogID,
			Source:     key.Source,
			Beta:       beta,
			CreateTime: key.CreateTime,
			CreateDate: key.CreateDate,
			UpdateTime: key.UpdateTime,
			UpdateDate: key.UpdateDate,
		}
	}

	return responses, nil
}

// CreateAPIKeyRequest create key request
type CreateAPIKeyRequest struct {
	Name string `json:"name" form:"name"`
}

// CreateAPIKey creates a new API key for a tenant
func (s *SystemService) CreateAPIKey(ctx context.Context, tenantID string, req *CreateAPIKeyRequest) (*APIKeyResponse, error) {
	APITokenDAO := dao.NewAPITokenDAO()

	// Generate key and beta values
	// key: "ragflow-" + secrets.token_urlsafe(32)
	APIToken := common.GenerateAPIToken()
	// beta: generate_confirmation_token().replace("ragflow-", "")[:32]
	betaAPIKey := common.GenerateBetaAPIToken()

	APIKeyData := &entity.APIToken{
		TenantID: tenantID,
		Token:    APIToken,
		Beta:     &betaAPIKey,
	}

	if err := APITokenDAO.Create(ctx, dao.DB, APIKeyData); err != nil {
		return nil, err
	}

	return &APIKeyResponse{
		TenantID:   APIKeyData.TenantID,
		Token:      APIKeyData.Token,
		DialogID:   APIKeyData.DialogID,
		Source:     APIKeyData.Source,
		Beta:       APIKeyData.Beta,
		CreateTime: APIKeyData.CreateTime,
		CreateDate: APIKeyData.CreateDate,
		UpdateTime: APIKeyData.UpdateTime,
		UpdateDate: APIKeyData.UpdateDate,
	}, nil
}

func (s *SystemService) DeleteAPIKey(ctx context.Context, tenantID, key string) error {
	APITokenDAO := dao.NewAPITokenDAO()
	_, err := APITokenDAO.DeleteByTenantIDAndToken(ctx, dao.DB, tenantID, key)
	return err
}
