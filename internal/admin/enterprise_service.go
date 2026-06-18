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
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

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
