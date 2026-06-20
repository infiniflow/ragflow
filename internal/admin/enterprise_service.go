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
			"error":   "'list roles' is implemented in enterprise edition",
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
		"error":       "'create role' is implemented in enterprise edition",
	}

	return result, nil
}

// ShowRole show role details
func (s *Service) ShowRole(roleName string) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command":   "show_role",
		"role_name": roleName,
		"error":     "'show role' is implemented in enterprise edition",
	}

	return result, nil

}

// UpdateRole update role
func (s *Service) UpdateRole(roleName, description string) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command":     "update_role",
		"role_name":   roleName,
		"description": description,
		"error":       "'update role' is implemented in enterprise edition",
	}

	return result, nil
}

// DeleteRole delete role
func (s *Service) DeleteRole(roleName string) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command":   "delete_role",
		"role_name": roleName,
		"error":     "'delete role' is implemented in enterprise edition",
	}

	return result, nil
}

// GetRolePermission get role permissions
func (s *Service) GetRolePermission(roleName string) ([]map[string]interface{}, error) {
	result := []map[string]interface{}{
		{
			"command":   "get_role_permission",
			"role_name": roleName,
			"error":     "'get role permissions' is implemented in enterprise edition",
		},
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
		"error":     "'grant role permission' is implemented in enterprise edition",
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
		"error":     "'revoke role permission' is implemented in enterprise edition",
	}

	return result, nil
}

// ListResources list role resources
func (s *Service) ListResources() (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command": "list_resources",
		"error":   "'list resources for role' is implemented in enterprise edition",
	}

	return result, nil
}

// ListAllModels list all models
func (s *Service) ListAllModels() ([]map[string]interface{}, error) {
	return []map[string]interface{}{
		{
			"command": "list_all_models",
			"error":   "'list all models' is implemented in enterprise edition",
		},
	}, nil
}

func (s *Service) GetModelByModelName(modelName string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"command":    "get_model_by_model_name",
		"model_name": modelName,
		"error":      "'get model by model name' is implemented in enterprise edition",
	}, nil
}

func (s *Service) GetSystemFingerprint() (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command": "get_system_fingerprint",
		"error":   "'get system fingerprint' is implemented in enterprise edition",
	}

	return result, nil
}

func (s *Service) SetSystemLicense(license string) error {
	return errors.New("'set system license' is implemented in enterprise edition")
}

func (s *Service) ShowSystemLicense(check bool) (map[string]interface{}, error) {
	var result map[string]interface{}
	if check {
		result = map[string]interface{}{
			"command": "check_system_license",
			"error":   "'check system license' is implemented in enterprise edition",
		}

	} else {
		result = map[string]interface{}{
			"command": "show_system_license",
			"error":   "'show system license' is implemented in enterprise edition",
		}
	}

	return result, nil
}

func (s *Service) UpdateSystemLicenseConfig(timeRecordSaveInterval, timeRecordTaskDuration int64) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command":                   "update_system_license_config",
		"time_record_save_interval": timeRecordSaveInterval,
		"time_record_task_duration": timeRecordTaskDuration,
		"error":                     "'update system license config' is implemented in enterprise edition",
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
		"error":    "'show user activity' is implemented in enterprise edition",
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
		"error":    "'show user dataset summary' is implemented in enterprise edition",
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
		"error":    "'show user summary' is implemented in enterprise edition",
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
		"error":    "'show user storage' is implemented in enterprise edition",
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
		"error":    "'show user quota' is implemented in enterprise edition",
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
		"error":    "'show user index' is implemented in enterprise edition",
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
		"error":    "'update user role' is implemented in enterprise edition",
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
		"error":    "'show user permission' is implemented in enterprise edition",
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
			"error":    "'list user datasets' is implemented in enterprise edition",
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
			"error":    "'list user agents' is implemented in enterprise edition",
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
			"error":    "'list user chats' is implemented in enterprise edition",
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
			"error":    "'list user searches' is implemented in enterprise edition",
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
			"error":    "'list user models' is implemented in enterprise edition",
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
			"error":    "'list user files' is implemented in enterprise edition",
		},
	}

	return result, nil
}

// ShowUsersSummary show users summary for enterprise edition
func (s *Service) ShowUsersSummary() (map[string]interface{}, error) {
	result := map[string]interface{}{
		"command": "show_users_summary",
		"error":   "'show users summary' is implemented in enterprise edition",
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
		"error":   "'show users activity' is implemented in enterprise edition",
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
		"error":      "'List users reports' is implemented in enterprise edition",
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
		"error":      "'List users storage' is implemented in enterprise edition",
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
		"error":      "'List users documents' is implemented in enterprise edition",
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
		"error":      "'List users index' is implemented in enterprise edition",
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
		"error":           "'List users quota' is implemented in enterprise edition",
	}

	return result, nil
}

// ShowUsersQuotaSummary show users quota summary for enterprise edition
func (s *Service) ShowUsersQuotaSummary() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_users_quota_summary",
		"error":   "'Show users quota summary' is implemented in enterprise edition",
	}

	return result, nil
}

// ShowIngestionTasksSummary show ingestion tasks summary for enterprise edition
func (s *Service) ShowIngestionTasksSummary() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_ingestion_tasks_summary",
		"error":   "'Show ingestion tasks summary' is implemented in enterprise edition",
	}

	return result, nil
}

// ShowDataSummary show data summary for enterprise edition
func (s *Service) ShowDataSummary() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_data_summary",
		"error":   "'Show data summary' is implemented in enterprise edition",
	}

	return result, nil
}

// ShowDataOrphan show data orphan for enterprise edition
func (s *Service) ShowDataOrphan() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_data_orphan",
		"error":   "'Show data orphan' is implemented in enterprise edition",
	}

	return result, nil
}

// ShowDataStorage show data storage for enterprise edition
func (s *Service) ShowDataStorage() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_data_storage",
		"error":   "'Show data storage' is implemented in enterprise edition",
	}

	return result, nil
}

// ShowDataIndex show data index for enterprise edition
func (s *Service) ShowDataIndex() (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "show_data_index",
		"error":   "'Show data index' is implemented in enterprise edition",
	}

	return result, nil
}

// PurgeOrphanData purge orphan data for enterprise edition
func (s *Service) PurgeOrphanData(preview bool) (map[string]interface{}, error) {

	result := map[string]interface{}{
		"command": "purge_orphan_data",
		"preview": preview,
		"error":   "'Purge orphan data' is implemented in enterprise edition",
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
		"error":    "'Purge user data' is implemented in enterprise edition",
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
		"error":    "'Purge users data' is implemented in enterprise edition",
	}

	return result, nil
}

// CreateUserAPIKey create tenant API key for tenant
func (s *Service) CreateUserAPIKey(username string) (map[string]interface{}, error) {

	user, err := s.userDAO.GetByEmail(username)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	result := map[string]interface{}{
		"command":  "create_user_api_key",
		"email":    user.Email,
		"nickname": user.Nickname,
		"error":    "'Create user API key' is implemented in enterprise edition",
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
		"error":    "'Delete user API key' is implemented in enterprise edition",
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
			"error":    "'List user API keys' is implemented in enterprise edition",
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
		"error":   "'List ingestion tasks by condition' is implemented in enterprise edition",
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
		"error":   "'Stop ingestion tasks by condition' is implemented in enterprise edition",
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
		"error":   "'Remove ingestion tasks by condition' is implemented in enterprise edition",
	}

	if email != nil {
		element["email"] = *email
	}
	if status != nil {
		element["status"] = *status
	}

	return []map[string]interface{}{element}, nil
}
