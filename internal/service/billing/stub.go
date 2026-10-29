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

//go:build !ee

package billing

import (
	"context"
	"fmt"

	"ragflow/internal/server/config"
)

// Service is the billing facade. The OSS build never enables it, so every
// method is a no-op and Get returns nil; call sites stay identical to the EE
// build.
type Service struct{}

// ResourceDelta is the resource increment a call site wants to reserve.
type ResourceDelta struct {
	Apps    int64
	Members int64
	Storage int64
}

// InsufficientResourceError reports a quota breach with the values the upgrade
// prompt renders.
type InsufficientResourceError struct {
	Resource string
	Current  int64
	Limit    int64
	Required int64
}

func (e *InsufficientResourceError) Error() string {
	return fmt.Sprintf("%s limit reached: current %d, limit %d, requested %d", e.Resource, e.Current, e.Limit, e.Required)
}

// Init is a no-op: billing is an enterprise capability.
func Init(ctx context.Context, cfg config.BillingConfig) error { return nil }

// Start is a no-op: billing is an enterprise capability.
func Start(ctx context.Context) error { return nil }

// Get returns the billing service, which the OSS build never creates.
func Get() *Service { return nil }

// Enabled reports whether billing is active. The OSS build never enables it.
func (s *Service) Enabled() bool { return false }

// CheckResources reports a quota breach, which cannot happen without billing.
func (s *Service) CheckResources(ctx context.Context, tenantID string, delta ResourceDelta) error {
	return nil
}

// SyncLoginTokenRateLimit applies the tenant plan to a freshly issued login
// token.
func (s *Service) SyncLoginTokenRateLimit(ctx context.Context, tenantID, accessToken string) {}

// SyncAPITokenAccess applies the tenant plan and per-token rate limits to a
// freshly issued set of API tokens.
func (s *Service) SyncAPITokenAccess(ctx context.Context, tenantID string, tokens ...string) {}

// DeleteRateLimitKeys drops the rate-limit entries of deleted tokens.
func (s *Service) DeleteRateLimitKeys(ctx context.Context, tokens ...string) {}

// APIAuthAllowed reports whether the tenant plan may authenticate with API
// keys.
func (s *Service) APIAuthAllowed(ctx context.Context, tenantID string) (bool, error) {
	return true, nil
}

// ChargeParsedDocument meters a parsed DeepDOC PDF.
func (s *Service) ChargeParsedDocument(ctx context.Context, tenantID, docID, runKey string, pageCount int64, deepdoc bool) error {
	return nil
}

// ReleaseDocumentHold releases a document's uncommitted parse hold.
func (s *Service) ReleaseDocumentHold(ctx context.Context, docID string) error { return nil }
