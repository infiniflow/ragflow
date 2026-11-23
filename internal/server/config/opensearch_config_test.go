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

// TestParseOpenSearchConfigDefaultsAndOverrides checks default OpenSearch values
// and that YAML configuration overrides them properly.
func TestParseOpenSearchConfigDefaultsAndOverrides(t *testing.T) {
	v := viper.New()
	c := &Config{}
	if err := c.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}
	defaults := c.GetOpenSearchConfig()
	if defaults.Hosts != "http://localhost:1201" || defaults.Username != "admin" ||
		defaults.Password != "infini_rag_flow_OS_01" || defaults.InsecureSkipVerify != false ||
		defaults.CACertPath != "" || defaults.ClientCertPath != "" || defaults.ClientKeyPath != "" {
		t.Fatalf("unexpected defaults: %#v", defaults)
	}

	v = viper.New()
	v.Set("os", map[string]interface{}{
		"hosts":                "https://os-node1:9200,https://os-node2:9200",
		"username":             "custom_admin",
		"password":             "custom_pass",
		"insecure_skip_verify": true,
		"ca_cert_path":         "/path/to/ca.pem",
		"client_cert_path":     "/path/to/client.pem",
		"client_key_path":      "/path/to/client-key.pem",
	})
	c = &Config{}
	if err := c.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}
	got := c.GetOpenSearchConfig()
	if got.Hosts != "https://os-node1:9200,https://os-node2:9200" ||
		got.Username != "custom_admin" ||
		got.Password != "custom_pass" ||
		got.InsecureSkipVerify != true ||
		got.CACertPath != "/path/to/ca.pem" ||
		got.ClientCertPath != "/path/to/client.pem" ||
		got.ClientKeyPath != "/path/to/client-key.pem" {
		t.Fatalf("unexpected overrides: %#v", got)
	}
}

// TestParseOpenSearchConfigEnvironmentOnly verifies that environment variables
// (e.g. RAGFLOW_OS_HOSTS) are preserved in containerized environments where no
// service_conf.yaml is present.
func TestParseOpenSearchConfigEnvironmentOnly(t *testing.T) {
	t.Setenv("RAGFLOW_OS_HOSTS", "https://prod-os:9200")
	t.Setenv("RAGFLOW_OS_USERNAME", "prod_user")
	t.Setenv("RAGFLOW_OS_PASSWORD", "prod_secret")
	t.Setenv("RAGFLOW_OS_INSECURE_SKIP_VERIFY", "true")
	t.Setenv("RAGFLOW_OS_CA_CERT_PATH", "/etc/ssl/ca.pem")
	t.Setenv("RAGFLOW_OS_CLIENT_CERT_PATH", "/etc/ssl/client.pem")
	t.Setenv("RAGFLOW_OS_CLIENT_KEY_PATH", "/etc/ssl/client.key")

	v := viper.New()
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	c := &Config{}
	if err := c.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}
	got := c.GetOpenSearchConfig()
	if got.Hosts != "https://prod-os:9200" ||
		got.Username != "prod_user" ||
		got.Password != "prod_secret" ||
		got.InsecureSkipVerify != true ||
		got.CACertPath != "/etc/ssl/ca.pem" ||
		got.ClientCertPath != "/etc/ssl/client.pem" ||
		got.ClientKeyPath != "/etc/ssl/client.key" {
		t.Fatalf("env-only overrides failed: %#v", got)
	}
}

// TestOpenSearchExportConfigsKeys checks that ExportConfigs carries all defined keys.
func TestOpenSearchExportConfigsKeys(t *testing.T) {
	exported := OpenSearchConfig{
		Hosts:              "https://os:9200",
		Username:           "admin",
		Password:           "secret",
		InsecureSkipVerify: true,
		CACertPath:         "/ca.pem",
		ClientCertPath:     "/client.pem",
		ClientKeyPath:      "/key.pem",
	}.ExportConfigs()

	for _, key := range []string{
		"hosts", "username", "password", "insecure_skip_verify",
		"ca_cert_path", "client_cert_path", "client_key_path",
	} {
		if _, present := exported[key]; !present {
			t.Fatalf("ExportConfigs missing key %q: %#v", key, exported)
		}
	}
}
