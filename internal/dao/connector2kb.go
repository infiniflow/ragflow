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

package dao

import (
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// Connector2KbDAO connector to knowledge base mapping data access object
type Connector2KbDAO struct{}

// NewConnector2KbDAO create connector2kb DAO
func NewConnector2KbDAO() *Connector2KbDAO {
	return &Connector2KbDAO{}
}

// DeleteByDatasetID deletes all connector mappings for a dataset within a transaction.
func (dao *Connector2KbDAO) DeleteByDatasetID(tx *gorm.DB, datasetID string) error {
	return tx.Where("kb_id = ?", datasetID).Delete(&entity.Connector2Kb{}).Error
}

// ListByDatasetID returns all connector mappings for a dataset within a transaction.
func (dao *Connector2KbDAO) ListByDatasetID(tx *gorm.DB, datasetID string) ([]*entity.Connector2Kb, error) {
	var mappings []*entity.Connector2Kb
	if err := tx.Where("kb_id = ?", datasetID).Find(&mappings).Error; err != nil {
		return nil, err
	}
	return mappings, nil
}

// UpdateAutoParse updates the auto_parse flag for a connector mapping within a transaction.
func (dao *Connector2KbDAO) UpdateAutoParse(tx *gorm.DB, datasetID, connectorID, autoParse string) error {
	return tx.Model(&entity.Connector2Kb{}).
		Where("kb_id = ? AND connector_id = ?", datasetID, connectorID).
		Update("auto_parse", autoParse).Error
}

// DeleteByDatasetIDAndConnectorID deletes one connector mapping within a transaction.
func (dao *Connector2KbDAO) DeleteByDatasetIDAndConnectorID(tx *gorm.DB, datasetID, connectorID string) error {
	return tx.Where("kb_id = ? AND connector_id = ?", datasetID, connectorID).Delete(&entity.Connector2Kb{}).Error
}

// Create creates a connector mapping within a transaction.
func (dao *Connector2KbDAO) Create(tx *gorm.DB, mapping *entity.Connector2Kb) error {
	return tx.Create(mapping).Error
}

// CreateMany creates multiple connector mappings within a transaction.
func (dao *Connector2KbDAO) CreateMany(tx *gorm.DB, mappings []*entity.Connector2Kb) error {
	if len(mappings) == 0 {
		return nil
	}
	return tx.Create(mappings).Error
}
