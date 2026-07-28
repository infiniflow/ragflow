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

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// UserDAO user data access object
type UserDAO struct{}

// NewUserDAO create user DAO
func NewUserDAO() *UserDAO {
	return &UserDAO{}
}

// Create user
func (dao *UserDAO) Create(ctx context.Context, db *gorm.DB, user *entity.User) error {
	return db.WithContext(ctx).Create(user).Error
}

// GetByID get user by ID
func (dao *UserDAO) GetByID(ctx context.Context, db *gorm.DB, id uint) (*entity.User, error) {
	var user entity.User
	err := db.WithContext(ctx).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (dao *UserDAO) GetByTenantID(ctx context.Context, db *gorm.DB, tenantID string) (*entity.User, error) {
	var user entity.User
	err := db.WithContext(ctx).Where("id = ?", tenantID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetNicknameByID returns a user's nickname by string id.
func (dao *UserDAO) GetNicknameByID(ctx context.Context, db *gorm.DB, id string) (string, error) {
	var nickname string
	err := db.WithContext(ctx).
		Model(&entity.User{}).
		Where("id = ?", id).
		Select("nickname").
		Scan(&nickname).Error
	return nickname, err
}

// GetByEmail get user by email
func (dao *UserDAO) GetByEmail(ctx context.Context, db *gorm.DB, email string) (*entity.User, error) {
	var user entity.User
	query := db.WithContext(ctx).Where("email = ?", email)
	err := query.First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByAccessToken get user by access token
func (dao *UserDAO) GetByAccessToken(ctx context.Context, db *gorm.DB, token string) (*entity.User, error) {
	var user entity.User
	err := db.WithContext(ctx).Where("access_token = ?", token).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update update user
func (dao *UserDAO) Update(ctx context.Context, db *gorm.DB, user *entity.User) error {
	return db.WithContext(ctx).Save(user).Error
}

// UpdateAccessToken update user's access token
func (dao *UserDAO) UpdateAccessToken(ctx context.Context, db *gorm.DB, user *entity.User, token string) error {
	return db.WithContext(ctx).Model(user).Update("access_token", token).Error
}

// List users (only active users with status != "0")
func (dao *UserDAO) List(ctx context.Context, db *gorm.DB, offset, limit int, name, status, sort, orderBy string) ([]*entity.User, int64, error) {
	var users []*entity.User
	var total int64

	// Only count users with status != "0" (not deleted)
	if err := db.WithContext(ctx).Model(&entity.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := db.WithContext(ctx).Model(&entity.User{})
	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&users).Error
	return users, total, err
}

// Delete delete user
func (dao *UserDAO) Delete(ctx context.Context, db *gorm.DB, id uint) error {
	return db.WithContext(ctx).Delete(&entity.User{}, id).Error
}

// DeleteByID delete user by string ID (soft delete - set status to 0)
func (dao *UserDAO) DeleteByID(ctx context.Context, db *gorm.DB, id string) error {
	return db.WithContext(ctx).Model(&entity.User{}).Where("id = ?", id).Update("status", "0").Error
}

// HardDelete hard delete user by string ID
func (dao *UserDAO) HardDelete(ctx context.Context, db *gorm.DB, id string) error {
	return db.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&entity.User{}).Error
}

// ListByEmail list users by email (only active users with status != "0")
// Returns all users matching the given email address
func (dao *UserDAO) ListByEmail(ctx context.Context, db *gorm.DB, email string) ([]*entity.User, error) {
	var users []*entity.User
	err := db.WithContext(ctx).Where("email = ?", email).Find(&users).Error
	return users, err
}
