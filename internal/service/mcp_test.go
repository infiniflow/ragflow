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
	"fmt"
	"testing"

	"ragflow/internal/entity"
)

func TestPaginateMCPServersNegativeValuesMatchPythonSlice(t *testing.T) {
	servers := makeMCPServers(13)

	got := paginateMCPServers(servers, -1, -2)

	if len(got) != 0 {
		t.Fatalf("expected empty page for negative pagination, got %d servers", len(got))
	}
}

func TestPaginateMCPServersKeepsUnpagedList(t *testing.T) {
	servers := makeMCPServers(3)

	got := paginateMCPServers(servers, 0, 0)

	if len(got) != len(servers) {
		t.Fatalf("expected unpaged list length %d, got %d", len(servers), len(got))
	}
}

func TestPaginateMCPServersPositiveValues(t *testing.T) {
	servers := makeMCPServers(5)

	got := paginateMCPServers(servers, 2, 2)

	if len(got) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(got))
	}
	if got[0].ID != "server-3" || got[1].ID != "server-4" {
		t.Fatalf("expected second page servers, got %q and %q", got[0].ID, got[1].ID)
	}
}

func makeMCPServers(count int) []*entity.MCPServer {
	servers := make([]*entity.MCPServer, 0, count)
	for i := 1; i <= count; i++ {
		servers = append(servers, &entity.MCPServer{ID: fmt.Sprintf("server-%d", i)})
	}
	return servers
}
