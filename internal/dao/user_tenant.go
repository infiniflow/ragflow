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
	"errors"
	"fmt"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// UserTenantDAO user tenant data access object
type UserTenantDAO struct{}

// NewUserTenantDAO create user tenant DAO
func NewUserTenantDAO() *UserTenantDAO {
	return &UserTenantDAO{}
}

// Create user tenant relationship
func (dao *UserTenantDAO) Create(ctx context.Context, db *gorm.DB, userTenant *entity.UserTenant) error {
	return db.WithContext(ctx).Create(userTenant).Error
}

// GetByID get user tenant relationship by ID
func (dao *UserTenantDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.UserTenant, error) {
	var userTenant entity.UserTenant
	err := db.WithContext(ctx).Where("id = ? AND status = ?", id, "1").First(&userTenant).Error
	if err != nil {
		return nil, err
	}
	return &userTenant, nil
}

// Update user tenant relationship
func (dao *UserTenantDAO) Update(ctx context.Context, db *gorm.DB, userTenant *entity.UserTenant) error {
	return db.WithContext(ctx).Save(userTenant).Error
}

// GetByUserID gets active user tenant relationships by user ID with context.
func (dao *UserTenantDAO) GetByUserID(ctx context.Context, db *gorm.DB, userID string) ([]*entity.UserTenant, error) {
	var relations []*entity.UserTenant
	err := db.WithContext(ctx).Where("user_id = ? AND status = ?", userID, "1").Find(&relations).Error
	return relations, err
}

// GetByTenantID get user tenant relationships by tenant ID
func (dao *UserTenantDAO) GetByTenantID(ctx context.Context, db *gorm.DB, tenantID string) ([]*entity.UserTenant, error) {
	var relations []*entity.UserTenant
	err := db.WithContext(ctx).Where("tenant_id = ? AND status = ?", tenantID, "1").Find(&relations).Error
	return relations, err
}

// GetTenantIDsByUserID get tenant ID list by user ID
func (dao *UserTenantDAO) GetTenantIDsByUserID(ctx context.Context, db *gorm.DB, userID string) ([]string, error) {
	var tenantIDs []string
	err := db.WithContext(ctx).Model(&entity.UserTenant{}).
		Select("tenant_id").
		Where("user_id = ? AND status = ?", userID, "1").
		Pluck("tenant_id", &tenantIDs).Error
	return tenantIDs, err
}

// FilterByUserIDAndTenantID filter user tenant relationship by user ID and tenant ID
func (dao *UserTenantDAO) FilterByUserIDAndTenantID(ctx context.Context, db *gorm.DB, userID, tenantID string) (*entity.UserTenant, error) {
	var userTenant entity.UserTenant
	err := db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND status = ?", userID, tenantID, "1").
		First(&userTenant).Error
	if err != nil {
		return nil, err
	}
	return &userTenant, nil
}

// GetOwnerByUserID returns the user's owner membership, falling back to the
// personal tenant row identified by user_id == tenant_id.
func (dao *UserTenantDAO) GetOwnerByUserID(ctx context.Context, db *gorm.DB, userID string) (*entity.UserTenant, error) {
	var membership entity.UserTenant
	err := db.WithContext(ctx).
		Where("user_id = ? AND role = ? AND status = ?", userID, "owner", entity.TenantMembershipStatusJoined).
		Take(&membership).Error
	if err == nil {
		return &membership, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	err = db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ? AND status = ?", userID, userID, entity.TenantMembershipStatusJoined).
		Take(&membership).Error
	if err != nil {
		return nil, err
	}
	return &membership, nil
}

// GetOwnerByTenantID returns the joined owner membership for tenantID, falling
// back to the personal tenant row identified by user_id == tenant_id.
func (dao *UserTenantDAO) GetOwnerByTenantID(ctx context.Context, db *gorm.DB, tenantID string) (*entity.UserTenant, error) {
	var membership entity.UserTenant
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND role = ? AND status = ?", tenantID, "owner", entity.TenantMembershipStatusJoined).
		Take(&membership).Error
	if err == nil {
		return &membership, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	err = db.WithContext(ctx).
		Where("tenant_id = ? AND user_id = ? AND status = ?", tenantID, tenantID, entity.TenantMembershipStatusJoined).
		Take(&membership).Error
	if err != nil {
		return nil, err
	}
	return &membership, nil
}

// GetByUserIDAndRole get user tenant relationships by user ID and role
func (dao *UserTenantDAO) GetByUserIDAndRole(ctx context.Context, db *gorm.DB, userID, role string) ([]*entity.UserTenant, error) {
	var relations []*entity.UserTenant
	err := db.WithContext(ctx).Where("user_id = ? AND role = ? AND status = ?", userID, role, "1").Find(&relations).Error
	return relations, err
}

// GetNumMembers get number of members in a tenant (excluding owner)
func (dao *UserTenantDAO) GetNumMembers(ctx context.Context, db *gorm.DB, tenantID string) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Model(&entity.UserTenant{}).
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
func (dao *UserTenantDAO) GetMembersByTenantID(ctx context.Context, db *gorm.DB, tenantID string) ([]*TenantMemberItem, error) {
	var results []*TenantMemberItem
	err := db.WithContext(ctx).Table("user_tenant").
		Select("user_tenant.id, user_tenant.user_id, user_tenant.role, user_tenant.status, "+
			"user.nickname, user.email, user.avatar, user.is_authenticated, "+
			"user.status AS is_active, user.is_anonymous, user.is_superuser, "+
			"DATE_FORMAT(user.update_date, '%Y-%m-%dT%H:%i:%s') AS update_date").
		Joins("JOIN user ON user_tenant.user_id = user.id").
		Where("user_tenant.tenant_id = ? AND user_tenant.status IN ? AND user_tenant.role != ?",
			tenantID, []string{
				entity.TenantMembershipStatusJoined,
				entity.TenantMembershipStatusInvited,
				entity.TenantMembershipStatusRequested,
			}, "owner").
		Scan(&results).Error
	return results, err
}

// GetTenantsByUserID get tenants by user ID with user details
func (dao *UserTenantDAO) GetTenantsByUserID(ctx context.Context, db *gorm.DB, userID string) ([]*TenantInfoByUserID, error) {
	var results []*TenantInfoByUserID
	err := db.WithContext(ctx).Table("user_tenant").
		Select("user_tenant.tenant_id, user_tenant.role, user.nickname, user.email, user.avatar, user.update_date").
		Joins("JOIN user ON user_tenant.tenant_id = user.id AND user_tenant.user_id = ? AND user_tenant.status = ?", userID, "1").
		Where("user_tenant.status = ?", "1").
		Scan(&results).Error
	return results, err
}

// DeleteByUserID delete user tenant relationships by user ID (hard delete)
func (dao *UserTenantDAO) DeleteByUserID(ctx context.Context, db *gorm.DB, userID string) (int64, error) {
	result := db.WithContext(ctx).Unscoped().Where("user_id = ?", userID).Delete(&entity.UserTenant{})
	return result.RowsAffected, result.Error
}

// GetByUserIDAll get all user tenant relationships by user ID (including deleted)
func (dao *UserTenantDAO) GetByUserIDAll(ctx context.Context, db *gorm.DB, userID string) ([]*entity.UserTenant, error) {
	var relations []*entity.UserTenant
	err := db.WithContext(ctx).Where("user_id = ?", userID).Find(&relations).Error
	return relations, err
}

// DeleteByUserAndTenant hard-deletes the join record for a specific user+tenant pair.
func (dao *UserTenantDAO) DeleteByUserAndTenant(ctx context.Context, db *gorm.DB, userID, tenantID string) error {
	return db.WithContext(ctx).Unscoped().
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Delete(&entity.UserTenant{}).Error
}

// DeleteByUserAndTenantAndRole deletes an active membership only when its role
// matches, and reports whether a row was removed.
func (dao *UserTenantDAO) DeleteByUserAndTenantAndRole(ctx context.Context, db *gorm.DB, userID, tenantID, role string) (bool, error) {
	result := db.WithContext(ctx).Unscoped().
		Where("user_id = ? AND tenant_id = ? AND role = ? AND status = ?", userID, tenantID, role, "1").
		Delete(&entity.UserTenant{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// UpdateRoleByUserAndTenant updates the role for a specific user+tenant pair.
// Returns an error if no matching row was found.
func (dao *UserTenantDAO) UpdateRoleByUserAndTenant(ctx context.Context, db *gorm.DB, userID, tenantID, role string) error {
	result := db.WithContext(ctx).Model(&entity.UserTenant{}).
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
