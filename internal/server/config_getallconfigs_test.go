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

package server

import (
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"strings"
	"testing"
)

// TestGetAllConfigsKvrocksCacheEngine reproduces Finding 1: when the Go stack is
// configured with `cache_engine: kvrocks`, GetAllConfigs must export the Kvrocks
// connection settings instead of failing with "not supported cache engine".
func TestGetAllConfigsKvrocksCacheEngine(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "service_conf.yaml")
	// Only cache_engine is overridden; the remaining engine types keep the
	// ParseGeneralConfig defaults (mysql/elasticsearch/minio/nats/clickhouse),
	// so GetAllConfigs only fails on the cache-engine dispatch.
	content := `
general:
  cache_engine: kvrocks
kvrocks:
  host: 'localhost:6379'
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}

	if err := Init(cfgPath); err != nil {
		t.Fatalf("Init: %v", err)
	}

	allConfigs, err := GetAllConfigs()
	if err != nil {
		t.Fatalf("GetAllConfigs with cache_engine=kvrocks: %v", err)
	}

	var foundKvrocks bool
	for _, cfg := range allConfigs {
		if host, ok := cfg["host"]; ok && host == "localhost" {
			if port, ok := cfg["port"]; ok && port == 6379 {
				foundKvrocks = true
			}
		}
	}
	if !foundKvrocks {
		t.Fatalf("Kvrocks config not exported in GetAllConfigs: %#v", allConfigs)
	}
}

func TestMCPConfigurationDoesNotLogKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service_conf.yaml")
	const secret = "test-only-mcp-secret"
	if err := os.WriteFile(path, []byte("mcp: {host_api_key: "+secret+", launch_mode: host}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	oldConfig, oldViper := globalConfig, globalViper
	oldLogger := common.Logger
	t.Cleanup(func() { globalConfig, globalViper = oldConfig, oldViper; common.Logger = oldLogger })
	core, logs := observer.New(zapcore.InfoLevel)
	common.Logger = zap.New(core)
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	if got := GetConfig().GetAPIServerConfig().MCP.HostAPIKey; got != secret {
		t.Fatal("key was not loaded")
	}
	PrintAll()
	if strings.Contains(fmt.Sprint(logs.All()), secret) {
		t.Fatal("MCP key leaked to configuration log")
	}
	all, err := GetAllConfigs()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprint(all), secret) {
		t.Fatal("MCP key exported")
	}
	if globalViper.GetString("mcp.host_api_key") != secret {
		t.Fatal("logging mutated loaded configuration")
	}
	t.Setenv("RAGFLOW_MCP_HOST_API_KEY", "different-test-secret")
	if err := Init(path); err != nil {
		t.Fatal(err)
	}
	PrintAll()
	if strings.Contains(fmt.Sprint(logs.All()), "different-test-secret") {
		t.Fatal("environment key leaked")
	}
}
