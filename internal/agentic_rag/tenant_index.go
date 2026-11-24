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

package agentic_rag

import (
	"context"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/dao"
)

// corpusIndexPrefix mirrors the doc store's chunk index naming: one index per
// tenant.
const corpusIndexPrefix = "ragflow_"

type datasetTenantsContextKey struct{}

// WithDatasetTenants pins the dataset → tenant resolution for one run, so every
// corpus tool of that run reads the same scope without re-querying the database.
// An empty resolution (a failed lookup) is NOT pinned: pinning it would turn one
// transient database error into "this turn has no corpus" for every tool, so the
// lookup is left to run per call and can recover within the turn.
func WithDatasetTenants(ctx context.Context, tenants map[string]string) context.Context {
	if len(tenants) == 0 {
		return ctx
	}
	return context.WithValue(ctx, datasetTenantsContextKey{}, tenants)
}

func datasetTenantsFromContext(ctx context.Context) (map[string]string, bool) {
	if ctx == nil {
		return nil, false
	}
	tenants, ok := ctx.Value(datasetTenantsContextKey{}).(map[string]string)
	return tenants, ok
}

// ResolveDatasetTenants resolves which tenant index each bound dataset lives in.
//
// The dataset row is the only authority here, never the conversation's tenant: a
// team-shared dataset stays in the sharing tenant's index while the chat reading
// it was created in the caller's own tenant, so the conversation tenant points
// at an index holding none of those datasets' chunks.
//
// An unresolvable scope yields an empty map; callers then report an empty corpus
// instead of widening the scope.
func ResolveDatasetTenants(ctx context.Context, datasetIDs []string) map[string]string {
	if len(datasetIDs) == 0 || dao.DB == nil {
		return nil
	}
	groups, err := dao.GroupKBIDsByTenant(ctx, dao.DB, datasetIDs)
	if err != nil {
		common.WarnCtx(ctx, "agentic_rag: resolve dataset tenants", zap.Error(err))
		return nil
	}
	tenants := make(map[string]string, len(datasetIDs))
	for _, group := range groups {
		if group.TenantID == "" {
			continue
		}
		for _, datasetID := range group.KBIDs {
			tenants[datasetID] = group.TenantID
		}
	}
	return tenants
}

// DatasetTenants returns the dataset → tenant map for a corpus scope: the run's
// pinned resolution when it exists, otherwise a database lookup.
func DatasetTenants(ctx context.Context, datasetIDs []string) map[string]string {
	if tenants, ok := datasetTenantsFromContext(ctx); ok {
		return tenants
	}
	return ResolveDatasetTenants(ctx, datasetIDs)
}

// IndexNamesFor maps the bound datasets to the chunk indexes that hold them: one
// ragflow_<tenant> index per distinct dataset tenant, in bound-dataset order.
// The engines reject an empty list, so an unresolved scope can only ever produce
// "no results", never an unscoped read.
func IndexNamesFor(ctx context.Context, datasetIDs []string) []string {
	tenants := DatasetTenants(ctx, datasetIDs)
	names := make([]string, 0, len(tenants))
	seen := make(map[string]struct{}, len(tenants))
	for _, datasetID := range datasetIDs {
		tenantID := tenants[datasetID]
		if tenantID == "" {
			continue
		}
		if _, ok := seen[tenantID]; ok {
			continue
		}
		seen[tenantID] = struct{}{}
		names = append(names, corpusIndexPrefix+tenantID)
	}
	return names
}

func groupDatasetsByTenant(ctx context.Context, datasetIDs []string) []dao.TenantKBIDs {
	tenants := DatasetTenants(ctx, datasetIDs)
	groups := make([]dao.TenantKBIDs, 0, len(tenants))
	positions := make(map[string]int, len(tenants))
	for _, datasetID := range datasetIDs {
		tenantID := tenants[datasetID]
		if tenantID == "" {
			continue
		}
		pos, ok := positions[tenantID]
		if !ok {
			pos = len(groups)
			positions[tenantID] = pos
			groups = append(groups, dao.TenantKBIDs{TenantID: tenantID})
		}
		groups[pos].KBIDs = append(groups[pos].KBIDs, datasetID)
	}
	return groups
}
