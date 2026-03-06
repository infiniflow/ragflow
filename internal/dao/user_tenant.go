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
	"ragflow/internal/model"
)

// UserTenantDAO user tenant data access object
type UserTenantDAO struct{}

// NewUserTenantDAO create user tenant DAO
func NewUserTenantDAO() *UserTenantDAO {
	return &UserTenantDAO{}
}

// Create create user tenant relationship
func (dao *UserTenantDAO) Create(userTenant *model.UserTenant) error {
	return DB.Create(userTenant).Error
}

// GetByID get user tenant relationship by ID
func (dao *UserTenantDAO) GetByID(id string) (*model.UserTenant, error) {
	var userTenant model.UserTenant
	err := DB.Where("id = ? AND status = ?", id, "1").First(&userTenant).Error
	if err != nil {
		return nil, err
	}
	return &userTenant, nil
}

// Update update user tenant relationship
func (dao *UserTenantDAO) Update(userTenant *model.UserTenant) error {
	return DB.Save(userTenant).Error
}

// Delete delete user tenant relationship (soft delete by setting status to "0")
func (dao *UserTenantDAO) Delete(id string) error {
	return DB.Model(&model.UserTenant{}).Where("id = ?", id).Update("status", "0").Error
}

// GetByUserID get user tenant relationships by user ID
func (dao *UserTenantDAO) GetByUserID(userID string) ([]*model.UserTenant, error) {
	var relations []*model.UserTenant
	err := DB.Where("user_id = ? AND status = ?", userID, "1").Find(&relations).Error
	return relations, err
}

// GetByTenantID get user tenant relationships by tenant ID
func (dao *UserTenantDAO) GetByTenantID(tenantID string) ([]*model.UserTenant, error) {
	var relations []*model.UserTenant
	err := DB.Where("tenant_id = ? AND status = ?", tenantID, "1").Find(&relations).Error
	return relations, err
}

// GetTenantIDsByUserID get tenant ID list by user ID
func (dao *UserTenantDAO) GetTenantIDsByUserID(userID string) ([]string, error) {
	var tenantIDs []string
	err := DB.Model(&model.UserTenant{}).
		Select("tenant_id").
		Where("user_id = ? AND status = ?", userID, "1").
		Pluck("tenant_id", &tenantIDs).Error
	return tenantIDs, err
}

// FilterByUserIDAndTenantID filter user tenant relationship by user ID and tenant ID
func (dao *UserTenantDAO) FilterByUserIDAndTenantID(userID, tenantID string) (*model.UserTenant, error) {
	var userTenant model.UserTenant
	err := DB.Where("user_id = ? AND tenant_id = ? AND status = ?", userID, tenantID, "1").
		First(&userTenant).Error
	if err != nil {
		return nil, err
	}
	return &userTenant, nil
}

// GetByUserIDAndRole get user tenant relationships by user ID and role
func (dao *UserTenantDAO) GetByUserIDAndRole(userID, role string) ([]*model.UserTenant, error) {
	var relations []*model.UserTenant
	err := DB.Where("user_id = ? AND role = ? AND status = ?", userID, role, "1").Find(&relations).Error
	return relations, err
}

// GetNumMembers get number of members in a tenant (excluding owner)
func (dao *UserTenantDAO) GetNumMembers(tenantID string) (int64, error) {
	var count int64
	err := DB.Model(&model.UserTenant{}).
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
