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

// UserDAO user data access object
type UserDAO struct{}

// NewUserDAO create user DAO
func NewUserDAO() *UserDAO {
	return &UserDAO{}
}

// Create create user
func (dao *UserDAO) Create(user *model.User) error {
	return DB.Create(user).Error
}

// GetByID get user by ID
func (dao *UserDAO) GetByID(id uint) (*model.User, error) {
	var user model.User
	err := DB.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByUsername get user by username
func (dao *UserDAO) GetByUsername(username string) (*model.User, error) {
	var user model.User
	err := DB.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByEmail get user by email
func (dao *UserDAO) GetByEmail(email string) (*model.User, error) {
	var user model.User
	query := DB.Where("email = ?", email)
	err := query.First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByAccessToken get user by access token
func (dao *UserDAO) GetByAccessToken(token string) (*model.User, error) {
	var user model.User
	err := DB.Where("access_token = ?", token).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update update user
func (dao *UserDAO) Update(user *model.User) error {
	return DB.Save(user).Error
}

// UpdateAccessToken update user's access token
func (dao *UserDAO) UpdateAccessToken(user *model.User, token string) error {
	return DB.Model(user).Update("access_token", token).Error
}

// List list users
func (dao *UserDAO) List(offset, limit int) ([]*model.User, int64, error) {
	var users []*model.User
	var total int64

	if err := DB.Model(&model.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := DB.Offset(offset).Limit(limit).Find(&users).Error
	return users, total, err
}

// Delete delete user
func (dao *UserDAO) Delete(id uint) error {
	return DB.Delete(&model.User{}, id).Error
}
