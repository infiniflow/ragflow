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
)

// CanvasService canvas service
// Provides business logic for canvas operations
type CanvasService struct {
	userCanvasDAO *dao.UserCanvasDAO
}

// NewCanvasService creates a new canvas service instance
//
// Returns:
//   - *CanvasService: a new canvas service instance
//
// Example:
//
//	canvasService := service.NewCanvasService()
//	basicInfo, err := canvasService.GetBasicInfoByCanvasIDs([]string{"canvas_id_1", "canvas_id_2"})
func NewCanvasService() *CanvasService {
	return &CanvasService{
		userCanvasDAO: dao.NewUserCanvasDAO(),
	}
}

// CanvasBasicInfoResponse represents the basic canvas information response
// Used for returning canvas id and title mapping
type CanvasBasicInfoResponse struct {
	ID    string  `json:"id"`
	Title *string `json:"title,omitempty"`
}

// GetBasicInfoByCanvasIDs retrieves basic information for multiple canvases by their IDs
//
// This method queries the canvas table and returns a list of canvas basic information
// including id and title. It's equivalent to the Python implementation:
// UserCanvasService.get_basic_info_by_canvas_ids
//
// Parameters:
//   - canvasIDs: a slice of canvas IDs to query
//
// Returns:
//   - []*CanvasBasicInfoResponse: a slice of canvas basic information containing id and title
//   - error: an error if the query fails, nil otherwise
//
// Example:
//
//	canvasService := NewCanvasService()
//	canvasIDs := []string{"canvas_id_1", "canvas_id_2", "canvas_id_3"}
//	basicInfo, err := canvasService.GetBasicInfoByCanvasIDs(canvasIDs)
//	if err != nil {
//	    log.Printf("Failed to get canvas basic info: %v", err)
//	    return
//	}
//	// basicInfo will be like: [{"id": "canvas_id_1", "title": "Canvas 1"}, ...]
//	for _, info := range basicInfo {
//	    fmt.Printf("Canvas ID: %s, Title: %s\n", info.ID, *info.Title)
//	}
func (s *CanvasService) GetBasicInfoByCanvasIDs(canvasIDs []string) ([]*CanvasBasicInfoResponse, error) {
	// Return empty slice if no canvas IDs provided
	if len(canvasIDs) == 0 {
		return []*CanvasBasicInfoResponse{}, nil
	}

	// Query canvas basic info using DAO
	// The DAO method GetAllCanvasesByTenantIDs is not suitable here,
	// so we need to query directly
	var results []*CanvasBasicInfoResponse
	err := dao.DB.Table("user_canvas").
		Select("id, title").
		Where("id IN (?)", canvasIDs).
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	return results, nil
}
