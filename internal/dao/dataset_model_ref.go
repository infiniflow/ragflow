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
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// DatasetModelRefDAO is the data-access object for the compilation model
// references a dataset carries.
type DatasetModelRefDAO struct{}

// NewDatasetModelRefDAO creates a dataset model reference DAO.
func NewDatasetModelRefDAO() *DatasetModelRefDAO {
	return &DatasetModelRefDAO{}
}

// ModelImpactDataset is one dataset the model library lists as affected by a
// change to a model.
type ModelImpactDataset struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	DocNum     int64  `json:"doc_num"`
	UpdateTime *int64 `json:"update_time,omitempty"`
}

// ReplaceDatasetModelRefs rewrites a dataset's stored references to exactly the
// models its config names.
//
// It deletes before inserting rather than upserting: the config is the whole
// truth, and a reference that was removed from it has to disappear here as
// well, or the model library would keep claiming the dataset still compiles
// against a model it no longer names.
//
// The caller supplies the dataset's tenant, because the reference rows carry it.
func ReplaceDatasetModelRefs(ctx context.Context, db *gorm.DB, datasetID, tenantID string, config interface{}) error {
	if err := db.WithContext(ctx).
		Where("dataset_id = ?", datasetID).
		Delete(&entity.DatasetModelRef{}).Error; err != nil {
		return err
	}
	refs := common.TemplateRefsFromConfig(config)
	if len(refs) == 0 {
		return nil
	}
	rows := make([]*entity.DatasetModelRef, 0, len(refs))
	for _, ref := range refs {
		rows = append(rows, &entity.DatasetModelRef{
			ID:        common.GenerateUUID(),
			TenantID:  tenantID,
			DatasetID: datasetID,
			RefType:   ref.Kind,
			ModelRef:  ref.ID,
		})
	}
	return db.WithContext(ctx).Create(&rows).Error
}

// CountByModel counts, per model reference, how many live datasets name it.
//
// One indexed query replaces the per-listing scan of every dataset config: the
// answer is what tells a publisher whether a change reaches anything at all, so
// it has to stay cheap enough to be shown without being asked for.
//
// Rows of soft-deleted datasets stay in the table — restoring a dataset would
// otherwise silently lose its references — so the count joins knowledgebase and
// filters on the valid status instead.
func (dao *DatasetModelRefDAO) CountByModel(ctx context.Context, db *gorm.DB, tenantID string) (map[common.ModelRef]int64, error) {
	type refCountRow struct {
		RefType  string `gorm:"column:ref_type"`
		ModelRef string `gorm:"column:model_ref"`
		Count    int64  `gorm:"column:ref_count"`
	}
	rows := make([]refCountRow, 0)
	if err := db.WithContext(ctx).
		Table("dataset_model_ref AS r").
		Select("r.ref_type, r.model_ref, COUNT(*) AS ref_count").
		Joins("JOIN knowledgebase AS k ON k.id = r.dataset_id AND k.status = ?", string(entity.StatusValid)).
		Where("r.tenant_id = ?", tenantID).
		Group("r.ref_type, r.model_ref").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	// One row per dataset per reference: uni_dataset_model_ref makes that an
	// invariant of the table, which is what lets COUNT(*) answer "how many
	// datasets" without a DISTINCT.
	counts := make(map[common.ModelRef]int64, len(rows))
	for _, row := range rows {
		counts[common.ModelRef{Kind: row.RefType, ID: row.ModelRef}] = row.Count
	}
	return counts, nil
}

// DatasetsByModel lists the live datasets that compile against one model.
//
// It is the drill-down behind the count: a publisher who is told "3 datasets
// are affected" needs to know which three, because that is the difference
// between a change they can make and one they have to schedule.
func (dao *DatasetModelRefDAO) DatasetsByModel(ctx context.Context, db *gorm.DB, tenantID string, ref common.ModelRef) ([]*ModelImpactDataset, error) {
	return dao.DatasetsByModelRefs(ctx, db, tenantID, []common.ModelRef{ref})
}

// DatasetsByModelRefs lists the live datasets that compile against ANY of the
// given model references, once each.
//
// A model is reachable two ways and both are real: a dataset may name the
// template directly, or name the template group that contains it. Resolving only
// one of them makes a model that is in use look unused — and because the same
// dataset can appear under both, the result is deduplicated by dataset id rather
// than concatenated.
func (dao *DatasetModelRefDAO) DatasetsByModelRefs(ctx context.Context, db *gorm.DB, tenantID string, refs []common.ModelRef) ([]*ModelImpactDataset, error) {
	datasets := make([]*ModelImpactDataset, 0)
	if len(refs) == 0 {
		return datasets, nil
	}
	conditions := make([]string, 0, len(refs))
	args := make([]interface{}, 0, len(refs)*2+1)
	args = append(args, tenantID)
	for _, ref := range refs {
		conditions = append(conditions, "(r.ref_type = ? AND r.model_ref = ?)")
		args = append(args, ref.Kind, ref.ID)
	}
	if err := db.WithContext(ctx).
		Table("dataset_model_ref AS r").
		Select("k.id AS id, k.name AS name, k.doc_num AS doc_num, k.update_time AS update_time").
		Joins("JOIN knowledgebase AS k ON k.id = r.dataset_id AND k.status = ?", string(entity.StatusValid)).
		Where("r.tenant_id = ? AND ("+strings.Join(conditions, " OR ")+")", args...).
		Group("k.id, k.name, k.doc_num, k.update_time").
		Order("k.name ASC, k.id ASC").
		Find(&datasets).Error; err != nil {
		return nil, err
	}
	return datasets, nil
}

// CountByModelRefs counts the live datasets naming any of the given references,
// once each. Same reasoning as DatasetsByModelRefs: a template inside a group is
// reachable by both references, so summing the per-reference counts would count
// one dataset twice.
func (dao *DatasetModelRefDAO) CountByModelRefs(ctx context.Context, db *gorm.DB, tenantID string, refs []common.ModelRef) (int64, error) {
	if len(refs) == 0 {
		return 0, nil
	}
	conditions := make([]string, 0, len(refs))
	args := make([]interface{}, 0, len(refs)*2+1)
	args = append(args, tenantID)
	for _, ref := range refs {
		conditions = append(conditions, "(r.ref_type = ? AND r.model_ref = ?)")
		args = append(args, ref.Kind, ref.ID)
	}
	var count int64
	if err := db.WithContext(ctx).
		Table("dataset_model_ref AS r").
		Select("COUNT(DISTINCT k.id)").
		Joins("JOIN knowledgebase AS k ON k.id = r.dataset_id AND k.status = ?", string(entity.StatusValid)).
		Where("r.tenant_id = ? AND ("+strings.Join(conditions, " OR ")+")", args...).
		Scan(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
