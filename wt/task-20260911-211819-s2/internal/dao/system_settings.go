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
	"context"
	"errors"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// SystemSettingsDAO system settings data access object
type SystemSettingsDAO struct{}

// NewSystemSettingsDAO create system settings DAO instance
func NewSystemSettingsDAO() *SystemSettingsDAO {
	return &SystemSettingsDAO{}
}

// GetAll get all system settings
// Returns all system settings records from database
func (d *SystemSettingsDAO) GetAll(ctx context.Context, db *gorm.DB) ([]entity.SystemSettings, error) {
	var settings []entity.SystemSettings
	err := db.WithContext(ctx).Order("name ASC").Find(&settings).Error
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// GetByName get system settings by name
// Returns settings records that match the given name
func (d *SystemSettingsDAO) GetByName(ctx context.Context, db *gorm.DB, name string) ([]entity.SystemSettings, error) {
	var settings []entity.SystemSettings
	err := db.WithContext(ctx).Where("name = ?", name).Order("name ASC").Find(&settings).Error
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// GetByNamePrefix get system settings by name prefix
// Returns settings records whose names start with the given prefix.
func (d *SystemSettingsDAO) GetByNamePrefix(ctx context.Context, db *gorm.DB, namePrefix string) ([]entity.SystemSettings, error) {
	var settings []entity.SystemSettings
	err := db.WithContext(ctx).Where("name LIKE ?", namePrefix+"%").Order("name ASC").Find(&settings).Error
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// UpdateByName update system settings by name
// Updates the setting with the given name using the provided data
func (d *SystemSettingsDAO) UpdateByName(ctx context.Context, db *gorm.DB, name string, setting *entity.SystemSettings) error {
	return db.WithContext(ctx).Model(&entity.SystemSettings{}).
		Where("name = ?", name).
		Updates(map[string]interface{}{
			"value":     setting.Value,
			"source":    setting.Source,
			"data_type": setting.DataType,
		}).Error
}

// Create a new system setting
// Inserts a new system setting record into database
func (d *SystemSettingsDAO) Create(ctx context.Context, db *gorm.DB, setting *entity.SystemSettings) error {
	return db.WithContext(ctx).Create(setting).Error
}

// SaveOrCreate update existing setting or create new one
// If setting exists, updates it; otherwise creates a new record
func (d *SystemSettingsDAO) SaveOrCreate(ctx context.Context, db *gorm.DB, name string, value string, source string, dataType string) error {
	settings, err := d.GetByName(ctx, db, name)
	if err != nil {
		return err
	}

	if len(settings) == 1 {
		setting := &settings[0]
		setting.Value = value
		setting.Source = source
		setting.DataType = dataType
		return d.UpdateByName(ctx, db, name, setting)
	} else if len(settings) > 1 {
		return errors.New("can't update more than 1 setting: " + name)
	}

	newSetting := &entity.SystemSettings{
		Name:     name,
		Value:    value,
		Source:   source,
		DataType: dataType,
	}
	return d.Create(ctx, db, newSetting)
}

// Count get total count of system settings
func (d *SystemSettingsDAO) Count(ctx context.Context, db *gorm.DB) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Model(&entity.SystemSettings{}).Count(&count).Error
	return count, err
}

// DeleteByName delete system setting by name
func (d *SystemSettingsDAO) DeleteByName(ctx context.Context, db *gorm.DB, name string) error {
	return db.WithContext(ctx).Where("name = ?", name).Delete(&entity.SystemSettings{}).Error
}

// Exists check if setting exists by name
func (d *SystemSettingsDAO) Exists(ctx context.Context, db *gorm.DB, name string) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&entity.SystemSettings{}).Where("name = ?", name).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetBySource get system settings by source
func (d *SystemSettingsDAO) GetBySource(ctx context.Context, db *gorm.DB, source string) ([]entity.SystemSettings, error) {
	var settings []entity.SystemSettings
	err := db.WithContext(ctx).Where("source = ?", source).Find(&settings).Error
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// GetByDataType get system settings by data type
func (d *SystemSettingsDAO) GetByDataType(ctx context.Context, db *gorm.DB, dataType string) ([]entity.SystemSettings, error) {
	var settings []entity.SystemSettings
	err := db.WithContext(ctx).Where("data_type = ?", dataType).Find(&settings).Error
	if err != nil {
		return nil, err
	}
	return settings, nil
}

// Transaction execute operations in a transaction
func (d *SystemSettingsDAO) Transaction(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(fn)
}

// CreateWithTx create setting within transaction
func (d *SystemSettingsDAO) CreateWithTx(ctx context.Context, tx *gorm.DB, setting *entity.SystemSettings) error {
	return tx.WithContext(ctx).Create(setting).Error
}

// UpdateByNameWithTx update setting within transaction
func (d *SystemSettingsDAO) UpdateByNameWithTx(ctx context.Context, tx *gorm.DB, name string, setting *entity.SystemSettings) error {
	return tx.WithContext(ctx).Model(&entity.SystemSettings{}).
		Where("name = ?", name).
		Updates(map[string]interface{}{
			"value":     setting.Value,
			"source":    setting.Source,
			"data_type": setting.DataType,
		}).Error
}
