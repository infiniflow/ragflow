// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dao

import (
	"context"
	"errors"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PlatformModelSettings stores model defaults and serializes provider creation.
type PlatformModelSettings struct{}

func (PlatformModelSettings) Default(ctx context.Context, db *gorm.DB, modelType string) (string, error) {
	var setting entity.SystemSettings
	err := db.WithContext(ctx).Take(&setting, "name = ?", entity.PlatformModelDefaultPrefix+modelType).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (PlatformModelSettings) SetDefault(ctx context.Context, db *gorm.DB, modelType, modelID string) error {
	name := entity.PlatformModelDefaultPrefix + modelType
	if modelID == "" {
		return db.WithContext(ctx).Where("name = ?", name).Delete(&entity.SystemSettings{}).Error
	}
	setting := entity.SystemSettings{Name: name, Value: modelID, Source: "admin", DataType: "string"}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "source", "data_type"}),
	}).Create(&setting).Error
}

// ValidatePlatformTenant rejects tenant and membership records using the reserved ID.
func (PlatformModelSettings) ValidatePlatformTenant(ctx context.Context, db *gorm.DB) error {
	for _, model := range []any{&entity.Tenant{}, &entity.UserTenant{}, &entity.User{}} {
		column := "tenant_id"
		switch model.(type) {
		case *entity.Tenant, *entity.User:
			column = "id"
		}
		var count int64
		if err := db.WithContext(ctx).Model(model).Where(column+" = ?", entity.PlatformTenantID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("platform tenant ID is already in use")
		}
	}
	return nil
}

// lockPlatform serializes initialization and mutations across processes.
func lockPlatform(ctx context.Context, tx *gorm.DB) error {
	lock := entity.SystemSettings{Name: entity.PlatformProviderLockSetting, Value: "initializing", Source: "system", DataType: "string"}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&lock).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&lock, "name = ?", entity.PlatformProviderLockSetting).Error; err != nil {
		return err
	}
	if err := (PlatformModelSettings{}).ValidatePlatformTenant(ctx, tx); err != nil {
		return err
	}
	if lock.Source != "system" || lock.DataType != "string" {
		return errors.New("invalid platform initialization marker")
	}
	switch lock.Value {
	case "ready":
		return nil
	case "initializing":
		var reserved int64
		if err := tx.Model(&entity.SystemSettings{}).Where("SUBSTR(name, 1, ?) = ?", len(entity.PlatformModelDefaultPrefix), entity.PlatformModelDefaultPrefix).Count(&reserved).Error; err != nil {
			return err
		}
		if reserved != 0 {
			return errors.New("unrecognized platform default settings")
		}
		var count int64
		if err := tx.Model(&entity.TenantModelProvider{}).Where("tenant_id = ?", entity.PlatformTenantID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return errors.New("unrecognized platform provider records")
		}
		return tx.Model(&entity.SystemSettings{}).Where("name = ?", entity.PlatformProviderLockSetting).Update("value", "ready").Error
	default:
		return errors.New("invalid platform initialization marker")
	}
}

// Initialize validates and reserves the platform identity.
func (PlatformModelSettings) Initialize(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return lockPlatform(ctx, tx) })
}

// CreateProvider serializes platform provider creation across server processes.
func (PlatformModelSettings) CreateProvider(ctx context.Context, db *gorm.DB, name, userID string) (*entity.TenantModelProvider, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.TrimSpace(userID) == "" {
		return nil, errors.New("provider name and creator are required")
	}
	var result entity.TenantModelProvider
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatform(ctx, tx); err != nil {
			return err
		}
		err := tx.Where("tenant_id = ? AND LOWER(provider_name) = LOWER(?)", entity.PlatformTenantID, name).Take(&result).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		result = entity.TenantModelProvider{ID: common.GenerateToken(), TenantID: entity.PlatformTenantID, UserID: userID, ProviderName: name}
		return tx.Create(&result).Error
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
