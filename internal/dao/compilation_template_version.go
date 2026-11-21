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

	"ragflow/internal/entity"
)

// CompilationTemplateVersionDAO is the data-access object for published model
// snapshots.
//
// The DAO exposes no update method on purpose. A snapshot that can be edited
// after the fact would let an assertion's stated model version stop describing
// the model it was extracted under, which is the one property the table exists
// to provide.
type CompilationTemplateVersionDAO struct{}

// NewCompilationTemplateVersionDAO creates a published-version DAO.
func NewCompilationTemplateVersionDAO() *CompilationTemplateVersionDAO {
	return &CompilationTemplateVersionDAO{}
}

// ListByTemplate returns every snapshot of a template, newest version first.
func (dao *CompilationTemplateVersionDAO) ListByTemplate(ctx context.Context, db *gorm.DB, tenantID, templateID string) ([]*entity.CompilationTemplateVersion, error) {
	var versions []*entity.CompilationTemplateVersion
	if err := db.WithContext(ctx).
		Where("tenant_id = ? AND template_id = ?", tenantID, templateID).
		Order("version DESC").
		Find(&versions).Error; err != nil {
		return nil, err
	}
	return versions, nil
}

// GetByVersion returns one snapshot. A missing snapshot returns
// gorm.ErrRecordNotFound, which the compile path treats as a broken invariant
// rather than as "no model": the template row says the version exists.
func (dao *CompilationTemplateVersionDAO) GetByVersion(ctx context.Context, db *gorm.DB, tenantID, templateID string, version int) (*entity.CompilationTemplateVersion, error) {
	var snapshot entity.CompilationTemplateVersion
	if err := db.WithContext(ctx).
		Where("tenant_id = ? AND template_id = ? AND version = ?", tenantID, templateID, version).
		Take(&snapshot).Error; err != nil {
		return nil, err
	}
	return &snapshot, nil
}
