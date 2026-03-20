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
	"ragflow/internal/model"
)

// UserTenantService user tenant service
// Provides business logic for user-tenant relationship management
type UserTenantService struct {
	userTenantDAO *dao.UserTenantDAO
}

// NewUserTenantService creates a new UserTenantService instance
//
// Returns:
//   - *UserTenantService: a new UserTenantService instance
//
// Example:
//
//	service := NewUserTenantService()
//	relations, err := service.GetUserTenantRelationByUserID("user123")
func NewUserTenantService() *UserTenantService {
	return &UserTenantService{
		userTenantDAO: dao.NewUserTenantDAO(),
	}
}

// UserTenantRelation represents a user-tenant relationship response
// This structure matches the Python implementation's return format
type UserTenantRelation struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
}

// GetUserTenantRelationByUserID retrieves all user-tenant relationships for a given user ID
//
// This method returns a list of user-tenant relationships with selected fields:
// - id: the relationship ID
// - user_id: the user ID
// - tenant_id: the tenant ID
// - role: the user's role in the tenant
//
// Parameters:
//   - userID: the unique identifier of the user
//
// Returns:
//   - []*UserTenantRelation: list of user-tenant relationships
//   - error: error if the operation fails, nil otherwise
//
// Example:
//
//	service := NewUserTenantService()
//	relations, err := service.GetUserTenantRelationByUserID("user123")
//	if err != nil {
//	    log.Printf("Failed to get user tenant relations: %v", err)
//	    return
//	}
//	for _, rel := range relations {
//	    fmt.Printf("User %s has role %s in tenant %s\n", rel.UserID, rel.Role, rel.TenantID)
//	}
func (s *UserTenantService) GetUserTenantRelationByUserID(userID string) ([]*UserTenantRelation, error) {
	// Get user tenant relationships from DAO
	relations, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return nil, err
	}

	// Convert model.UserTenant to UserTenantRelation format
	result := make([]*UserTenantRelation, len(relations))
	for i, rel := range relations {
		result[i] = convertToUserTenantRelation(rel)
	}

	return result, nil
}

// convertToUserTenantRelation converts model.UserTenant to UserTenantRelation
//
// Parameters:
//   - userTenant: the model.UserTenant to convert
//
// Returns:
//   - *UserTenantRelation: the converted UserTenantRelation
func convertToUserTenantRelation(userTenant *model.UserTenant) *UserTenantRelation {
	return &UserTenantRelation{
		ID:       userTenant.ID,
		UserID:   userTenant.UserID,
		TenantID: userTenant.TenantID,
		Role:     userTenant.Role,
	}
}
