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
	"encoding/json"
	"ragflow/internal/entity"
)

type ExportMCPServer struct {
	Type               string         `json:"type"`
	URL                string         `json:"url"`
	Name               string         `json:"name"`
	AuthorizationToken interface{}    `json:"authorization_token"`
	Tools              interface{}    `json:"tools"`
	Headers            entity.JSONMap `json:"headers"`
	variables          entity.JSONMap
}

// The import format keeps substitution variables alongside the connection
// fields. Preserve them without changing the saved server during export.
func (server ExportMCPServer) MarshalJSON() ([]byte, error) {
	config := make(map[string]interface{}, len(server.variables)+6)
	for key, value := range server.variables {
		config[key] = value
	}
	config["type"] = server.Type
	config["url"] = server.URL
	if _, exists := config["name"]; !exists {
		config["name"] = server.Name
	}
	config["authorization_token"] = server.AuthorizationToken
	config["tools"] = server.Tools
	headers := server.Headers
	if headers == nil {
		headers = entity.JSONMap{}
	}
	config["headers"] = headers
	return json.Marshal(config)
}

type ExportMCPServerResponse struct {
	MCPServers map[string]ExportMCPServer `json:"mcpServers"`
}

func newExportMCPServerResponse(server *entity.MCPServer) *ExportMCPServerResponse {
	vars := server.Variables
	if vars == nil {
		vars = entity.JSONMap{}
	}

	token := interface{}("")
	if value, ok := vars["authorization_token"]; ok {
		token = value
	}
	tools := vars["tools"]
	if tools == nil {
		tools = map[string]interface{}{}
	}
	return &ExportMCPServerResponse{
		MCPServers: map[string]ExportMCPServer{
			server.Name: {
				Type:               server.ServerType,
				URL:                server.URL,
				Name:               server.Name,
				AuthorizationToken: token,
				Tools:              tools,
				Headers:            server.Headers,
				variables:          vars,
			},
		},
	}
}
