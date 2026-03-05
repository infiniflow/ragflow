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
	"ragflow/internal/cache"
	"ragflow/internal/dao"
	"ragflow/internal/model"
	"ragflow/internal/server"
	"ragflow/internal/utility"
	"time"
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

	// Check if user is active
	if user.IsActive != "1" {
		return nil, errors.New("user is not active")
	}

	// Generate access token
	token := utility.GenerateToken()
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

// generateRandomHex generate random hex string
func generateRandomHex(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// User management methods

// ListUsers list all users
func (s *Service) ListUsers() ([]map[string]interface{}, error) {
	users, _, err := s.userDAO.List(0, 0)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, 0, len(users))
	for _, user := range users {
		result = append(result, map[string]interface{}{
			"email":        user.Email,
			"nickname":     user.Nickname,
			"create_date":  user.CreateTime,
			"is_active":    user.IsActive,
			"is_superuser": user.IsSuperuser,
		})
	}
	return result, nil
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
		"id":          user.ID,
		"email":       user.Email,
		"nickname":    user.Nickname,
		"is_active":   user.IsActive,
		"create_time": user.CreateTime,
		"update_time": user.UpdateTime,
	}, nil
}

// DeleteUser delete user
func (s *Service) DeleteUser(username string) error {
	// TODO: Implement user deletion
	return nil
}

// ChangePassword change user password
func (s *Service) ChangePassword(username, newPassword string) error {
	// TODO: Implement password change
	return nil
}

// UpdateUserActivateStatus update user activate status
func (s *Service) UpdateUserActivateStatus(username string, isActive bool) error {
	// TODO: Implement activate status update
	return nil
}

// GrantAdmin grant admin privileges
func (s *Service) GrantAdmin(username string) error {
	// TODO: Implement grant admin
	return nil
}

// RevokeAdmin revoke admin privileges
func (s *Service) RevokeAdmin(username string) error {
	// TODO: Implement revoke admin
	return nil
}

// GetUserDatasets get user datasets
func (s *Service) GetUserDatasets(username string) ([]map[string]interface{}, error) {
	// TODO: Implement get user datasets
	return []map[string]interface{}{}, nil
}

// GetUserAgents get user agents
func (s *Service) GetUserAgents(username string) ([]map[string]interface{}, error) {
	// TODO: Implement get user agents
	return []map[string]interface{}{}, nil
}

// API Key methods

// GetUserAPIKeys get user API keys
func (s *Service) GetUserAPIKeys(username string) ([]map[string]interface{}, error) {
	// TODO: Implement get API keys
	return []map[string]interface{}{}, nil
}

// GenerateUserAPIKey generate API key for user
func (s *Service) GenerateUserAPIKey(username string) (map[string]interface{}, error) {
	// TODO: Implement generate API key
	return map[string]interface{}{}, nil
}

// DeleteUserAPIKey delete user API key
func (s *Service) DeleteUserAPIKey(username, key string) error {
	// TODO: Implement delete API key
	return nil
}

// Role management methods

// ListRoles list all roles
func (s *Service) ListRoles() ([]map[string]interface{}, error) {
	// TODO: Implement list roles
	return []map[string]interface{}{}, nil
}

// CreateRole create a new role
func (s *Service) CreateRole(roleName, description string) (map[string]interface{}, error) {
	// TODO: Implement create role
	return map[string]interface{}{}, nil
}

// GetRole get role details
func (s *Service) GetRole(roleName string) (map[string]interface{}, error) {
	// TODO: Implement get role
	return map[string]interface{}{}, nil
}

// UpdateRole update role
func (s *Service) UpdateRole(roleName, description string) (map[string]interface{}, error) {
	// TODO: Implement update role
	return map[string]interface{}{}, nil
}

// DeleteRole delete role
func (s *Service) DeleteRole(roleName string) error {
	// TODO: Implement delete role
	return nil
}

// GetRolePermission get role permissions
func (s *Service) GetRolePermission(roleName string) ([]map[string]interface{}, error) {
	// TODO: Implement get role permissions
	return []map[string]interface{}{}, nil
}

// GrantRolePermission grant permission to role
func (s *Service) GrantRolePermission(roleName string, actions []string, resource string) (map[string]interface{}, error) {
	// TODO: Implement grant role permission
	return map[string]interface{}{}, nil
}

// RevokeRolePermission revoke permission from role
func (s *Service) RevokeRolePermission(roleName string, actions []string, resource string) (map[string]interface{}, error) {
	// TODO: Implement revoke role permission
	return map[string]interface{}{}, nil
}

// UpdateUserRole update user role
func (s *Service) UpdateUserRole(username, roleName string) ([]map[string]interface{}, error) {
	// TODO: Implement update user role
	return []map[string]interface{}{}, nil
}

// GetUserPermission get user permissions
func (s *Service) GetUserPermission(username string) ([]map[string]interface{}, error) {
	// TODO: Implement get user permissions
	return []map[string]interface{}{}, nil
}

// GetAllServices get all services
func (s *Service) GetAllServices() ([]map[string]interface{}, error) {
	allConfigs := server.GetAllConfigs()

	var result []map[string]interface{}
	for _, configDict := range allConfigs {
		// Get service details to check status
		serviceDetail, err := s.GetServiceDetails(configDict)
		if err == nil {
			if status, ok := serviceDetail["status"]; ok {
				configDict["status"] = status
			} else {
				configDict["status"] = "timeout"
			}
		} else {
			configDict["status"] = "timeout"
		}
		result = append(result, configDict)
	}

	return result, nil
}

// GetServicesByType get services by type
func (s *Service) GetServicesByType(serviceType string) ([]map[string]interface{}, error) {
	return nil, errors.New("get_services_by_type: not implemented")
}

// GetServiceDetails get service details
func (s *Service) GetServiceDetails(configDict map[string]interface{}) (map[string]interface{}, error) {
	serviceType, _ := configDict["service_type"].(string)
	name, _ := configDict["name"].(string)

	// Call detail function based on service type
	switch serviceType {
	case "meta_data":
		return s.getMySQLStatus(name)
	case "message_queue":
		return s.getRedisInfo(name)
	case "retrieval":
		// Check the extra.retrieval_type to determine which retrieval service
		if extra, ok := configDict["extra"].(map[string]interface{}); ok {
			if retrievalType, ok := extra["retrieval_type"].(string); ok {
				if retrievalType == "infinity" {
					return s.getInfinityStatus(name)
				}
			}
		}
		return s.getESClusterStats(name)
	case "ragflow_server":
		return s.checkRAGFlowServerAlive(name)
	case "file_store":
		return s.checkMinioAlive(name)
	case "task_executor":
		return s.checkTaskExecutorAlive(name)
	default:
		return map[string]interface{}{
			"service_name": name,
			"status":       "unknown",
			"message":      "Service type not supported",
		}, nil
	}
}

// getMySQLStatus gets MySQL service status
func (s *Service) getMySQLStatus(name string) (map[string]interface{}, error) {
	startTime := time.Now()

	// Check basic connectivity with SELECT 1
	sqlDB, err := dao.DB.DB()
	if err != nil {
		return map[string]interface{}{
			"service_name": name,
			"status":       "timeout",
			"elapsed":      fmt.Sprintf("%.1f", time.Since(startTime).Milliseconds()),
			"message":      err.Error(),
		}, nil
	}

	// Execute SELECT 1 to check connectivity
	_, err = sqlDB.Exec("SELECT 1")
	if err != nil {
		return map[string]interface{}{
			"service_name": name,
			"status":       "timeout",
			"elapsed":      fmt.Sprintf("%.1f", time.Since(startTime).Milliseconds()),
			"message":      err.Error(),
		}, nil
	}

	return map[string]interface{}{
		"service_name": name,
		"status":       "alive",
		"elapsed":      fmt.Sprintf("%.1f", time.Since(startTime).Milliseconds()),
		"message":      "MySQL connection successful",
	}, nil
}

// getRedisInfo gets Redis service info
func (s *Service) getRedisInfo(name string) (map[string]interface{}, error) {
	startTime := time.Now()

	redisClient := cache.Get()
	if redisClient == nil {
		return map[string]interface{}{
			"service_name": name,
			"status":       "timeout",
			"elapsed":      fmt.Sprintf("%.1f", time.Since(startTime).Milliseconds()),
			"error":        "Redis client not initialized",
		}, nil
	}

	// Check health
	if !redisClient.Health() {
		return map[string]interface{}{
			"service_name": name,
			"status":       "timeout",
			"elapsed":      fmt.Sprintf("%.1f", time.Since(startTime).Milliseconds()),
			"error":        "Redis health check failed",
		}, nil
	}

	return map[string]interface{}{
		"service_name": name,
		"status":       "alive",
		"elapsed":      fmt.Sprintf("%.1f", time.Since(startTime).Milliseconds()),
		"message":      "Redis connection successful",
	}, nil
}

// getESClusterStats gets Elasticsearch cluster stats
func (s *Service) getESClusterStats(name string) (map[string]interface{}, error) {
	// TODO: Implement actual ES health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
		"message":      "Elasticsearch health check not implemented",
	}, nil
}

// getInfinityStatus gets Infinity service status
func (s *Service) getInfinityStatus(name string) (map[string]interface{}, error) {
	// TODO: Implement actual Infinity health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
		"message":      "Infinity health check not implemented",
	}, nil
}

// checkRAGFlowServerAlive checks if RAGFlow server is alive
func (s *Service) checkRAGFlowServerAlive(name string) (map[string]interface{}, error) {
	// TODO: Implement actual RAGFlow server health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
		"message":      "RAGFlow server health check not implemented",
	}, nil
}

// checkMinioAlive checks if MinIO is alive
func (s *Service) checkMinioAlive(name string) (map[string]interface{}, error) {
	// TODO: Implement actual MinIO health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
		"message":      "MinIO health check not implemented",
	}, nil
}

// checkTaskExecutorAlive checks if task executor is alive
func (s *Service) checkTaskExecutorAlive(name string) (map[string]interface{}, error) {
	// TODO: Implement actual task executor health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
		"message":      "Task executor health check not implemented",
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
	return map[string]interface{}{}, nil
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
		"config":        config,
		"connected":     true,
	}, nil
}
