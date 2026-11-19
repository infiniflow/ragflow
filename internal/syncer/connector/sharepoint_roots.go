// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package connector

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func sharePointRootIDs(config map[string]any) ([]string, error) {
	value, present := config["root_folder_ids"]
	if present && value == nil {
		return nil, fmt.Errorf("root_folder_ids must be a list of strings")
	}
	if !present {
		id := strings.TrimSpace(stringConfig(config["root_folder_id"]))
		if id == "" {
			return nil, nil
		}
		value = []string{id}
	}
	ids, err := filterStringList(value)
	if err != nil {
		return nil, fmt.Errorf("root_folder_ids: %w", err)
	}
	unique := make(map[string]bool)
	roots := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || strings.ContainsAny(id, "/\\,;\x00\r\n") {
			return nil, fmt.Errorf("root_folder_ids must contain separate, non-empty Graph item IDs")
		}
		if !unique[id] {
			roots = append(roots, id)
			unique[id] = true
		}
	}
	sort.Strings(roots)
	return roots, nil
}

// Resolve overlap before applying filters: explicitly selecting an excluded
// descendant must not bypass its selected ancestor's exclusions.
func (c *SharePointConnector) independentRoots(ctx context.Context, driveID string) ([]string, error) {
	if len(c.rootFolderIDs) < 2 {
		return c.rootFolderIDs, nil
	}
	selected := make(map[string]bool, len(c.rootFolderIDs))
	parents := make(map[string]string)
	for _, id := range c.rootFolderIDs {
		selected[id] = true
	}
	var roots []string
	for _, id := range c.rootFolderIDs {
		nested, err := c.hasSelectedAncestor(ctx, driveID, id, selected, parents)
		if err != nil {
			return nil, err
		}
		if !nested {
			roots = append(roots, id)
		}
	}
	return roots, nil
}

func (c *SharePointConnector) hasSelectedAncestor(ctx context.Context, driveID, id string, selected map[string]bool, parents map[string]string) (bool, error) {
	seen := make(map[string]bool)
	nested := false
	for id != "" {
		if seen[id] {
			return false, fmt.Errorf("SharePoint folder ancestry contains a cycle")
		}
		seen[id] = true
		parent, cached := parents[id]
		if !cached {
			item, err := c.getItem(ctx, driveID, id)
			if err != nil {
				return false, err
			}
			parent = item.ParentReference.ID
			parents[id] = parent
		}
		nested = nested || selected[parent]
		id = parent
	}
	return nested, nil
}

func (c *SharePointConnector) walkSelectedRoots(ctx context.Context, driveID string) ([]sharePointItem, error) {
	roots, err := c.independentRoots(ctx, driveID)
	if err != nil {
		return nil, err
	}
	if len(roots) == 0 {
		roots = []string{""}
	}
	seen := make(map[string]bool)
	var files []sharePointItem
	for _, root := range roots {
		items, err := c.walkFiles(ctx, driveID, root, c.filters)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if !seen[item.ID] {
				seen[item.ID] = true
				files = append(files, item)
			}
		}
	}
	return files, nil
}
