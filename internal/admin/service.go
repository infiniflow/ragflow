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
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"ragflow/internal/cache"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/model"
	"ragflow/internal/server"
	"ragflow/internal/utility"
	"strconv"
	"time"
)

// Service errors
var (
	ErrInvalidToken = errors.New("invalid token")
	ErrNotAdmin     = errors.New("user is not admin")
	ErrUserInactive = errors.New("user is inactive")
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

// Logout user logout
func (s *Service) Logout(user interface{}) error {
	// Invalidate token by setting it to INVALID_ prefix
	if u, ok := user.(*model.User); ok {
		invalidToken := "INVALID_" + generateRandomHex(16)
		return s.userDAO.UpdateAccessToken(u, invalidToken)
	}
	return nil
}

// GetUserByToken get user by access token
func (s *Service) GetUserByToken(token string) (*model.User, error) {
	user, err := s.userDAO.GetByAccessToken(token)
	if err != nil {
		return nil, ErrInvalidToken
	}

	if user.IsSuperuser == nil || !*user.IsSuperuser {
		return nil, ErrNotAdmin
	}

	if user.IsActive != "1" {
		return nil, fmt.Errorf("user inactive")
	}

	return user, nil
}

// generateRandomHex generate random hex string
func generateRandomHex(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

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

// ListServices get all services
func (s *Service) ListServices() ([]map[string]interface{}, error) {
	allConfigs := server.GetAllConfigs()

	var result []map[string]interface{}
	for _, configDict := range allConfigs {
		serviceType := configDict["service_type"]
		if serviceType != "ragflow_server" {
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

	}

	id := len(result)
	serverList := GlobalServerStatusStore.GetAllStatuses()
	for _, serverStatus := range serverList {
		serverItem := make(map[string]interface{})
		serverItem["name"] = serverStatus.ServerName
		serverItem["service_type"] = serverStatus.ServerType
		serverItem["id"] = id
		id++
		serverItem["host"] = serverStatus.Host
		serverItem["port"] = serverStatus.Port
		serverItem["status"] = "alive"
		result = append(result, serverItem)
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
	// Check if Elasticsearch is the doc engine
	docEngine := os.Getenv("DOC_ENGINE")
	if docEngine == "" {
		docEngine = "elasticsearch"
	}
	if docEngine != "elasticsearch" {
		return map[string]interface{}{
			"service_name": name,
			"status":       "timeout",
			"message":      "error: Elasticsearch is not in use.",
		}, nil
	}

	// Get ES config from server config
	cfg := server.GetConfig()
	if cfg == nil || cfg.DocEngine.ES == nil {
		return map[string]interface{}{
			"service_name": name,
			"status":       "timeout",
			"message":      "error: Elasticsearch configuration not found",
		}, nil
	}

	// Create ES engine and get cluster stats
	esEngine, err := elasticsearch.NewEngine(cfg.DocEngine.ES)
	if err != nil {
		return map[string]interface{}{
			"service_name": name,
			"status":       "timeout",
			"message":      fmt.Sprintf("error: %s", err.Error()),
		}, nil
	}
	defer esEngine.Close()

	clusterStats, err := esEngine.GetClusterStats()
	if err != nil {
		return map[string]interface{}{
			"service_name": name,
			"status":       "timeout",
			"message":      fmt.Sprintf("error: %s", err.Error()),
		}, nil
	}

	return map[string]interface{}{
		"service_name": name,
		"status":       "alive",
		"message":      clusterStats,
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
	startTime := time.Now()

	// Get ragflow config from allConfigs
	var host string
	var port int
	allConfigs := server.GetAllConfigs()
	for _, config := range allConfigs {
		if serviceType, ok := config["service_type"].(string); ok && serviceType == "ragflow_server" {
			if h, ok := config["host"].(string); ok {
				host = h
			}
			if p, ok := config["port"].(int); ok {
				port = p
			}
			break
		}
	}

	// Default values
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		port = 9380
	}

	// Replace 0.0.0.0 with 127.0.0.1 for local check
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}

	url := fmt.Sprintf("http://%s:%d/v1/system/ping", host, port)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return map[string]interface{}{
			"service_name": name,
			"status":       "timeout",
			"message":      fmt.Sprintf("error: %s", err.Error()),
		}, nil
	}
	defer resp.Body.Close()

	elapsed := time.Since(startTime).Milliseconds()
	if resp.StatusCode == 200 {
		return map[string]interface{}{
			"service_name": name,
			"status":       "alive",
			"message":      fmt.Sprintf("Confirm elapsed: %.1f ms.", float64(elapsed)),
		}, nil
	}

	return map[string]interface{}{
		"service_name": name,
		"status":       "timeout",
		"message":      fmt.Sprintf("Confirm elapsed: %.1f ms.", float64(elapsed)),
	}, nil
}

// checkMinioAlive checks if MinIO is alive
func (s *Service) checkMinioAlive(name string) (map[string]interface{}, error) {
	startTime := time.Now()

	// Get minio config from allConfigs
	var host string
	var port int
	var secure bool
	var verify bool = true

	allConfigs := server.GetAllConfigs()
	for _, config := range allConfigs {
		if serviceType, ok := config["service_type"].(string); ok && serviceType == "file_store" {
			// Get host from config
			if h, ok := config["host"].(string); ok {
				host = h
			}

			if p, ok := config["port"].(int); ok {
				port = p
			} else if p, ok := config["port"].(float64); ok {
				port = int(p)
			} else if p, ok := config["port"].(string); ok {
				if parsedPort, err := strconv.Atoi(p); err == nil {
					port = parsedPort
				}
			}
			// Get secure from extra config
			if extra, ok := config["extra"].(map[string]interface{}); ok {
				if s, ok := extra["secure"].(bool); ok {
					secure = s
				} else if s, ok := extra["secure"].(string); ok {
					secure = s == "true" || s == "1" || s == "yes"
				}
				if v, ok := extra["verify"].(bool); ok {
					verify = v
				} else if v, ok := extra["verify"].(string); ok {
					verify = !(v == "false" || v == "0" || v == "no")
				}
			}
			break
		}
	}

	// Default host
	if host == "" {
		host = "localhost"
	}
	if port == 0 {
		port = 9000
	}

	// Determine scheme
	scheme := "http"
	if secure {
		scheme = "https"
	}

	url := fmt.Sprintf("%s://%s:%d/minio/health/live", scheme, host, port)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// If verify is false, we need to skip SSL verification
	if !verify && scheme == "https" {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Get(url)
	if err != nil {
		return map[string]interface{}{
			"service_name": name,
			"status":       "timeout",
			"message":      fmt.Sprintf("error: %s", err.Error()),
		}, nil
	}
	defer resp.Body.Close()

	elapsed := time.Since(startTime).Milliseconds()
	if resp.StatusCode == 200 {
		return map[string]interface{}{
			"service_name": name,
			"status":       "alive",
			"message":      fmt.Sprintf("Confirm elapsed: %.1f ms.", float64(elapsed)),
		}, nil
	}

	return map[string]interface{}{
		"service_name": name,
		"status":       "timeout",
		"message":      fmt.Sprintf("Confirm elapsed: %.1f ms.", float64(elapsed)),
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

// HandleHeartbeat handle heartbeat
func (s *Service) HandleHeartbeat(msg *common.BaseMessage) error {
	status := &common.BaseMessage{
		ServerName: msg.ServerName,
		ServerType: msg.ServerType,
		Host:       msg.Host,
		Port:       msg.Port,
		Version:    msg.Version,
		Timestamp:  msg.Timestamp,
		Ext:        msg.Ext,
	}
	GlobalServerStatusStore.UpdateStatus(msg.ServerName, status)
	return nil
}

// InitDefaultAdmin initialize default admin user
// This matches Python's init_default_admin behavior
func (s *Service) InitDefaultAdmin() error {
	// Default superuser settings (matching Python's DEFAULT_SUPERUSER_* defaults)
	defaultNickname := "admin"
	defaultEmail := "admin@ragflow.io"
	defaultPassword := "admin"

	// Query superusers
	var users []*model.User
	err := dao.DB.Where("is_superuser = ? AND status = ?", true, "1").Find(&users).Error
	if err != nil {
		return fmt.Errorf("failed to query superusers: %w", err)
	}

	if len(users) == 0 {
		now := time.Now().Unix()
		nowDate := time.Now()
		userID := utility.GenerateToken()
		accessToken := utility.GenerateToken()
		status := "1"
		loginChannel := "password"
		isSuperuser := true

		// Python: password = encode_to_base64(password) = base64.b64encode(password)
		// Then: generate_password_hash(base64_password) creates werkzeug hash
		password := base64.StdEncoding.EncodeToString([]byte(defaultPassword))
		hashedPassword, err := GenerateWerkzeugPasswordHash(password, 150000)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}

		user := &model.User{
			ID:              userID,
			Email:           defaultEmail,
			Nickname:        defaultNickname,
			Password:        &hashedPassword,
			AccessToken:     &accessToken,
			Status:          &status,
			IsActive:        "1",
			IsAuthenticated: "1",
			IsAnonymous:     "0",
			LoginChannel:    &loginChannel,
			IsSuperuser:     &isSuperuser,
			BaseModel: model.BaseModel{
				CreateTime: &now,
				CreateDate: &nowDate,
				UpdateTime: &now,
				UpdateDate: &nowDate,
			},
		}

		if err := dao.DB.Create(user).Error; err != nil {
			return fmt.Errorf("can't init admin: %w", err)
		}

		if err := s.addTenantForAdmin(userID, defaultNickname); err != nil {
			return fmt.Errorf("failed to add tenant for admin: %w", err)
		}

		return nil
	}

	for _, user := range users {
		if user.IsActive != "1" {
			return fmt.Errorf("no active admin. Please update 'is_active' in db manually")
		}
	}

	for _, user := range users {
		if user.Email == defaultEmail {
			// Check if tenant exists
			var count int64
			dao.DB.Model(&model.UserTenant{}).Where("user_id = ? AND status = ?", user.ID, "1").Count(&count)
			if count == 0 {
				nickname := defaultNickname
				if user.Nickname != "" {
					nickname = user.Nickname
				}
				if err := s.addTenantForAdmin(user.ID, nickname); err != nil {
					return err
				}
			}
			break
		}
	}

	return nil
}

// addTenantForAdmin add tenant for admin user
func (s *Service) addTenantForAdmin(userID, nickname string) error {
	now := time.Now().Unix()
	nowDate := time.Now()
	status := "1"
	role := "owner"
	tenantName := nickname + "'s Kingdom"

	tenant := &model.Tenant{
		ID:   userID,
		Name: &tenantName,
		BaseModel: model.BaseModel{
			CreateTime: &now,
			CreateDate: &nowDate,
			UpdateTime: &now,
			UpdateDate: &nowDate,
		},
	}

	if err := dao.DB.Create(tenant).Error; err != nil {
		return err
	}

	userTenant := &model.UserTenant{
		TenantID:  userID,
		UserID:    userID,
		InvitedBy: userID,
		Role:      role,
		Status:    &status,
		BaseModel: model.BaseModel{
			CreateTime: &now,
			CreateDate: &nowDate,
			UpdateTime: &now,
			UpdateDate: &nowDate,
		},
	}

	return dao.DB.Create(userTenant).Error
}
