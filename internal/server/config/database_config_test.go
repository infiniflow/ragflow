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

func TestParseMySQLConfigTLSDefaults(t *testing.T) {
	c := &Config{}
	c.parseMySQLConfig(viper.New())
	mysqlConfig := c.GetMySQLConfig()
	if mysqlConfig.TLS != "" || mysqlConfig.TLSCAFile != "" {
		t.Fatalf("tls settings should default to off, got %#v", mysqlConfig)
	}
}

func TestParseMySQLConfigTLSOverrides(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	yamlConfig := `
mysql:
  host: 'db.example.com'
  tls: 'custom'
  tls_ca_file: '/etc/ssl/mysql-ca.pem'
`
	if err := v.ReadConfig(strings.NewReader(yamlConfig)); err != nil {
		t.Fatal(err)
	}
	c := &Config{}
	c.parseMySQLConfig(v)
	mysqlConfig := c.GetMySQLConfig()
	if mysqlConfig.Host != "db.example.com" {
		t.Errorf("host = %q", mysqlConfig.Host)
	}
	if mysqlConfig.TLS != "custom" {
		t.Errorf("tls = %q", mysqlConfig.TLS)
	}
	if mysqlConfig.TLSCAFile != "/etc/ssl/mysql-ca.pem" {
		t.Errorf("tls_ca_file = %q", mysqlConfig.TLSCAFile)
	}
}
