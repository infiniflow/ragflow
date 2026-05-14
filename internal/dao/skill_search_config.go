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

// SkillSearchConfigDAO data access object for skill search config
type SkillSearchConfigDAO struct{}

const defaultSkillSpaceID = "default"

func normalizeSpaceID(spaceID string) string {
	spaceID = strings.TrimSpace(spaceID)
	if spaceID == "" {
		return defaultSkillSpaceID
	}
	return spaceID
}

// NewSkillSearchConfigDAO creates a new SkillSearchConfigDAO
func NewSkillSearchConfigDAO() *SkillSearchConfigDAO {
	return &SkillSearchConfigDAO{}
}

// Create creates a new skill search config
func (dao *SkillSearchConfigDAO) Create(config *entity.SkillSearchConfig) error {
	return DB.Create(config).Error
}

// GetByID retrieves a skill search config by ID
func (dao *SkillSearchConfigDAO) GetByID(id string) (*entity.SkillSearchConfig, error) {
	var config entity.SkillSearchConfig
	err := DB.Where("id = ? AND status = ?", id, "1").First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetByTenantID retrieves a skill search config by tenant ID
func (dao *SkillSearchConfigDAO) GetByTenantID(tenantID, spaceID string) (*entity.SkillSearchConfig, error) {
	var config entity.SkillSearchConfig
	err := DB.Where("tenant_id = ? AND space_id = ? AND status = ?", tenantID, normalizeSpaceID(spaceID), "1").First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetLatestByTenantID retrieves the latest skill search config by tenant ID (ordered by update_time desc)
// Prioritizes configs with non-empty embd_id to return user-saved configs over auto-created ones
func (dao *SkillSearchConfigDAO) GetLatestByTenantID(tenantID, spaceID string) (*entity.SkillSearchConfig, error) {
	var config entity.SkillSearchConfig
	// First try to get the latest config with non-empty embd_id (user-saved config)
	err := DB.Where("tenant_id = ? AND space_id = ? AND status = ? AND embd_id != ?", tenantID, normalizeSpaceID(spaceID), "1", "").Order("update_time desc").First(&config).Error
	if err == nil {
		return &config, nil
	}
	// If no user-saved config found, get any config
	err = DB.Where("tenant_id = ? AND space_id = ? AND status = ?", tenantID, normalizeSpaceID(spaceID), "1").Order("update_time desc").First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetByTenantAndEmbdID retrieves a skill search config by tenant ID and embedding ID
func (dao *SkillSearchConfigDAO) GetByTenantAndEmbdID(tenantID, spaceID, embdID string) (*entity.SkillSearchConfig, error) {
	var config entity.SkillSearchConfig
	err := DB.Where("tenant_id = ? AND space_id = ? AND embd_id = ? AND status = ?", tenantID, normalizeSpaceID(spaceID), embdID, "1").First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetOrCreate retrieves existing config or creates default one
func (dao *SkillSearchConfigDAO) GetOrCreate(tenantID, spaceID, embdID string) (*entity.SkillSearchConfig, error) {
	spaceID = normalizeSpaceID(spaceID)
	config, err := dao.GetByTenantAndEmbdID(tenantID, spaceID, embdID)
	if err == nil {
		return config, nil
	}

	// Create default config
	return dao.CreateWithTenantSpace(tenantID, spaceID, embdID)
}

// CreateWithTenantSpace creates a new config for tenant+space
func (dao *SkillSearchConfigDAO) CreateWithTenantSpace(tenantID, spaceID, embdID string) (*entity.SkillSearchConfig, error) {
	spaceID = normalizeSpaceID(spaceID)
	timestamp := time.Now().UnixMilli()
	defaultFieldConfig := entity.DefaultFieldConfig()
	fieldConfigMap := entity.JSONMap{
		"name": map[string]interface{}{
			"enabled": defaultFieldConfig.Name.Enabled,
			"weight":  defaultFieldConfig.Name.Weight,
		},
		"tags": map[string]interface{}{
			"enabled": defaultFieldConfig.Tags.Enabled,
			"weight":  defaultFieldConfig.Tags.Weight,
		},
		"description": map[string]interface{}{
			"enabled": defaultFieldConfig.Description.Enabled,
			"weight":  defaultFieldConfig.Description.Weight,
		},
		"content": map[string]interface{}{
			"enabled": defaultFieldConfig.Content.Enabled,
			"weight":  defaultFieldConfig.Content.Weight,
		},
	}

	defaultConfig := &entity.SkillSearchConfig{
		ID:                     generateID(),
		TenantID:               tenantID,
		SpaceID:                spaceID,
		EmbdID:                 embdID,
		VectorSimilarityWeight: 0.3,
		SimilarityThreshold:    0.2,
		FieldConfig:            fieldConfigMap,
		TopK:                   10,
		Status:                 "1",
		CreateTime:             &timestamp,
	}

	if err := dao.Create(defaultConfig); err != nil {
		return nil, err
	}
	return defaultConfig, nil
}

// DeleteAllByTenantSpace deletes all configs for a tenant+space (for cleanup before creating new one)
func (dao *SkillSearchConfigDAO) DeleteAllByTenantSpace(tenantID, spaceID string) error {
	spaceID = normalizeSpaceID(spaceID)
	return DB.Model(&entity.SkillSearchConfig{}).
		Where("tenant_id = ? AND space_id = ?", tenantID, spaceID).
		Update("status", "0").Error
}

// DeleteAllByTenantSpaceExceptID deletes all active configs for a tenant+space except the specified ID
func (dao *SkillSearchConfigDAO) DeleteAllByTenantSpaceExceptID(tenantID, spaceID, exceptID string) error {
	spaceID = normalizeSpaceID(spaceID)
	return DB.Model(&entity.SkillSearchConfig{}).
		Where("tenant_id = ? AND space_id = ? AND id != ? AND status = ?", tenantID, spaceID, exceptID, "1").
		Update("status", "0").Error
}

// Update updates a skill search config with the given updates map
func (dao *SkillSearchConfigDAO) Update(id string, updates map[string]interface{}) error {
	updates["update_time"] = time.Now()
	return DB.Model(&entity.SkillSearchConfig{}).Where("id = ? AND status = ?", id, "1").Updates(updates).Error
}

// UpdateByTenantID updates config by tenant ID
func (dao *SkillSearchConfigDAO) UpdateByTenantID(tenantID, spaceID string, updates map[string]interface{}) error {
	updates["update_time"] = time.Now()
	result := DB.Model(&entity.SkillSearchConfig{}).Where("tenant_id = ? AND space_id = ? AND status = ?", tenantID, normalizeSpaceID(spaceID), "1").Updates(updates)
	return result.Error
}

// UpdateByTenantAndEmbdID updates config by tenant ID and embedding ID
func (dao *SkillSearchConfigDAO) UpdateByTenantAndEmbdID(tenantID, spaceID, embdID string, updates map[string]interface{}) error {
	updates["update_time"] = time.Now()
	result := DB.Model(&entity.SkillSearchConfig{}).Where("tenant_id = ? AND space_id = ? AND embd_id = ? AND status = ?", tenantID, normalizeSpaceID(spaceID), embdID, "1").Updates(updates)
	return result.Error
}

// Delete deletes a skill search config by ID (soft delete)
func (dao *SkillSearchConfigDAO) Delete(id string) error {
	return DB.Model(&entity.SkillSearchConfig{}).Where("id = ?", id).Update("status", "0").Error
}

// generateID generates a unique ID
func generateID() string {
	return strings.ReplaceAll(uuid.New().String(), "-", "")[:32]
}
