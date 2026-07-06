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

package dao

import (
	"context"
	"fmt"

	"ragflow/internal/entity"
)

// UserTenantDAO user tenant data access object
type UserTenantDAO struct{}

// NewUserTenantDAO create user tenant DAO
func NewUserTenantDAO() *UserTenantDAO {
	return &UserTenantDAO{}
}

// Create create user tenant relationship
func (dao *UserTenantDAO) Create(userTenant *entity.UserTenant) error {
	return DB.Create(userTenant).Error
}

// GetByID get user tenant relationship by ID
func (dao *UserTenantDAO) GetByID(id string) (*entity.UserTenant, error) {
	var userTenant entity.UserTenant
	err := DB.Where("id = ? AND status = ?", id, "1").First(&userTenant).Error
	if err != nil {
		return nil, err
	}
	return &userTenant, nil
}

// Update update user tenant relationship
func (dao *UserTenantDAO) Update(userTenant *entity.UserTenant) error {
	return DB.Save(userTenant).Error
}

// Delete delete user tenant relationship (soft delete by setting status to "0")
func (dao *UserTenantDAO) Delete(id string) error {
	return DB.Model(&entity.UserTenant{}).Where("id = ?", id).Update("status", "0").Error
}

// GetByUserID get user tenant relationships by user ID
func (dao *UserTenantDAO) GetByUserID(userID string) ([]*entity.UserTenant, error) {
	return dao.GetByUserIDWithContext(context.Background(), userID)
}

// GetByUserIDWithContext gets active user tenant relationships by user ID with context.
func (dao *UserTenantDAO) GetByUserIDWithContext(ctx context.Context, userID string) ([]*entity.UserTenant, error) {
	var relations []*entity.UserTenant
	err := DB.WithContext(ctx).Where("user_id = ? AND status = ?", userID, "1").Find(&relations).Error
	return relations, err
}

// GetByTenantID get user tenant relationships by tenant ID
func (dao *UserTenantDAO) GetByTenantID(tenantID string) ([]*entity.UserTenant, error) {
	var relations []*entity.UserTenant
	err := DB.Where("tenant_id = ? AND status = ?", tenantID, "1").Find(&relations).Error
	return relations, err
}

// GetTenantIDsByUserID get tenant ID list by user ID
func (dao *UserTenantDAO) GetTenantIDsByUserID(userID string) ([]string, error) {
	var tenantIDs []string
	err := DB.Model(&entity.UserTenant{}).
		Select("tenant_id").
		Where("user_id = ? AND status = ?", userID, "1").
		Pluck("tenant_id", &tenantIDs).Error
	return tenantIDs, err
}

// FilterByUserIDAndTenantID filter user tenant relationship by user ID and tenant ID
func (dao *UserTenantDAO) FilterByUserIDAndTenantID(userID, tenantID string) (*entity.UserTenant, error) {
	var userTenant entity.UserTenant
	err := DB.Where("user_id = ? AND tenant_id = ? AND status = ?", userID, tenantID, "1").
		First(&userTenant).Error
	if err != nil {
		return nil, err
	}
	return &userTenant, nil
}

// GetByUserIDAndRole get user tenant relationships by user ID and role
func (dao *UserTenantDAO) GetByUserIDAndRole(userID, role string) ([]*entity.UserTenant, error) {
	var relations []*entity.UserTenant
	err := DB.Where("user_id = ? AND role = ? AND status = ?", userID, role, "1").Find(&relations).Error
	return relations, err
}

// GetNumMembers get number of members in a tenant (excluding owner)
func (dao *UserTenantDAO) GetNumMembers(tenantID string) (int64, error) {
	var count int64
	err := DB.Model(&entity.UserTenant{}).
		Where("tenant_id = ? AND status = ? AND role != ?", tenantID, "1", "owner").
		Count(&count).Error
	return count, err
}

// TenantInfoByUserID tenant info with user details
type TenantInfoByUserID struct {
	TenantID   string `json:"tenant_id"`
	Role       string `json:"role"`
	Nickname   string `json:"nickname"`
	Email      string `json:"email"`
	Avatar     string `json:"avatar"`
	UpdateDate string `json:"update_date"`
}

// TenantMemberItem holds user details for a tenant member listing.
type TenantMemberItem struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	Role            string `json:"role"`
	Status          string `json:"status"`
	Nickname        string `json:"nickname"`
	Email           string `json:"email"`
	Avatar          string `json:"avatar"`
	IsAuthenticated bool   `json:"is_authenticated"`
	IsActive        string `json:"is_active"`
	IsAnonymous     bool   `json:"is_anonymous"`
	IsSuperuser     bool   `json:"is_superuser"`
	UpdateDate      string `json:"update_date"`
}

// GetMembersByTenantID returns all non-owner members of a tenant with user details.
// update_date is formatted as "2006-01-02T15:04:05" (no timezone) to match the Python API.
func (dao *UserTenantDAO) GetMembersByTenantID(tenantID string) ([]*TenantMemberItem, error) {
	var results []*TenantMemberItem
	err := DB.Table("user_tenant").
		Select("user_tenant.id, user_tenant.user_id, user_tenant.role, user_tenant.status, "+
			"user.nickname, user.email, user.avatar, user.is_authenticated, "+
			"user.status AS is_active, user.is_anonymous, user.is_superuser, "+
			"DATE_FORMAT(user.update_date, '%Y-%m-%dT%H:%i:%s') AS update_date").
		Joins("JOIN user ON user_tenant.user_id = user.id").
		Where("user_tenant.tenant_id = ? AND user_tenant.status = ? AND user_tenant.role != ?",
			tenantID, "1", "owner").
		Scan(&results).Error
	return results, err
}

// GetTenantsByUserID get tenants by user ID with user details
func (dao *UserTenantDAO) GetTenantsByUserID(userID string) ([]*TenantInfoByUserID, error) {
	var results []*TenantInfoByUserID
	err := DB.Table("user_tenant").
		Select("user_tenant.tenant_id, user_tenant.role, user.nickname, user.email, user.avatar, user.update_date").
		Joins("JOIN user ON user_tenant.tenant_id = user.id AND user_tenant.user_id = ? AND user_tenant.status = ?", userID, "1").
		Where("user_tenant.status = ?", "1").
		Scan(&results).Error
	return results, err
}

// DeleteByUserID delete user tenant relationships by user ID (hard delete)
func (dao *UserTenantDAO) DeleteByUserID(userID string) (int64, error) {
	result := DB.Unscoped().Where("user_id = ?", userID).Delete(&entity.UserTenant{})
	return result.RowsAffected, result.Error
}

// DeleteByTenantID delete user tenant relationships by tenant ID (hard delete)
func (dao *UserTenantDAO) DeleteByTenantID(tenantID string) (int64, error) {
	result := DB.Unscoped().Where("tenant_id = ?", tenantID).Delete(&entity.UserTenant{})
	return result.RowsAffected, result.Error
}

// GetByUserIDAll get all user tenant relationships by user ID (including deleted)
func (dao *UserTenantDAO) GetByUserIDAll(userID string) ([]*entity.UserTenant, error) {
	var relations []*entity.UserTenant
	err := DB.Where("user_id = ?", userID).Find(&relations).Error
	return relations, err
}

// DeleteByUserAndTenant hard-deletes the join record for a specific user+tenant pair.
func (dao *UserTenantDAO) DeleteByUserAndTenant(userID, tenantID string) error {
	return DB.Unscoped().
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Delete(&entity.UserTenant{}).Error
}

// UpdateRoleByUserAndTenant updates the role for a specific user+tenant pair.
// Returns an error if no matching row was found.
func (dao *UserTenantDAO) UpdateRoleByUserAndTenant(userID, tenantID, role string) error {
	result := DB.Model(&entity.UserTenant{}).
		Where("user_id = ? AND tenant_id = ? AND status = ?", userID, tenantID, "1").
		Update("role", role)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("no active membership found for user %s in tenant %s", userID, tenantID)
	}
	return nil
}
