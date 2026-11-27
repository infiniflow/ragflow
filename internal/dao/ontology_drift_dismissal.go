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

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ragflow/internal/entity"
)

// OntologyDriftDismissalDAO is the data-access object for drift dismissals.
type OntologyDriftDismissalDAO struct{}

// NewOntologyDriftDismissalDAO creates a drift dismissal DAO.
func NewOntologyDriftDismissalDAO() *OntologyDriftDismissalDAO {
	return &OntologyDriftDismissalDAO{}
}

// Upsert writes the decision, replacing an earlier reason for the same finding.
// Re-dismissing with a new reason is a correction, not a second record: two rows
// for one class would leave the reader guessing which one is current.
func (dao *OntologyDriftDismissalDAO) Upsert(ctx context.Context, db *gorm.DB, row *entity.OntologyDriftDismissal) error {
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"}, {Name: "template_id"},
			{Name: "dataset_id"}, {Name: "class_name"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"reason", "dismissed_by", "update_time"}),
	}).Create(row).Error
}

// Delete removes the decision so the finding is listed again.
func (dao *OntologyDriftDismissalDAO) Delete(ctx context.Context, db *gorm.DB, tenantID, templateID, datasetID, className string) error {
	return db.WithContext(ctx).
		Where("tenant_id = ? AND template_id = ? AND dataset_id = ? AND class_name = ?",
			tenantID, templateID, datasetID, className).
		Delete(&entity.OntologyDriftDismissal{}).Error
}

// ListByScope returns the decisions for one model × dataset.
func (dao *OntologyDriftDismissalDAO) ListByScope(ctx context.Context, db *gorm.DB, tenantID, templateID, datasetID string) ([]*entity.OntologyDriftDismissal, error) {
	var rows []*entity.OntologyDriftDismissal
	err := db.WithContext(ctx).
		Where("tenant_id = ? AND template_id = ? AND dataset_id = ?", tenantID, templateID, datasetID).
		Order("class_name ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
