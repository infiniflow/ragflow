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

func parseAdminYAML(t *testing.T, yaml string) *Config {
	t.Helper()
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(yaml)); err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	c := &Config{}
	if err := c.ParseAdminConfig(v); err != nil {
		t.Fatalf("ParseAdminConfig: %v", err)
	}
	return c
}

func TestAdminHeartbeatEnabledByDefault(t *testing.T) {
	cases := map[string]*Config{
		"nil config":        nil,
		"zero-value config": {},
		"option unset":      parseAdminYAML(t, "admin:\n  http_port: 9381\n"),
		"no admin section":  parseAdminYAML(t, "mysql:\n  name: rag_flow\n"),
		"explicit false":    parseAdminYAML(t, "admin:\n  disable_heartbeat: false\n"),
	}
	for name, c := range cases {
		if c.AdminHeartbeatDisabled() {
			t.Errorf("%s: AdminHeartbeatDisabled() = true, want false", name)
		}
	}
}

func TestAdminHeartbeatCanBeDisabled(t *testing.T) {
	c := parseAdminYAML(t, "admin:\n  disable_heartbeat: true\n")
	if !c.AdminHeartbeatDisabled() {
		t.Fatal("AdminHeartbeatDisabled() = false, want true when disable_heartbeat is true")
	}
	if got := c.GetAdminServerConfig().HTTPPort; got != 9381 {
		t.Fatalf("HTTPPort = %d, want default 9381 to be unaffected", got)
	}
}

func TestAdminHeartbeatDisabledHonorsEnvOverride(t *testing.T) {
	t.Setenv("RAGFLOW_ADMIN_DISABLE_HEARTBEAT", "true")
	v := viper.New()
	v.SetEnvPrefix("RAGFLOW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	c := &Config{}
	if err := c.ParseAdminConfig(v); err != nil {
		t.Fatalf("ParseAdminConfig: %v", err)
	}
	if !c.AdminHeartbeatDisabled() {
		t.Fatal("AdminHeartbeatDisabled() = false, want true from env override")
	}
}

// A malformed value must never turn the heartbeat off: viper's GetBool yields
// false for anything it cannot convert, which here means "keep the check".
func TestAdminHeartbeatMalformedValueKeepsHeartbeat(t *testing.T) {
	for _, value := range []string{"ture", "maybe", "yes please"} {
		v := viper.New()
		v.Set("admin.disable_heartbeat", value)
		c := &Config{}
		if err := c.ParseAdminConfig(v); err != nil {
			t.Errorf("value %q: ParseAdminConfig error = %v, want nil", value, err)
			continue
		}
		if c.AdminHeartbeatDisabled() {
			t.Errorf("value %q: malformed value must not disable the heartbeat", value)
		}
	}
}

func TestParseAdminConfigResetsHeartbeatOption(t *testing.T) {
	c := parseAdminYAML(t, "admin:\n  disable_heartbeat: true\n")
	if err := c.ParseAdminConfig(viper.New()); err != nil {
		t.Fatalf("ParseAdminConfig: %v", err)
	}
	if c.AdminHeartbeatDisabled() {
		t.Fatal("re-parsing without the option must restore the default (heartbeat on)")
	}
}
