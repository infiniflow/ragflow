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

	"gorm.io/gorm"
)

// APITokenDAO API token data access object
type APITokenDAO struct{}

// NewAPITokenDAO create API token DAO
func NewAPITokenDAO() *APITokenDAO {
	return &APITokenDAO{}
}

// Create creates a new API token
func (dao *APITokenDAO) Create(ctx context.Context, db *gorm.DB, apiToken *entity.APIToken) error {
	return db.WithContext(ctx).Create(apiToken).Error
}

// GetByTenantID gets API tokens by tenant ID
func (dao *APITokenDAO) GetByTenantID(ctx context.Context, db *gorm.DB, tenantID string) ([]*entity.APIToken, error) {
	var tokens []*entity.APIToken
	err := db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&tokens).Error
	return tokens, err
}

// DeleteByTenantID deletes all API tokens by tenant ID (hard delete)
func (dao *APITokenDAO) DeleteByTenantID(ctx context.Context, db *gorm.DB, tenantID string) (int64, error) {
	result := db.WithContext(ctx).Where("tenant_id = ?", tenantID).Delete(&entity.APIToken{})
	return result.RowsAffected, result.Error
}

// GetByAPIToken gets user by API token
func (dao *APITokenDAO) GetByAPIToken(ctx context.Context, db *gorm.DB, token string) (*entity.APIToken, error) {
	var apiToken entity.APIToken
	tx := db.WithContext(ctx).Where("token = ?", token).Find(&apiToken)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, nil
	}
	return &apiToken, nil
}

// GetByBeta gets API tokens by beta key (SDK/bot authorization token).
// Mirrors Python's APIToken.query(beta=token), which returns a list.
func (dao *APITokenDAO) GetByBeta(ctx context.Context, db *gorm.DB, beta string) ([]*entity.APIToken, error) {
	var tokens []*entity.APIToken
	err := db.WithContext(ctx).Where("beta = ?", beta).Find(&tokens).Error
	return tokens, err
}

// DeleteByDialogIDs deletes API tokens by dialog IDs (hard delete)
func (dao *APITokenDAO) DeleteByDialogIDs(ctx context.Context, db *gorm.DB, dialogIDs []string) (int64, error) {
	if len(dialogIDs) == 0 {
		return 0, nil
	}
	result := db.WithContext(ctx).Where("dialog_id IN ?", dialogIDs).Delete(&entity.APIToken{})
	return result.RowsAffected, result.Error
}

// DeleteByTenantIDAndToken deletes a specific API token by tenant ID and token value
func (dao *APITokenDAO) DeleteByTenantIDAndToken(ctx context.Context, db *gorm.DB, tenantID, token string) (int64, error) {
	result := db.WithContext(ctx).Where("tenant_id = ? AND token = ?", tenantID, token).Delete(&entity.APIToken{})
	return result.RowsAffected, result.Error
}
