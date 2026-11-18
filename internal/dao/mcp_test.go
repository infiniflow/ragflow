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

package dao

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMCPServerOrderColumnInvalidField(t *testing.T) {
	_, err := mcpServerOrderColumn("bad_field")
	if err == nil {
		t.Fatal("expected invalid orderby error")
	}

	var orderbyErr *InvalidMCPServerOrderByError
	if !errors.As(err, &orderbyErr) {
		t.Fatalf("expected InvalidMCPServerOrderByError, got %T", err)
	}

	want := `AttributeError("type object 'MCPServer' has no attribute 'bad_field'")`
	if err.Error() != want {
		t.Fatalf("expected %q, got %q", want, err.Error())
	}
}

func TestListMCPServersCountsAndPaginatesAfterSQLAuthorization(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatalf("migrate MCP server: %v", err)
	}
	for _, server := range []*entity.MCPServer{
		{ID: "mcp-a", TenantID: "tenant-1", Name: "alpha", URL: "https://example.test/a", ServerType: "sse"},
		{ID: "mcp-b", TenantID: "tenant-1", Name: "bravo", URL: "https://example.test/b", ServerType: "sse"},
		{ID: "mcp-c", TenantID: "tenant-1", Name: "charlie", URL: "https://example.test/c", ServerType: "sse"},
		{ID: "other-tenant", TenantID: "tenant-2", Name: "delta", URL: "https://example.test/d", ServerType: "sse"},
	} {
		if err := db.Create(server).Error; err != nil {
			t.Fatalf("create MCP server %q: %v", server.ID, err)
		}
	}

	authorize := func(query *gorm.DB) *gorm.DB { return query.Where("name <> ?", "alpha") }
	servers, total, err := NewMCPServerDAO().ListMCPServers(context.Background(), db, "tenant-1", nil, "", "name", false, 2, 1, authorize)
	if err != nil {
		t.Fatalf("ListMCPServers(): %v", err)
	}
	if total != 2 || len(servers) != 1 || servers[0].ID != "mcp-c" {
		t.Fatalf("ListMCPServers() = (total %d, %#v), want total 2 and page [mcp-c]", total, servers)
	}
}

func TestMCPServerPageRangeMatchesSliceBounds(t *testing.T) {
	for _, test := range []struct {
		name               string
		total              int64
		page, pageSize     int
		wantStart, wantEnd int
	}{
		{name: "positive page", total: 5, page: 2, pageSize: 2, wantStart: 2, wantEnd: 4},
		{name: "negative slice", total: 13, page: -1, pageSize: 5, wantStart: 3, wantEnd: 8},
		{name: "reversed slice", total: 13, page: -1, pageSize: -2, wantStart: 4, wantEnd: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			start, end := mcpServerPageRange(test.total, test.page, test.pageSize)
			if start != test.wantStart || end != test.wantEnd {
				t.Fatalf("mcpServerPageRange() = (%d, %d), want (%d, %d)", start, end, test.wantStart, test.wantEnd)
			}
		})
	}
}
