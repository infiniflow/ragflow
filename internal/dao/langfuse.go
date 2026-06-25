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
	"errors"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// LangfuseDAO is the data access object for tenant Langfuse credentials.
type LangfuseDAO struct{}

// NewLangfuse creates a new Langfuse DAO.
func NewLangfuse() *LangfuseDAO {
	return &LangfuseDAO{}
}

// GetByTenantID returns the Langfuse credentials row for a tenant.
// It returns (nil, nil) when no row exists, mirroring the Python
// TenantLangfuseService.filter_by_tenant behaviour (DoesNotExist -> None).
func (dao *LangfuseDAO) GetByTenantID(tenantID string) (*entity.TenantLangfuse, error) {
	return dao.getByTenantID(DB, tenantID)
}

func (dao *LangfuseDAO) getByTenantID(db *gorm.DB, tenantID string) (*entity.TenantLangfuse, error) {
	var row entity.TenantLangfuse
	err := db.Where("tenant_id = ?", tenantID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// Create inserts a new Langfuse credentials row (mirrors save).
func (dao *LangfuseDAO) Create(row *entity.TenantLangfuse) error {
	return dao.create(DB, row)
}

func (dao *LangfuseDAO) create(db *gorm.DB, row *entity.TenantLangfuse) error {
	return db.Create(row).Error
}

// UpdateByTenantID updates the Langfuse credentials row for a tenant
func (dao *LangfuseDAO) UpdateByTenantID(tenantID string, updates map[string]any) error {
	return dao.updateByTenantID(DB, tenantID, updates)
}

func (dao *LangfuseDAO) updateByTenantID(db *gorm.DB, tenantID string, updates map[string]any) error {
	res := db.Model(&entity.TenantLangfuse{}).Where("tenant_id = ?", tenantID).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeleteByTenantID deletes the Langfuse credentials row for a tenant
// (mirrors delete_model / delete_ty_tenant_id).
func (dao *LangfuseDAO) DeleteByTenantID(tenantID string) error {
	return dao.deleteByTenantID(DB, tenantID)
}

func (dao *LangfuseDAO) deleteByTenantID(db *gorm.DB, tenantID string) error {
	res := db.Where("tenant_id = ?", tenantID).Delete(&entity.TenantLangfuse{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (dao *LangfuseDAO) SaveByTenantID(row *entity.TenantLangfuse) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		existing, err := dao.getByTenantID(tx, row.TenantID)
		if err != nil {
			return err
		}
		if existing == nil {
			return dao.create(tx, row)
		}
		return dao.updateByTenantID(tx, row.TenantID, map[string]any{
			"secret_key": row.SecretKey,
			"public_key": row.PublicKey,
			"host":       row.Host,
		})
	})
}

func (dao *LangfuseDAO) DeleteExistingByTenantID(tenantID string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		row, err := dao.getByTenantID(tx, tenantID)
		if err != nil {
			return err
		}
		if row == nil {
			return gorm.ErrRecordNotFound
		}
		return tx.Delete(row).Error
	})
}
