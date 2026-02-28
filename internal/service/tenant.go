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
	"time"

	"ragflow/internal/dao"
)

// TenantService tenant service
type TenantService struct {
	tenantDAO     *dao.TenantDAO
	userTenantDAO *dao.UserTenantDAO
}

// NewTenantService create tenant service
func NewTenantService() *TenantService {
	return &TenantService{
		tenantDAO:     dao.NewTenantDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
	}
}

// TenantInfoResponse tenant information response
type TenantInfoResponse struct {
	TenantID  string  `json:"tenant_id"`
	Name      *string `json:"name,omitempty"`
	LLMID     string  `json:"llm_id"`
	EmbDID    string  `json:"embd_id"`
	RerankID  string  `json:"rerank_id"`
	ASRID     string  `json:"asr_id"`
	Img2TxtID string  `json:"img2txt_id"`
	TTSID     *string `json:"tts_id,omitempty"`
	ParserIDs string  `json:"parser_ids"`
	Role      string  `json:"role"`
}

// GetTenantInfo get tenant information for the current user (owner tenant)
func (s *TenantService) GetTenantInfo(userID string) (*TenantInfoResponse, error) {
	tenantInfos, err := s.tenantDAO.GetInfoByUserID(userID)
	if err != nil {
		return nil, err
	}
	if len(tenantInfos) == 0 {
		return nil, nil // No tenant found (should not happen for valid user)
	}
	// Return the first tenant (should be only one owner tenant per user)
	ti := tenantInfos[0]
	return &TenantInfoResponse{
		TenantID:  ti.TenantID,
		Name:      ti.Name,
		LLMID:     ti.LLMID,
		EmbDID:    ti.EmbDID,
		RerankID:  ti.RerankID,
		ASRID:     ti.ASRID,
		Img2TxtID: ti.Img2TxtID,
		TTSID:     ti.TTSID,
		ParserIDs: ti.ParserIDs,
		Role:      ti.Role,
	}, nil
}

// TenantListItem tenant list item response
type TenantListItem struct {
	TenantID     string  `json:"tenant_id"`
	Role         string  `json:"role"`
	Nickname     string  `json:"nickname"`
	Email        string  `json:"email"`
	Avatar       string  `json:"avatar"`
	UpdateDate   string  `json:"update_date"`
	DeltaSeconds float64 `json:"delta_seconds"`
}

// GetTenantList get tenant list for a user
func (s *TenantService) GetTenantList(userID string) ([]*TenantListItem, error) {
	tenants, err := s.userTenantDAO.GetTenantsByUserID(userID)
	if err != nil {
		return nil, err
	}

	result := make([]*TenantListItem, len(tenants))
	now := time.Now()

	for i, t := range tenants {
		// Parse update_date and calculate delta_seconds
		var deltaSeconds float64
		if t.UpdateDate != "" {
			if updateTime, err := time.Parse("2006-01-02 15:04:05", t.UpdateDate); err == nil {
				deltaSeconds = now.Sub(updateTime).Seconds()
			}
		}

		result[i] = &TenantListItem{
			TenantID:     t.TenantID,
			Role:         t.Role,
			Nickname:     t.Nickname,
			Email:        t.Email,
			Avatar:       t.Avatar,
			UpdateDate:   t.UpdateDate,
			DeltaSeconds: deltaSeconds,
		}
	}

	return result, nil
}
