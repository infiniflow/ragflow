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
	"ragflow/internal/entity"
	"time"

	"gorm.io/gorm"
)

// LicenseDAO license data access object
type LicenseDAO struct{}

// NewLicenseDAO create license DAO
func NewLicenseDAO() *LicenseDAO {
	return &LicenseDAO{}
}

// Create creates a new license record
func (dao *LicenseDAO) Create(ctx context.Context, db *gorm.DB, licenseID, licenseStr string) error {
	license := entity.License{
		ID:        licenseID,
		License:   licenseStr,
		CreatedAt: time.Now(),
	}
	return db.WithContext(ctx).Create(license).Error
}

// GetLatest gets the latest license record by creation time
func (dao *LicenseDAO) GetLatest(ctx context.Context, db *gorm.DB) (*entity.License, error) {
	var license entity.License
	err := db.WithContext(ctx).Order("created_at DESC").First(&license).Error
	if err != nil {
		return nil, err
	}
	return &license, nil
}
