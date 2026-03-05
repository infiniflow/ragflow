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
	"net/url"
	"os"
	"ragflow/internal/dao"
	"ragflow/internal/model"
	"ragflow/internal/server"
	"ragflow/internal/utility"
	"strconv"
	"strings"
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

// Service management methods

// parseHostPort parses host:port string and returns host and port
func parseHostPort(hostPort string) (string, int) {
	if hostPort == "" {
		return "", 0
	}

	// Handle URL format like http://host:port
	if strings.Contains(hostPort, "://") {
		u, err := url.Parse(hostPort)
		if err == nil {
			hostPort = u.Host
		}
	}

	// Split host:port
	parts := strings.Split(hostPort, ":")
	host := parts[0]
	port := 0
	if len(parts) > 1 {
		port, _ = strconv.Atoi(parts[1])
	}
	return host, port
}

// getString gets string value from map
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// getInt gets int value from map
func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key].(int); ok {
		return v
	}
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	return 0
}

// GetAllServices get all services
func (s *Service) GetAllServices() ([]map[string]interface{}, error) {
	viperConfig := server.GetGlobalViperConfig()
	if viperConfig == nil {
		return nil, errors.New("configuration not initialized")
	}

	docEngine := os.Getenv("DOC_ENGINE")
	if docEngine == "" {
		docEngine = "elasticsearch"
	}

	var result []map[string]interface{}
	id := 0

	for k, v := range viperConfig.AllSettings() {
		configDict, ok := v.(map[string]interface{})
		if !ok {
			continue
		}

		switch k {
		case "ragflow":
			configDict["id"] = id
			configDict["name"] = fmt.Sprintf("ragflow_%d", id)
			configDict["service_type"] = "ragflow_server"
			configDict["extra"] = map[string]interface{}{}
			configDict["port"] = configDict["http_port"]
			delete(configDict, "http_port")
		case "es":
			// Skip if retrieval_type doesn't match doc_engine
			if docEngine != "elasticsearch" {
				continue
			}
			hosts := getString(configDict, "hosts")
			host, port := parseHostPort(hosts)
			username := getString(configDict, "username")
			password := getString(configDict, "password")
			configDict["id"] = id
			configDict["name"] = "elasticsearch"
			configDict["host"] = host
			configDict["port"] = port
			configDict["service_type"] = "retrieval"
			configDict["extra"] = map[string]interface{}{
				"retrieval_type": "elasticsearch",
				"username":       username,
				"password":       password,
			}
			delete(configDict, "hosts")
			delete(configDict, "username")
			delete(configDict, "password")
		case "infinity":
			// Skip if retrieval_type doesn't match doc_engine
			if docEngine != "infinity" {
				continue
			}
			uri := getString(configDict, "uri")
			host, port := parseHostPort(uri)
			dbName := getString(configDict, "db_name")
			if dbName == "" {
				dbName = "default_db"
			}
			configDict["id"] = id
			configDict["name"] = "infinity"
			configDict["host"] = host
			configDict["port"] = port
			configDict["service_type"] = "retrieval"
			configDict["extra"] = map[string]interface{}{
				"retrieval_type": "infinity",
				"db_name":        dbName,
			}
		case "minio":
			hostPort := getString(configDict, "host")
			host, port := parseHostPort(hostPort)
			user := getString(configDict, "user")
			password := getString(configDict, "password")
			configDict["id"] = id
			configDict["name"] = "minio"
			configDict["host"] = host
			configDict["port"] = port
			configDict["service_type"] = "file_store"
			configDict["extra"] = map[string]interface{}{
				"store_type": "minio",
				"user":       user,
				"password":   password,
			}
			delete(configDict, "bucket")
			delete(configDict, "user")
			delete(configDict, "password")
		case "redis":
			hostPort := getString(configDict, "host")
			host, port := parseHostPort(hostPort)
			password := getString(configDict, "password")
			db := getInt(configDict, "db")
			configDict["id"] = id
			configDict["name"] = "redis"
			configDict["host"] = host
			configDict["port"] = port
			configDict["service_type"] = "message_queue"
			configDict["extra"] = map[string]interface{}{
				"mq_type":  "redis",
				"database": db,
				"password": password,
			}
			delete(configDict, "password")
			delete(configDict, "db")
		case "mysql":
			host := getString(configDict, "host")
			port := getInt(configDict, "port")
			user := getString(configDict, "user")
			password := getString(configDict, "password")
			configDict["id"] = id
			configDict["name"] = "mysql"
			configDict["host"] = host
			configDict["port"] = port
			configDict["service_type"] = "meta_data"
			configDict["extra"] = map[string]interface{}{
				"meta_type": "mysql",
				"username":  user,
				"password":  password,
			}
			delete(configDict, "stale_timeout")
			delete(configDict, "max_connections")
			delete(configDict, "max_allowed_packet")
			delete(configDict, "user")
			delete(configDict, "password")
		case "task_executor":
			mqType := getString(configDict, "message_queue_type")
			configDict["id"] = id
			configDict["name"] = "task_executor"
			configDict["service_type"] = "task_executor"
			configDict["extra"] = map[string]interface{}{
				"message_queue_type": mqType,
			}
			delete(configDict, "message_queue_type")
		case "admin":
			// Skip admin section
			continue
		default:
			// Skip unknown sections
			continue
		}

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

		// Set default values for empty host/port
		if configDict["host"] == "" {
			configDict["host"] = "-"
		}
		if configDict["port"] == 0 {
			configDict["port"] = "-"
		}

		delete(configDict, "prefix_path")
		delete(configDict, "username")
		result = append(result, configDict)
		id++
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
	// TODO: Implement actual MySQL health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
	}, nil
}

// getRedisInfo gets Redis service info
func (s *Service) getRedisInfo(name string) (map[string]interface{}, error) {
	// TODO: Implement actual Redis health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
	}, nil
}

// getESClusterStats gets Elasticsearch cluster stats
func (s *Service) getESClusterStats(name string) (map[string]interface{}, error) {
	// TODO: Implement actual ES health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
	}, nil
}

// getInfinityStatus gets Infinity service status
func (s *Service) getInfinityStatus(name string) (map[string]interface{}, error) {
	// TODO: Implement actual Infinity health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
	}, nil
}

// checkRAGFlowServerAlive checks if RAGFlow server is alive
func (s *Service) checkRAGFlowServerAlive(name string) (map[string]interface{}, error) {
	// TODO: Implement actual RAGFlow server health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
	}, nil
}

// checkMinioAlive checks if MinIO is alive
func (s *Service) checkMinioAlive(name string) (map[string]interface{}, error) {
	// TODO: Implement actual MinIO health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
	}, nil
}

// checkTaskExecutorAlive checks if task executor is alive
func (s *Service) checkTaskExecutorAlive(name string) (map[string]interface{}, error) {
	// TODO: Implement actual task executor health check
	return map[string]interface{}{
		"service_name": name,
		"status":       "unknown",
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
