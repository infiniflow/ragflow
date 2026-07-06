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
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// Role management methods

// ListRoles list all roles
func (s *Service) ListRoles() ([]map[string]interface{}, error) {
	result := []map[string]interface{}{
		{
			"command": "list_roles",
			"error":   "'list roles' is not supported",
		},
	}

	return result, nil
}

// CreateRole create a new role
func (s *Service) CreateRole(roleName, description string) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command":     "create_role",
		"role_name":   roleName,
		"description": description,
		"error":       "'create role' is not supported",
	}

	return result, nil
}

// ShowRole show role details
func (s *Service) ShowRole(roleName string) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command":   "show_role",
		"role_name": roleName,
		"error":     "'show role' is not supported",
	}

	return result, nil

}

// UpdateRole update role
func (s *Service) UpdateRole(roleName, description string) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command":     "update_role",
		"role_name":   roleName,
		"description": description,
		"error":       "'update role' is not supported",
	}

	return result, nil
}

// DropRole drop role
func (s *Service) DropRole(roleName string) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command":   "drop_role",
		"role_name": roleName,
		"error":     "'drop role' is not supported",
	}

	return result, nil
}

// ShowRolePermission get role permissions
func (s *Service) ShowRolePermission(roleName string) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command":   "show_role_permission",
		"role_name": roleName,
		"error":     "'show role permissions' is not supported",
	}

	return result, nil
}

// GrantRolePermission grant permission to role
func (s *Service) GrantRolePermission(roleName string, actions []string, resource string) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command":   "grant_role_permission",
		"role_name": roleName,
		"actions":   actions,
		"resource":  resource,
		"error":     "'grant role permission' is not supported",
	}

	return result, nil
}

// RevokeRolePermission revoke permission from role
func (s *Service) RevokeRolePermission(roleName string, actions []string, resource string) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command":   "revoke_role_permission",
		"role_name": roleName,
		"actions":   actions,
		"resource":  resource,
		"error":     "'revoke role permission' is not supported",
	}

	return result, nil
}

// ListResources list role resources
func (s *Service) ListResources() (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command": "list_resources",
		"error":   "'list resources for role' is not supported",
	}

	return result, nil
}

func (s *Service) ShowRoleDefaultModels(roleName string) ([]map[string]interface{}, error) {
	return []map[string]interface{}{
		{
			"command":   "show_role_default_models",
			"role_name": roleName,
			"error":     "'show role default models' is not supported",
		},
	}, nil
}

// SetRoleDefaultModel set role default model
func (s *Service) SetRoleDefaultModel(roleName, modelID, modelType string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"command":    "set_role_default_model",
		"role_name":  roleName,
		"model_id":   modelID,
		"model_type": modelType,
		"error":      "'set role default model' is not supported",
	}, nil
}

// ResetRoleDefaultModel reset role default model
func (s *Service) ResetRoleDefaultModel(roleName, modelType string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"command":    "reset_role_default_model",
		"role_name":  roleName,
		"model_type": modelType,
		"error":      "'reset role default model' is not supported",
	}, nil
}

// ListModelProviders list model providers
func (s *Service) ListModelProviders() ([]map[string]interface{}, error) {
	return []map[string]interface{}{
		{
			"command": "list_model_providers",
			"error":   "'list model providers' is not supported",
		},
	}, nil
}

// AddModelProvider Add model provider
func (s *Service) AddModelProvider(userID, providerName string) (map[string]interface{}, error) {

	return map[string]interface{}{
		"command":     "add_model_provider",
		"user_id":     userID,
		"provider_id": providerName,
		"error":       "'add model provider' is not supported",
	}, nil
}

// DeleteModelProviders delete model providers
func (s *Service) DeleteModelProviders(userID string, providerNames []string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"command":        "delete_model_providers",
		"user_id":        userID,
		"provider_names": providerNames,
		"error":          "'delete model providers' is not supported",
	}, nil
}

// ListModelInstances list model instances
func (s *Service) ListModelInstances(userID, providerName string) ([]map[string]interface{}, error) {

	return []map[string]interface{}{
		{
			"command":     "list_model_instances",
			"user_id":     userID,
			"provider_id": providerName,
			"error":       "'list model instances' is not supported",
		},
	}, nil
}

// ShowProviderInstance show provider instance
func (s *Service) ShowProviderInstance(userID, providerName, instanceName string) (map[string]interface{}, error) {

	return map[string]interface{}{
		"command":       "show_provider_instance",
		"user_id":       userID,
		"provider_id":   providerName,
		"instance_name": instanceName,
		"error":         "'show provider instance' is not supported",
	}, nil
}

// ShowProviderInstanceBalance show provider instance balance
func (s *Service) ShowProviderInstanceBalance(userID, providerName, instanceName string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"command":       "show_provider_instance_balance",
		"user_id":       userID,
		"provider_id":   providerName,
		"instance_name": instanceName,
		"error":         "'show provider instance balance' is not supported",
	}, nil
}

// CheckInstanceConnection check instance connection
func (s *Service) CheckInstanceConnection(userID, providerName, instanceName string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"command":       "check_instance_connection",
		"user_id":       userID,
		"provider_id":   providerName,
		"instance_name": instanceName,
		"error":         "'check instance connection' is not supported",
	}, nil
}

// CheckProviderConnection check provider connection
func (s *Service) CheckProviderConnection(userID, providerName, region, apiKey, baseURL string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"command":     "check_provider_connection",
		"user_id":     userID,
		"provider_id": providerName,
		"region":      region,
		"api_key":     apiKey,
		"base_url":    baseURL,
	}, nil
}

// AlterProviderInstance alter provider instance
func (s *Service) AlterProviderInstance(userID, providerName, instanceName, newInstanceName, newAPIKey string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"command":           "alter_provider_instance",
		"user_id":           userID,
		"provider_id":       providerName,
		"instance_name":     instanceName,
		"new_instance_name": newInstanceName,
		"new_api_key":       newAPIKey,
		"error":             "'alter provider instance' is not supported",
	}, nil
}

// AddModelInstance Add model instance
func (s *Service) AddModelInstance(userID, providerName, instanceName string) (map[string]interface{}, error) {

	return map[string]interface{}{
		"command":       "add_model_instance",
		"user_id":       userID,
		"provider_id":   providerName,
		"instance_name": instanceName,
		"error":         "'add model instance' is not supported",
	}, nil
}

// DeleteModelInstances delete model instances
func (s *Service) DeleteModelInstances(userID, providerName string, instances []string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"command":     "delete_model_instances",
		"user_id":     userID,
		"provider_id": providerName,
		"instances":   instances,
		"error":       "'delete model instances' is not supported",
	}, nil
}

// ListInstanceModels list models for instance
func (s *Service) ListInstanceModels(userID, providerName, instanceName string) ([]map[string]interface{}, error) {
	return []map[string]interface{}{
		{
			"command":       "list_instance_models",
			"user_id":       userID,
			"provider_id":   providerName,
			"instance_name": instanceName,
			"error":         "'list instance models' is not supported",
		},
	}, nil
}

func (s *Service) EnableOrDisableModel(userID, providerName, instanceName, modelName, modelID, status string) (map[string]interface{}, error) {

	return map[string]interface{}{
		"command":       "enable_or_disable_model",
		"user_id":       userID,
		"provider_id":   providerName,
		"instance_name": instanceName,
		"model_name":    modelName,
		"model_id":      modelID,
		"status":        status,
		"error":         "'enable or disable model' is not supported",
	}, nil
}

// AddModel Add model

// AddModels Add models
func (s *Service) AddModels(userID, providerName, instanceName string, modelNames []string) (map[string]interface{}, error) {

	return map[string]interface{}{
		"command":       "add_model",
		"user_id":       userID,
		"provider_id":   providerName,
		"instance_name": instanceName,
		"model_names":   modelNames,
		"error":         "'add model' is not supported",
	}, nil
}

// DeleteModels delete models
func (s *Service) DeleteModels(userID, providerName, instanceName string, models []string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"command":       "delete_models",
		"user_id":       userID,
		"provider_id":   providerName,
		"instance_name": instanceName,
		"models":        models,
		"error":         "'delete models' is not supported",
	}, nil
}

func (s *Service) GetSystemFingerprint() (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command": "get_system_fingerprint",
		"error":   "'get system fingerprint' is not supported",
	}

	return result, nil
}

func (s *Service) SetSystemLicense(license string) error {
	return errors.New("'set system license' is not supported")
}

func (s *Service) ShowSystemLicense(check bool) (map[string]interface{}, error) {
	var result map[string]interface{}
	if check {
		result = map[string]interface{}{
			"command": "check_system_license",
			"error":   "'check system license' is not supported",
		}

	} else {
		result = map[string]interface{}{
			"command": "show_system_license",
			"error":   "'show system license' is not supported",
		}
	}

	return result, nil
}

func (s *Service) UpdateSystemLicenseConfig(timeRecordSaveInterval, timeRecordTaskDuration int64) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command":                   "update_system_license_config",
		"time_record_save_interval": timeRecordSaveInterval,
		"time_record_task_duration": timeRecordTaskDuration,
		"error":                     "'update system license config' is not supported",
	}

	return result, nil
}

// ShowUserActivity show user activity for enterprise edition
func (s *Service) ShowUserActivity(email string, days int) (map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := map[string]interface{}{
		"email":    user.Email,
		"nickname": user.Nickname,
		"days":     days,
		"error":    "'show user activity' is not supported",
	}

	return result, nil
}

// ShowUserDatasetSummary show user dataset summary for enterprise edition
func (s *Service) ShowUserDatasetSummary(email, dataset string) (map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := map[string]interface{}{
		"email":    user.Email,
		"nickname": user.Nickname,
		"dataset":  dataset,
		"error":    "'show user dataset summary' is not supported",
	}

	return result, nil
}

// GetUserSummary get user summary for enterprise edition
func (s *Service) ShowUserSummary(email string) (map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := map[string]interface{}{
		"email":    user.Email,
		"nickname": user.Nickname,
		"error":    "'show user summary' is not supported",
	}

	return result, nil
}

// ShowUserStorage show user storage for enterprise edition
func (s *Service) ShowUserStorage(email string) (map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := map[string]interface{}{
		"email":    user.Email,
		"nickname": user.Nickname,
		"error":    "'show user storage' is not supported",
	}

	return result, nil
}

// ShowUserQuota show user quota for enterprise edition
func (s *Service) ShowUserQuota(email string) (map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := map[string]interface{}{
		"email":    user.Email,
		"nickname": user.Nickname,
		"error":    "'show user quota' is not supported",
	}

	return result, nil
}

// ShowUserIndex show user index for enterprise edition
func (s *Service) ShowUserIndex(email string) (map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := map[string]interface{}{
		"email":    user.Email,
		"nickname": user.Nickname,
		"error":    "'show user index' is not supported",
	}

	return result, nil
}

// UpdateUserRole update user role
func (s *Service) UpdateUserRole(email, roleName string) (map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := map[string]interface{}{
		"command":  "update_user_role",
		"role":     roleName,
		"email":    user.Email,
		"nickname": user.Nickname,
		"error":    "'update user role' is not supported",
	}

	return result, nil
}

// ShowUserPermission show user permissions for enterprise edition
func (s *Service) ShowUserPermission(email string) (map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := map[string]interface{}{
		"command":  "show_user_permission",
		"email":    user.Email,
		"nickname": user.Nickname,
		"error":    "'show user permission' is not supported",
	}

	return result, nil
}

// ListUserDatasets show user datasets for enterprise edition
func (s *Service) ListUserDatasets(email string) ([]map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := []map[string]interface{}{
		{
			"command":  "list_user_datasets",
			"email":    user.Email,
			"nickname": user.Nickname,
			"error":    "'list user datasets' is not supported",
		},
	}

	return result, nil
}

// ListUserAgents show user agents for enterprise edition
func (s *Service) ListUserAgents(email string) ([]map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := []map[string]interface{}{
		{
			"command":  "list_user_agents",
			"email":    user.Email,
			"nickname": user.Nickname,
			"error":    "'list user agents' is not supported",
		},
	}

	return result, nil
}

// ListUserChats show user chats for enterprise edition
func (s *Service) ListUserChats(email string) ([]map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := []map[string]interface{}{
		{
			"command":  "list_user_chats",
			"email":    user.Email,
			"nickname": user.Nickname,
			"error":    "'list user chats' is not supported",
		},
	}

	return result, nil
}

// ListUserSearches show user searches for enterprise edition
func (s *Service) ListUserSearches(email string) ([]map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := []map[string]interface{}{
		{
			"command":  "list_user_searches",
			"email":    user.Email,
			"nickname": user.Nickname,
			"error":    "'list user searches' is not supported",
		},
	}

	return result, nil
}

// ListUserModels show user models for enterprise edition
func (s *Service) ListUserModels(email string) ([]map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := []map[string]interface{}{
		{
			"command":  "list_user_models",
			"email":    user.Email,
			"nickname": user.Nickname,
			"error":    "'list user models' is not supported",
		},
	}

	return result, nil
}

// ListUserFiles show user files for enterprise edition
func (s *Service) ListUserFiles(email string) ([]map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := []map[string]interface{}{
		{
			"command":  "list_user_files",
			"email":    user.Email,
			"nickname": user.Nickname,
			"error":    "'list user files' is not supported",
		},
	}

	return result, nil
}

// ListUserProviders show user providers for enterprise edition
func (s *Service) ListUserProviders(email string) ([]map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := []map[string]interface{}{
		{
			"command":  "list_user_providers",
			"email":    user.Email,
			"nickname": user.Nickname,
			"error":    "'list user providers' is not supported",
		},
	}

	return result, nil
}

// ListUserProviderInstances show user provider instances for enterprise edition
func (s *Service) ListUserProviderInstances(email, providerName string) ([]map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := []map[string]interface{}{
		{
			"command":       "list_user_provider_instances",
			"email":         user.Email,
			"nickname":      user.Nickname,
			"provider_name": providerName,
			"error":         "'list user provider instances' is not supported",
		},
	}

	return result, nil
}

// ListUserProviderInstanceModels show user provider instance models for enterprise edition
func (s *Service) ListUserProviderInstanceModels(email, providerName, instanceName string) ([]map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := []map[string]interface{}{
		{
			"command":       "list_user_provider_instance_models",
			"email":         user.Email,
			"nickname":      user.Nickname,
			"provider_name": providerName,
			"instance_name": instanceName,
			"error":         "'list user provider instance models' is not supported",
		},
	}

	return result, nil
}

// ListUserDefaultModels show user default models for enterprise edition
func (s *Service) ListUserDefaultModels(email string) ([]map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := []map[string]interface{}{
		{
			"command":  "list_user_default_models",
			"email":    user.Email,
			"nickname": user.Nickname,
			"error":    "'list user default models' is not supported",
		},
	}

	return result, nil
}

// ShowUsersSummary show users summary for enterprise edition
func (s *Service) ShowUsersSummary() (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command": "show_users_summary",
		"error":   "'show users summary' is not supported",
	}

	return result, nil
}

// ShowUsersActivity show users activity for enterprise edition
func (s *Service) ShowUsersActivity(days, windows *int) (map[string]interface{}, error) {
	daysInt := 0
	if days != nil {
		daysInt = *days
	}
	windowsInt := 0
	if windows != nil {
		windowsInt = *windows
	}
	result := map[string]interface{}{
		"days":    daysInt,
		"windows": windowsInt,
		"command": "show_users_activity",
		"error":   "'show users activity' is not supported",
	}

	return result, nil
}

func (s *Service) ListUsersEnterprise(pageIndex, pageSize int, status, orderBy, plan *string, top, days, quota *int) ([]map[string]interface{}, error) {
	item := map[string]interface{}{}
	if status != nil {
		item["status"] = *status
	}
	if orderBy != nil {
		item["order_by"] = *orderBy
	}
	if plan != nil {
		item["plan"] = *plan
	}
	if top != nil {
		item["top"] = *top
	}
	if days != nil {
		item["days"] = *days
	}
	if quota != nil {
		item["quota"] = *quota
	}

	var result []map[string]interface{}
	result = append(result, item)
	return result, nil
}

// ListUsersReports list users reports for enterprise edition
func (s *Service) ListUsersReports(pageIndex, pageSize int, status, plan *string, days *int) (map[string]interface{}, error) {

	statusStr := "all"
	if status != nil {
		statusStr = *status
	}
	planStr := "all"
	daysInt := 0
	if days != nil {
		daysInt = *days
	}
	if plan != nil {
		planStr = *plan
	}

	result := map[string]interface{}{
		"page_index": pageIndex,
		"page_size":  pageSize,
		"status":     statusStr,
		"plan":       planStr,
		"days":       daysInt,
		"command":    "list_users_reports",
		"error":      "'List users reports' is not supported",
	}

	return result, nil
}

// ListUsersStorage list users storage for enterprise edition
func (s *Service) ListUsersStorage(pageIndex, pageSize, top int) (map[string]interface{}, error) {

	result := map[string]interface{}{
		"page_index": pageIndex,
		"page_size":  pageSize,
		"top":        top,
		"command":    "list_users_storage",
		"error":      "'List users storage' is not supported",
	}

	return result, nil
}

// ListUsersDocuments list users documents for enterprise edition
func (s *Service) ListUsersDocuments(pageIndex, pageSize, top int) (map[string]interface{}, error) {

	result := map[string]interface{}{
		"page_index": pageIndex,
		"page_size":  pageSize,
		"top":        top,
		"command":    "list_users_documents",
		"error":      "'List users documents' is not supported",
	}

	return result, nil
}

// ListUsersIndex list users index for enterprise edition
func (s *Service) ListUsersIndex(pageIndex, pageSize, top int) (map[string]interface{}, error) {

	result := map[string]interface{}{
		"page_index": pageIndex,
		"page_size":  pageSize,
		"top":        top,
		"command":    "list_users_index",
		"error":      "'List users index' is not supported",
	}

	return result, nil
}

// ListUsersQuota list users quota for enterprise edition
func (s *Service) ListUsersQuota(pageIndex, pageSize, top int, quotaThreshold *int, plan *string, days *int) (map[string]interface{}, error) {

	quotaThresholdInt := 0
	if quotaThreshold != nil {
		quotaThresholdInt = *quotaThreshold
	}
	planStr := "all"
	daysInt := 0
	if days != nil {
		daysInt = *days
	}
	if plan != nil {
		planStr = *plan
	}

	result := map[string]interface{}{
		"page_index":      pageIndex,
		"page_size":       pageSize,
		"top":             top,
		"quota_threshold": quotaThresholdInt,
		"plan":            planStr,
		"days":            daysInt,
		"command":         "list_users_quota",
		"error":           "'List users quota' is not supported",
	}

	return result, nil
}

// ShowUsersPlanSummary show users plan summary for enterprise edition
func (s *Service) ShowUsersPlanSummary() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_users_plan_summary",
		"error":   "'Show users plan summary' is not supported",
	}

	return result, nil
}

// ShowUsersQuotaSummary show users quota summary for enterprise edition
func (s *Service) ShowUsersQuotaSummary() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_users_quota_summary",
		"error":   "'Show users quota summary' is not supported",
	}

	return result, nil
}

// ShowIngestionTasksSummary show ingestion tasks summary for enterprise edition
func (s *Service) ShowIngestionTasksSummary() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_ingestion_tasks_summary",
		"error":   "'Show ingestion tasks summary' is not supported",
	}

	return result, nil
}

// ShowDataSummary show data summary for enterprise edition
func (s *Service) ShowDataSummary() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_data_summary",
		"error":   "'Show data summary' is not supported",
	}

	return result, nil
}

// ShowDataOrphan show data orphan for enterprise edition
func (s *Service) ShowDataOrphan() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_data_orphan",
		"error":   "'Show data orphan' is not supported",
	}

	return result, nil
}

// ShowDataStorage show data storage for enterprise edition
func (s *Service) ShowDataStorage() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_data_storage",
		"error":   "'Show data storage' is not supported",
	}

	return result, nil
}

// ShowDataIndex show data index for enterprise edition
func (s *Service) ShowDataIndex() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_data_index",
		"error":   "'Show data index' is not supported",
	}

	return result, nil
}

// PurgeOrphanData purge orphan data for enterprise edition
func (s *Service) PurgeOrphanData(preview bool) (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "purge_orphan_data",
		"preview": preview,
		"error":   "'Purge orphan data' is not supported",
	}

	return result, nil
}

// PurgeUserData purge user data for enterprise edition
func (s *Service) PurgeUserData(email string, preview bool) (map[string]interface{}, error) {
	// Query user by email
	var user entity.User
	err := dao.DB.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, common.ErrUserNotFound
	}

	result := map[string]interface{}{
		"email":    user.Email,
		"nickname": user.Nickname,
		"preview":  preview,
		"error":    "'Purge user data' is not supported",
	}

	return result, nil
}

// PurgeUsersData purge users data for enterprise edition
func (s *Service) PurgeUsersData(preview bool, days int, userPlan *string, userActivity *string) (map[string]interface{}, error) {

	plan := "all"
	activity := "all"
	if userPlan != nil {
		plan = *userPlan
	}
	if userActivity != nil {
		activity = *userActivity
	}

	result := map[string]interface{}{
		"command":  "purge_users_data",
		"preview":  preview,
		"days":     days,
		"plan":     plan,
		"activity": activity,
		"error":    "'Purge users data' is not supported",
	}

	return result, nil
}

// GenerateUserAPIKey create tenant API key for tenant
func (s *Service) GenerateUserAPIKey(username string) (map[string]interface{}, error) {

	user, err := s.userDAO.GetByEmail(username)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	result := map[string]interface{}{
		"command":  "create_user_api_key",
		"email":    user.Email,
		"nickname": user.Nickname,
		"error":    "'Create user API key' is not supported",
	}

	return result, nil
}

// DeleteUserAPIKey delete user API key
func (s *Service) DeleteUserAPIKey(username, key string) (map[string]interface{}, error) {

	user, err := s.userDAO.GetByEmail(username)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	result := map[string]interface{}{
		"command":  "delete_user_api_key",
		"email":    user.Email,
		"nickname": user.Nickname,
		"api_key":  key,
		"error":    "'Delete user API key' is not supported",
	}

	return result, nil
}

// ListUserAPIKeys list user API keys
func (s *Service) ListUserAPIKeys(username string) ([]map[string]interface{}, error) {

	user, err := s.userDAO.GetByEmail(username)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	result := []map[string]interface{}{
		{
			"command":  "list_user_api_keys",
			"email":    user.Email,
			"nickname": user.Nickname,
			"error":    "'List user API keys' is not supported",
		},
	}

	return result, nil
}

func (s *Service) ListIngestionTasksByCondition(email, status *string) ([]map[string]interface{}, error) {

	if email == nil && status == nil {
		return nil, fmt.Errorf("email or status are required")
	}

	element := map[string]interface{}{
		"command": "list_ingestion_tasks_by_condition",
		"error":   "'List ingestion tasks by condition' is not supported",
	}

	if email != nil {
		element["email"] = *email
	}
	if status != nil {
		element["status"] = *status
	}

	return []map[string]interface{}{element}, nil
}

func (s *Service) StopIngestionTasksByCondition(tasks []string, email, status *string) ([]map[string]interface{}, error) {

	if email == nil && status == nil {
		return nil, fmt.Errorf("email or status are required")
	}

	element := map[string]interface{}{
		"command": "stop_ingestion_tasks_by_condition",
		"tasks":   tasks,
		"error":   "'Stop ingestion tasks by condition' is not supported",
	}

	if email != nil {
		element["email"] = *email
	}
	if status != nil {
		element["status"] = *status
	}

	return []map[string]interface{}{element}, nil
}

func (s *Service) RemoveIngestionTasksByCondition(tasks []string, email, status *string) ([]map[string]interface{}, error) {

	if email == nil && status == nil {
		return nil, fmt.Errorf("email or status are required")
	}

	element := map[string]interface{}{
		"command": "remove_ingestion_tasks_by_condition",
		"tasks":   tasks,
		"error":   "'Remove ingestion tasks by condition' is not supported",
	}

	if email != nil {
		element["email"] = *email
	}
	if status != nil {
		element["status"] = *status
	}

	return []map[string]interface{}{element}, nil
}
