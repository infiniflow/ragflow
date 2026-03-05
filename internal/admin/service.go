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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/model"
	"ragflow/internal/utility"
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

// Logout user logout
func (s *Service) Logout(user interface{}) error {
	// Invalidate token by setting it to INVALID_ prefix
	if u, ok := user.(*model.User); ok {
		invalidToken := "INVALID_" + generateRandomHex(16)
		return s.userDAO.UpdateAccessToken(u, invalidToken)
	}
	return nil
}

// ValidateToken validate access token
func (s *Service) ValidateToken(token string) (*model.User, error) {
	// Check if token starts with INVALID_
	if len(token) > 8 && token[:8] == "INVALID_" {
		return nil, ErrInvalidToken
	}

	user, err := s.userDAO.GetByAccessToken(token)
	if err != nil {
		return nil, ErrInvalidToken
	}
	return user, nil
}

// ListUsers list all users
func (s *Service) ListUsers() ([]map[string]interface{}, error) {
	// For now, return empty list - needs implementation with proper DAO methods
	return []map[string]interface{}{}, nil
}

// CreateUser create a new user
func (s *Service) CreateUser(username, password, role string) (map[string]interface{}, error) {
	// TODO: Implement user creation with proper password hashing
	return map[string]interface{}{
		"username": username,
		"role":     role,
	}, nil
}

// GetUserDetails get user details
func (s *Service) GetUserDetails(username string) (map[string]interface{}, error) {
	// Query user by email/username
	var user model.User
	err := dao.DB.Where("email = ?", username).First(&user).Error
	if err != nil {
		return nil, ErrUserNotFound
	}

	return map[string]interface{}{
		"id":         user.ID,
		"email":      user.Email,
		"nickname":   user.Nickname,
		"is_active":  user.IsActive,
		"create_time": user.CreateTime,
		"update_time": user.UpdateTime,
	}, nil
}

// DeleteUser delete user
func (s *Service) DeleteUser(username string) error {
	return dao.DB.Where("email = ?", username).Delete(&model.User{}).Error
}

// UpdateUserPassword update user password
func (s *Service) UpdateUserPassword(username, newPassword string) error {
	// TODO: Implement password hashing
	return dao.DB.Model(&model.User{}).Where("email = ?", username).Update("password", newPassword).Error
}

// UpdateUserActivateStatus update user activate status
func (s *Service) UpdateUserActivateStatus(username, activateStatus string) error {
	return dao.DB.Model(&model.User{}).Where("email = ?", username).Update("is_active", activateStatus).Error
}

// GrantAdmin grant admin role
func (s *Service) GrantAdmin(username string) error {
	isSuperuser := true
	return dao.DB.Model(&model.User{}).Where("email = ?", username).Update("is_superuser", &isSuperuser).Error
}

// RevokeAdmin revoke admin role
func (s *Service) RevokeAdmin(username string) error {
	isSuperuser := false
	return dao.DB.Model(&model.User{}).Where("email = ?", username).Update("is_superuser", &isSuperuser).Error
}

// GetUserDatasets get user datasets
func (s *Service) GetUserDatasets(username string) ([]map[string]interface{}, error) {
	// TODO: Implement with proper DAO
	return []map[string]interface{}{}, nil
}

// GetUserAgents get user agents
func (s *Service) GetUserAgents(username string) ([]map[string]interface{}, error) {
	// TODO: Implement with proper DAO
	return []map[string]interface{}{}, nil
}

// GetUserAPIKeys get user API keys
func (s *Service) GetUserAPIKeys(username string) ([]map[string]interface{}, error) {
	// TODO: Implement with proper DAO
	return []map[string]interface{}{}, nil
}

// GenerateUserAPIKey generate user API key
func (s *Service) GenerateUserAPIKey(username string) (map[string]interface{}, error) {
	// Get user details
	userDetails, err := s.GetUserDetails(username)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// TODO: Get tenant info
	_ = userDetails

	key := generateConfirmationToken()
	beta := generateRandomString(32)
	now := time.Now()

	obj := map[string]interface{}{
		"tenant_id":   "", // TODO: Get from tenant
		"token":       key,
		"beta":        beta,
		"create_time": now.Unix(),
		"create_date": now.Format("2006-01-02 15:04:05"),
		"update_time": nil,
		"update_date": nil,
	}

	// TODO: Save API key to database
	_ = obj

	return obj, nil
}

// DeleteUserAPIKey delete user API key
func (s *Service) DeleteUserAPIKey(username, key string) error {
	// TODO: Implement with proper DAO
	_ = username
	_ = key
	return nil
}

// Role related methods

// ListRoles list all roles
func (s *Service) ListRoles() ([]map[string]interface{}, error) {
	// TODO: Implement with proper DAO
	return []map[string]interface{}{}, nil
}

// CreateRole create role
func (s *Service) CreateRole(roleName, description string) (map[string]interface{}, error) {
	// TODO: Implement with proper DAO
	return map[string]interface{}{
		"role_name":   roleName,
		"description": description,
	}, nil
}

// GetRole get role
func (s *Service) GetRole(roleName string) (map[string]interface{}, error) {
	// TODO: Implement with proper DAO
	return map[string]interface{}{
		"role_name": roleName,
	}, nil
}

// UpdateRole update role
func (s *Service) UpdateRole(roleName, description string) (map[string]interface{}, error) {
	// TODO: Implement with proper DAO
	return map[string]interface{}{
		"role_name":   roleName,
		"description": description,
	}, nil
}

// DeleteRole delete role
func (s *Service) DeleteRole(roleName string) error {
	// TODO: Implement with proper DAO
	_ = roleName
	return nil
}

// GetRolePermission get role permission
func (s *Service) GetRolePermission(roleName string) (map[string]interface{}, error) {
	// TODO: Implement with proper DAO
	return map[string]interface{}{
		"role_name":   roleName,
		"permissions": []map[string]interface{}{},
	}, nil
}

// GrantRolePermission grant role permission
func (s *Service) GrantRolePermission(roleName string, actions []string, resource string) (map[string]interface{}, error) {
	// TODO: Implement with proper DAO
	return map[string]interface{}{
		"role_name": roleName,
		"actions":   actions,
		"resource":  resource,
	}, nil
}

// RevokeRolePermission revoke role permission
func (s *Service) RevokeRolePermission(roleName string, actions []string, resource string) (map[string]interface{}, error) {
	// TODO: Implement with proper DAO
	return map[string]interface{}{
		"role_name": roleName,
		"actions":   actions,
		"resource":  resource,
	}, nil
}

// UpdateUserRole update user role
func (s *Service) UpdateUserRole(username, roleName string) (map[string]interface{}, error) {
	// TODO: Implement with proper DAO
	return map[string]interface{}{
		"username":  username,
		"role_name": roleName,
	}, nil
}

// GetUserPermission get user permission
func (s *Service) GetUserPermission(username string) (map[string]interface{}, error) {
	// TODO: Implement with proper DAO
	return map[string]interface{}{
		"username":    username,
		"permissions": []map[string]interface{}{},
	}, nil
}

// Service management methods

// GetAllServices get all services
func (s *Service) GetAllServices() ([]map[string]interface{}, error) {
	// TODO: Implement with proper service manager
	return []map[string]interface{}{}, nil
}

// GetServicesByType get services by type
func (s *Service) GetServicesByType(serviceType string) ([]map[string]interface{}, error) {
	// TODO: Implement with proper service manager
	_ = serviceType
	return []map[string]interface{}{}, nil
}

// GetServiceDetails get service details
func (s *Service) GetServiceDetails(serviceID string) (map[string]interface{}, error) {
	// TODO: Implement with proper service manager
	return map[string]interface{}{
		"service_id": serviceID,
	}, nil
}

// ShutdownService shutdown service
func (s *Service) ShutdownService(serviceID string) (map[string]interface{}, error) {
	// TODO: Implement with proper service manager
	return map[string]interface{}{
		"service_id": serviceID,
		"status":     "shutdown",
	}, nil
}

// RestartService restart service
func (s *Service) RestartService(serviceID string) (map[string]interface{}, error) {
	// TODO: Implement with proper service manager
	return map[string]interface{}{
		"service_id": serviceID,
		"status":     "restarted",
	}, nil
}

// Variable/Settings methods

// GetVariable get variable
func (s *Service) GetVariable(varName string) (map[string]interface{}, error) {
	// TODO: Implement with settings manager
	return map[string]interface{}{
		"var_name":  varName,
		"var_value": "",
	}, nil
}

// GetAllVariables get all variables
func (s *Service) GetAllVariables() ([]map[string]interface{}, error) {
	// TODO: Implement with settings manager
	return []map[string]interface{}{}, nil
}

// SetVariable set variable
func (s *Service) SetVariable(varName, varValue string) error {
	// TODO: Implement with settings manager
	_ = varName
	_ = varValue
	return nil
}

// Config methods

// GetAllConfigs get all configs
func (s *Service) GetAllConfigs() ([]map[string]interface{}, error) {
	// TODO: Implement with config manager
	return []map[string]interface{}{}, nil
}

// Environment methods

// GetAllEnvironments get all environments
func (s *Service) GetAllEnvironments() ([]map[string]interface{}, error) {
	// TODO: Implement with environment manager
	return []map[string]interface{}{}, nil
}

// Version methods

// GetVersion get RAGFlow version
func (s *Service) GetVersion() string {
	return utility.GetRAGFlowVersion()
}

// Sandbox methods

// ListSandboxProviders list sandbox providers
func (s *Service) ListSandboxProviders() ([]map[string]interface{}, error) {
	// TODO: Implement with sandbox manager
	return []map[string]interface{}{}, nil
}

// GetSandboxProviderSchema get sandbox provider schema
func (s *Service) GetSandboxProviderSchema(providerID string) (map[string]interface{}, error) {
	// TODO: Implement with sandbox manager
	return map[string]interface{}{
		"provider_id": providerID,
		"schema":      map[string]interface{}{},
	}, nil
}

// GetSandboxConfig get sandbox config
func (s *Service) GetSandboxConfig() (map[string]interface{}, error) {
	// TODO: Implement with sandbox manager
	return map[string]interface{}{}, nil
}

// SetSandboxConfig set sandbox config
func (s *Service) SetSandboxConfig(providerType string, config map[string]interface{}, setActive bool) (map[string]interface{}, error) {
	// TODO: Implement with sandbox manager
	return map[string]interface{}{
		"provider_type": providerType,
		"config":        config,
		"set_active":    setActive,
	}, nil
}

// TestSandboxConnection test sandbox connection
func (s *Service) TestSandboxConnection(providerType string, config map[string]interface{}) (map[string]interface{}, error) {
	// TODO: Implement with sandbox manager
	return map[string]interface{}{
		"provider_type": providerType,
		"status":        "ok",
	}, nil
}

// Helper functions

// generateToken generate a simple token
func generateToken() string {
	return fmt.Sprintf("ragflow-%d-%s", time.Now().Unix(), generateRandomString(16))
}

// generateConfirmationToken generate confirmation token
func generateConfirmationToken() string {
	return "ragflow-" + generateRandomString(32)
}

// generateRandomString generate random string
func generateRandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		randByte := make([]byte, 1)
		rand.Read(randByte)
		b[i] = letters[int(randByte[0])%len(letters)]
	}
	return string(b)
}

// generateRandomHex generate random hex string
func generateRandomHex(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
