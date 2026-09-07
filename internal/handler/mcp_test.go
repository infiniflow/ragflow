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

package handler

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"ragflow/internal/entity"
)

func TestNewMCPServerResponsePreservesNullDescriptionAndFormatsDates(t *testing.T) {
	createTime := int64(1779949820000)
	updateTime := int64(1779953420000)
	location := time.FixedZone("UTC+8", 8*60*60)
	createDate := time.Date(2026, 5, 28, 10, 30, 20, 0, location)
	updateDate := time.Date(2026, 5, 28, 11, 30, 20, 0, location)

	response := newMCPServerResponse(&entity.MCPServer{
		ID:         "mcp-id",
		Name:       "server",
		TenantID:   "tenant-id",
		URL:        "https://example.com/mcp",
		ServerType: "sse",
		Variables:  entity.JSONMap{"tools": map[string]interface{}{}},
		Headers:    entity.JSONMap{"Authorization": "Bearer token"},
		BaseModel: entity.BaseModel{
			CreateTime: &createTime,
			CreateDate: &createDate,
			UpdateTime: &updateTime,
			UpdateDate: &updateDate,
		},
	})

	if response.Description != nil {
		t.Fatalf("description = %q, want nil", *response.Description)
	}
	if response.CreateDate != "2026-05-28T10:30:20" {
		t.Fatalf("create_date = %q, want date without timezone", response.CreateDate)
	}
	if response.UpdateDate != "2026-05-28T11:30:20" {
		t.Fatalf("update_date = %q, want date without timezone", response.UpdateDate)
	}

	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !bytes.Contains(payload, []byte(`"description":null`)) {
		t.Fatalf("payload %s does not preserve description:null", payload)
	}
	if bytes.Contains(payload, []byte(`+08:00`)) {
		t.Fatalf("payload %s includes timezone in date fields", payload)
	}
}
