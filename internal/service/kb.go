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
	"ragflow/internal/dao"
	"ragflow/internal/model"
)

// KnowledgebaseService knowledge base service
type KnowledgebaseService struct {
	kbDAO         *dao.KnowledgebaseDAO
	userTenantDAO *dao.UserTenantDAO
}

// NewKnowledgebaseService create knowledge base service
func NewKnowledgebaseService() *KnowledgebaseService {
	return &KnowledgebaseService{
		kbDAO:         dao.NewKnowledgebaseDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
	}
}

// ListKbsRequest list knowledge bases request
type ListKbsRequest struct {
	Keywords *string   `json:"keywords,omitempty"`
	Page     *int      `json:"page,omitempty"`
	PageSize *int      `json:"page_size,omitempty"`
	ParserID *string   `json:"parser_id,omitempty"`
	Orderby  *string   `json:"orderby,omitempty"`
	Desc     *bool     `json:"desc,omitempty"`
	OwnerIDs *[]string `json:"owner_ids,omitempty"`
}

// ListKbsResponse list knowledge bases response
type ListKbsResponse struct {
	KBs   []*model.Knowledgebase `json:"kbs"`
	Total int64                  `json:"total"`
}

// ListKbs list knowledge bases
func (s *KnowledgebaseService) ListKbs(keywords string, page int, pageSize int, parserID string, orderby string, desc bool, ownerIDs []string, userID string) (*ListKbsResponse, error) {
	var kbs []*model.Knowledgebase
	var total int64
	var err error

	// If owner IDs are provided, filter by them
	if ownerIDs != nil && len(ownerIDs) > 0 {
		kbs, total, err = s.kbDAO.ListByOwnerIDs(ownerIDs, page, pageSize, orderby, desc, keywords, parserID)
	} else {
		// Get tenant IDs by user ID
		tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
		if err != nil {
			return nil, err
		}

		kbs, total, err = s.kbDAO.ListByTenantIDs(tenantIDs, userID, page, pageSize, orderby, desc, keywords, parserID)
	}

	if err != nil {
		return nil, err
	}

	return &ListKbsResponse{
		KBs:   kbs,
		Total: total,
	}, nil
}
