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
	"errors"

	"ragflow/internal/dao"
)

// ErrTenantNotFound indicates the current user has no tenant relation.
var ErrTenantNotFound = errors.New("Tenant not found!")

// StatPoint matches the frontend [date, value] tuple shape.
type StatPoint [2]interface{}

// StatsResponse matches Python GET /api/v1/system/stats response data.
type StatsResponse struct {
	PV      []StatPoint `json:"pv"`
	UV      []StatPoint `json:"uv"`
	Speed   []StatPoint `json:"speed"`
	Tokens  []StatPoint `json:"tokens"`
	Round   []StatPoint `json:"round"`
	ThumbUp []StatPoint `json:"thumb_up"`
}

// GetStats returns daily API conversation statistics for the first tenant of a user.
func (s *SystemService) GetStats(userID, fromDate, toDate string, source *string) (*StatsResponse, error) {
	userTenantDAO := dao.NewUserTenantDAO()
	tenants, err := userTenantDAO.GetByUserID(userID)
	if err != nil || len(tenants) == 0 {
		return nil, ErrTenantNotFound
	}

	rows, err := dao.NewAPI4ConversationDAO().Stats(tenants[0].TenantID, fromDate, toDate, source)
	if err != nil {
		return nil, err
	}

	response := &StatsResponse{
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
