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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ragflow/internal/entity"
)

const maxPipelineDSLVersionInsertAttempts = 16

// ErrPipelineDSLVersionContention is returned when concurrent writers prevent
// a version from being selected or inserted within the bounded retry budget.
var ErrPipelineDSLVersionContention = errors.New("pipeline_dsl_version: concurrent update contention")

// PipelineDSLVersionDAO persists immutable pipeline DSL definitions.
type PipelineDSLVersionDAO struct{}

// NewPipelineDSLVersionDAO returns a stateless PipelineDSLVersionDAO.
func NewPipelineDSLVersionDAO() *PipelineDSLVersionDAO {
	return &PipelineDSLVersionDAO{}
}

// GetLatest returns the highest version for dslID, or nil when the identity has
// no stored definition.
func (dao *PipelineDSLVersionDAO) GetLatest(ctx context.Context, db *gorm.DB, dslID string) (*entity.PipelineDSLVersion, error) {
	var version entity.PipelineDSLVersion
	err := db.WithContext(ctx).
		Where("dsl_id = ?", dslID).
		Order("version DESC").
		First(&version).Error
	if err != nil {
		if IsNotFoundErr(err) {
			return nil, nil
		}
		return nil, err
	}
	return &version, nil
}

// GetOrCreate reuses the latest version when its DSL is unchanged. Otherwise
// it appends the next version. A composite-primary-key conflict means another
// writer advanced the same DSL identity, so the operation re-reads the latest
// row and retries with a bounded budget.
func (dao *PipelineDSLVersionDAO) GetOrCreate(ctx context.Context, db *gorm.DB, dslID string, dsl entity.JSONMap) (*entity.PipelineDSLVersion, error) {
	dslID = strings.TrimSpace(dslID)
	if dslID == "" {
		return nil, errors.New("pipeline_dsl_version: dsl id is required")
	}
	if dsl == nil {
		return nil, errors.New("pipeline_dsl_version: dsl is required")
	}
	if _, err := json.Marshal(dsl); err != nil {
		return nil, fmt.Errorf("pipeline_dsl_version: encode dsl: %w", err)
	}

	for range maxPipelineDSLVersionInsertAttempts {
		latest, err := dao.GetLatest(ctx, db, dslID)
		if err != nil {
			return nil, err
		}
		if latest != nil {
			same, compareErr := equalJSONMaps(latest.DSL, dsl)
			if compareErr != nil {
				return nil, compareErr
			}
			if same {
				return latest, nil
			}
		}

		nextVersion := int64(1)
		if latest != nil {
			nextVersion = latest.Version + 1
		}
		candidate := &entity.PipelineDSLVersion{
			DSLID:   dslID,
			Version: nextVersion,
			DSL:     dsl,
		}
		result := db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "dsl_id"}, {Name: "version"}},
			DoNothing: true,
		}).Create(candidate)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 1 {
			return candidate, nil
		}
	}

	return nil, fmt.Errorf("%w: dsl_id=%s attempts=%d", ErrPipelineDSLVersionContention, dslID, maxPipelineDSLVersionInsertAttempts)
}

func equalJSONMaps(left, right entity.JSONMap) (bool, error) {
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false, fmt.Errorf("pipeline_dsl_version: encode stored dsl: %w", err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false, fmt.Errorf("pipeline_dsl_version: encode candidate dsl: %w", err)
	}
	return bytes.Equal(leftJSON, rightJSON), nil
}
