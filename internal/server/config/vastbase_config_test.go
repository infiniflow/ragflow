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
	"testing"

	"github.com/spf13/viper"
)

func TestParseVastbaseConfigDefaultsAndOverrides(t *testing.T) {
	t.Setenv("DB_TYPE", "")
	v := viper.New()
	c := &Config{}
	if err := c.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}
	defaults := c.GetVastbaseConfig()
	if defaults.Host != "vastbase" || defaults.Port != 5432 || defaults.User != "ragflow" ||
		defaults.DBName != "ragflow" || defaults.DBCompatibility != "PG" || defaults.SSLMode != "disable" {
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
