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

package service

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// DriftDismissal is one finding the operator decided not to act on.
type DriftDismissal struct {
	ClassName   string `json:"class_name"`
	Reason      string `json:"reason"`
	DismissedBy string `json:"dismissed_by,omitempty"`
}

// OntologyDriftDismissalService records and lifts those decisions.
//
// It exists because a drift list that comes back unchanged after every compile
// stops being read. The row is a decision about a scope, not a measurement of
// it, so recompiling never clears it.
type OntologyDriftDismissalService struct {
	dismissalDAO *dao.OntologyDriftDismissalDAO
}

// NewOntologyDriftDismissalService creates the service.
func NewOntologyDriftDismissalService() *OntologyDriftDismissalService {
	return &OntologyDriftDismissalService{
		dismissalDAO: dao.NewOntologyDriftDismissalDAO(),
	}
}

// List returns the dismissed classes for one model × dataset.
func (s *OntologyDriftDismissalService) List(ctx context.Context, db *gorm.DB, tenantID, templateID, datasetID string) ([]DriftDismissal, error) {
	rows, err := s.dismissalDAO.ListByScope(ctx, db, tenantID, templateID, datasetID)
	if err != nil {
		return nil, err
	}
	out := make([]DriftDismissal, 0, len(rows))
	for _, row := range rows {
		out = append(out, DriftDismissal{
			ClassName:   row.ClassName,
			Reason:      row.Reason,
			DismissedBy: row.DismissedBy,
		})
	}
	return out, nil
}

// Dismiss records "seen, deliberately not changing the model", with a reason.
//
// The reason is required. A dismissal without one is indistinguishable from
// forgetting, and the next reader cannot judge whether it still holds.
func (s *OntologyDriftDismissalService) Dismiss(ctx context.Context, db *gorm.DB, tenantID, templateID, datasetID, className, reason, actor string) ([]DriftDismissal, error) {
	className = strings.TrimSpace(className)
	reason = strings.TrimSpace(reason)
	if className == "" {
		return nil, fmt.Errorf("class name is required")
	}
	if reason == "" {
		return nil, fmt.Errorf("a dismissal needs a reason")
	}
	row := &entity.OntologyDriftDismissal{
		ID:          common.GenerateUUID(),
		TenantID:    tenantID,
		TemplateID:  templateID,
		DatasetID:   datasetID,
		ClassName:   className,
		Reason:      reason,
		DismissedBy: actor,
	}
	if err := s.dismissalDAO.Upsert(ctx, db, row); err != nil {
		return nil, err
	}
	return s.List(ctx, db, tenantID, templateID, datasetID)
}

// Restore lifts a dismissal so the finding is listed again.
func (s *OntologyDriftDismissalService) Restore(ctx context.Context, db *gorm.DB, tenantID, templateID, datasetID, className string) ([]DriftDismissal, error) {
	if strings.TrimSpace(className) == "" {
		return nil, fmt.Errorf("class name is required")
	}
	if err := s.dismissalDAO.Delete(ctx, db, tenantID, templateID, datasetID, strings.TrimSpace(className)); err != nil {
		return nil, err
	}
	return s.List(ctx, db, tenantID, templateID, datasetID)
}
