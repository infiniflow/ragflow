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

package admin

import (
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/model"
)

// Service admin service layer
type Service struct {
	userDAO *dao.UserDAO
}

// NewService create admin service
func NewService() *Service {
	return &Service{
		userDAO: dao.NewUserDAO(),
	}
}

// LoginRequest login request
type LoginRequest struct {
	Email    string
	Password string
}

// LoginResponse login response
type LoginResponse struct {
	Token    string
	UserID   string
	Email    string
	Nickname string
}

// Login admin login
func (s *Service) Login(req *LoginRequest) (*LoginResponse, error) {
	// Get user by email
	user, err := s.userDAO.GetByEmail(req.Email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	// Verify password
	if user.Password == nil || *user.Password != req.Password {
		return nil, ErrInvalidCredentials
	}

	// Generate access token
	token := generateToken()
	if err := s.userDAO.UpdateAccessToken(user, token); err != nil {
		return nil, err
	}

	return &LoginResponse{
		Token:    token,
		UserID:   user.ID,
		Email:    user.Email,
		Nickname: user.Nickname,
	}, nil
}

// ListUsersRequest list users request
type ListUsersRequest struct {
	Offset int
	Limit  int
}

// ListUsersResponse list users response
type ListUsersResponse struct {
	Users []*UserInfo
	Total int64
}

// UserInfo user info
type UserInfo struct {
	ID         string
	Email      string
	Nickname   string
	IsActive   string
	CreateTime *int64
	UpdateTime *int64
}

// ListUsers list all users
func (s *Service) ListUsers(req *ListUsersRequest) (*ListUsersResponse, error) {
	users, total, err := s.userDAO.List(req.Offset, req.Limit)
	if err != nil {
		return nil, err
	}

	var result []*UserInfo
	for _, user := range users {
		result = append(result, &UserInfo{
			ID:         user.ID,
			Email:      user.Email,
			Nickname:   user.Nickname,
			IsActive:   user.IsActive,
			CreateTime: user.CreateTime,
			UpdateTime: user.UpdateTime,
		})
	}

	return &ListUsersResponse{
		Users: result,
		Total: total,
	}, nil
}

// GetUserRequest get user request
type GetUserRequest struct {
	ID string
}

// GetUser get user by ID
func (s *Service) GetUser(req *GetUserRequest) (*UserInfo, error) {
	var user model.User
	err := dao.DB.Where("id = ?", req.ID).First(&user).Error
	if err != nil {
		return nil, ErrUserNotFound
	}

	return &UserInfo{
		ID:         user.ID,
		Email:      user.Email,
		Nickname:   user.Nickname,
		IsActive:   user.IsActive,
		CreateTime: user.CreateTime,
		UpdateTime: user.UpdateTime,
	}, nil
}

// UpdateUserRequest update user request
type UpdateUserRequest struct {
	ID       string
	Nickname string
	IsActive *string
}

// UpdateUser update user
func (s *Service) UpdateUser(req *UpdateUserRequest) error {
	var user model.User
	if err := dao.DB.Where("id = ?", req.ID).First(&user).Error; err != nil {
		return ErrUserNotFound
	}

	if req.Nickname != "" {
		user.Nickname = req.Nickname
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}

	return dao.DB.Save(&user).Error
}

// DeleteUserRequest delete user request
type DeleteUserRequest struct {
	ID string
}

// DeleteUser delete user
func (s *Service) DeleteUser(req *DeleteUserRequest) error {
	return dao.DB.Where("id = ?", req.ID).Delete(&model.User{}).Error
}

// GetSystemConfig get system config
func (s *Service) GetSystemConfig() map[string]interface{} {
	// TODO: Load from database or config file
	return map[string]interface{}{
		"system_name": "RAGFlow Admin",
		"version":     "1.0.0",
	}
}

// UpdateSystemConfig update system config
func (s *Service) UpdateSystemConfig(config map[string]interface{}) error {
	// TODO: Save to database or config file
	return nil
}

// GetSystemStatus get system status
func (s *Service) GetSystemStatus() map[string]interface{} {
	// TODO: Get real status from services
	return map[string]interface{}{
		"status":    "running",
		"uptime":    time.Since(time.Now()).String(),
		"db_status": "connected",
	}
}

// ValidateToken validate access token
func (s *Service) ValidateToken(token string) (*model.User, error) {
	user, err := s.userDAO.GetByAccessToken(token)
	if err != nil {
		return nil, ErrInvalidToken
	}
	return user, nil
}

// generateToken generate a simple token
func generateToken() string {
	return time.Now().Format("20060102150405") + randomString(16)
}

// randomString generate random string
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}
