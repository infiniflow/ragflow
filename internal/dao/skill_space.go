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

// SkillSpaceDAO data access object for skills space
type SkillSpaceDAO struct{}

// NewSkillSpaceDAO creates a new SkillSpaceDAO
func NewSkillSpaceDAO() *SkillSpaceDAO {
	return &SkillSpaceDAO{}
}

// Create creates a new skills space
func (dao *SkillSpaceDAO) Create(space *entity.SkillSpace) error {
	return DB.Create(space).Error
}

// GetByID retrieves a skills space by ID (active only)
func (dao *SkillSpaceDAO) GetByID(id string) (*entity.SkillSpace, error) {
	var space entity.SkillSpace
	err := DB.Where("id = ? AND status = ?", id, entity.SpaceStatusActive).First(&space).Error
	if err != nil {
		return nil, err
	}
	return &space, nil
}

// GetByTenantID retrieves all skills spaces by tenant ID (active only)
func (dao *SkillSpaceDAO) GetByTenantID(tenantID string) ([]*entity.SkillSpace, error) {
	var spaces []*entity.SkillSpace
	err := DB.Where("tenant_id = ? AND status = ?", tenantID, entity.SpaceStatusActive).Order("create_time DESC").Find(&spaces).Error
	return spaces, err
}

// GetByTenantAndName retrieves a skills space by tenant ID and name (active only)
func (dao *SkillSpaceDAO) GetByTenantAndName(tenantID, name string) (*entity.SkillSpace, error) {
	var space entity.SkillSpace
	err := DB.Where("tenant_id = ? AND name = ? AND status = ?", tenantID, name, entity.SpaceStatusActive).First(&space).Error
	if err != nil {
		return nil, err
	}
	return &space, nil
}

// GetByTenantAndNameAnyStatus retrieves a skills space by tenant ID and name regardless of status
func (dao *SkillSpaceDAO) GetByTenantAndNameAnyStatus(tenantID, name string) (*entity.SkillSpace, error) {
	var space entity.SkillSpace
	err := DB.Where("tenant_id = ? AND name = ?", tenantID, name).First(&space).Error
	if err != nil {
		return nil, err
	}
	return &space, nil
}

// GetByIDAnyStatus retrieves a skills space by ID regardless of status
func (dao *SkillSpaceDAO) GetByIDAnyStatus(id string) (*entity.SkillSpace, error) {
	var space entity.SkillSpace
	err := DB.Where("id = ?", id).First(&space).Error
	if err != nil {
		return nil, err
	}
	return &space, nil
}

// GetByFolderID retrieves a skills space by folder ID (active only)
func (dao *SkillSpaceDAO) GetByFolderID(folderID string) (*entity.SkillSpace, error) {
	var space entity.SkillSpace
	err := DB.Where("folder_id = ? AND status = ?", folderID, entity.SpaceStatusActive).First(&space).Error
	if err != nil {
		return nil, err
	}
	return &space, nil
}

// Update updates a skills space
func (dao *SkillSpaceDAO) Update(space *entity.SkillSpace) error {
	return DB.Save(space).Error
}

// UpdateByID updates skills space by ID
func (dao *SkillSpaceDAO) UpdateByID(id string, updates map[string]interface{}) error {
	updates["update_time"] = time.Now()
	return DB.Model(&entity.SkillSpace{}).Where("id = ?", id).Updates(updates).Error
}

// Delete deletes a skills space by ID (soft delete)
func (dao *SkillSpaceDAO) Delete(id string) error {
	return DB.Model(&entity.SkillSpace{}).Where("id = ?", id).Update("status", entity.SpaceStatusDeleted).Error
}

// CASStatus performs a compare-and-swap on the space status atomically
// Returns true if the update was applied, false if the current status didn't match expected
func (dao *SkillSpaceDAO) CASStatus(id string, expectedStatus, newStatus string) (bool, error) {
	result := DB.Model(&entity.SkillSpace{}).
		Where("id = ? AND status = ?", id, expectedStatus).
		Update("status", newStatus)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// DeletePermanentByName permanently deletes a skills space by tenant ID and name
// This is used to clean up previously deleted spaces (only deletes status='0' deleted spaces, NOT deleting spaces)
func (dao *SkillSpaceDAO) DeletePermanentByName(tenantID, name string) error {
	return DB.Unscoped().Where("tenant_id = ? AND name = ? AND status = ?", tenantID, name, entity.SpaceStatusDeleted).Delete(&entity.SkillSpace{}).Error
}

// CountByTenant counts skills spaces by tenant ID
func (dao *SkillSpaceDAO) CountByTenant(tenantID string) (int64, error) {
	var count int64
	err := DB.Model(&entity.SkillSpace{}).Where("tenant_id = ? AND status = ?", tenantID, entity.SpaceStatusActive).Count(&count).Error
	return count, err
}

// generateSpaceID generates a unique ID
func generateSpaceID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")[:32]
}
