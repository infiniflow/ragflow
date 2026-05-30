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
)

// APITokenDAO API token data access object
type APITokenDAO struct{}

// NewAPITokenDAO create API token DAO
func NewAPITokenDAO() *APITokenDAO {
	return &APITokenDAO{}
}

// Create creates a new API token
func (dao *APITokenDAO) Create(apiToken *entity.APIToken) error {
	return DB.Create(apiToken).Error
}

// GetByTenantID gets API tokens by tenant ID
func (dao *APITokenDAO) GetByTenantID(tenantID string) ([]*entity.APIToken, error) {
	var tokens []*entity.APIToken
	err := DB.Where("tenant_id = ?", tenantID).Find(&tokens).Error
	return tokens, err
}

// DeleteByTenantID deletes all API tokens by tenant ID (hard delete)
func (dao *APITokenDAO) DeleteByTenantID(tenantID string) (int64, error) {
	result := DB.Unscoped().Where("tenant_id = ?", tenantID).Delete(&entity.APIToken{})
	return result.RowsAffected, result.Error
}

// GetByToken gets API token by access key
func (dao *APITokenDAO) GetUserByAPIToken(token string) (*entity.APIToken, error) {
	var apiToken entity.APIToken
	err := DB.Where("token = ?", token).First(&apiToken).Error
	if err != nil {
		return nil, err
	}
	return &apiToken, nil
}

// DeleteByDialogIDs deletes API tokens by dialog IDs (hard delete)
func (dao *APITokenDAO) DeleteByDialogIDs(dialogIDs []string) (int64, error) {
	if len(dialogIDs) == 0 {
		return 0, nil
	}
	result := DB.Unscoped().Where("dialog_id IN ?", dialogIDs).Delete(&entity.APIToken{})
	return result.RowsAffected, result.Error
}

// DeleteByTenantIDAndToken deletes a specific API token by tenant ID and token value
func (dao *APITokenDAO) DeleteByTenantIDAndToken(tenantID, token string) (int64, error) {
	result := DB.Unscoped().Where("tenant_id = ? AND token = ?", tenantID, token).Delete(&entity.APIToken{})
	return result.RowsAffected, result.Error
}

// API4ConversationDAO API for conversation data access object
type API4ConversationDAO struct{}

// NewAPI4ConversationDAO create API4Conversation DAO
func NewAPI4ConversationDAO() *API4ConversationDAO {
	return &API4ConversationDAO{}
}

// ConversationStatsRow is one daily aggregate row for api_4_conversation.
type ConversationStatsRow struct {
	Dt       string  `gorm:"column:dt"`
	PV       int64   `gorm:"column:pv"`
	UV       int64   `gorm:"column:uv"`
	Tokens   float64 `gorm:"column:tokens"`
	Duration float64 `gorm:"column:duration"`
	Round    float64 `gorm:"column:round"`
	ThumbUp  int64   `gorm:"column:thumb_up"`
}

// Stats returns daily conversation aggregates for a tenant.
func (dao *API4ConversationDAO) Stats(tenantID, fromDate, toDate string, source *string) ([]ConversationStatsRow, error) {
	var rows []ConversationStatsRow
	dateExpr := "DATE_FORMAT(a.create_date, '%Y-%m-%d 00:00:00')"
	db := DB.Table("api_4_conversation AS a").
		Select(`
			DATE_FORMAT(a.create_date, '%Y-%m-%d 00:00:00') AS dt,
			COUNT(a.id) AS pv,
			COUNT(DISTINCT a.user_id) AS uv,
			COALESCE(SUM(a.tokens), 0) AS tokens,
			COALESCE(SUM(a.duration), 0) AS duration,
			COALESCE(AVG(a.round), 0) AS round,
			COALESCE(SUM(a.thumb_up), 0) AS thumb_up
		`).
		Joins("JOIN dialog AS d ON a.dialog_id = d.id AND d.tenant_id = ?", tenantID).
		Where("a.create_date >= ? AND a.create_date <= ?", fromDate, toDate)

	if source == nil {
		db = db.Where("a.source IS NULL")
	} else {
		db = db.Where("a.source = ?", *source)
	}

	err := db.Group(dateExpr).
		Order(dateExpr).
		Scan(&rows).Error
	return rows, err
}

// DeleteByDialogIDs deletes API4Conversations by dialog IDs (hard delete)
func (dao *API4ConversationDAO) DeleteByDialogIDs(dialogIDs []string) (int64, error) {
	if len(dialogIDs) == 0 {
		return 0, nil
	}
	result := DB.Unscoped().Where("dialog_id IN ?", dialogIDs).Delete(&entity.API4Conversation{})
	return result.RowsAffected, result.Error
}
