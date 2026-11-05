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
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestParseOpenSearchConfigDefaultsAndOverrides(t *testing.T) {
	v := viper.New()
	c := &Config{}
	if err := c.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}

	defaults := c.GetOpenSearchConfig()
	if defaults.Hosts != "http://localhost:1201" ||
		defaults.Username != "admin" ||
		defaults.Password != "infini_rag_flow_OS_01" ||
		defaults.InsecureSkipVerify ||
		defaults.CACertPath != "" ||
		defaults.ClientCertPath != "" ||
		defaults.ClientKeyPath != "" {
		t.Fatalf("opensearch defaults = %#v", defaults)
	}

	if !c.IsOpenSearchConfigured() {
		t.Fatalf("expected IsOpenSearchConfigured to be true with default hosts")
	}

	v = viper.New()
	v.Set("os", map[string]interface{}{
		"hosts":                "https://os.example.com:9200",
		"username":             "os-user",
		"password":             "os-secret",
		"ca_cert_path":         "/path/to/ca.crt",
		"client_cert_path":     "/path/to/client.crt",
		"client_key_path":      "/path/to/client.key",
		"insecure_skip_verify": true,
	})
	c = &Config{}
	if err := c.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}

	got := c.GetOpenSearchConfig()
	if got.Hosts != "https://os.example.com:9200" ||
		got.Username != "os-user" ||
		got.Password != "os-secret" ||
		got.CACertPath != "/path/to/ca.crt" ||
		got.ClientCertPath != "/path/to/client.crt" ||
		got.ClientKeyPath != "/path/to/client.key" ||
		!got.InsecureSkipVerify {
		t.Fatalf("opensearch overrides = %#v", got)
	}

	exported := got.ExportConfigs()
	if exported["hosts"] != "https://os.example.com:9200" ||
		exported["username"] != "os-user" ||
		exported["password"] != "os-secret" {
		t.Fatalf("exported configs = %#v", exported)
	}

	// Test environment variable without os section in YAML
	t.Setenv("RAGFLOW_OS_HOSTS", "https://env-os.example.com:9200")
	v = viper.New()
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	c = &Config{}
	if err := c.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}
	if gotHost := c.GetOpenSearchConfig().Hosts; gotHost != "https://env-os.example.com:9200" {
		t.Fatalf("expected host from env 'https://env-os.example.com:9200', got '%s'", gotHost)
	}
}

func TestParseOpenSearchEnvironments(t *testing.T) {
	t.Setenv("DOC_ENGINE", "opensearch")
	c := &Config{}
	if err := c.GetEnvironments(); err != nil {
		t.Fatalf("unexpected error parsing DOC_ENGINE=opensearch: %v", err)
	}
	if got := c.DocEngineType(); got != "opensearch" {
		t.Fatalf("expected DocEngineType 'opensearch', got '%s'", got)
	}
}
