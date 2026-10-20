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

package cli

import (
	"strings"
	"testing"
)

func TestParseAdminListUserAgentsRequiresEE(t *testing.T) {
	cmd, err := NewParser("LIST USER 'user@example.com' AGENTS;").Parse(AdminMode)
	if cmd != nil || err == nil || !strings.Contains(err.Error(), "enterprise edition") {
		t.Fatalf("Parse() = (%v, %v), want nil command and enterprise edition error", cmd, err)
	}
}

func TestAdminListUserAgentsRequiresEE(t *testing.T) {
	cmd := NewCommand("admin_list_user_agents")
	cmd.Params["user_name"] = "user@example.com"
	response, err := (&CLI{}).AdminListUserAgentsCommand(1, cmd)
	if response != nil || err == nil || !strings.Contains(err.Error(), "enterprise edition") {
		t.Fatalf("AdminListUserAgentsCommand() = (%v, %v), want nil response and enterprise edition error", response, err)
	}
}

func TestParseAPIListAgents(t *testing.T) {
	cmd, err := NewParser("LIST AGENTS;").Parse(APIMode)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cmd == nil || cmd.Type != "api_list_agents" {
		t.Fatalf("Parse() command = %v, want api_list_agents", cmd)
	}
}

func TestParseAdminListUserDatasetsRequiresEE(t *testing.T) {
	cmd, err := NewParser("LIST USER 'user@example.com' DATASETS;").Parse(AdminMode)
	if cmd != nil || err == nil || !strings.Contains(err.Error(), "enterprise edition") {
		t.Fatalf("Parse() = (%v, %v), want nil command and enterprise edition error", cmd, err)
	}
}

func TestAdminListUserDatasetsRequiresEE(t *testing.T) {
	cmd := NewCommand("admin_list_user_datasets")
	cmd.Params["user_name"] = "user@example.com"
	response, err := (&CLI{}).AdminListUserDatasetsCommand(1, cmd)
	if response != nil || err == nil || !strings.Contains(err.Error(), "enterprise edition") {
		t.Fatalf("AdminListUserDatasetsCommand() = (%v, %v), want nil response and enterprise edition error", response, err)
	}
}

func TestParseAPIListDatasets(t *testing.T) {
	cmd, err := NewParser("LIST DATASETS;").Parse(APIMode)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cmd == nil || cmd.Type != "api_list_datasets" {
		t.Fatalf("Parse() command = %v, want api_list_datasets", cmd)
	}
}
