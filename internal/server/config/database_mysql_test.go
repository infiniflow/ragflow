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

func TestParseMySQLConfigTLSCAFile(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(`
mysql:
  tls: 'custom'
  tls_ca_file: '/etc/ssl/mysql-ca.pem'
`)); err != nil {
		t.Fatal(err)
	}
	c := &Config{}
	c.parseMySQLConfig(v)
	got := c.GetMySQLConfig()
	if got.TLS != "custom" {
		t.Errorf("tls = %q, want custom", got.TLS)
	}
	if got.TLSCAFile != "/etc/ssl/mysql-ca.pem" {
		t.Errorf("tls_ca_file = %q", got.TLSCAFile)
	}
	if exported := got.ExportConfigs(); exported["tls_ca_file"] != got.TLSCAFile {
		t.Errorf("exported tls_ca_file = %v", exported["tls_ca_file"])
	}
}

func TestParseMySQLConfigTLSCAFileDefaultsEmpty(t *testing.T) {
	c := &Config{}
	c.parseMySQLConfig(viper.New())
	if got := c.GetMySQLConfig().TLSCAFile; got != "" {
		t.Errorf("tls_ca_file = %q, want empty by default", got)
	}
}
