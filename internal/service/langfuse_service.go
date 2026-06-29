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

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// langfuseVerifier abstracts Langfuse credential verification so the business
// logic can be unit-tested without performing real network calls.
type langfuseVerifier interface {
	// AuthCheck mirrors the Python langfuse SDK auth_check().
	AuthCheck(ctx context.Context, host, publicKey, secretKey string) (bool, error)
	// GetProject mirrors api.projects.get().dict()["data"][0] (id, name).
	GetProject(ctx context.Context, host, publicKey, secretKey string) (string, string, error)
}

// defaultLangfuseVerifier uses a real LangfuseClient for verification.
type defaultLangfuseVerifier struct{}

func (defaultLangfuseVerifier) AuthCheck(ctx context.Context, host, publicKey, secretKey string) (bool, error) {
	client := NewLangfuseClient(host, publicKey, secretKey)
	defer client.Shutdown(context.Background())
	return client.AuthCheck(ctx)
}

func (defaultLangfuseVerifier) GetProject(ctx context.Context, host, publicKey, secretKey string) (string, string, error) {
	client := NewLangfuseClient(host, publicKey, secretKey)
	defer client.Shutdown(context.Background())
	return client.GetProject(ctx)
}

// LangfuseService implements the /langfuse/api-key business logic, mirroring
// the Python TenantLangfuseService + langfuse_api handlers.
type LangfuseService struct {
	langfuseDAO *dao.LangfuseDAO
	verifier    langfuseVerifier
}

// NewLangfuseService creates a LangfuseService with the default (live) verifier.
func NewLangfuseService() *LangfuseService {
	return &LangfuseService{
		langfuseDAO: dao.NewLangfuse(),
		verifier:    defaultLangfuseVerifier{},
	}
}

// SetAPIKey validates and stores (insert or update) the Langfuse credentials
// for a tenant.
func (s *LangfuseService) SetAPIKey(tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error) {
	if secretKey == "" || publicKey == "" || host == "" {
		return nil, common.CodeDataError, errors.New("Missing required fields")
	}

	ok, err := s.verifier.AuthCheck(context.Background(), host, publicKey, secretKey)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !ok {
		return nil, common.CodeDataError, errors.New("Invalid Langfuse keys")
	}

	row := &entity.TenantLangfuse{
		TenantID:  tenantID,
		SecretKey: secretKey,
		PublicKey: publicKey,
		Host:      host,
	}

	if err := s.langfuseDAO.SaveByTenantID(row); err != nil {
		return nil, common.CodeServerError, err
	}

	return row, common.CodeSuccess, nil
}

// GetAPIKey returns the stored credentials enriched with the Langfuse project
// id/name.
func (s *LangfuseService) GetAPIKey(tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error) {
	row, err := s.langfuseDAO.GetByTenantID(tenantID)
	if err != nil {
		return nil, common.CodeServerError, "", err
	}
	if row == nil {
		return nil, common.CodeSuccess, "Have not record any Langfuse keys.", nil
	}

	projectID, projectName, err := s.verifier.GetProject(context.Background(), row.Host, row.PublicKey, row.SecretKey)
	if err != nil {
		if errors.Is(err, ErrLangfuseUnauthorized) {
			return nil, common.CodeDataError, "Invalid Langfuse keys loaded", err
		}
		if IsLangfuseAPIError(err) {
			return nil, common.CodeSuccess, fmt.Sprintf("Error from Langfuse: %s", err.Error()), nil
		}
		return nil, common.CodeServerError, "", err
	}

	info := &entity.LangfuseInfoResponse{
		TenantID:    row.TenantID,
		Host:        row.Host,
		SecretKey:   row.SecretKey,
		PublicKey:   row.PublicKey,
		ProjectID:   projectID,
		ProjectName: projectName,
	}
	return info, common.CodeSuccess, "success", nil
}

// DeleteAPIKey removes the stored credentials for a tenant.
func (s *LangfuseService) DeleteAPIKey(tenantID string) (bool, common.ErrorCode, string, error) {
	if err := s.langfuseDAO.DeleteExistingByTenantID(tenantID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, common.CodeSuccess, "Have not record any Langfuse keys.", nil
		}
		return false, common.CodeServerError, "", err
	}
	return true, common.CodeSuccess, "", nil
}
