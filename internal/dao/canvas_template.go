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

// CanvasTemplateDAO data-access object for the canvas_template table.
type CanvasTemplateDAO struct{}

// NewCanvasTemplateDAO creates a CanvasTemplate DAO.
func NewCanvasTemplateDAO() *CanvasTemplateDAO {
	return &CanvasTemplateDAO{}
}

// GetAll returns every row in canvas_template ordered by create_time desc, so
// templates appear newest first in the UI. Mirrors the Python
// CanvasTemplateService.get_all() behaviour.
func (dao *CanvasTemplateDAO) GetAll() ([]*entity.CanvasTemplate, error) {
	var templates []*entity.CanvasTemplate
	if err := DB.Order("create_time desc").Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}
