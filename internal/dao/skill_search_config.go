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

const defaultSkillHubID = "default"

func normalizeHubID(hubID string) string {
	hubID = strings.TrimSpace(hubID)
	if hubID == "" {
		return defaultSkillHubID
	}
	return hubID
}

// NewSkillSearchConfigDAO creates a new SkillSearchConfigDAO
func NewSkillSearchConfigDAO() *SkillSearchConfigDAO {
	return &SkillSearchConfigDAO{}
}

// Create creates a new skill search config
func (dao *SkillSearchConfigDAO) Create(config *entity.SkillSearchConfig) error {
	return DB.Create(config).Error
}

// GetByTenantID retrieves a skill search config by tenant ID
func (dao *SkillSearchConfigDAO) GetByTenantID(tenantID, hubID string) (*entity.SkillSearchConfig, error) {
	var config entity.SkillSearchConfig
	err := DB.Where("tenant_id = ? AND hub_id = ? AND status = ?", tenantID, normalizeHubID(hubID), "1").First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetLatestByTenantID retrieves the latest skill search config by tenant ID (ordered by update_time desc)
// Prioritizes configs with non-empty embd_id to return user-saved configs over auto-created ones
func (dao *SkillSearchConfigDAO) GetLatestByTenantID(tenantID, hubID string) (*entity.SkillSearchConfig, error) {
	var config entity.SkillSearchConfig
	// First try to get the latest config with non-empty embd_id (user-saved config)
	err := DB.Where("tenant_id = ? AND hub_id = ? AND status = ? AND embd_id != ?", tenantID, normalizeHubID(hubID), "1", "").Order("update_time desc").First(&config).Error
	if err == nil {
		return &config, nil
	}
	// If no user-saved config found, get any config
	err = DB.Where("tenant_id = ? AND hub_id = ? AND status = ?", tenantID, normalizeHubID(hubID), "1").Order("update_time desc").First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetByTenantAndEmbdID retrieves a skill search config by tenant ID and embedding ID
func (dao *SkillSearchConfigDAO) GetByTenantAndEmbdID(tenantID, hubID, embdID string) (*entity.SkillSearchConfig, error) {
	var config entity.SkillSearchConfig
	err := DB.Where("tenant_id = ? AND hub_id = ? AND embd_id = ? AND status = ?", tenantID, normalizeHubID(hubID), embdID, "1").First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// GetOrCreate retrieves existing config or creates default one
func (dao *SkillSearchConfigDAO) GetOrCreate(tenantID, hubID, embdID string) (*entity.SkillSearchConfig, error) {
	hubID = normalizeHubID(hubID)
	config, err := dao.GetByTenantAndEmbdID(tenantID, hubID, embdID)
	if err == nil {
		return config, nil
	}

	// Create default config
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
		HubID:                  hubID,
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

// Update updates a skill search config
func (dao *SkillSearchConfigDAO) Update(config *entity.SkillSearchConfig) error {
	return DB.Save(config).Error
}

// UpdateByTenantID updates config by tenant ID
func (dao *SkillSearchConfigDAO) UpdateByTenantID(tenantID, hubID string, updates map[string]interface{}) error {
	updates["update_time"] = time.Now()
	result := DB.Model(&entity.SkillSearchConfig{}).Where("tenant_id = ? AND hub_id = ? AND status = ?", tenantID, normalizeHubID(hubID), "1").Updates(updates)
	return result.Error
}

// UpdateByTenantAndEmbdID updates config by tenant ID and embedding ID
func (dao *SkillSearchConfigDAO) UpdateByTenantAndEmbdID(tenantID, hubID, embdID string, updates map[string]interface{}) error {
	updates["update_time"] = time.Now()
	result := DB.Model(&entity.SkillSearchConfig{}).Where("tenant_id = ? AND hub_id = ? AND embd_id = ? AND status = ?", tenantID, normalizeHubID(hubID), embdID, "1").Updates(updates)
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
