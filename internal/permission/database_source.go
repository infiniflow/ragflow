//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package permission

import (
	"context"
	"errors"
	"fmt"

	"ragflow/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type databaseSource struct {
	db       *gorm.DB
	lockRows bool
}

// NewDatabaseChecker creates a Checker backed by the application's SQL database.
// It currently loads dataset authorization facts.
func NewDatabaseChecker(db *gorm.DB) *Checker {
	return NewChecker(&databaseSource{db: db})
}

// NewLockingDatabaseChecker creates a database-backed Checker that locks loaded
// resource and membership rows. Use it only with a short-lived transaction.
func NewLockingDatabaseChecker(tx *gorm.DB) *Checker {
	return NewChecker(&databaseSource{db: tx, lockRows: true})
}

func (s *databaseSource) GetMembership(ctx context.Context, userID, tenantID string) (*Membership, error) {
	if s == nil || s.db == nil {
		return nil, ErrSourceUnavailable
	}
	query := s.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND status = ?", userID, tenantID, string(entity.StatusValid))
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var relation entity.UserTenant
	if err := query.First(&relation).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &Membership{
		ID:       relation.ID,
		UserID:   relation.UserID,
		TenantID: relation.TenantID,
		Role:     TenantRole(relation.Role),
		Active:   relation.Status != nil && *relation.Status == string(entity.StatusValid),
	}, nil
}

func (s *databaseSource) GetResources(ctx context.Context, refs []ResourceRef) ([]Resource, error) {
	if s == nil || s.db == nil {
		return nil, ErrSourceUnavailable
	}
	refs = uniqueRefs(refs)
	datasetIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Kind != ResourceKindDataset {
			return nil, fmt.Errorf("%w: unsupported resource kind %q", ErrInvalidPermission, ref.Kind)
		}
		datasetIDs = append(datasetIDs, ref.ID)
	}
	datasetsByID, ownerIDsByTenant, err := s.loadDatasets(ctx, datasetIDs)
	if err != nil {
		return nil, err
	}

	resources := make([]Resource, 0, len(refs))
	for _, ref := range refs {
		if dataset, ok := datasetsByID[ref.ID]; ok {
			resources = append(resources, datasetResource(dataset, ownerIDsByTenant))
		}
	}
	return resources, nil
}

func (s *databaseSource) ListResourceRefs(ctx context.Context, subject Subject, kind ResourceKind, parent ResourceRef) ([]ResourceRef, error) {
	if s == nil || s.db == nil {
		return nil, ErrSourceUnavailable
	}
	var ids []string
	switch kind {
	case ResourceKindDataset:
		if parent.ID != "" || parent.Kind != "" {
			return nil, fmt.Errorf("%w: datasets cannot have a parent scope", ErrInvalidPermission)
		}
		var tenantIDs []string
		if err := s.db.WithContext(ctx).Model(&entity.UserTenant{}).
			Distinct("tenant_id").
			Where("user_id = ? AND status = ? AND role IN ?", subject.UserID, string(entity.StatusValid), []string{
				string(RoleOwner), string(RoleAdmin), string(RoleNormal),
			}).
			Pluck("tenant_id", &tenantIDs).Error; err != nil {
			return nil, err
		}
		if len(tenantIDs) == 0 {
			return []ResourceRef{}, nil
		}
		err := s.db.WithContext(ctx).Model(&entity.Knowledgebase{}).
			Where("tenant_id IN ? AND status = ?", tenantIDs, string(entity.StatusValid)).
			Pluck("id", &ids).Error
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("%w: unsupported scope resource kind %q", ErrInvalidPermission, kind)
	}

	refs := make([]ResourceRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, ResourceRef{Kind: kind, ID: id})
	}
	return refs, nil
}

func (s *databaseSource) loadDatasets(ctx context.Context, ids []string) (map[string]*entity.Knowledgebase, map[string]string, error) {
	datasetsByID := make(map[string]*entity.Knowledgebase)
	if len(ids) == 0 {
		return datasetsByID, nil, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.Knowledgebase{}).Where("id IN ? AND status = ?", ids, string(entity.StatusValid))
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var datasets []*entity.Knowledgebase
	if err := query.Find(&datasets).Error; err != nil {
		return nil, nil, err
	}
	tenantIDs := make([]string, 0, len(datasets))
	seenTenantIDs := make(map[string]struct{}, len(datasets))
	for _, dataset := range datasets {
		if _, seen := seenTenantIDs[dataset.TenantID]; seen {
			continue
		}
		seenTenantIDs[dataset.TenantID] = struct{}{}
		tenantIDs = append(tenantIDs, dataset.TenantID)
	}

	ownerQuery := s.db.WithContext(ctx).Model(&entity.UserTenant{}).
		Where("tenant_id IN ? AND role = ? AND status = ?", tenantIDs, string(RoleOwner), string(entity.StatusValid))
	if s.lockRows {
		ownerQuery = ownerQuery.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var owners []entity.UserTenant
	if err := ownerQuery.Find(&owners).Error; err != nil {
		return nil, nil, err
	}
	ownerIDsByTenant := make(map[string]string, len(owners))
	for _, owner := range owners {
		if existingOwnerID, exists := ownerIDsByTenant[owner.TenantID]; exists && existingOwnerID != owner.UserID {
			return nil, nil, fmt.Errorf("%w: tenant %q has multiple active owners", ErrInvalidPermission, owner.TenantID)
		}
		ownerIDsByTenant[owner.TenantID] = owner.UserID
	}
	for _, dataset := range datasets {
		ownerUserID := ownerIDsByTenant[dataset.TenantID]
		if ownerUserID == "" {
			return nil, nil, fmt.Errorf("%w: tenant %q has no active owner", ErrInvalidPermission, dataset.TenantID)
		}
		datasetsByID[dataset.ID] = dataset
	}
	return datasetsByID, ownerIDsByTenant, nil
}

func datasetResource(dataset *entity.Knowledgebase, ownerIDsByTenant map[string]string) Resource {
	ownerUserID := ownerIDsByTenant[dataset.TenantID]
	visibility := VisibilityPrivate
	switch dataset.Permission {
	case string(entity.TenantPermissionMe):
		visibility = VisibilityPrivate
	case string(entity.TenantPermissionTeam):
		visibility = VisibilityTenant
	}
	return Resource{
		Ref:             ResourceRef{Kind: ResourceKindDataset, ID: dataset.ID},
		TenantID:        dataset.TenantID,
		CreatedBy:       dataset.CreatedBy,
		OwnerUserID:     ownerUserID,
		Visibility:      visibility,
		OwnerOperations: []Operation{OperationRead, OperationCreate, OperationUpdate, OperationDelete, OperationShare, OperationRun, OperationUse},
		Active:          dataset.Status != nil && *dataset.Status == string(entity.StatusValid),
	}
}
