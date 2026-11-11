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
	"net/url"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	permissionresponse "ragflow/internal/permission/response"
)

// futureAGIVerifier checks a key pair against the collector before it is
// stored, so a bad key fails the save instead of silently dropping traces.
type futureAGIVerifier interface {
	Verify(ctx context.Context, endpoint, apiKey, secretKey string) error
}

// ErrFutureAGIUnauthorized means the collector rejected the key pair.
var ErrFutureAGIUnauthorized = errors.New("invalid Future AGI keys")

type defaultFutureAGIVerifier struct{}

// Verify posts an empty OTLP export. The collector authenticates it like any
// export and stores nothing.
func (defaultFutureAGIVerifier) Verify(ctx context.Context, endpoint, apiKey, secretKey string) error {
	client := &FutureAGIClient{Endpoint: endpoint, APIKey: apiKey, SecretKey: secretKey}
	err := client.post(ctx, []byte(`{"resourceSpans":[]}`))
	if err == nil {
		return nil
	}
	if strings.HasSuffix(err.Error(), "HTTP 401") || strings.HasSuffix(err.Error(), "HTTP 403") {
		return ErrFutureAGIUnauthorized
	}
	return err
}

// FutureAGIService implements /futureagi/api-key: per-tenant Future AGI
// credentials for the chat trace export.
type FutureAGIService struct {
	futureAGIDAO *dao.FutureAGIDAO
	verifier     futureAGIVerifier
}

// NewFutureAGIService creates the service with the live verifier.
func NewFutureAGIService() *FutureAGIService {
	return &FutureAGIService{futureAGIDAO: dao.NewFutureAGI(), verifier: defaultFutureAGIVerifier{}}
}

// FutureAGISettings is the request body of a save.
type FutureAGISettings struct {
	APIKey      string `json:"api_key"`
	SecretKey   string `json:"secret_key"`
	ProjectName string `json:"project_name"`
	Endpoint    string `json:"endpoint"`
	Redact      bool   `json:"redact"`
}

// FutureAGIInfo is what a read returns. Keys are masked.
type FutureAGIInfo struct {
	TenantID    string `json:"tenant_id"`
	Endpoint    string `json:"endpoint"`
	ProjectName string `json:"project_name"`
	Redact      bool   `json:"redact"`
	APIKey      string `json:"api_key"`
	SecretKey   string `json:"secret_key"`
}

func maskFutureAGIKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}

func futureAGIInfo(row *entity.TenantFutureAGI) *FutureAGIInfo {
	return &FutureAGIInfo{
		TenantID:    row.TenantID,
		Endpoint:    row.Endpoint,
		ProjectName: row.ProjectName,
		Redact:      row.Redact,
		APIKey:      maskFutureAGIKey(row.APIKey),
		SecretKey:   maskFutureAGIKey(row.SecretKey),
	}
}

// SetAPIKey validates and stores (insert or replace) the tenant's settings.
func (s *FutureAGIService) SetAPIKey(ctx context.Context, req FutureAGISettings) (*FutureAGIInfo, common.ErrorCode, error) {
	tenantID, err := requireCurrentTenantID(ctx)
	if err != nil {
		code, normalized := permissionresponse.NormalizeHidden(err)
		return nil, code, normalized
	}
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.SecretKey = strings.TrimSpace(req.SecretKey)
	req.ProjectName = strings.TrimSpace(req.ProjectName)
	req.Endpoint = strings.TrimRight(strings.TrimSpace(req.Endpoint), "/")
	if req.Endpoint == "" {
		req.Endpoint = FutureAGIDefaultEndpoint
	}
	existing, err := s.futureAGIDAO.GetByTenantID(ctx, dao.DB, tenantID)
	if err != nil {
		return nil, common.CodeServerError, errors.New("cannot read Future AGI settings")
	}
	if existing != nil {
		// Never send retained credentials to a new destination. Changing the
		// endpoint requires explicitly entering both keys again.
		if req.Endpoint != existing.Endpoint && (req.APIKey == "" || req.SecretKey == "") {
			return nil, common.CodeDataError, errors.New("enter both keys when changing the endpoint")
		}
		if req.APIKey == "" {
			req.APIKey = existing.APIKey
		}
		if req.SecretKey == "" {
			req.SecretKey = existing.SecretKey
		}
	}
	if req.APIKey == "" || req.SecretKey == "" || req.ProjectName == "" {
		return nil, common.CodeDataError, errors.New("missing required fields: api_key, secret_key and project_name are required")
	}
	if u, err := url.Parse(req.Endpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(req.Endpoint, "#") {
		return nil, common.CodeDataError, errors.New("endpoint must be an http(s) URL without credentials, query or fragment")
	}
	if len(req.ProjectName) > 128 || len(req.Endpoint) > 256 || len(req.APIKey) > 256 || len(req.SecretKey) > 256 {
		return nil, common.CodeDataError, errors.New("a Future AGI setting exceeds its maximum length")
	}

	if err := s.verifier.Verify(ctx, req.Endpoint, req.APIKey, req.SecretKey); err != nil {
		if errors.Is(err, ErrFutureAGIUnauthorized) {
			return nil, common.CodeDataError, err
		}
		return nil, common.CodeDataError, fmt.Errorf("cannot verify the Future AGI keys: %s", err.Error())
	}

	row := &entity.TenantFutureAGI{
		TenantID:    tenantID,
		APIKey:      req.APIKey,
		SecretKey:   req.SecretKey,
		Endpoint:    req.Endpoint,
		ProjectName: req.ProjectName,
		Redact:      req.Redact,
	}
	if err := s.futureAGIDAO.SaveByTenantID(ctx, dao.DB, row); err != nil {
		return nil, common.CodeServerError, errors.New("cannot save Future AGI settings")
	}
	forgetFutureAGIClient(tenantID)
	return futureAGIInfo(row), common.CodeSuccess, nil
}

// GetAPIKey returns the tenant's settings with masked keys.
func (s *FutureAGIService) GetAPIKey(ctx context.Context) (*FutureAGIInfo, common.ErrorCode, string, error) {
	tenantID, err := requireCurrentTenantID(ctx)
	if err != nil {
		code, normalized := permissionresponse.NormalizeHidden(err)
		return nil, code, "", normalized
	}
	row, err := s.futureAGIDAO.GetByTenantID(ctx, dao.DB, tenantID)
	if err != nil {
		return nil, common.CodeServerError, "", err
	}
	if row == nil {
		return nil, common.CodeSuccess, "No Future AGI configuration saved.", nil
	}
	return futureAGIInfo(row), common.CodeSuccess, "success", nil
}

// DeleteAPIKey removes the tenant's settings; export stops with the next turn.
func (s *FutureAGIService) DeleteAPIKey(ctx context.Context) (bool, common.ErrorCode, string, error) {
	tenantID, err := requireCurrentTenantID(ctx)
	if err != nil {
		code, normalized := permissionresponse.NormalizeHidden(err)
		return false, code, "", normalized
	}
	if err := s.futureAGIDAO.DeleteByTenantID(ctx, dao.DB, tenantID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, common.CodeSuccess, "No Future AGI configuration saved.", nil
		}
		return false, common.CodeServerError, "", err
	}
	forgetFutureAGIClient(tenantID)
	return true, common.CodeSuccess, "", nil
}
