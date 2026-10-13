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
	"fmt"

	"ragflow/internal/entity"

	"gorm.io/gorm/clause"
)

func (s *databaseSource) loadModelProviders(ctx context.Context, ids []string) (map[string]*entity.TenantModelProvider, error) {
	providersByID := make(map[string]*entity.TenantModelProvider)
	if len(ids) == 0 {
		return providersByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.TenantModelProvider{}).Where("id IN ?", uniqueStringValues(ids))
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var providers []*entity.TenantModelProvider
	if err := query.Find(&providers).Error; err != nil {
		return nil, err
	}
	for _, provider := range providers {
		providersByID[provider.ID] = provider
	}
	return providersByID, nil
}

func (s *databaseSource) loadModelInstances(ctx context.Context, ids []string) (map[string]*entity.TenantModelInstance, error) {
	instancesByID := make(map[string]*entity.TenantModelInstance)
	if len(ids) == 0 {
		return instancesByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.TenantModelInstance{}).Where("id IN ?", uniqueStringValues(ids))
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var instances []*entity.TenantModelInstance
	if err := query.Find(&instances).Error; err != nil {
		return nil, err
	}
	for _, instance := range instances {
		instancesByID[instance.ID] = instance
	}
	return instancesByID, nil
}

func (s *databaseSource) loadTenantModels(ctx context.Context, ids []string) (map[string]*entity.TenantModel, error) {
	modelsByID := make(map[string]*entity.TenantModel)
	if len(ids) == 0 {
		return modelsByID, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.TenantModel{}).Where("id IN ?", uniqueStringValues(ids))
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var models []*entity.TenantModel
	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}
	for _, model := range models {
		modelsByID[model.ID] = model
	}
	return modelsByID, nil
}

func (s *databaseSource) loadTenantOwnerIDs(ctx context.Context, tenantIDs []string) (map[string]string, error) {
	ownersByTenant := make(map[string]string)
	tenantIDs = uniqueStringValues(tenantIDs)
	if len(tenantIDs) == 0 {
		return ownersByTenant, nil
	}
	query := s.db.WithContext(ctx).Model(&entity.UserTenant{}).
		Where("tenant_id IN ? AND role = ? AND status = ?", tenantIDs, string(RoleOwner), string(entity.StatusValid))
	if s.lockRows {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var owners []entity.UserTenant
	if err := query.Find(&owners).Error; err != nil {
		return nil, err
	}
	for _, owner := range owners {
		if existingOwnerID, exists := ownersByTenant[owner.TenantID]; exists && existingOwnerID != owner.UserID {
			return nil, fmt.Errorf("%w: tenant %q has multiple active owners", ErrInvalidPermission, owner.TenantID)
		}
		ownersByTenant[owner.TenantID] = owner.UserID
	}
	return ownersByTenant, nil
}

func (s *databaseSource) tenantIDsForSubject(ctx context.Context, subject Subject) ([]string, error) {
	var tenantIDs []string
	err := s.db.WithContext(ctx).Model(&entity.UserTenant{}).
		Distinct("tenant_id").
		Where("user_id = ? AND status = ? AND role IN ?", subject.UserID, string(entity.StatusValid), []string{
			string(RoleOwner), string(RoleAdmin), string(RoleNormal),
		}).
		Pluck("tenant_id", &tenantIDs).Error
	if err != nil {
		return nil, err
	}
	return uniqueStringValues(append(tenantIDs, subject.UserID)), nil
}

func modelProviderResource(provider *entity.TenantModelProvider, ownerUserID string) Resource {
	return Resource{
		Ref:               ResourceRef{Kind: ResourceKindModelProvider, ID: provider.ID},
		TenantID:          provider.TenantID,
		OwnerUserID:       ownerUserID,
		Visibility:        VisibilityTenant,
		TenantRequirement: TenantMember,
		OwnerOperations:   []Operation{OperationRead, OperationCreate, OperationUpdate, OperationDelete},
		TenantOperations:  []Operation{OperationRead},
		Active:            true,
	}
}

func modelInstanceResource(instance *entity.TenantModelInstance, provider *entity.TenantModelProvider, ownerUserID string) Resource {
	return Resource{
		Ref:               ResourceRef{Kind: ResourceKindModelInstance, ID: instance.ID},
		TenantID:          provider.TenantID,
		OwnerUserID:       ownerUserID,
		Visibility:        VisibilityTenant,
		TenantRequirement: TenantMember,
		OwnerOperations:   []Operation{OperationRead, OperationCreate, OperationUpdate, OperationDelete, OperationUse},
		TenantOperations:  []Operation{OperationUse},
		Active:            true,
	}
}

func tenantModelResource(model *entity.TenantModel, provider *entity.TenantModelProvider, ownerUserID string) Resource {
	return Resource{
		Ref:               ResourceRef{Kind: ResourceKindModel, ID: model.ID},
		TenantID:          provider.TenantID,
		OwnerUserID:       ownerUserID,
		Visibility:        VisibilityTenant,
		TenantRequirement: TenantMember,
		OwnerOperations:   []Operation{OperationRead, OperationCreate, OperationUpdate, OperationDelete, OperationUse},
		TenantOperations:  []Operation{OperationRead, OperationUse},
		Active:            true,
	}
}
