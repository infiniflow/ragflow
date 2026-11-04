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

	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

// AgentVersionMetadata describes a saved version without its DSL.
type AgentVersionMetadata struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Alias string `json:"alias"`
}

// AgentVersionCatalogItem groups a canvas's saved version metadata.
type AgentVersionCatalogItem struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Versions []AgentVersionMetadata `json:"versions"`
}

// ListVersionCatalog returns accessible canvases in the selected category and
// their saved versions, newest first. Empty canvases keep an empty version list.
func (s *AgentService) ListVersionCatalog(ctx context.Context, canvasCategory string) ([]AgentVersionCatalogItem, error) {
	scope, err := permission.NewDatabaseChecker(dao.DB).Scope(ctx, permission.ScopeQuery{
		Kind: permission.ResourceKindCanvas, Operation: permission.OperationRead,
	})
	if err != nil {
		return nil, fmt.Errorf("scope version catalog: %w", err)
	}
	items := make([]AgentVersionCatalogItem, 0)
	if scope.Mode == permission.ScopeNone {
		return items, nil
	}
	canvases, err := s.canvasDAO.ListMetadataByIDs(ctx, dao.DB, scope.ResourceIDs, canvasCategory)
	if err != nil {
		return nil, fmt.Errorf("list version catalog canvases: %w", err)
	}
	canvasIDs := make([]string, 0, len(canvases))
	indexByID := make(map[string]int, len(canvases))
	for _, canvas := range canvases {
		indexByID[canvas.ID] = len(items)
		canvasIDs = append(canvasIDs, canvas.ID)
		items = append(items, AgentVersionCatalogItem{
			ID: canvas.ID, Name: derefString(canvas.Title),
			Versions: make([]AgentVersionMetadata, 0),
		})
	}
	versions, err := s.versionDAO.ListMetadataByCanvasIDs(ctx, dao.DB, canvasIDs)
	if err != nil {
		return nil, fmt.Errorf("list version catalog versions: %w", err)
	}
	for _, version := range versions {
		item := &items[indexByID[version.UserCanvasID]]
		item.Versions = append(item.Versions, AgentVersionMetadata{
			ID: version.ID, Name: derefString(version.Title),
			Alias: derefString(version.Alias),
		})
	}
	return items, nil
}
