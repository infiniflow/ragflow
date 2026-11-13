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

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// MCPConfig configures the API server's optional standalone MCP listener.
type MCPConfig struct {
	Enabled        bool
	Host           string
	Port           int
	LaunchMode     string
	HostAPIKey     string
	SSE            bool
	StreamableHTTP bool
	JSONResponse   bool
}

func (c *Config) parseMCPConfig(v *viper.Viper) error {
	// LookupEnv preserves explicitly empty overrides. Viper's AutomaticEnv
	// ignores them by default; its nested Sub views also lose the root prefix.
	value := func(key, fallback string) string {
		if value, ok := os.LookupEnv("RAGFLOW_MCP_" + strings.ToUpper(key)); ok {
			return value
		}
		if v.IsSet("mcp." + key) {
			return v.GetString("mcp." + key)
		}
		return fallback
	}
	port, err := strconv.Atoi(value("port", "9382"))
	if err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("invalid MCP port: %s", value("port", "9382"))
	}
	mcp := MCPConfig{
		Enabled:        parseMCPBool(value("enabled", "false")),
		Host:           value("host", "127.0.0.1"),
		Port:           port,
		LaunchMode:     value("launch_mode", "self-host"),
		HostAPIKey:     value("host_api_key", ""),
		SSE:            parseMCPBool(value("transport_sse_enabled", "true")),
		StreamableHTTP: parseMCPBool(value("transport_streamable_enabled", "true")),
		JSONResponse:   parseMCPBool(value("json_response", "true")),
	}
	if mcp.LaunchMode != "self-host" && mcp.LaunchMode != "host" {
		return fmt.Errorf("invalid MCP mode: %s", mcp.LaunchMode)
	}
	// Match Python: disable JSON before the both-transports-disabled fallback.
	if !mcp.StreamableHTTP {
		mcp.JSONResponse = false
	}
	if !mcp.SSE && !mcp.StreamableHTTP {
		mcp.StreamableHTTP = true
	}
	if mcp.Enabled && mcp.LaunchMode == "self-host" && mcp.HostAPIKey == "" {
		return fmt.Errorf("MCP self-host mode requires mcp.host_api_key or RAGFLOW_MCP_HOST_API_KEY")
	}
	c.apiServer.MCP = mcp
	return nil
}

func parseMCPBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
