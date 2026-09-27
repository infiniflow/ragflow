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

package mcp

import (
	"context"
	"encoding/json"
	"strings"
)

// ServiceConnector implements the Connector interface using in-process
// service layer calls, avoiding HTTP round-trips to self.
type ServiceConnector struct {
	userID       string
	listDatasets func(ctx context.Context, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error)
	listChats    func(ctx context.Context, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error)
	retrieval    func(ctx context.Context, userID string, req RetrievalRequest) (string, error)
}

// NewServiceConnector creates a ServiceConnector.
// The function arguments abstract the service dependencies so this package
// does not import the service layer directly.
func NewServiceConnector(
	userID string,
	listDatasetsFunc func(ctx context.Context, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error),
	listChatsFunc func(ctx context.Context, userID string, page, pageSize int, orderby string, desc bool) ([]map[string]interface{}, int64, error),
	retrievalFunc func(ctx context.Context, userID string, req RetrievalRequest) (string, error),
) *ServiceConnector {
	return &ServiceConnector{
		userID:       userID,
		listDatasets: listDatasetsFunc,
		listChats:    listChatsFunc,
		retrieval:    retrievalFunc,
	}
}

// ListDatasets fetches every page for discovery (-1), or one capped REST-sized page.
func (c *ServiceConnector) ListDatasets(ctx context.Context, page, pageSize int, orderby string, desc bool) (string, error) {
	var data []map[string]any
	if pageSize == -1 {
		for page = 1; ; page++ {
			items, total, err := c.listDatasets(ctx, c.userID, page, 100, orderby, desc)
			if err != nil {
				return "", err
			}
			data = append(data, items...)
			if len(items) == 0 || (total > 0 && int64(len(data)) >= total) {
				break
			}
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
	} else {
		var err error
		data, _, err = c.listDatasets(ctx, c.userID, page, min(pageSize, 100), orderby, desc)
		if err != nil {
			return "", err
		}
	}
	return listText(data, false)
}

func (c *ServiceConnector) ListChats(ctx context.Context, page, pageSize int, orderby string, desc bool) (string, error) {
	data, _, err := c.listChats(ctx, c.userID, page, pageSize, orderby, desc)
	if err != nil {
		return "", err
	}
	return listText(data, true)
}

func listText(data []map[string]any, chats bool) (string, error) {
	lines := make([]string, 0, len(data))
	for _, d := range data {
		description, ok := d["description"]
		if chats && !ok {
			description = ""
		}
		item := map[string]any{"id": d["id"], "name": d["name"], "description": description}
		b, err := json.Marshal(item)
		if err != nil {
			return "", err
		}
		lines = append(lines, string(b))
	}
	return strings.Join(lines, "\n"), nil
}

func (c *ServiceConnector) Retrieval(ctx context.Context, req RetrievalRequest) (string, error) {
	return c.retrieval(ctx, c.userID, req)
}
