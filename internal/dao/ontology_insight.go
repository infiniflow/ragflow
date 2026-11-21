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

// OntologyInsightDAO is the data-access object for the measured model snapshot.
//
// There is no history here on purpose: a reader asks "what does my corpus look
// like now", and the answer that matters is the latest compile. Keeping every
// measurement would turn a rewrite-per-compile table into one that grows with
// the number of compiles, and nothing reads the old ones.
type OntologyInsightDAO struct{}

// NewOntologyInsightDAO creates an insight DAO.
func NewOntologyInsightDAO() *OntologyInsightDAO {
	return &OntologyInsightDAO{}
}

// Upsert writes a measurement, replacing the previous one for the same scope.
//
// The conflict target is the scope triple, not the id: a compile does not know
// whether this is the first measurement of the dataset, and looking that up
// first would race two concurrent compiles into a duplicate row.
func (dao *OntologyInsightDAO) Upsert(ctx context.Context, db *gorm.DB, insight *entity.OntologyInsight) error {
	return db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "tenant_id"}, {Name: "template_id"}, {Name: "dataset_id"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"version", "as_of", "payload", "update_time", "update_date",
			}),
		}).
		Create(insight).Error
}

// Get returns the stored measurement for a scope, or gorm.ErrRecordNotFound.
func (dao *OntologyInsightDAO) Get(ctx context.Context, db *gorm.DB, tenantID, templateID, datasetID string) (*entity.OntologyInsight, error) {
	var insight entity.OntologyInsight
	if err := db.WithContext(ctx).
		Where("tenant_id = ? AND template_id = ? AND dataset_id = ?", tenantID, templateID, datasetID).
		Take(&insight).Error; err != nil {
		return nil, err
	}
	return &insight, nil
}

// ListByTemplate returns every dataset's measurement for one model. The publish
// gate reads this to answer "is this model answerable where it is used", which
// is a question about all the datasets at once.
func (dao *OntologyInsightDAO) ListByTemplate(ctx context.Context, db *gorm.DB, tenantID, templateID string) ([]*entity.OntologyInsight, error) {
	var insights []*entity.OntologyInsight
	if err := db.WithContext(ctx).
		Where("tenant_id = ? AND template_id = ?", tenantID, templateID).
		Order("dataset_id ASC").
		Find(&insights).Error; err != nil {
		return nil, err
	}
	return insights, nil
}

// DeleteByScope removes a dataset's measurement, used when the dataset stops
// compiling against the model or is deleted.
func (dao *OntologyInsightDAO) DeleteByScope(ctx context.Context, db *gorm.DB, tenantID, templateID, datasetID string) error {
	return db.WithContext(ctx).
		Where("tenant_id = ? AND template_id = ? AND dataset_id = ?", tenantID, templateID, datasetID).
		Delete(&entity.OntologyInsight{}).Error
}
