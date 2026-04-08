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
	"ragflow/internal/entity"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SkillsHubDAO data access object for skills hub
type SkillsHubDAO struct{}

// NewSkillsHubDAO creates a new SkillsHubDAO
func NewSkillsHubDAO() *SkillsHubDAO {
	return &SkillsHubDAO{}
}

// Create creates a new skills hub
func (dao *SkillsHubDAO) Create(hub *entity.SkillsHub) error {
	return DB.Create(hub).Error
}

// GetByID retrieves a skills hub by ID (active only)
func (dao *SkillsHubDAO) GetByID(id string) (*entity.SkillsHub, error) {
	var hub entity.SkillsHub
	err := DB.Where("id = ? AND status = ?", id, entity.HubStatusActive).First(&hub).Error
	if err != nil {
		return nil, err
	}
	return &hub, nil
}

// GetByTenantID retrieves all skills hubs by tenant ID (active only)
func (dao *SkillsHubDAO) GetByTenantID(tenantID string) ([]*entity.SkillsHub, error) {
	var hubs []*entity.SkillsHub
	err := DB.Where("tenant_id = ? AND status = ?", tenantID, entity.HubStatusActive).Order("create_time DESC").Find(&hubs).Error
	return hubs, err
}

// GetByTenantAndName retrieves a skills hub by tenant ID and name (active only)
func (dao *SkillsHubDAO) GetByTenantAndName(tenantID, name string) (*entity.SkillsHub, error) {
	var hub entity.SkillsHub
	err := DB.Where("tenant_id = ? AND name = ? AND status = ?", tenantID, name, entity.HubStatusActive).First(&hub).Error
	if err != nil {
		return nil, err
	}
	return &hub, nil
}

// GetByTenantAndNameAnyStatus retrieves a skills hub by tenant ID and name regardless of status
func (dao *SkillsHubDAO) GetByTenantAndNameAnyStatus(tenantID, name string) (*entity.SkillsHub, error) {
	var hub entity.SkillsHub
	err := DB.Where("tenant_id = ? AND name = ?", tenantID, name).First(&hub).Error
	if err != nil {
		return nil, err
	}
	return &hub, nil
}

// GetByIDAnyStatus retrieves a skills hub by ID regardless of status
func (dao *SkillsHubDAO) GetByIDAnyStatus(id string) (*entity.SkillsHub, error) {
	var hub entity.SkillsHub
	err := DB.Where("id = ?", id).First(&hub).Error
	if err != nil {
		return nil, err
	}
	return &hub, nil
}

// GetByFolderID retrieves a skills hub by folder ID (active only)
func (dao *SkillsHubDAO) GetByFolderID(folderID string) (*entity.SkillsHub, error) {
	var hub entity.SkillsHub
	err := DB.Where("folder_id = ? AND status = ?", folderID, entity.HubStatusActive).First(&hub).Error
	if err != nil {
		return nil, err
	}
	return &hub, nil
}

// Update updates a skills hub
func (dao *SkillsHubDAO) Update(hub *entity.SkillsHub) error {
	return DB.Save(hub).Error
}

// UpdateByID updates skills hub by ID
func (dao *SkillsHubDAO) UpdateByID(id string, updates map[string]interface{}) error {
	updates["update_time"] = time.Now()
	return DB.Model(&entity.SkillsHub{}).Where("id = ?", id).Updates(updates).Error
}

// Delete deletes a skills hub by ID (soft delete)
func (dao *SkillsHubDAO) Delete(id string) error {
	return DB.Model(&entity.SkillsHub{}).Where("id = ?", id).Update("status", entity.HubStatusDeleted).Error
}

// CASStatus performs a compare-and-swap on the hub status atomically
// Returns true if the update was applied, false if the current status didn't match expected
func (dao *SkillsHubDAO) CASStatus(id string, expectedStatus, newStatus string) (bool, error) {
	result := DB.Model(&entity.SkillsHub{}).
		Where("id = ? AND status = ?", id, expectedStatus).
		Update("status", newStatus)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// DeletePermanentByName permanently deletes a skills hub by tenant ID and name
// This is used to clean up previously deleted hubs (only deletes status='0' deleted hubs, NOT deleting hubs)
func (dao *SkillsHubDAO) DeletePermanentByName(tenantID, name string) error {
	return DB.Unscoped().Where("tenant_id = ? AND name = ? AND status = ?", tenantID, name, entity.HubStatusDeleted).Delete(&entity.SkillsHub{}).Error
}

// CountByTenant counts skills hubs by tenant ID
func (dao *SkillsHubDAO) CountByTenant(tenantID string) (int64, error) {
	var count int64
	err := DB.Model(&entity.SkillsHub{}).Where("tenant_id = ? AND status = ?", tenantID, entity.HubStatusActive).Count(&count).Error
	return count, err
}

// generateID generates a unique ID
func generateHubID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")[:32]
}
