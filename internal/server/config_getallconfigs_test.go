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
	"os"
	"path/filepath"
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
