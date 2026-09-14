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
	"reflect"
	"testing"

	"github.com/spf13/viper"
)

func TestParseExistingOceanBaseAndSeekDBConfig(t *testing.T) {
	t.Setenv("DB_TYPE", "")
	mysqlCredential := t.Name() + "-mysql"
	oceanBaseCredential := t.Name() + "-oceanbase"
	ignoredCredential := t.Name() + "-ignored"
	v := viper.New()
	v.Set("mysql", map[string]interface{}{
		"name": "rag_flow", "user": "mysql-user", "password": mysqlCredential,
		"host": "mysql-host", "port": 3307, "max_connections": 456,
	})
	v.Set("oceanbase", map[string]interface{}{
		"scheme": "oceanbase",
		"config": map[string]interface{}{
			"db_name": "legacy_doc", "user": "root@ragflow", "password": oceanBaseCredential,
			"host": "ob-host", "port": 2881, "max_connections": 123,
		},
	})
	v.Set("seekdb", map[string]interface{}{
		"scheme": "mysql",
		"config": map[string]interface{}{
			"db_name": "legacy_seekdb", "user": "ignored-user", "password": ignoredCredential,
			"host": "ignored-host", "port": 2881, "max_connections": 12,
		},
	})

	config := &Config{}
	if err := config.ParseGeneralConfig(v); err != nil {
		t.Fatal(err)
	}
	if err := config.ParseDatabaseConfig(v); err != nil {
		t.Fatal(err)
	}
	if err := config.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}

	oceanBase, err := config.ResolveOceanBaseConnection("oceanbase")
	if err != nil {
		t.Fatal(err)
	}
	wantOceanBase := OceanBaseConnectionConfig{
		DBName: "legacy_doc", User: "root@ragflow", Password: oceanBaseCredential,
		Host: "ob-host", Port: 2881, MaxConnections: 123,
	}
	if !reflect.DeepEqual(oceanBase, wantOceanBase) {
		t.Fatalf("oceanbase config = %#v, want %#v", oceanBase, wantOceanBase)
	}

	seekDB, err := config.ResolveOceanBaseConnection("seekdb")
	if err != nil {
		t.Fatal(err)
	}
	wantSeekDB := OceanBaseConnectionConfig{
		DBName: "legacy_seekdb", User: "mysql-user", Password: mysqlCredential,
		Host: "mysql-host", Port: 3307, MaxConnections: 456,
	}
	if !reflect.DeepEqual(seekDB, wantSeekDB) {
		t.Fatalf("seekdb config = %#v, want %#v", seekDB, wantSeekDB)
	}
}

func TestOceanBaseDefaultCredentialMatchesMySQLWireConfig(t *testing.T) {
	t.Setenv("DB_TYPE", "")
	v := viper.New()
	config := &Config{}
	if err := config.ParseGeneralConfig(v); err != nil {
		t.Fatal(err)
	}
	if err := config.ParseDatabaseConfig(v); err != nil {
		t.Fatal(err)
	}
	if err := config.ParseDocEngineConfig(v); err != nil {
		t.Fatal(err)
	}

	oceanBase, err := config.ResolveOceanBaseConnection("oceanbase")
	if err != nil {
		t.Fatal(err)
	}
	if oceanBase.Password != config.GetMySQLConfig().Password {
		t.Fatal("OceanBase default credential diverged from the MySQL-wire default")
	}
}

func TestOceanBaseEnvironmentTypesAreAccepted(t *testing.T) {
	t.Setenv("DOC_ENGINE", "seekdb")
	t.Setenv("DB_TYPE", "oceanbase")
	config := &Config{}
	if err := config.GetEnvironments(); err != nil {
		t.Fatal(err)
	}
	if got := config.DocEngineType(); got != "seekdb" {
		t.Fatalf("doc engine = %q, want seekdb", got)
	}
	if got := config.DatabaseType(); got != "oceanbase" {
		t.Fatalf("database type = %q, want oceanbase", got)
	}
}

func TestParseOceanBaseAsMainDatabaseFromNestedConfig(t *testing.T) {
	t.Setenv("DB_TYPE", "oceanbase")
	oceanBaseCredential := t.Name() + "-oceanbase"
	v := viper.New()
	v.Set("oceanbase", map[string]interface{}{
		"scheme": "oceanbase",
		"config": map[string]interface{}{
			"db_name": "rag_flow_ob", "user": "root@tenant", "password": oceanBaseCredential,
			"host": "ob-main", "port": 2881, "max_connections": 300,
		},
	})
	config := &Config{}
	if err := config.ParseGeneralConfig(v); err != nil {
		t.Fatal(err)
	}
	if err := config.ParseDatabaseConfig(v); err != nil {
		t.Fatal(err)
	}
	got := config.GetMySQLConfig()
	if got.DatabaseName != "rag_flow_ob" || got.User != "root@tenant" || got.Host != "ob-main" || got.Port != 2881 {
		t.Fatalf("main OceanBase config was not mapped to MySQL wire config: %#v", got)
	}
}
