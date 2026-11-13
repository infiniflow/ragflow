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

// TestParseVastbaseConfigDefaultsAndOverrides checks the config defaults
// against an empty viper instance and that file values override them.
func TestParseVastbaseConfigDefaultsAndOverrides(t *testing.T) {
	t.Setenv("DB_TYPE", "")
	v := viper.New()
	c := &Config{}
	if err := c.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}
	defaults := c.GetVastbaseConfig()
	if defaults.Host != "vastbase" || defaults.Port != 5432 || defaults.User != "ragflow" ||
		defaults.DBName != "ragflow" || defaults.DBCompatibility != "PG" || defaults.SSLMode != "" {
		t.Fatalf("vastbase defaults = %#v", defaults)
	}

	v = viper.New()
	v.Set("vastbase", map[string]interface{}{
		"host": "vb-host", "port": 15432, "user": "vb-user",
		"password": "vb-secret", "db_name": "rag_vec",
		"dbcompatibility": "b", "ssl_mode": "verify-full",
	})
	c = &Config{}
	if err := c.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}
	got := c.GetVastbaseConfig()
	if got.Host != "vb-host" || got.Port != 15432 || got.User != "vb-user" ||
		got.Password != "vb-secret" || got.DBName != "rag_vec" ||
		got.DBCompatibility != "B" || got.SSLMode != "verify-full" {
		t.Fatalf("vastbase overrides = %#v", got)
	}
}

// TestParseVastbaseConfigNormalizesCompatibility checks the accepted
// spellings of the dbcompatibility setting and their canonical B/PG forms.
func TestParseVastbaseConfigNormalizesCompatibility(t *testing.T) {
	for _, test := range []struct{ raw, want string }{
		{" b ", "B"},
		{"pg", "PG"},
		{"nonsense", "PG"},
		{"", "PG"},
	} {
		v := viper.New()
		if test.raw != "" {
			v.Set("vastbase", map[string]interface{}{"dbcompatibility": test.raw})
		}
		c := &Config{}
		if err := c.ParseDocEngineConfig(v); err != nil {
			t.Fatal(err)
		}
		if got := c.GetVastbaseConfig().DBCompatibility; got != test.want {
			t.Fatalf("dbcompatibility(%q) = %q, want %q", test.raw, got, test.want)
		}
	}
}

// Env-only deployments (no service_conf.yaml) configure Vastbase through
// RAGFLOW_VASTBASE_* variables: the parser must read them through the parent
// viper's AutomaticEnv, not a Sub view that drops it.
func TestParseVastbaseConfigEnvironmentOnly(t *testing.T) {
	t.Setenv("RAGFLOW_VASTBASE_HOST", "env-host")
	t.Setenv("RAGFLOW_VASTBASE_PORT", "25432")
	t.Setenv("RAGFLOW_VASTBASE_PASSWORD", "env-secret")
	t.Setenv("RAGFLOW_VASTBASE_DBCOMPATIBILITY", "b")
	v := viper.New()
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	c := &Config{}
	if err := c.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}
	got := c.GetVastbaseConfig()
	if got.Host != "env-host" || got.Port != 25432 ||
		got.Password != "env-secret" || got.DBCompatibility != "B" {
		t.Fatalf("vastbase env-only overrides = %#v", got)
	}
	// Fields without an env override keep their defaults (SSLMode stays
	// unset: the engine derives it from the host).
	if got.User != "ragflow" || got.DBName != "ragflow" || got.SSLMode != "" {
		t.Fatalf("unset vastbase fields must keep defaults: %#v", got)
	}
}

// TestVastbaseEnvironmentTypeIsAccepted checks that DOC_ENGINE=vastbase is a
// selectable engine type from the environment.
func TestVastbaseEnvironmentTypeIsAccepted(t *testing.T) {
	t.Setenv("DOC_ENGINE", "vastbase")
	c := &Config{}
	if err := c.GetEnvironments(); err != nil {
		t.Fatal(err)
	}
	if got := c.DocEngineType(); got != "vastbase" {
		t.Fatalf("doc engine = %q, want vastbase", got)
	}
}

// TestVastbaseExportConfigsKeys checks the exported vastbase configuration
// carries exactly the expected keys.
func TestVastbaseExportConfigsKeys(t *testing.T) {
	exported := VastbaseConfig{
		Host: "h", Port: 1, User: "u", Password: "p",
		DBName: "d", DBCompatibility: "B", SSLMode: "disable",
	}.ExportConfigs()
	for _, key := range []string{"host", "port", "user", "password", "db_name", "dbcompatibility", "ssl_mode"} {
		if _, present := exported[key]; !present {
			t.Fatalf("ExportConfigs missing key %q: %#v", key, exported)
		}
	}
}

// TestParseVastbaseDatabaseConfigDefaultsAndOverrides checks the metadata-side
// defaults of the shared vastbase: section and that file values override them
// without disturbing the doc-engine keys.
func TestParseVastbaseDatabaseConfigDefaultsAndOverrides(t *testing.T) {
	t.Setenv("DB_TYPE", "vastbase")
	v := viper.New()
	c := &Config{}
	if err := c.ParseGeneralConfig(v); err != nil {
		t.Fatal(err)
	}
	if err := c.ParseDatabaseConfig(v); err != nil {
		t.Fatal(err)
	}
	defaults := c.GetVastbaseMetaConfig()
	if defaults.DatabaseName != "rag_flow" || defaults.Host != "vastbase" || defaults.Port != 5432 ||
		defaults.User != "ragflow" || defaults.Password != "Infini_Rag@123" ||
		defaults.MaxConnections != 50 || defaults.StaleTimeout != 300 ||
		defaults.SSLMode != "" || defaults.DisableReturning {
		t.Fatalf("vastbase metadata defaults = %#v", defaults)
	}

	v = viper.New()
	v.Set("vastbase", map[string]interface{}{
		"host": "vb-host", "port": 15432, "user": "vb-user",
		"password": "vb-secret", "name": "meta_db",
		"max_connections": 11, "stale_timeout": 22,
		"ssl_mode": "require", "disable_returning": true,
	})
	c = &Config{}
	if err := c.ParseDatabaseConfig(v); err != nil {
		t.Fatal(err)
	}
	got := c.GetVastbaseMetaConfig()
	if got.DatabaseName != "meta_db" || got.Host != "vb-host" || got.Port != 15432 ||
		got.User != "vb-user" || got.Password != "vb-secret" ||
		got.MaxConnections != 11 || got.StaleTimeout != 22 ||
		got.SSLMode != "require" || !got.DisableReturning {
		t.Fatalf("vastbase metadata overrides = %#v", got)
	}
}

// TestParseDatabaseConfigAcceptsVastbaseType checks DB_TYPE=vastbase routes to
// the vastbase parser instead of the "not supported" error.
func TestParseDatabaseConfigAcceptsVastbaseType(t *testing.T) {
	t.Setenv("DB_TYPE", "vastbase")
	v := viper.New()
	c := &Config{}
	if err := c.ParseDatabaseConfig(v); err != nil {
		t.Fatalf("ParseDatabaseConfig(vastbase): %v", err)
	}
	if got := c.GetVastbaseMetaConfig().DatabaseName; got != "rag_flow" {
		t.Fatalf("metadata database name = %q, want rag_flow", got)
	}
}

// TestParseVastbaseDatabaseConfigEnvironmentOnly mirrors the doc-engine env
// test: the metadata keys must be readable from RAGFLOW_VASTBASE_* variables
// through the parent viper's AutomaticEnv.
func TestParseVastbaseDatabaseConfigEnvironmentOnly(t *testing.T) {
	t.Setenv("DB_TYPE", "vastbase")
	t.Setenv("RAGFLOW_VASTBASE_HOST", "env-host")
	t.Setenv("RAGFLOW_VASTBASE_NAME", "env_meta")
	t.Setenv("RAGFLOW_VASTBASE_MAX_CONNECTIONS", "17")
	v := viper.New()
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	c := &Config{}
	if err := c.ParseGeneralConfig(v); err != nil {
		t.Fatal(err)
	}
	if err := c.ParseDatabaseConfig(v); err != nil {
		t.Fatal(err)
	}
	got := c.GetVastbaseMetaConfig()
	if got.Host != "env-host" || got.DatabaseName != "env_meta" || got.MaxConnections != 17 {
		t.Fatalf("vastbase metadata env-only overrides = %#v", got)
	}
}

// TestVastbaseMetaExportConfigsKeys checks the exported metadata configuration
// carries exactly the expected keys.
func TestVastbaseMetaExportConfigsKeys(t *testing.T) {
	exported := (&VastbaseMetaConfig{
		DatabaseName: "d", User: "u", Password: "p", Host: "h", Port: 1,
		SSLMode: "disable", MaxConnections: 2, StaleTimeout: 3,
	}).ExportConfigs()
	for _, key := range []string{"database_name", "username", "password", "host", "port", "ssl_mode", "max_connections", "stale_timeout"} {
		if _, present := exported[key]; !present {
			t.Fatalf("ExportConfigs missing key %q: %#v", key, exported)
		}
	}
}

// TestGetEnvironmentsAcceptsVastbaseDBType guards the DB_TYPE env gate: the
// server aborts startup with "invalid database type" before the vastbase
// dialector is reached if this case list drifts from the supported set.
func TestGetEnvironmentsAcceptsVastbaseDBType(t *testing.T) {
	t.Setenv("DB_TYPE", "vastbase")
	c := &Config{}
	if err := c.GetEnvironments(); err != nil {
		t.Fatalf("GetEnvironments with DB_TYPE=vastbase: %v", err)
	}
	if got := c.GetEnvDatabaseType(); got != "vastbase" {
		t.Fatalf("env database type = %q, want vastbase", got)
	}
}
