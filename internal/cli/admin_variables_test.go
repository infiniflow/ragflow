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
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestParseAdminVariableCommands(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		command   string
		varName   string
		varValue  string
		hasValue  bool
		adminMode bool
	}{
		{
			name:      "list variables",
			input:     "list vars;",
			command:   "list_variables",
			adminMode: true,
		},
		{
			name:      "show variables by prefix",
			input:     "show var mail;",
			command:   "show_variable",
			varName:   "mail",
			adminMode: true,
		},
		{
			name:      "set integer variable",
			input:     "set var mail.port 15;",
			command:   "set_variable",
			varName:   "mail.port",
			varValue:  "15",
			hasValue:  true,
			adminMode: true,
		},
		{
			name:      "set quoted string variable",
			input:     `set var mail.server "local host";`,
			command:   "set_variable",
			varName:   "mail.server",
			varValue:  "local host",
			hasValue:  true,
			adminMode: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := NewParser(tt.input).Parse(tt.adminMode)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if cmd.Type != tt.command {
				t.Fatalf("command type = %q, want %q", cmd.Type, tt.command)
			}
			if tt.varName != "" && cmd.Params["var_name"] != tt.varName {
				t.Fatalf("var_name = %v, want %q", cmd.Params["var_name"], tt.varName)
			}
			if tt.hasValue && cmd.Params["var_value"] != tt.varValue {
				t.Fatalf("var_value = %v, want %q", cmd.Params["var_value"], tt.varValue)
			}
		})
	}
}

func newAdminTestClient(t *testing.T, handler http.HandlerFunc) (*RAGFlowClient, func()) {
	t.Helper()

	server := httptest.NewServer(handler)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	host, portText, err := net.SplitHostPort(serverURL.Host)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	client := NewRAGFlowClient("admin")
	client.HTTPClient.Host = host
	client.HTTPClient.Port = port
	client.HTTPClient.client = server.Client()
	client.HTTPClient.LoginToken = "test-token"

	return client, server.Close
}

func TestListVariablesUsesAdminVariablesEndpoint(t *testing.T) {
	client, closeServer := newAdminTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
			return
		}
		if r.URL.Path != "/api/v1/admin/variables" {
			t.Errorf("path = %s, want /api/v1/admin/variables", r.URL.Path)
			return
		}
		if r.Header.Get("Authorization") != "test-token" {
			t.Errorf("Authorization header = %q", r.Header.Get("Authorization"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"message": "",
			"data": []map[string]interface{}{
				{"data_type": "string", "name": "mail.server", "source": "variable", "value": "localhost"},
			},
		})
	})
	defer closeServer()

	resp, err := client.ListVariables(NewCommand("list_variables"))
	if err != nil {
		t.Fatalf("ListVariables() error = %v", err)
	}
	result := resp.(*CommonResponse)
	if got := result.Data[0]["setting_type"]; got != "config" {
		t.Fatalf("setting_type = %v, want config", got)
	}
	if _, ok := result.Data[0]["source"]; ok {
		t.Fatalf("source column should be normalized away: %#v", result.Data[0])
	}
}

func TestShowVariableSendsRequestedName(t *testing.T) {
	client, closeServer := newAdminTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		var request map[string]string
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("request body is not JSON: %v", err)
			return
		}
		if request["var_name"] != "mail" {
			t.Errorf("var_name = %q, want mail", request["var_name"])
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"message": "",
			"data": []map[string]interface{}{
				{"data_type": "string", "name": "mail.server", "setting_type": "config", "value": "localhost"},
			},
		})
	})
	defer closeServer()

	cmd := NewCommand("show_variable")
	cmd.Params["var_name"] = "mail"
	resp, err := client.ShowVariable(cmd)
	if err != nil {
		t.Fatalf("ShowVariable() error = %v", err)
	}
	result := resp.(*CommonResponse)
	if got := result.Data[0]["name"]; got != "mail.server" {
		t.Fatalf("name = %v, want mail.server", got)
	}
}

func TestSetVariableReturnsServerConfirmation(t *testing.T) {
	client, closeServer := newAdminTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
			return
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("request body is not JSON: %v", err)
			return
		}
		if request["var_name"] != "mail.server" {
			t.Errorf("var_name = %q, want mail.server", request["var_name"])
			return
		}
		if request["var_value"] != "localhost" {
			t.Errorf("var_value = %q, want localhost", request["var_value"])
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"message": "Set variable successfully",
			"data":    nil,
		})
	})
	defer closeServer()

	cmd := NewCommand("set_variable")
	cmd.Params["var_name"] = "mail.server"
	cmd.Params["var_value"] = "localhost"
	resp, err := client.SetVariable(cmd)
	if err != nil {
		t.Fatalf("SetVariable() error = %v", err)
	}
	result := resp.(*MessageResponse)
	if result.Message != "Set variable successfully" {
		t.Fatalf("message = %q, want Set variable successfully", result.Message)
	}
}
