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
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"reflect"
	"strings"
	"testing"
)

func TestPrintAllMasksCredentials(t *testing.T) {
	oldViper, oldLogger := globalViper, common.Logger
	t.Cleanup(func() {
		globalViper, common.Logger = oldViper, oldLogger
	})
	globalViper = viper.New()
	globalViper.SetConfigType("yaml")
	if err := globalViper.ReadConfig(strings.NewReader(`
password: test-only-root-password
os: {hosts: search.example, username: admin, password: test-only-search-password}
mcp: {host_api_key: test-only-mcp-key}
backends:
  - config: {password: test-only-nested-password, enabled: true}
  - password: ""
  - password: null
password_policy: strict
enable_register: true
max_token: 64
`)); err != nil {
		t.Fatal(err)
	}
	wantCredentials := make(map[string]any)
	for _, key := range []string{
		"mail_password", "secret", "secret_key", "client_secret", "api_key",
		"host_api_key", "http_app_key", "http_secret_key", "access_key", "secret_id", "session_token", "sas_token",
		"open_dataloader_api_key", "paddle_ocr_access_token", "somark_api_key", "tcad_api_key",
		"docling_api_key", "mineru_api_key", "tavily_api_key", "querit_api_key",
	} {
		globalViper.Set("credentials."+key, "test-only-credential")
		wantCredentials[key] = "******"
	}
	original := fmt.Sprint(globalViper.AllSettings())
	core, logs := observer.New(zapcore.InfoLevel)
	common.Logger = zap.New(core)
	PrintAll()

	settings := make(map[string]any)
	for _, entry := range logs.FilterMessage("config").All() {
		fields := entry.ContextMap()
		settings[fields["key"].(string)] = fields["value"]
	}
	want := map[string]any{
		"password": "******",
		"os":       map[string]any{"hosts": "search.example", "username": "admin", "password": "******"},
		"mcp":      map[string]any{"host_api_key": "******"},
		"backends": []any{
			map[string]any{"config": map[string]any{"password": "******", "enabled": true}},
			map[string]any{"password": "******"},
			map[string]any{"password": "******"},
		},
		"credentials":     wantCredentials,
		"password_policy": "strict",
		"enable_register": true,
		"max_token":       int64(64),
	}
	if !reflect.DeepEqual(settings, want) {
		t.Fatalf("logged settings = %#v, want %#v", settings, want)
	}
	if fmt.Sprint(globalViper.AllSettings()) != original {
		t.Fatal("logging mutated the loaded configuration")
	}
}

func TestMaskConfigCredentialsIgnoresCase(t *testing.T) {
	input := map[string]any{"API_KEY": "test-only-secret", "PASSWORD_POLICY": "strict"}
	want := map[string]any{"API_KEY": "******", "PASSWORD_POLICY": "strict"}
	if got := maskConfigCredentials(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("masked settings = %#v, want %#v", got, want)
	}
}

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
