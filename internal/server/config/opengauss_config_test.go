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

func TestOpenGaussDocEngineConfig(t *testing.T) {
	t.Setenv("OPENGAUSS_PASSWORD", "O'Brien:#$[]")
	t.Setenv("DOC_ENGINE", "opengauss")
	t.Setenv("DB_TYPE", "mysql")
	t.Setenv("RAGFLOW_OPENGAUSS_USER", "docstore")
	v := viper.New()
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.Set("opengauss.host", "gauss.example")
	v.Set("opengauss.schema", "ragflow")
	var c Config
	if err := c.GetEnvironments(); err != nil {
		t.Fatal(err)
	}
	if err := c.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}
	if c.DocEngineType() != "opengauss" || c.DatabaseType() != "mysql" {
		t.Fatalf("engines = %s, %s", c.DocEngineType(), c.DatabaseType())
	}
	got := c.GetOpenGaussConfig()
	if got.Password != "O'Brien:#$[]" {
		t.Fatal("openGauss password was altered")
	}
	if got.Host != "gauss.example" || got.Port != 5432 || got.Database != "postgres" || got.Schema != "ragflow" || got.User != "docstore" {
		t.Fatalf("host=%q port=%d database=%q schema=%q user=%q", got.Host, got.Port, got.Database, got.Schema, got.User)
	}
}

func TestOpenGaussKeepsMetadataDatabase(t *testing.T) {
	t.Setenv("DOC_ENGINE", "opengauss")
	for _, dbType := range []string{"mysql", "oceanbase", "vastbase"} {
		t.Run(dbType, func(t *testing.T) {
			t.Setenv("DB_TYPE", dbType)
			var c Config
			if err := c.GetEnvironments(); err != nil {
				t.Fatal(err)
			}
			if c.DatabaseType() != dbType || c.DocEngineType() != "opengauss" {
				t.Fatalf("metadata=%s document=%s", c.DatabaseType(), c.DocEngineType())
			}
		})
	}
}
