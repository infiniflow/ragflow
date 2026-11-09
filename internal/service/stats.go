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
	"errors"

	"ragflow/internal/dao"
)

// ErrTenantNotFound indicates the current user has no tenant relation.
var ErrTenantNotFound = errors.New("Tenant not found!")

// StatPoint matches the frontend [date, value] tuple shape.
type StatPoint [2]interface{}

type StatsService struct{}

// NewStatsService creates a stats service.
func NewStatsService() *StatsService {
	return &StatsService{}
}

// ChatStatsResponse matches Python GET /api/v1/system/stats response data.
type ChatStatsResponse struct {
	PV      []StatPoint `json:"pv"`
	UV      []StatPoint `json:"uv"`
	Speed   []StatPoint `json:"speed"`
	Tokens  []StatPoint `json:"tokens"`
	Round   []StatPoint `json:"round"`
	ThumbUp []StatPoint `json:"thumb_up"`
}

// GetChatStats returns daily API conversation statistics for the current tenant.
func (s *StatsService) GetChatStats(ctx context.Context, fromDate, toDate string, source *string) (*ChatStatsResponse, error) {
	tenantID, err := requireCurrentTenantID(ctx)
	if err != nil {
		return nil, ErrTenantNotFound
	}

	rows, err := dao.NewAPI4ConversationDAO().Stats(ctx, dao.DB, tenantID, fromDate, toDate, source)
	if err != nil {
		return nil, err
	}

	response := &ChatStatsResponse{
		PV:      make([]StatPoint, 0, len(rows)),
		UV:      make([]StatPoint, 0, len(rows)),
		Speed:   make([]StatPoint, 0, len(rows)),
		Tokens:  make([]StatPoint, 0, len(rows)),
		Round:   make([]StatPoint, 0, len(rows)),
		ThumbUp: make([]StatPoint, 0, len(rows)),
	}

	for _, row := range rows {
		response.PV = append(response.PV, StatPoint{row.Dt, row.PV})
		response.UV = append(response.UV, StatPoint{row.Dt, row.UV})
		response.Speed = append(response.Speed, StatPoint{row.Dt, row.Tokens / (row.Duration + 0.1)})
		response.Tokens = append(response.Tokens, StatPoint{row.Dt, row.Tokens / 1000.0})
		response.Round = append(response.Round, StatPoint{row.Dt, row.Round})
		response.ThumbUp = append(response.ThumbUp, StatPoint{row.Dt, row.ThumbUp})
	}

	return response, nil
}
