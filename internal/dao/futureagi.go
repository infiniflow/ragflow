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

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"ragflow/internal/entity"
)

// FutureAGIDAO is the data access object for tenant Future AGI credentials.
type FutureAGIDAO struct{}

// NewFutureAGI creates a new Future AGI DAO.
func NewFutureAGI() *FutureAGIDAO {
	return &FutureAGIDAO{}
}

// GetByTenantID returns the tenant's row, or (nil, nil) when there is none.
func (dao *FutureAGIDAO) GetByTenantID(ctx context.Context, db *gorm.DB, tenantID string) (*entity.TenantFutureAGI, error) {
	var row entity.TenantFutureAGI
	err := db.WithContext(ctx).Where("tenant_id = ?", tenantID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// SaveByTenantID inserts the row or replaces the stored settings.
func (dao *FutureAGIDAO) SaveByTenantID(ctx context.Context, db *gorm.DB, row *entity.TenantFutureAGI) error {
	return db.WithContext(ctx).Session(&gorm.Session{Logger: db.Logger.LogMode(logger.Silent)}).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"api_key":      row.APIKey,
			"secret_key":   row.SecretKey,
			"endpoint":     row.Endpoint,
			"project_name": row.ProjectName,
			"redact":       row.Redact,
		}),
	}).Create(row).Error
}

// DeleteByTenantID removes the tenant's row. It returns gorm.ErrRecordNotFound
// when there is none.
func (dao *FutureAGIDAO) DeleteByTenantID(ctx context.Context, db *gorm.DB, tenantID string) error {
	res := db.WithContext(ctx).Where("tenant_id = ?", tenantID).Delete(&entity.TenantFutureAGI{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
