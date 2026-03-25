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

package contextfs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/model"
)

// Service provides context filesystem operations
type Service struct {
	kbDAO *dao.KnowledgebaseDAO
}

// NewService creates a new ContextFS service
func NewService() *Service {
	return &Service{
		kbDAO: dao.NewKnowledgebaseDAO(),
	}
}

// List returns the contents of a directory
func (s *Service) List(ctx context.Context, userID, path string) ([]*Node, error) {
	// Parse the path to determine what to list
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Root path or /datasets - list datasets
	if path == "" || path == "/" || path == "/datasets" || (len(parts) == 1 && parts[0] == "datasets") {
		return s.listDatasets(ctx, userID)
	}

	// Handle dataset contents
	if len(parts) >= 2 && parts[0] == "datasets" {
		datasetName := parts[1]
		return s.listDatasetContents(ctx, userID, datasetName, parts[2:])
	}

	return nil, fmt.Errorf("path not found: %s", path)
}

// listDatasets returns all datasets for a user as filesystem nodes
func (s *Service) listDatasets(ctx context.Context, userID string) ([]*Node, error) {
	// Get user info to find tenant
	userDAO := dao.NewUserDAO()
	user, err := userDAO.GetByAccessToken(userID)
	if err != nil {
		// Try to get by ID if not by token
		return s.listDatasetsByCreatedBy(ctx, userID)
	}

	// Get user-tenant relationship
	userTenantDAO := dao.NewUserTenantDAO()
	userTenants, err := userTenantDAO.GetByUserID(user.ID)
	if err != nil || len(userTenants) == 0 {
		return s.listDatasetsByCreatedBy(ctx, userID)
	}

	// Get datasets for this tenant
	var allKbs []*Node
	for _, ut := range userTenants {
		// Use Query to get knowledgebases by tenant_id
		kbs, err := s.kbDAO.Query(map[string]interface{}{
			"tenant_id": ut.TenantID,
		})
		if err != nil {
			continue
		}
		for _, kb := range kbs {
			allKbs = append(allKbs, knowledgebaseToNode(kb))
		}
	}

	return allKbs, nil
}

// listDatasetsByCreatedBy returns datasets created by the user
func (s *Service) listDatasetsByCreatedBy(ctx context.Context, userID string) ([]*Node, error) {
	kbs, err := s.kbDAO.GetByCreatedBy(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasets: %w", err)
	}

	// Convert knowledgebases to nodes
	nodes := make([]*Node, 0, len(kbs))
	for _, kb := range kbs {
		nodes = append(nodes, knowledgebaseToNode(kb))
	}

	return nodes, nil
}

// knowledgebaseToNode converts a Knowledgebase model to a filesystem Node
// Only includes name, path, created_at, updated_at fields for ls output
func knowledgebaseToNode(kb *model.Knowledgebase) *Node {
	node := &Node{
		Name: kb.Name,
		Path: fmt.Sprintf("/datasets/%s", kb.Name),
	}

	// Handle time fields
	if kb.CreateDate != nil {
		node.CreatedAt = *kb.CreateDate
	} else if kb.CreateTime != nil {
		node.CreatedAt = time.Unix(*kb.CreateTime, 0)
	} else {
		node.CreatedAt = time.Now()
	}

	if kb.UpdateDate != nil {
		node.UpdatedAt = *kb.UpdateDate
	} else if kb.UpdateTime != nil {
		node.UpdatedAt = time.Unix(*kb.UpdateTime, 0)
	} else {
		node.UpdatedAt = time.Now()
	}

	return node
}

// listDatasetContents returns the contents of a dataset directory
func (s *Service) listDatasetContents(ctx context.Context, userID, datasetName string, subPath []string) ([]*Node, error) {
	// For now, return empty list for dataset contents
	// This will be extended to show documents in the dataset
	return []*Node{}, nil
}

// Stat returns information about a node
func (s *Service) Stat(ctx context.Context, userID, path string) (*Node, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")

	if path == "" || path == "/" {
		return &Node{
			Name:      "/",
			Path:      "/",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	if len(parts) == 1 && parts[0] == "datasets" {
		return &Node{
			Name:      "datasets",
			Path:      "/datasets",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	if len(parts) >= 2 && parts[0] == "datasets" {
		// Try to find dataset by name
		kbs, err := s.kbDAO.GetByCreatedBy(userID)
		if err != nil {
			return nil, err
		}

		datasetName := parts[1]
		for _, kb := range kbs {
			if kb.Name == datasetName {
				return knowledgebaseToNode(kb), nil
			}
		}
	}

	return nil, fmt.Errorf("path not found: %s", path)
}

// Search searches for nodes matching the query
func (s *Service) Search(ctx context.Context, opts SearchOptions) ([]*SearchResult, int, error) {
	// For now, search only datasets
	if opts.Path == "/" || opts.Path == "/datasets" || opts.Path == "" {
		nodes, err := s.listDatasets(ctx, opts.UserID)
		if err != nil {
			return nil, 0, err
		}

		var results []*SearchResult
		query := strings.ToLower(opts.Query)
		for _, node := range nodes {
			if strings.Contains(strings.ToLower(node.Name), query) {
				results = append(results, &SearchResult{
					Node:  node,
					Score: 1.0,
				})
			}
		}

		return results, len(results), nil
	}

	return nil, 0, fmt.Errorf("search not implemented for path: %s", opts.Path)
}
