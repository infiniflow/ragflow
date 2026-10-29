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
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

var ErrInvalidAgentWebhookCredential = errors.New("invalid agent webhook credential")

// AgentWebhookCredentialService issues and resolves bearer capabilities for
// external Agent webhook callers.
type AgentWebhookCredentialService struct {
	db *gorm.DB
}

func NewAgentWebhookCredentialService(db *gorm.DB) *AgentWebhookCredentialService {
	return &AgentWebhookCredentialService{db: db}
}

// Issue creates or rotates the credential for the authenticated user and the
// canvas tenant. The raw token is returned once; only its SHA-256 hash is saved.
func (s *AgentWebhookCredentialService) Issue(ctx context.Context, subject permission.Subject, canvas *entity.UserCanvas) (string, error) {
	if s == nil || s.db == nil || canvas == nil {
		return "", gorm.ErrInvalidDB
	}
	userID := strings.TrimSpace(subject.UserID)
	tenantID := canvas.EffectiveTenantID()
	if userID == "" || tenantID == "" || strings.TrimSpace(canvas.ID) == "" {
		return "", permission.ErrInvalidSubject
	}
	subject.TenantID = tenantID
	checker := permission.NewDatabaseChecker(s.db)
	if err := checker.CheckTenant(ctx, subject, tenantID, permission.TenantMember); err != nil {
		return "", err
	}
	if err := checker.CheckResource(ctx, subject, permission.ResourceRef{
		Kind: permission.ResourceKindCanvas,
		ID:   canvas.ID,
	}, permission.OperationRun); err != nil {
		return "", err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	tokenHash := hashAgentWebhookCredential(token)
	credential := &entity.AgentWebhookCredential{
		ID:        common.GenerateToken(),
		CanvasID:  canvas.ID,
		UserID:    userID,
		TenantID:  tenantID,
		TokenHash: tokenHash,
	}
	if err := dao.SaveAgentWebhookCredential(ctx, s.db, credential); err != nil {
		return "", err
	}
	return token, nil
}

// Resolve validates a raw webhook capability, confirms its canvas tenant has
// not changed, and rechecks the bound user's membership and run permission.
func (s *AgentWebhookCredentialService) Resolve(ctx context.Context, canvasID, token string) (*entity.AgentWebhookCredential, error) {
	if s == nil || s.db == nil || strings.TrimSpace(canvasID) == "" {
		return nil, ErrInvalidAgentWebhookCredential
	}
	if len(token) != 43 {
		return nil, ErrInvalidAgentWebhookCredential
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != token {
		return nil, ErrInvalidAgentWebhookCredential
	}
	credential, err := dao.GetActiveAgentWebhookCredential(ctx, s.db, canvasID, hashAgentWebhookCredential(token))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidAgentWebhookCredential
		}
		return nil, fmt.Errorf("load agent webhook credential: %w", err)
	}

	var canvas entity.UserCanvas
	if err := s.db.WithContext(ctx).Where("id = ?", canvasID).Take(&canvas).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidAgentWebhookCredential
		}
		return nil, fmt.Errorf("load webhook canvas: %w", err)
	}
	if canvas.EffectiveTenantID() != credential.TenantID {
		return nil, ErrInvalidAgentWebhookCredential
	}
	subject := permission.Subject{UserID: credential.UserID, TenantID: credential.TenantID}
	checker := permission.NewDatabaseChecker(s.db)
	if err := checker.CheckTenant(ctx, subject, credential.TenantID, permission.TenantMember); err != nil {
		if errors.Is(err, permission.ErrMembershipNotFound) || errors.Is(err, permission.ErrPermissionDenied) || errors.Is(err, permission.ErrInvalidSubject) {
			return nil, ErrInvalidAgentWebhookCredential
		}
		return nil, fmt.Errorf("check webhook tenant membership: %w", err)
	}
	if err := checker.CheckResource(ctx, subject, permission.ResourceRef{
		Kind: permission.ResourceKindCanvas,
		ID:   canvasID,
	}, permission.OperationRun); err != nil {
		if errors.Is(err, permission.ErrMembershipNotFound) || errors.Is(err, permission.ErrPermissionDenied) || errors.Is(err, permission.ErrResourceNotFound) || errors.Is(err, permission.ErrInvalidSubject) {
			return nil, ErrInvalidAgentWebhookCredential
		}
		return nil, fmt.Errorf("check webhook run permission: %w", err)
	}
	return credential, nil
}

// Revoke removes the authenticated user's ability to trigger this canvas with
// their webhook credential. The operation is idempotent.
func (s *AgentWebhookCredentialService) Revoke(ctx context.Context, canvasID, userID string) error {
	if s == nil || s.db == nil {
		return gorm.ErrInvalidDB
	}
	if strings.TrimSpace(canvasID) == "" || strings.TrimSpace(userID) == "" {
		return permission.ErrInvalidSubject
	}
	return dao.RevokeAgentWebhookCredential(ctx, s.db, canvasID, userID)
}

func hashAgentWebhookCredential(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
